package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/zwh8800/phosche/internal/decoder"
	"github.com/zwh8800/phosche/internal/types"
	"github.com/zwh8800/phosche/internal/watcher"
)

const (
	defaultConcurrency       = 4
	defaultQueueSize         = 100
	defaultRetryInterval     = 5 * time.Minute
	defaultMaxPendingRetries = 10
	defaultDrainTimeout      = 5 * time.Minute
)

type Analyzer interface {
	Analyze(ctx context.Context, imageData []byte) (*types.AnalysisResult, error)
}

type Indexer interface {
	IndexPhoto(ctx context.Context, doc *types.PhotoDocument, indexName string) error
	UpdateStatus(ctx context.Context, path string, status types.JobStatus, indexName string) error
	ListAnalyzed(ctx context.Context, indexName string) (map[string]int64, error)
	Stop()
}

type PipelineConfig struct {
	Watcher           watcher.Watcher
	Scanner           watcher.Scanner
	Analyzer          Analyzer
	Indexer           Indexer
	IndexName         string
	Dirs              []string
	Recursive         bool
	Concurrency       int
	QueueSize         int
	RetryInterval     time.Duration
	MaxPendingRetries int
}

type pendingItem struct {
	path       string
	retryCount int
}

type Pipeline struct {
	cfg       PipelineConfig
	inputCh   chan string
	pendingMu sync.Mutex
	pending   map[string]*pendingItem

	analyzedMu sync.RWMutex
	analyzed   map[string]int64 // path → mtime, loaded from ES on startup
}

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
		cfg:      cfg,
		inputCh:  make(chan string, cfg.QueueSize),
		pending:  make(map[string]*pendingItem),
		analyzed: make(map[string]int64),
	}
}

func (p *Pipeline) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var fwWg, workersWg, retryWg sync.WaitGroup

	// Start workers before scan so they can consume as scan feeds the channel.
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

	if err := p.scanExisting(ctx); err != nil {
		return fmt.Errorf("pipeline: initial scan: %w", err)
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

func (p *Pipeline) scanExisting(ctx context.Context) error {
	slog.Info("pipeline: starting initial scan", "dirs", p.cfg.Dirs, "recursive", p.cfg.Recursive)

	analyzed, err := p.cfg.Indexer.ListAnalyzed(ctx, p.cfg.IndexName)
	if err != nil {
		slog.Warn("pipeline: list analyzed failed, will process all files", "error", err)
	} else {
		p.analyzedMu.Lock()
		p.analyzed = analyzed
		p.analyzedMu.Unlock()
		slog.Info("pipeline: loaded analyzed photos from ES", "count", len(analyzed))
	}

	paths, err := p.cfg.Scanner.Scan(ctx, p.cfg.Dirs, nil)
	if err != nil {
		return err
	}

	slog.Info("pipeline: initial scan complete", "found", len(paths))

	if len(paths) == 0 {
		slog.Warn("pipeline: no photos found in watched directories, waiting for new files")
		return nil
	}

	queued := 0
	for _, path := range paths {
		select {
		case p.inputCh <- path:
			queued++
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	slog.Info("pipeline: queued scanned photos for processing", "queued", queued, "total", len(paths))
	return nil
}

func (p *Pipeline) forwardEvents(ctx context.Context, eventCh <-chan types.FileEvent) {
	for event := range eventCh {
		if event.Op == types.OpDelete {
			continue
		}
		select {
		case p.inputCh <- event.Path:
		case <-ctx.Done():
			return
		}
	}
}

func (p *Pipeline) worker(_ context.Context) {
	for path := range p.inputCh {
		slog.Info("pipeline: processing photo", "path", path)
		ctx, cancel := context.WithTimeout(context.Background(), defaultDrainTimeout)
		p.processPath(ctx, path)
		cancel()
	}
}

func (p *Pipeline) processPath(ctx context.Context, path string) {
	// Check if already analyzed with same mtime
	info, err := os.Stat(path)
	if err != nil {
		slog.Warn("pipeline: stat failed", "path", path, "error", err)
		return
	}
	mtime := info.ModTime().Unix()

	p.analyzedMu.RLock()
	storedMtime, exists := p.analyzed[path]
	p.analyzedMu.RUnlock()

	if exists && storedMtime == mtime {
		slog.Debug("pipeline: skipping already analyzed", "path", path)
		return
	}

	now := time.Now().Unix()

	// Create an initial document so UpdateStatus has something to update.
	initDoc := &types.PhotoDocument{
		Photo: types.Photo{
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

	if err := p.cfg.Indexer.IndexPhoto(ctx, doc, p.cfg.IndexName); err != nil {
		slog.Warn("pipeline: index failed", "path", path, "error", err)
		return
	}

	p.pendingMu.Lock()
	delete(p.pending, path)
	p.pendingMu.Unlock()
}

type decodeAnalyzeResult struct {
	exif     *types.EXIFInfo
	analysis *types.AnalysisResult
}

func (p *Pipeline) decodeAndAnalyze(ctx context.Context, path string) *decodeAnalyzeResult {
	decodeResult, err := decoder.DecodeImage(path)
	if err != nil {
		slog.Warn("pipeline: decode failed", "path", path, "error", err)
		p.updateErrorStatus(ctx, path, err)
		return nil
	}

	imageBytes, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("pipeline: read file failed", "path", path, "error", err)
		p.updateErrorStatus(ctx, path, err)
		return nil
	}

	analysis, err := p.cfg.Analyzer.Analyze(ctx, imageBytes)
	if err != nil {
		slog.Warn("pipeline: analysis failed", "path", path, "error", err)
		p.updateErrorStatus(ctx, path, err)
		return nil
	}

	return &decodeAnalyzeResult{
		exif:     decodeResult.EXIF,
		analysis: analysis,
	}
}

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
