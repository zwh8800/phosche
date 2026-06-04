package indexer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"github.com/opensearch-project/opensearch-go/v4/opensearchutil"
	apperrors "github.com/zwh8800/phosche/internal/errors"
	"github.com/zwh8800/phosche/internal/types"
)

// queueItemKind 区分重试队列中不同类型操作的枚举。
// 用于 drainQueue 时根据类型调用对应的直接写入方法。
type queueItemKind int

const (
	queueItemIndex        queueItemKind = iota // 索引（写入/覆盖）文档操作
	queueItemUpdateStatus                      // 仅更新文档 status 字段的操作
)

// queueItem 表示一个被延迟到重试队列中的索引器操作。
// 当 ES 不可用时（断路器打开），写入操作不会立即失败，而是封装为 queueItem 放入有界通道中，
// 待 ES 恢复后由 drainQueue 统一重放。
type queueItem struct {
	kind      queueItemKind       // 操作类型：索引文档 or 更新状态
	doc       *types.PhotoDocument // queueItemIndex 时使用，完整的照片文档
	path      string               // queueItemUpdateStatus 时使用，照片路径
	status    types.JobStatus      // queueItemUpdateStatus 时使用，目标状态
	indexName string               // 目标 ES 索引名称
}

// IndexerService 提供对 OpenSearch 的 CRUD 操作，并内置断路器保护和有界重试队列。
//
// 断路器机制：
//   连续写入失败 ≥ maxFailures(3) → 断路器打开 → 后续写入进入重试队列
//   每 healthCheckInterval(5s) 检查 ES 健康 → 恢复后关闭断路器 → 排空队列
//
// 队列设计：
//   使用有界 channel (容量由 queueSize 参数指定)，队列满时丢弃最旧的项并记录警告。
//   这种"熔断 + 队列"模式确保 ES 短暂不可用时不会阻塞整个照片处理流水线。
type IndexerService struct {
	client              *OSClient          // OS 客户端封装
	queue               chan queueItem     // 重试队列（有界 channel），存放 ES 不可用期间的待处理操作
	done                chan struct{}      // 关闭信号通道，用于优雅停止后台 goroutine
	mu                  sync.RWMutex       // 保护断路器状态和失败计数的读写锁
	circuitOpen         bool               // 断路器是否打开（true=拒绝直接写入，仅入队）
	failureCount        int                // 连续写入失败计数，成功时重置为 0
	maxFailures         int                // 断路阈值，默认 3，达到后打开断路器
	healthCheckInterval time.Duration      // 健康检查间隔，默认 5 秒
	logger              *slog.Logger       // 结构化日志记录器
}

// NewIndexerService 创建 IndexerService 实例并启动后台断路器 goroutine。
//
// 参数：
//   client    - 已初始化的 ES 客户端
//   queueSize - 内存重试队列容量，例如 100。队列满时新项会被丢弃
//
// 启动时会立即通过 go svc.runCircuitBreaker() 启动后台健康检查循环。
func NewIndexerService(client *OSClient, queueSize int) *IndexerService {
	svc := &IndexerService{
		client:              client,
		queue:               make(chan queueItem, queueSize),
		done:                make(chan struct{}),
		maxFailures:         3,
		healthCheckInterval: 5 * time.Second,
		logger:              slog.Default(),
	}
	go svc.runCircuitBreaker()
	return svc
}

// Stop 关闭后台协程并排水队列中的待重试项。
// 关闭 done 通道通知 runCircuitBreaker 退出，后者会自动调用 drainQueue 排空队列。
func (s *IndexerService) Stop() {
	close(s.done)
}

// --------------------------------------------------------------------------
// 公开写入操作 — ES 失败时返回 nil（不阻塞调用方），操作进入重试队列
// --------------------------------------------------------------------------

// IndexPhoto 索引一张照片文档到 ES。文档 _id 为 path 的 SHA-256 哈希值。
//
// 断路器逻辑：
//   - 断路器打开 → 文档直接进入重试队列，返回 nil
//   - 断路器关闭 → 尝试直接写入 ES
//     - 成功 → recordWriteResult(true)，返回 nil
//     - 失败 → 入队 + recordWriteResult(false)，返回 nil
//
// 注意：本方法永远不会向调用方返回错误。采用"发后即忘"模式配合重试队列，
// 确保即使 ES 完全不可用也不会阻塞照片处理流水线。
func (s *IndexerService) IndexPhoto(ctx context.Context, doc *types.PhotoDocument, indexName string) error {
	s.logger.Debug("IndexPhoto", "path", doc.Path, "status", doc.Status, "index", indexName)

	if s.isCircuitOpen() {
		s.queueDocument(doc, indexName)
		return nil
	}

	if err := s.indexDocDirect(ctx, doc, indexName); err != nil {
		s.logger.Warn("index failed, queuing", "path", doc.Path, "error", err)
		s.queueDocument(doc, indexName)
		s.recordWriteResult(false)
		return nil
	}

	s.recordWriteResult(true)
	return nil
}

// UpdateStatus 仅更新照片文档的 status 字段，使用 ES Update API 的 "doc" 部分更新。
// 其他字段不受影响，这是 OpenSearch 的 partial update 机制。
//
// 断路器逻辑与 IndexPhoto 相同：断路器打开时入队，关闭时直接更新，失败时入队并记录失败。
// 同样永远不向调用方返回错误。
func (s *IndexerService) UpdateStatus(ctx context.Context, path string, status types.JobStatus, indexName string) error {
	s.logger.Debug("UpdateStatus", "path", path, "status", status, "index", indexName)

	if s.isCircuitOpen() {
		s.queueStatusUpdate(path, status, indexName)
		return nil
	}

	if err := s.updateStatusDirect(ctx, path, status, indexName); err != nil {
		s.logger.Warn("update status failed, queuing", "path", path, "error", err)
		s.queueStatusUpdate(path, status, indexName)
		s.recordWriteResult(false)
		return nil
	}

	s.recordWriteResult(true)
	return nil
}

// BulkIndex 使用 ES Bulk API 批量索引多个文档。
//
// 与单个索引操作不同，本方法的返回值区分了两种失败场景：
//   - 连接级失败（ES 不可达）→ 所有文档入队重试，返回 nil
//   - 部分文档失败（连接正常但个别文档写入失败）→ 返回包含失败数量的错误，调用方可据此处理
//
// 断路器打开时，所有文档直接入队，不尝试写入。
func (s *IndexerService) BulkIndex(ctx context.Context, docs []*types.PhotoDocument, indexName string) error {
	s.logger.Debug("BulkIndex", "count", len(docs), "index", indexName)

	if s.isCircuitOpen() {
		for _, doc := range docs {
			s.queueDocument(doc, indexName)
		}
		return nil
	}

	failed, err := s.bulkIndexDirect(ctx, docs, indexName)
	if err != nil {
		s.logger.Warn("bulk index connection failed, queuing", "count", len(docs), "error", err)
		for _, doc := range docs {
			s.queueDocument(doc, indexName)
		}
		s.recordWriteResult(false)
		return nil
	}

	if failed > 0 {
		s.recordWriteResult(true)
		return fmt.Errorf("bulk index: %d of %d documents failed", failed, len(docs))
	}

	s.recordWriteResult(true)
	return nil
}

// --------------------------------------------------------------------------
// 公开读取操作 — 直通 ES，无需断路器（读操作不改变状态）
// --------------------------------------------------------------------------

// UpdateEXIF 仅更新照片文档的 EXIF 字段，使用 ES Update API 的 "doc" 部分更新。
// AI 分析结果、GeoInfo 等其他字段不受影响。
func (s *IndexerService) UpdateEXIF(ctx context.Context, path string, exif *types.EXIFInfo, indexName string) error {
	docID := sha256hex(path)

	updateBody := map[string]any{
		"doc": map[string]any{
			"exif": exif,
		},
	}
	bodyBytes, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("marshal update body: %w", err)
	}

	_, err = s.client.Client().Update(ctx, opensearchapi.UpdateReq{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(bodyBytes),
		Params:     opensearchapi.UpdateParams{Refresh: "true"},
	})
	if err != nil {
		return fmt.Errorf("update exif: %w", err)
	}

	return nil
}

// DeletePhoto 根据文件路径删除对应的 ES 文档。
// 文档 _id 为 path 的 SHA-256 哈希值。即使文档不存在（404）也不返回错误，
// 因为删除一个不存在的文档在语义上是成功的。
func (s *IndexerService) DeletePhoto(ctx context.Context, path string, indexName string) error {
	docID := sha256hex(path)
	s.logger.Debug("DeletePhoto", "path", path, "doc_id", docID, "index", indexName)

	_, err := s.client.Client().Document.Delete(ctx, opensearchapi.DocumentDeleteReq{
		Index:      indexName,
		DocumentID: docID,
	})
	if err != nil {
		// opensearch-go returns error even on 404; treat not_found as success
		if strings.Contains(err.Error(), "not_found") || strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("delete document: %w", err)
	}

	return nil
}

// GetPhoto 根据文件路径查询照片文档。
// 返回 *AppError（code=NOT_FOUND）当文档不存在时（HTTP 404），
// 其他 ES 错误包装为普通 error 返回。
func (s *IndexerService) GetPhoto(ctx context.Context, path string, indexName string) (*types.PhotoDocument, error) {
	docID := sha256hex(path)
	s.logger.Debug("GetPhoto", "path", path, "doc_id", docID, "index", indexName)

	resp, err := s.client.Client().Document.Get(ctx, opensearchapi.DocumentGetReq{
		Index:      indexName,
		DocumentID: docID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not_found") || strings.Contains(err.Error(), "404") {
			return nil, apperrors.NewNotFoundError("photo not found: " + path)
		}
		return nil, fmt.Errorf("get document: %w", err)
	}
	if !resp.Found {
		return nil, apperrors.NewNotFoundError("photo not found: " + path)
	}

	var doc types.PhotoDocument
	if err := json.Unmarshal(resp.Source, &doc); err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}

	return &doc, nil
}

// GetPhotoByID 根据照片 ID 从 OpenSearch 获取照片文档。
// 返回完整的 PhotoDocument，包含 EXIF、分析结果、地理位置等信息。
// 如果文档不存在，返回 AppError（code=NOT_FOUND）。
func (s *IndexerService) GetPhotoByID(ctx context.Context, id string, indexName string) (*types.PhotoDocument, error) {
	s.logger.Debug("GetPhotoByID", "id", id, "index", indexName)

	resp, err := s.client.Client().Document.Get(ctx, opensearchapi.DocumentGetReq{
		Index:      indexName,
		DocumentID: id,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not_found") || strings.Contains(err.Error(), "404") {
			return nil, apperrors.NewNotFoundError("photo not found: " + id)
		}
		return nil, fmt.Errorf("get document: %w", err)
	}
	if !resp.Found {
		return nil, apperrors.NewNotFoundError("photo not found: " + id)
	}

	var doc types.PhotoDocument
	if err := json.Unmarshal(resp.Source, &doc); err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}

	return &doc, nil
}

// ScrollAll 使用 search_after 遍历索引中的所有文档，支持超过 10k 条记录。
// 对每个文档调用 callback 函数，callback 返回 error 时终止遍历。
func (s *IndexerService) ScrollAll(ctx context.Context, indexName string, callback func(*types.PhotoDocument) error) error {
	query := map[string]any{
		"query": map[string]any{
			"match_all": map[string]any{},
		},
		"size": 1000,
		"sort": []map[string]any{
			{"_doc": "asc"},
		},
	}

	var searchAfter []any
	for {
		if searchAfter != nil {
			query["search_after"] = searchAfter
		}

		bodyBytes, err := json.Marshal(query)
		if err != nil {
			return fmt.Errorf("marshal scroll query: %w", err)
		}

		resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
			Indices: []string{indexName},
			Body:    bytes.NewReader(bodyBytes),
		})
		if err != nil {
			return fmt.Errorf("scroll search: %w", err)
		}

		if len(resp.Hits.Hits) == 0 {
			return nil
		}

		for _, hit := range resp.Hits.Hits {
			var doc types.PhotoDocument
			if err := json.Unmarshal(hit.Source, &doc); err != nil {
				s.logger.Warn("scroll: unmarshal failed, skipping", "id", hit.ID, "error", err)
				continue
			}
			if err := callback(&doc); err != nil {
				return err
			}
		}

		lastHit := resp.Hits.Hits[len(resp.Hits.Hits)-1]
		searchAfter = lastHit.Sort
	}
}

// ListAnalyzed 查询所有 status=analyzed 的文档，返回 path → mtime 映射。
//
// 用途：流水线启动时调用，快速获取已分析照片列表。
// 通过对比文件系统中的 mtime 与 ES 中记录的 mtime 来判断照片是否需要重新分析。
// 最大返回 10000 条记录，仅获取 path 和 mtime 两个字段以节省带宽。
func (s *IndexerService) ListAnalyzed(ctx context.Context, indexName string) (map[string]int64, error) {
	result := make(map[string]int64)

	query := map[string]any{
		"query": map[string]any{
			"term": map[string]any{"status": "analyzed"},
		},
		"size":    10000,
		"_source": []string{"path", "mtime"},
	}

	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("list analyzed: %w", err)
	}

	for _, hit := range resp.Hits.Hits {
		var src struct {
			Path  string `json:"path"`
			MTime int64  `json:"mtime"`
		}
		if err := json.Unmarshal(hit.Source, &src); err != nil {
			continue
		}
		result[src.Path] = src.MTime
	}

	s.logger.Debug("ListAnalyzed", "count", len(result), "index", indexName)
	return result, nil
}

// ListByStatuses 查询指定状态的文档列表，返回 path 列表。
//
// 用途：流水线启动时调用，优先获取失败或待重试的照片路径。
// 这些照片应优先于文件系统扫描进行处理。
// 最大返回 10000 条记录，仅获取 path 字段以节省带宽。
func (s *IndexerService) ListByStatuses(ctx context.Context, indexName string, statuses []types.JobStatus) ([]string, error) {
	if len(statuses) == 0 {
		return nil, nil
	}

	statusValues := make([]string, len(statuses))
	for i, st := range statuses {
		statusValues[i] = string(st)
	}

	query := map[string]any{
		"query": map[string]any{
			"terms": map[string]any{"status": statusValues},
		},
		"size":    10000,
		"_source": []string{"path"},
	}

	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("list by statuses: %w", err)
	}

	paths := make([]string, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		var src struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(hit.Source, &src); err != nil {
			s.logger.Warn("ListByStatuses: unmarshal failed, skipping", "id", hit.ID, "error", err)
			continue
		}
		if src.Path != "" {
			paths = append(paths, src.Path)
		}
	}

	s.logger.Debug("ListByStatuses", "statuses", statusValues, "count", len(paths), "index", indexName)
	return paths, nil
}

// --------------------------------------------------------------------------
// 内部方法：直接 ES 操作（不走断路器，供重试/排空队列时使用）
// --------------------------------------------------------------------------

// indexDocDirect 执行原始的 ES index 请求，文档 _id = sha256(path)。
// 设置 Refresh=true 确保写入立即可见（适合低吞吐场景）。
// 成功时记录 result/seq_no/primary_term 等 ES 返回的元信息。
func (s *IndexerService) indexDocDirect(ctx context.Context, doc *types.PhotoDocument, indexName string) error {
	docID := sha256hex(doc.Path)
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal document: %w", err)
	}

	resp, err := s.client.Client().Index(ctx, opensearchapi.IndexReq{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(body),
		Params:     opensearchapi.IndexParams{Refresh: "true"},
	})
	if err != nil {
		return err
	}

	s.logger.Debug("indexed document",
		"path", doc.Path,
		"result", resp.Result,
		"seq_no", resp.SeqNo,
		"primary_term", resp.PrimaryTerm,
	)
	return nil
}

// updateStatusDirect 使用 ES Update API 的 "doc" 模式仅更新 status 字段。
// 这是 partial update，不会覆盖文档中的其他字段（如 description、tags 等）。
// 设置 Refresh=true 确保更新立即可搜索到。
func (s *IndexerService) updateStatusDirect(ctx context.Context, path string, status types.JobStatus, indexName string) error {
	docID := sha256hex(path)

	updateBody := map[string]any{
		"doc": map[string]any{
			"status": status,
		},
	}
	bodyBytes, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("marshal update body: %w", err)
	}

	_, err = s.client.Client().Update(ctx, opensearchapi.UpdateReq{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(bodyBytes),
		Params:     opensearchapi.UpdateParams{Refresh: "true"},
	})
	if err != nil {
		return err
	}

	return nil
}

// bulkIndexDirect 使用 opensearchutil.BulkIndexer 执行批量索引。
//
// BulkIndexer 配置：
//   - NumWorkers: 4 — 4 个并发 worker 发送批量请求
//   - FlushBytes: 5MB — 累积 5MB 数据后自动刷新
//   - FlushInterval: 30s — 最长 30 秒必须刷新一次
//
// 返回值：
//   - failed: 个别文档写入失败的数量（通过 OnFailure 回调统计）
//   - error: 连接级错误（如 BulkIndexer 创建失败、Close 失败）
//
// OnFailure 回调在文档写入失败时被调用，使用 mutex 保护 failed 计数器的并发更新。
func (s *IndexerService) bulkIndexDirect(ctx context.Context, docs []*types.PhotoDocument, indexName string) (int, error) {
	bi, err := opensearchutil.NewBulkIndexer(opensearchutil.BulkIndexerConfig{
		Index:         indexName,
		Client:        s.client.Client(),
		NumWorkers:    4,
		FlushBytes:    5 * 1024 * 1024,
		FlushInterval: 30 * time.Second,
	})
	if err != nil {
		return 0, fmt.Errorf("create bulk indexer: %w", err)
	}

	var (
		mu     sync.Mutex
		failed int
	)

	for _, doc := range docs {
		docID := sha256hex(doc.Path)
		body, err := json.Marshal(doc)
		if err != nil {
			mu.Lock()
			failed++
			mu.Unlock()
			continue
		}

		err = bi.Add(ctx, opensearchutil.BulkIndexerItem{
			Action:     "index",
			DocumentID: docID,
			Body:       bytes.NewReader(body),
			OnFailure: func(_ context.Context, _ opensearchutil.BulkIndexerItem, _ opensearchapi.BulkRespItem, _ error) {
				mu.Lock()
				failed++
				mu.Unlock()
			},
		})
		if err != nil {
			mu.Lock()
			failed++
			mu.Unlock()
		}
	}

	if closeErr := bi.Close(ctx); closeErr != nil {
		return 0, fmt.Errorf("close bulk indexer: %w", closeErr)
	}

	return failed, nil
}

// --------------------------------------------------------------------------
// 断路器与重试队列 — 健康检查、失败计数、队列管理
// --------------------------------------------------------------------------

// isCircuitOpen 返回断路器当前状态（读锁保护）。
func (s *IndexerService) isCircuitOpen() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.circuitOpen
}

// recordWriteResult 根据写入结果更新断路器状态机。
//
// 状态转移规则：
//   - 写入成功 → failureCount 重置为 0，若断路器之前是打开状态则关闭（记录 Info 日志）
//   - 写入失败 → failureCount 递增
//     - 若 failureCount >= maxFailures(3) 且断路器尚未打开 → 打开断路器（记录 Warn 日志）
//
// 使用写锁保护，确保状态变更的原子性。
// recordWriteResult 记录写入结果。成功时重置失败计数并关闭断路器；
// 失败时累加计数，达到 maxFailures(3) 阈值后打开断路器。
func (s *IndexerService) recordWriteResult(success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if success {
		s.failureCount = 0
		if s.circuitOpen {
			s.circuitOpen = false
			s.logger.Info("circuit breaker closed")
		}
	} else {
		s.failureCount++
		if s.failureCount >= s.maxFailures && !s.circuitOpen {
			s.circuitOpen = true
			s.logger.Warn("circuit breaker opened", "failure_count", s.failureCount)
		}
	}
}

// queueDocument 将文档索引操作放入重试队列（非阻塞）。
// 队列满时丢弃该项并记录 Warn 日志——这是有意的设计选择：
// 宁可丢失部分写入也不阻塞生产端。
// queueDocument 将文档索引操作放入重试队列。队列满时丢弃并记录警告。
func (s *IndexerService) queueDocument(doc *types.PhotoDocument, indexName string) {
	select {
	case s.queue <- queueItem{kind: queueItemIndex, doc: doc, indexName: indexName}:
		s.logger.Debug("queued document for retry", "path", doc.Path)
	default:
		s.logger.Warn("queue full, dropping document", "path", doc.Path)
	}
}

// queueStatusUpdate 将状态更新操作放入重试队列（非阻塞）。
// 队列满时丢弃该项并记录 Warn 日志。
// queueStatusUpdate 将状态更新操作放入重试队列。队列满时丢弃并记录警告。
func (s *IndexerService) queueStatusUpdate(path string, status types.JobStatus, indexName string) {
	select {
	case s.queue <- queueItem{kind: queueItemUpdateStatus, path: path, status: status, indexName: indexName}:
		s.logger.Debug("queued status update for retry", "path", path)
	default:
		s.logger.Warn("queue full, dropping status update", "path", path)
	}
}

// runCircuitBreaker 后台协程，每 healthCheckInterval(5s) 检查断路器状态。
// 断路器打开时调用 pingES 检测 ES 健康，恢复后关闭断路器并排空队列。
// 收到 done 信号后先排空队列再退出。
// runCircuitBreaker 是后台 goroutine，负责断路器健康检查和队列排空。
//
// 工作循环：
//  1. 每 healthCheckInterval(5s) 触发一次 tick
//  2. 如果断路器关闭 → 跳过（无操作）
//  3. 如果断路器打开 → ping ES
//     - ping 成功 → 关闭断路器、重置 failureCount、排空重试队列
//     - ping 失败 → 继续等待下一次 tick
//  4. 收到 done 信号 → 执行最终 drainQueue 后退出
func (s *IndexerService) runCircuitBreaker() {
	ticker := time.NewTicker(s.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			s.drainQueue()
			return
		case <-ticker.C:
			s.mu.RLock()
			open := s.circuitOpen
			s.mu.RUnlock()

			if !open {
				continue
			}

			if s.pingES() {
				s.logger.Info("ES health check passed, draining queue")
				s.mu.Lock()
				s.circuitOpen = false
				s.failureCount = 0
				s.mu.Unlock()
				s.drainQueue()
			}
		}
	}
}

// pingES 通过调用 ES Info API 检查 ES 是否可达且响应正常。
// 返回 true 表示 ES 健康，false 表示连接失败或返回错误状态码。
func (s *IndexerService) pingES() bool {
	_, err := s.client.Client().Info(context.Background(), nil)
	return err == nil
}

// drainQueue 排空重试队列中的所有待处理项。
//
// 处理逻辑：
//   - 非阻塞地从 channel 中取出每一项
//   - 每项使用 10 秒超时的 context 执行对应的直接写入操作
//   - 写入成功 → 计数 +1
//   - 写入失败 → 记录 Warn 日志并丢弃该项（不会重新入队，避免无限循环）
//   - 队列为空时打印排空统计并返回
//
// 调用时机：断路器恢复时 & 优雅关闭时（收到 done 信号后）。
func (s *IndexerService) drainQueue() {
	count := 0
	for {
		select {
		case item := <-s.queue:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			var retryErr error
			switch item.kind {
			case queueItemIndex:
				retryErr = s.indexDocDirect(ctx, item.doc, item.indexName)
			case queueItemUpdateStatus:
				retryErr = s.updateStatusDirect(ctx, item.path, item.status, item.indexName)
			}
			cancel()

			if retryErr != nil {
				s.logger.Warn("retry failed, dropping queue item",
					"kind", item.kind,
					"index", item.indexName,
					"error", retryErr,
				)
			} else {
				count++
			}
		default:
			if count > 0 {
				s.logger.Info("queue drained", "count", count)
			}
			return
		}
	}
}

// sha256hex 返回字符串 s 的十六进制 SHA-256 哈希值。
// 用作 ES 文档 _id，确保相同路径始终映射到相同文档 ID。
func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
