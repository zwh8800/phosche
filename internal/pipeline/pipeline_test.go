package pipeline_test

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zwh8800/phosche/internal/pipeline"
	"github.com/zwh8800/phosche/internal/types"
)

func createTestJPEG(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, jpeg.Encode(f, img, &jpeg.Options{Quality: 85}))
	return path
}

type mockScanner struct {
	files []string
	err   error
}

func (m *mockScanner) Scan(_ context.Context, _ []string, _ map[string]int64) (<-chan string, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan string, len(m.files)+1)
	go func() {
		defer close(ch)
		for _, f := range m.files {
			ch <- f
		}
	}()
	return ch, nil
}

type mockWatcher struct {
	events chan types.FileEvent
	mu     sync.Mutex
	closed bool
}

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		events: make(chan types.FileEvent, 10),
	}
}

func (m *mockWatcher) Watch(_ context.Context, _ []string, _ bool) (<-chan types.FileEvent, error) {
	return m.events, nil
}

func (m *mockWatcher) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.events)
	}
	return nil
}

func (m *mockWatcher) Send(event types.FileEvent) {
	m.events <- event
}

type mockAnalyzer struct {
	mu            sync.Mutex
	concurrent    int32
	maxConcurrent int32
	blockCh       chan struct{}
	result        *types.AnalysisResult
	err           error
	analyzeFn     func(ctx context.Context, imageData []byte, locationContext string) (*types.AnalysisResult, error)
}

func (m *mockAnalyzer) Analyze(ctx context.Context, imageData []byte, locationContext string) (*types.AnalysisResult, error) {
	if m.analyzeFn != nil {
		return m.analyzeFn(ctx, imageData, locationContext)
	}

	cur := atomic.AddInt32(&m.concurrent, 1)
	defer atomic.AddInt32(&m.concurrent, -1)

	for {
		old := atomic.LoadInt32(&m.maxConcurrent)
		if cur <= old {
			break
		}
		if atomic.CompareAndSwapInt32(&m.maxConcurrent, old, cur) {
			break
		}
	}

	if m.blockCh != nil {
		select {
		case <-m.blockCh:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return m.result, m.err
}

func (m *mockAnalyzer) MaxConcurrent() int32 {
	return atomic.LoadInt32(&m.maxConcurrent)
}

type statusUpdate struct {
	path   string
	status types.JobStatus
}

type mockIndexer struct {
	mu            sync.Mutex
	indexedDocs   []*types.PhotoDocument
	statusUpdates []statusUpdate
	stopCalled    bool
	indexErr      error
	statusErr     error
}

func (m *mockIndexer) IndexPhoto(_ context.Context, doc *types.PhotoDocument, _ string) error {
	m.mu.Lock()
	m.indexedDocs = append(m.indexedDocs, doc)
	m.mu.Unlock()
	return m.indexErr
}

func (m *mockIndexer) UpdateStatus(_ context.Context, path string, status types.JobStatus, _ string) error {
	m.mu.Lock()
	m.statusUpdates = append(m.statusUpdates, statusUpdate{path: path, status: status})
	m.mu.Unlock()
	return m.statusErr
}

func (m *mockIndexer) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalled = true
}

func (m *mockIndexer) GetPhoto(_ context.Context, _ string, _ string) (*types.PhotoDocument, error) {
	return nil, nil
}

func (m *mockIndexer) LastStatusFor(path string) (types.JobStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.statusUpdates) - 1; i >= 0; i-- {
		if m.statusUpdates[i].path == path {
			return m.statusUpdates[i].status, true
		}
	}
	return "", false
}

func (m *mockIndexer) IndexedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.indexedDocs)
}

func runPipeline(t *testing.T, p *pipeline.Pipeline) (cancel context.CancelFunc, wait func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- p.Run(ctx) }()
	return cancel, func() error {
		select {
		case err := <-errCh:
			return err
		case <-time.After(10 * time.Second):
			t.Fatal("pipeline did not exit within timeout")
			return nil
		}
	}
}

func TestPipeline_E2E(t *testing.T) {
	dir := t.TempDir()
	photoPath := createTestJPEG(t, dir, "photo1.jpg")

	indexer := &mockIndexer{}
	analyzer := &mockAnalyzer{
		result: &types.AnalysisResult{Description: "a test photo"},
	}

	p := pipeline.NewPipeline(pipeline.PipelineConfig{
		Scanner:     &mockScanner{files: []string{photoPath}},
		Watcher:     newMockWatcher(),
		Analyzer:    analyzer,
		Indexer:     indexer,
		IndexName:   "photos",
		Dirs:        []string{dir},
		Concurrency: 1,
	InitialScan: true,
		QueueSize:   10,
	})

	cancel, wait := runPipeline(t, p)

	require.Eventually(t, func() bool {
		return indexer.IndexedCount() >= 2
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, wait())

	lastDoc := indexer.indexedDocs[len(indexer.indexedDocs)-1]
	require.Equal(t, 2, indexer.IndexedCount())
	require.Equal(t, types.StatusAnalyzed, lastDoc.Status)
	require.Equal(t, "a test photo", lastDoc.Description)
	require.True(t, indexer.stopCalled)
}

func TestPipeline_Failure(t *testing.T) {
	dir := t.TempDir()
	goodPath := createTestJPEG(t, dir, "good.jpg")
	badPath := createTestJPEG(t, dir, "bad.jpg")

	indexer := &mockIndexer{}

	var callCount int32
	analyzer := &mockAnalyzer{
		analyzeFn: func(_ context.Context, imageData []byte, locationContext string) (*types.AnalysisResult, error) {
			c := atomic.AddInt32(&callCount, 1)
			if c == 1 {
				return nil, errors.New("permanent decode error")
			}
			return &types.AnalysisResult{Description: "ok"}, nil
		},
	}

	p := pipeline.NewPipeline(pipeline.PipelineConfig{
		Scanner:     &mockScanner{files: []string{badPath, goodPath}},
		Watcher:     newMockWatcher(),
		Analyzer:    analyzer,
		Indexer:     indexer,
		IndexName:   "photos",
		Dirs:        []string{dir},
		Concurrency: 1,
	InitialScan: true,
		QueueSize:   10,
	})

	cancel, wait := runPipeline(t, p)

	require.Eventually(t, func() bool {
		return indexer.IndexedCount() >= 2
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, wait())

	lastDoc := indexer.indexedDocs[len(indexer.indexedDocs)-1]
	require.Equal(t, 3, indexer.IndexedCount())
	require.Equal(t, types.StatusAnalyzed, lastDoc.Status)

	status, found := indexer.LastStatusFor(badPath)
	require.True(t, found)
	require.Equal(t, types.StatusFailed, status)
}

func TestPipeline_LLMUnavailable(t *testing.T) {
	dir := t.TempDir()
	photoPath := createTestJPEG(t, dir, "photo.jpg")

	indexer := &mockIndexer{}
	analyzer := &mockAnalyzer{
		analyzeFn: func(_ context.Context, imageData []byte, locationContext string) (*types.AnalysisResult, error) {
			return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		},
	}

	p := pipeline.NewPipeline(pipeline.PipelineConfig{
		Scanner:           &mockScanner{files: []string{photoPath}},
		Watcher:           newMockWatcher(),
		Analyzer:          analyzer,
		Indexer:           indexer,
		IndexName:         "photos",
		Dirs:              []string{dir},
		Concurrency:       1,
	InitialScan:       true,
		QueueSize:         10,
		RetryInterval:     50 * time.Millisecond,
		MaxPendingRetries: 2,
	})

	cancel, wait := runPipeline(t, p)

	require.Eventually(t, func() bool {
		s, _ := indexer.LastStatusFor(photoPath)
		return s == types.StatusPendingAnalysis
	}, 2*time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		s, _ := indexer.LastStatusFor(photoPath)
		return s == types.StatusFailed
	}, 5*time.Second, 50*time.Millisecond)

	cancel()
	require.NoError(t, wait())
}

func TestPipeline_Concurrency(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 4)
	for i := 0; i < 4; i++ {
		paths[i] = createTestJPEG(t, dir, fmt.Sprintf("photo%d.jpg", i))
	}

	indexer := &mockIndexer{}

	blockCh := make(chan struct{})
	analyzer := &mockAnalyzer{
		blockCh: blockCh,
		result:  &types.AnalysisResult{Description: "ok"},
	}

	p := pipeline.NewPipeline(pipeline.PipelineConfig{
		Scanner:     &mockScanner{files: paths},
		Watcher:     newMockWatcher(),
		Analyzer:    analyzer,
		Indexer:     indexer,
		IndexName:   "photos",
		Dirs:        []string{dir},
		Concurrency: 2,
	InitialScan: true,
		QueueSize:   10,
	})

	cancel, wait := runPipeline(t, p)

	time.Sleep(300 * time.Millisecond)
	require.Equal(t, int32(2), analyzer.MaxConcurrent())

	close(blockCh)

	require.Eventually(t, func() bool {
		return indexer.IndexedCount() >= 8
	}, 10*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, wait())
	require.Equal(t, 8, indexer.IndexedCount())
}

func TestPipeline_GracefulShutdown(t *testing.T) {
	dir := t.TempDir()
	photoPath := createTestJPEG(t, dir, "photo.jpg")

	blockCh := make(chan struct{})
	indexer := &mockIndexer{}
	analyzer := &mockAnalyzer{
		blockCh: blockCh,
		result:  &types.AnalysisResult{Description: "ok"},
	}

	p := pipeline.NewPipeline(pipeline.PipelineConfig{
		Scanner:     &mockScanner{files: []string{photoPath}},
		Watcher:     newMockWatcher(),
		Analyzer:    analyzer,
		Indexer:     indexer,
		IndexName:   "photos",
		Dirs:        []string{dir},
		Concurrency: 1,
	InitialScan: true,
		QueueSize:   10,
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- p.Run(ctx) }()

	time.Sleep(200 * time.Millisecond)
	cancel()
	close(blockCh)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout: pipeline did not exit")
	}

	require.True(t, indexer.stopCalled)
	require.Equal(t, 2, indexer.IndexedCount())
}

func TestPipeline_Backpressure(t *testing.T) {
	dir := t.TempDir()
	createTestJPEG(t, dir, "photo1.jpg")
	createTestJPEG(t, dir, "photo2.jpg")
	createTestJPEG(t, dir, "photo3.jpg")

	indexer := &mockIndexer{}
	analyzer := &mockAnalyzer{
		result: &types.AnalysisResult{Description: "ok"},
	}

	w := newMockWatcher()

	p := pipeline.NewPipeline(pipeline.PipelineConfig{
		Scanner:     &mockScanner{files: nil},
		Watcher:     w,
		Analyzer:    analyzer,
		Indexer:     indexer,
		IndexName:   "photos",
		Dirs:        []string{dir},
		Concurrency: 1,
	InitialScan: true,
		QueueSize:   1,
	})

	cancel, wait := runPipeline(t, p)

	w.Send(types.FileEvent{Path: filepath.Join(dir, "photo1.jpg"), Op: types.OpCreate})
	w.Send(types.FileEvent{Path: filepath.Join(dir, "photo2.jpg"), Op: types.OpCreate})
	w.Send(types.FileEvent{Path: filepath.Join(dir, "photo3.jpg"), Op: types.OpCreate})

	require.Eventually(t, func() bool {
		return indexer.IndexedCount() >= 6
	}, 10*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, wait())
	require.Equal(t, 6, indexer.IndexedCount())
}

func TestPipeline_WatcherEvent(t *testing.T) {
	dir := t.TempDir()
	photoPath := createTestJPEG(t, dir, "new_photo.jpg")

	indexer := &mockIndexer{}
	analyzer := &mockAnalyzer{
		result: &types.AnalysisResult{Description: "watcher event"},
	}

	w := newMockWatcher()

	p := pipeline.NewPipeline(pipeline.PipelineConfig{
		Scanner:     &mockScanner{files: nil},
		Watcher:     w,
		Analyzer:    analyzer,
		Indexer:     indexer,
		IndexName:   "photos",
		Dirs:        []string{dir},
		Concurrency: 1,
	InitialScan: true,
		QueueSize:   10,
	})

	cancel, wait := runPipeline(t, p)

	time.Sleep(50 * time.Millisecond)

	w.Send(types.FileEvent{
		Path: photoPath,
		Op:   types.OpCreate,
	})

	require.Eventually(t, func() bool {
		return indexer.IndexedCount() >= 2
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, wait())

	lastDoc := indexer.indexedDocs[len(indexer.indexedDocs)-1]
	require.Equal(t, 2, indexer.IndexedCount())
	require.Equal(t, photoPath, lastDoc.Path)
	require.Equal(t, "watcher event", lastDoc.Description)
}

func TestPipeline_StatusUpdate(t *testing.T) {
	dir := t.TempDir()
	photoPath := createTestJPEG(t, dir, "photo.jpg")

	indexer := &mockIndexer{}
	analyzer := &mockAnalyzer{
		result: &types.AnalysisResult{Description: "status test"},
	}

	p := pipeline.NewPipeline(pipeline.PipelineConfig{
		Scanner:     &mockScanner{files: []string{photoPath}},
		Watcher:     newMockWatcher(),
		Analyzer:    analyzer,
		Indexer:     indexer,
		IndexName:   "photos",
		Dirs:        []string{dir},
		Concurrency: 1,
	InitialScan: true,
		QueueSize:   10,
	})

	cancel, wait := runPipeline(t, p)

	require.Eventually(t, func() bool {
		return indexer.IndexedCount() >= 2
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, wait())

	require.Equal(t, types.StatusAnalyzing, indexer.indexedDocs[0].Status)

	lastDoc := indexer.indexedDocs[len(indexer.indexedDocs)-1]
	require.Equal(t, 2, indexer.IndexedCount())
	require.Equal(t, types.StatusAnalyzed, lastDoc.Status)
}

func TestIsLLMConnectionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"net.OpError", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"wrapped net error", fmt.Errorf("wrap: %w", &net.OpError{Op: "read", Err: errors.New("timeout")}), true},
		{"regular error", errors.New("something failed"), false},
		{"decode error", fmt.Errorf("decode: %w", errors.New("invalid JPEG")), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, pipeline.IsLLMConnectionError(tt.err))
		})
	}
}
