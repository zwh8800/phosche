// Package pipeline 实现照片处理流水线的核心编排逻辑：扫描已有文件 → 实时监控 →
// 解码图片 → AI 分析 → ES 索引 → 失败重试。
//
// 流水线组件：
//   - 初始扫描：启动时通过 Scanner 扫描监控目录中已有文件
//   - 文件监控：通过 Watcher 实时监听文件创建/修改事件
//   - Worker 池：并发处理照片（解码 → AI 分析 → ES 索引）
//   - 重试循环：LLM 连接失败的照片进入 pending 队列，每 5 分钟重试，最多 10 次
//   - 优雅关闭：收到取消信号后顺序关闭 watcher → inputCh → workers → retry → indexer
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zwh8800/phosche/internal/decoder"
	"github.com/zwh8800/phosche/internal/geocoder"
	"github.com/zwh8800/phosche/internal/types"
	"github.com/zwh8800/phosche/internal/watcher"
)

// 默认配置常量，当 PipelineConfig 中对应字段为零值时使用。
const (
	defaultConcurrency       = 4                // 并发 worker 数量，即同时处理照片的 goroutine 数
	defaultQueueSize         = 100              // inputCh 通道容量，限制待处理照片的最大积压量
	defaultRetryInterval     = 5 * time.Minute  // pending_analysis 照片的重试间隔
	defaultMaxPendingRetries = 10               // 最大重试次数，超过后标记为 failed
	defaultDrainTimeout      = 5 * time.Minute  // 单个 worker 处理单张照片的超时时间
)

// Analyzer 是 LLM 图片分析的抽象接口。
// 实现者负责将图片数据发送给 AI 模型并返回结构化的分析结果。
type Analyzer interface {
	Analyze(ctx context.Context, imageData []byte, locationContext string) (*types.AnalysisResult, error)
}

// Indexer 是流水线所需的索引操作抽象接口。
// 只暴露流水线实际使用的方法，便于测试时 mock。
type Indexer interface {
	IndexPhoto(ctx context.Context, doc *types.PhotoDocument, indexName string) error
	UpdateStatus(ctx context.Context, path string, status types.JobStatus, indexName string) error
	GetPhoto(ctx context.Context, path string, indexName string) (*types.PhotoDocument, error)
	Stop()
}

// PipelineConfig 集中管理流水线所需的所有外部依赖和运行时参数。
type PipelineConfig struct {
	Watcher           watcher.Watcher  // 文件系统监控器（fsnotify 实现）
	Scanner           watcher.Scanner  // 目录扫描器（启动时遍历已有文件）
	Analyzer          Analyzer         // LLM 分析器
	Geocoder          *geocoder.Geocoder // 逆地理编码器
	Indexer           Indexer          // ES 索引服务
	IndexName         string           // ES 索引名称
	Dirs              []string         // 监控的目录列表
	Recursive         bool             // 是否递归监控子目录
	ExcludeDirs       []string         // 排除的目录名列表
	InitialScan       bool             // 启动时是否扫描已有文件
	Concurrency       int              // 并发 worker 数（0 使用默认值 4）
	QueueSize         int              // 输入通道容量（0 使用默认值 100）
	RetryInterval     time.Duration    // 重试间隔（0 使用默认值 5min）
	MaxPendingRetries int              // 最大重试次数（0 使用默认值 10）
}

// pendingItem 跟踪一张等待重试的照片及其重试次数。
type pendingItem struct {
	path       string // 照片文件路径
	retryCount int    // 已重试次数
}

// Pipeline 是照片处理流水线的主体结构。
type Pipeline struct {
	cfg       PipelineConfig           // 配置（包含所有外部依赖）
	inputCh   chan string              // 输入通道，worker 从此 channel 接收待处理的文件路径
	pendingMu sync.Mutex               // 保护 pending map 的互斥锁
	pending   map[string]*pendingItem  // 待重试的照片映射表，key 为文件路径
}

// NewPipeline 创建流水线实例，对零值配置应用默认值。
func NewPipeline(cfg PipelineConfig) *Pipeline {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = defaultConcurrency
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultQueueSize
	}
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = defaultRetryInterval
	}
	if cfg.MaxPendingRetries <= 0 {
		cfg.MaxPendingRetries = defaultMaxPendingRetries
	}
	return &Pipeline{
		cfg:     cfg,
		inputCh: make(chan string, cfg.QueueSize),
		pending: make(map[string]*pendingItem),
	}
}

// Run 启动照片处理流水线，阻塞直到 context 被取消或发生致命错误。
//
// 启动顺序：
//  1. 启动 concurrency 个 worker 协程（先于扫描，确保扫描结果能被消费）
//  2. 启动 retryLoop 协程（定时重试 pending 照片）
//  3. 执行 scanExisting（扫描已有文件入队）
//  4. 启动 Watcher 并转发事件到 inputCh
//  5. 阻塞等待 ctx.Done()（由 SIGINT/SIGTERM 触发）
//
// 关闭顺序：关闭 Watcher → 等待事件转发完成 → 关闭 inputCh →
// 等待所有 worker → 等待 retryLoop → 调用 Indexer.Stop() 排空重试队列
func (p *Pipeline) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var fwWg, workersWg, retryWg sync.WaitGroup

	// 先启动 worker 协程（先于扫描），确保扫描结果能被及时消费。
	for i := 0; i < p.cfg.Concurrency; i++ {
		workersWg.Add(1)
		go func() {
			defer workersWg.Done()
			p.worker(ctx)
		}()
	}

	retryWg.Add(1)
	go func() {
		defer retryWg.Done()
		p.retryLoop(ctx)
	}()

	if p.cfg.InitialScan {
		if err := p.scanExisting(ctx); err != nil {
			return fmt.Errorf("pipeline: initial scan: %w", err)
		}
	} else {
		slog.Info("pipeline: initial scan disabled by config")
	}

	eventCh, err := p.cfg.Watcher.Watch(ctx, p.cfg.Dirs, p.cfg.Recursive)
	if err != nil {
		return fmt.Errorf("pipeline: start watcher: %w", err)
	}

	fwWg.Add(1)
	go func() {
		defer fwWg.Done()
		p.forwardEvents(ctx, eventCh)
	}()

	<-ctx.Done()
	slog.Info("pipeline: shutdown initiated")

	p.cfg.Watcher.Close()
	fwWg.Wait()
	close(p.inputCh)
	workersWg.Wait()
	retryWg.Wait()
	p.cfg.Indexer.Stop()

	slog.Info("pipeline: shutdown complete")
	return nil
}

// scanExisting 扫描监控目录中已有的照片文件，将新增/变更的文件流式入队处理。
// 使用 Scanner.Scan 返回的 channel，边扫描边处理，无需等待全量扫描完成。
func (p *Pipeline) scanExisting(ctx context.Context) error {
	slog.Info("pipeline: starting initial scan", "dirs", p.cfg.Dirs, "recursive", p.cfg.Recursive)

	pathCh, err := p.cfg.Scanner.Scan(ctx, p.cfg.Dirs, nil)
	if err != nil {
		return err
	}

	queued := 0
	for path := range pathCh {
		if p.isExcluded(path) {
			continue
		}
		select {
		case p.inputCh <- path:
			queued++
			if queued%100 == 0 {
				slog.Debug("pipeline: scan progress", "queued", queued)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if queued == 0 {
		slog.Warn("pipeline: no photos found in watched directories, waiting for new files")
	} else {
		slog.Info("pipeline: initial scan complete", "queued", queued)
	}
	return nil
}

// forwardEvents 将文件监控事件转发到流水线输入通道（跳过删除事件）。
// forwardEvents 从文件监控 channel 读取事件，过滤掉删除事件后将路径转发到 inputCh。
// 当 eventCh 关闭或 ctx 取消时退出。
func (p *Pipeline) forwardEvents(ctx context.Context, eventCh <-chan types.FileEvent) {
	for event := range eventCh {
		if event.Op == types.OpDelete {
			continue
		}
		if p.isExcluded(event.Path) {
			continue
		}
		select {
		case p.inputCh <- event.Path:
		case <-ctx.Done():
			return
		}
	}
}

func (p *Pipeline) isExcluded(path string) bool {
	for _, d := range p.cfg.ExcludeDirs {
		// 前缀匹配：/Volumes/photo/#recycle 匹配 /Volumes/photo/#recycle/xxx
		if strings.HasPrefix(path, d+string(filepath.Separator)) || path == d {
			return true
		}
		// 目录名匹配：#recycle 匹配任意路径中名为 #recycle 的目录
		for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
			if part == d {
				return true
			}
		}
	}
	return false
}

// worker 从输入通道读取路径并处理，使用独立的超时上下文（5 分钟）。
// worker 是流水线的消费端 goroutine。
// 从 inputCh 读取文件路径，为每个照片创建独立超时 context 后调用 processPath 处理。
// inputCh 关闭时 worker 自动退出。
func (p *Pipeline) worker(_ context.Context) {
	for path := range p.inputCh {
		slog.Info("pipeline: processing photo", "path", path)
		ctx, cancel := context.WithTimeout(context.Background(), defaultDrainTimeout)
		p.processPath(ctx, path)
		cancel()
	}
}

// processPath 处理单张照片：检查是否已分析且 mtime 未变 → 创建 initializing 文档 → 解码并分析 → 写入完整文档 → 从待重试列表移除。
// processPath 处理单张照片的完整流程。
//
// 步骤：
//  1. os.Stat 获取文件大小和修改时间
//  2. 查询 ES 是否已有 status=analyzed 且 mtime 匹配的文档 → 是则跳过（幂等去重）
//  3. 创建初始文档（status=analyzing）写入 ES，为此后的 UpdateStatus 提供文档基础
//  4. 调用 decodeAndAnalyze 执行图片解码 + AI 分析
//  5. 分析成功 → 创建完整文档（status=analyzed）索引到 ES
//  6. 从 pending 映射中删除该路径（说明重试成功）
func (p *Pipeline) processPath(ctx context.Context, path string) {
	// 检查是否已分析且 mtime 未变
	info, err := os.Stat(path)
	if err != nil {
		slog.Warn("pipeline: stat failed", "path", path, "error", err)
		return
	}
	mtime := info.ModTime().Unix()

	existingDoc, err := p.cfg.Indexer.GetPhoto(ctx, path, p.cfg.IndexName)
	if err == nil && existingDoc != nil {
		if existingDoc.Status == types.StatusAnalyzed && existingDoc.MTime == mtime {
			slog.Debug("pipeline: skipping already analyzed", "path", path)
			return
		}
	}

	now := time.Now().Unix()
	id := sha256hex(path)

	// 创建 initializing 占位文档，以便 UpdateStatus 有文档可更新。
	initDoc := &types.PhotoDocument{
		Photo: types.Photo{
			ID:        id,
			Path:      path,
			MTime:     mtime,
			Size:      info.Size(),
			Status:    types.StatusAnalyzing,
			CreatedAt: now,
		},
	}
	_ = p.cfg.Indexer.IndexPhoto(ctx, initDoc, p.cfg.IndexName)

	r := p.decodeAndAnalyze(ctx, path)
	if r == nil {
		return
	}

	doc := &types.PhotoDocument{
		Photo: types.Photo{
			ID:         id,
			Path:       path,
			MTime:      mtime,
			Size:       info.Size(),
			Status:     types.StatusAnalyzed,
			AnalyzedAt: &now,
			EXIF:       r.exif,
			CreatedAt:  now,
		},
		AnalysisResult: *r.analysis,
	}
	if r.geo != nil {
		doc.GeoInfo = *r.geo
	}

	if err := p.cfg.Indexer.IndexPhoto(ctx, doc, p.cfg.IndexName); err != nil {
		slog.Warn("pipeline: index failed", "path", path, "error", err)
		return
	}

	p.pendingMu.Lock()
	delete(p.pending, path)
	p.pendingMu.Unlock()
}

// decodeAnalyzeResult 是 decodeAndAnalyze 的返回结构，包含解码后的 EXIF 信息和 AI 分析结果。
// decodeAnalyzeResult 封装图片解码和 AI 分析的联合结果。
type decodeAnalyzeResult struct {
	exif     *types.EXIFInfo       // 从图片中提取的 EXIF 元数据
	geo      *types.GeoInfo        // 逆地理编码结果
	analysis *types.AnalysisResult // LLM 返回的结构化分析结果
}

// decodeAndAnalyze 执行图片解码 → 文件读取 → AI 分析的三步流程。
// 任何一步失败都会调用 updateErrorStatus 更新 ES 中的照片状态。
func (p *Pipeline) decodeAndAnalyze(ctx context.Context, path string) *decodeAnalyzeResult {
	decodeResult, err := decoder.DecodeImage(path)
	if err != nil {
		slog.Warn("pipeline: decode failed", "path", path, "error", err)
		p.updateErrorStatus(ctx, path, err)
		return nil
	}

	var geoInfo *types.GeoInfo
	locationContext := ""
	if decodeResult.EXIF != nil && decodeResult.EXIF.GPSLat != 0 && decodeResult.EXIF.GPSLon != 0 {
		slog.Debug("pipeline: GPS found",
			"path", path,
			"lat", decodeResult.EXIF.GPSLat,
			"lon", decodeResult.EXIF.GPSLon,
		)
		if p.cfg.Geocoder != nil {
			geoInfo, err = p.cfg.Geocoder.ReverseGeocode(ctx, decodeResult.EXIF.GPSLat, decodeResult.EXIF.GPSLon)
			if err != nil {
				slog.Warn("pipeline: reverse geocode failed", "path", path, "error", err)
				geoInfo = nil
			}
		}
	} else {
		slog.Debug("pipeline: no GPS data", "path", path)
	}
	if geoInfo != nil {
		parts := make([]string, 0, 3)
		if geoInfo.Province != "" {
			parts = append(parts, geoInfo.Province)
		}
		if geoInfo.City != "" {
			parts = append(parts, geoInfo.City)
		}
		if geoInfo.District != "" {
			parts = append(parts, geoInfo.District)
		}
		locationContext = strings.Join(parts, "")
	}

	imageBytes, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("pipeline: read file failed", "path", path, "error", err)
		p.updateErrorStatus(ctx, path, err)
		return nil
	}

	analysis, err := p.cfg.Analyzer.Analyze(ctx, imageBytes, locationContext)
	if err != nil {
		slog.Warn("pipeline: analysis failed", "path", path, "error", err)
		p.updateErrorStatus(ctx, path, err)
		return nil
	}

	return &decodeAnalyzeResult{
		exif:     decodeResult.EXIF,
		geo:      geoInfo,
		analysis: analysis,
	}
}

// updateErrorStatus 根据错误类型更新照片状态：
// LLM 连接错误 → pending_analysis（进入重试队列），其他错误 → failed。
// updateErrorStatus 根据错误类型将照片标记为不同状态。
//
// 错误分类：
//   - LLM 连接错误（网络超时、连接被拒等） → pending_analysis
//     - 将照片加入 pending 映射并在 ES 中标记状态
//     - 后续由 retryLoop 定时重试
//   - 其他错误（图片损坏、格式不支持等） → failed
//     - 不可恢复，直接标记为失败
func (p *Pipeline) updateErrorStatus(ctx context.Context, path string, err error) {
	if IsLLMConnectionError(err) {
		p.pendingMu.Lock()
		if _, exists := p.pending[path]; !exists {
			p.pending[path] = &pendingItem{path: path}
		}
		p.pendingMu.Unlock()
		_ = p.cfg.Indexer.UpdateStatus(ctx, path, types.StatusPendingAnalysis, p.cfg.IndexName)
	} else {
		_ = p.cfg.Indexer.UpdateStatus(ctx, path, types.StatusFailed, p.cfg.IndexName)
	}
}

// IsLLMConnectionError 判断错误是否为 LLM 连接相关错误。
//
// 通过 errors.As 解包错误链，查找是否存在 net.Error 类型的错误。
// net.Error 表示网络层面的失败（连接被拒绝、超时、DNS 解析失败等），
// 这类错误是可恢复的，应触发重试而非直接标记为 failed。
func IsLLMConnectionError(err error) bool {
	for {
		var netErr net.Error
		if errors.As(err, &netErr) {
			return true
		}
		err = errors.Unwrap(err)
		if err == nil {
			return false
		}
	}
}

// retryLoop 定时重试待处理照片（默认每 5 分钟）。
// retryLoop 是后台定时器 goroutine，周期性调用 retryPending 重试失败的照片。
// 间隔由 RetryInterval 配置（默认 5 分钟）。context 取消时退出（不执行最终重试）。
func (p *Pipeline) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.retryPending(ctx)
		}
	}
}

// retryPending 执行一次重试：遍历 pending 映射，跳过已达最大重试次数的项（标记为 failed），其余项调用 processPath。
// retryPending 遍历 pending 映射中所有待重试的照片并执行重试。
//
// 重试逻辑：
//   - 检查 retryCount 是否已达到 MaxPendingRetries
//     - 达到 → 从 pending 删除、标记为 failed（放弃重试）
//     - 未达到 → 递增 retryCount、调用 processPath 重新处理
//
// 使用锁分段保护：先加锁读取 pending 列表，释放锁后逐个处理，
// 处理每个项时再加锁检查和更新重试计数。
func (p *Pipeline) retryPending(ctx context.Context) {
	p.pendingMu.Lock()
	paths := make([]string, 0, len(p.pending))
	for pth := range p.pending {
		paths = append(paths, pth)
	}
	p.pendingMu.Unlock()

	if len(paths) == 0 {
		return
	}

	slog.Info("pipeline: retrying pending items", "count", len(paths))

	for _, pth := range paths {
		p.pendingMu.Lock()
		item, ok := p.pending[pth]
		if !ok {
			p.pendingMu.Unlock()
			continue
		}
		if item.retryCount >= p.cfg.MaxPendingRetries {
			delete(p.pending, pth)
			p.pendingMu.Unlock()
			_ = p.cfg.Indexer.UpdateStatus(ctx, pth, types.StatusFailed, p.cfg.IndexName)
			continue
		}
		item.retryCount++
		p.pendingMu.Unlock()

		p.processPath(ctx, pth)
	}
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
