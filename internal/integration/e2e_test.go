package integration

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zwh8800/phosche/internal/config"
	"github.com/zwh8800/phosche/internal/indexer"
	"github.com/zwh8800/phosche/internal/pipeline"
	"github.com/zwh8800/phosche/internal/search"
	"github.com/zwh8800/phosche/internal/types"
)

func dockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

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
}

func (m *mockScanner) Scan(_ context.Context, _ []string, _ map[string]int64) (<-chan string, error) {
	ch := make(chan string, len(m.files))
	for _, f := range m.files {
		ch <- f
	}
	close(ch)
	return ch, nil
}

type mockWatcher struct {
	events chan types.FileEvent
	mu     sync.Mutex
	closed bool
}

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		events: make(chan types.FileEvent),
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

type mockAnalyzer struct {
	result *types.AnalysisResult
}

func (m *mockAnalyzer) Analyze(_ context.Context, _ []byte, _ string) (*types.AnalysisResult, error) {
	return m.result, nil
}

func startESContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "opensearchproject/opensearch:2.19.5",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":            "single-node",
			"DISABLE_SECURITY_PLUGIN":   "true",
			"OPENSEARCH_JAVA_OPTS":      "-Xms512m -Xmx512m",
		},
		WaitingFor: wait.ForHTTP("/").WithPort("9200/tcp").WithStartupTimeout(90 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start OpenSearch container")

	cleanup := func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer termCancel()
		if err := container.Terminate(termCtx); err != nil {
			t.Logf("failed to terminate ES container: %v", err)
		}
	}

	mappedPort, err := container.MappedPort(ctx, "9200")
	require.NoError(t, err, "failed to get mapped port")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get container host")

	address := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	return container, address, cleanup
}

func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, address, cleanup := startESContainer(t, ctx)
	defer cleanup()
	_ = container

	tempDir := t.TempDir()
	photoPath := createTestJPEG(t, tempDir, "test_photo.jpg")

	osCfg := config.OSConfig{Addresses: []string{address}}
	osClient, err := indexer.NewOSClient(osCfg)
	require.NoError(t, err)

	indexName := "test_photos_e2e"
	err = osClient.EnsureIndex(ctx, indexName, 0)
	require.NoError(t, err)

	idxService := indexer.NewIndexerService(osClient, 10)

	scn := &mockScanner{files: []string{photoPath}}
	watcher := newMockWatcher()
	analyzer := &mockAnalyzer{
		result: &types.AnalysisResult{
			Description: "test image",
			Tags:        []string{"test"},
			Objects:     []string{"test"},
			SceneType:   "indoor",
			Colors:      []types.ColorInfo{{Name: "红色", Hex: "#EF4444"}},
			PeopleCount: 0,
			HasText:     false,
		},
	}

	p := pipeline.NewPipeline(pipeline.PipelineConfig{
		Watcher:     watcher,
		Scanner:     scn,
		Analyzer:    analyzer,
		Indexer:     idxService,
		IndexName:   indexName,
		Dirs:        []string{tempDir},
		Concurrency: 1,
		QueueSize:   10,
	})

	pipelineCtx, pipelineCancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- p.Run(pipelineCtx) }()

	searchSvc := search.NewSearchService(osClient)

	require.Eventually(t, func() bool {
		resp, err := searchSvc.Search(context.Background(), indexName, &types.SearchRequest{
			Status:   "analyzed",
			Page:     1,
			PageSize: 10,
		}, "")
		if err != nil {
			t.Logf("search poll error: %v", err)
			return false
		}
		return resp.Total >= 1
	}, 30*time.Second, 500*time.Millisecond, "photo should be analyzed within 30s")

	doc, err := idxService.GetPhoto(ctx, photoPath, indexName)
	require.NoError(t, err, "GetPhoto should find the indexed document")
	assert.Equal(t, types.StatusAnalyzed, doc.Status)
	assert.Equal(t, "test image", doc.Description)
	assert.Equal(t, []string{"test"}, doc.Tags)
	assert.Equal(t, []string{"test"}, doc.Objects)
	assert.Equal(t, "indoor", doc.SceneType)
	require.Len(t, doc.Colors, 1)
	assert.Equal(t, "红色", doc.Colors[0].Name)
	assert.Equal(t, "#EF4444", doc.Colors[0].Hex)
	assert.Equal(t, 0, doc.PeopleCount)
	assert.False(t, doc.HasText)

	resp, err := searchSvc.Search(context.Background(), indexName, &types.SearchRequest{
		Query:    "test",
		Page:     1,
		PageSize: 10,
	}, "")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, resp.Total, int64(1), "search should find the photo")

	found := false
	for _, hit := range resp.Hits {
		if hit.Path == photoPath {
			found = true
			break
		}
	}
	assert.True(t, found, "should find the test photo in search results")

	pipelineCancel()
	select {
	case pipeErr := <-errCh:
		assert.NoError(t, pipeErr)
	case <-time.After(30 * time.Second):
		t.Fatal("pipeline did not exit within timeout")
	}
}
