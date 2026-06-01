package indexer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zwh8800/phosche/internal/types"
)

func setupIndexerTest(t *testing.T) (*IndexerService, string, func()) {
	t.Helper()

	esClient, esCleanup := setupTestES(t)

	ctx := context.Background()
	indexName := fmt.Sprintf("test_indexer_%d", time.Now().UnixNano())

	err := esClient.EnsureIndex(ctx, indexName, 0)
	require.NoError(t, err, "EnsureIndex should succeed")

	svc := NewIndexerService(esClient, 100)

	cleanup := func() {
		svc.Stop()
		esCleanup()
	}

	return svc, indexName, cleanup
}

func newTestDoc(path string) *types.PhotoDocument {
	return &types.PhotoDocument{
		Photo: types.Photo{
			Path:   path,
			MTime:  1234567890,
			Size:   1024,
			Status: types.StatusUnanalyzed,
		},
		AnalysisResult: types.AnalysisResult{
			Description: "Test photo at " + path,
			Tags:        []string{"test"},
			SceneType:   "indoor",
			Confidence:  0.95,
		},
	}
}

// ---------------------------------------------------------------------------
// IndexPhoto
// ---------------------------------------------------------------------------

func TestIndexer_IndexPhoto(t *testing.T) {
	svc, indexName, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()
	doc := newTestDoc("/photos/sunset.jpg")

	err := svc.IndexPhoto(ctx, doc, indexName)
	require.NoError(t, err, "IndexPhoto should succeed")

	got, err := svc.GetPhoto(ctx, "/photos/sunset.jpg", indexName)
	require.NoError(t, err, "GetPhoto should retrieve indexed doc")
	assert.Equal(t, "/photos/sunset.jpg", got.Path)
	assert.Equal(t, types.StatusUnanalyzed, got.Status)
	assert.Equal(t, "Test photo at /photos/sunset.jpg", got.Description)
	assert.Contains(t, got.Tags, "test")
	assert.Equal(t, "indoor", got.SceneType)
	assert.Equal(t, float64(0.95), got.Confidence)
}

func TestIndexer_Upsert(t *testing.T) {
	svc, indexName, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()
	doc := newTestDoc("/photos/cat.jpg")
	doc.Description = "A sleeping cat"

	err := svc.IndexPhoto(ctx, doc, indexName)
	require.NoError(t, err)

	got, err := svc.GetPhoto(ctx, "/photos/cat.jpg", indexName)
	require.NoError(t, err)
	assert.Equal(t, "A sleeping cat", got.Description)

	doc.Description = "A wakeful cat"
	doc.Status = types.StatusAnalyzed
	err = svc.IndexPhoto(ctx, doc, indexName)
	require.NoError(t, err)

	got, err = svc.GetPhoto(ctx, "/photos/cat.jpg", indexName)
	require.NoError(t, err)
	assert.Equal(t, "A wakeful cat", got.Description)
	assert.Equal(t, types.StatusAnalyzed, got.Status)
}

// ---------------------------------------------------------------------------
// UpdateStatus
// ---------------------------------------------------------------------------

func TestIndexer_UpdateStatus(t *testing.T) {
	svc, indexName, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()
	doc := newTestDoc("/photos/dog.jpg")
	doc.Description = "A happy dog"
	doc.Status = types.StatusUnanalyzed

	err := svc.IndexPhoto(ctx, doc, indexName)
	require.NoError(t, err)

	err = svc.UpdateStatus(ctx, "/photos/dog.jpg", types.StatusAnalyzed, indexName)
	require.NoError(t, err)

	got, err := svc.GetPhoto(ctx, "/photos/dog.jpg", indexName)
	require.NoError(t, err)
	assert.Equal(t, types.StatusAnalyzed, got.Status)
	assert.Equal(t, "A happy dog", got.Description, "description should be unchanged")
	assert.Contains(t, got.Tags, "test", "tags should be unchanged")
	assert.Equal(t, "indoor", got.SceneType, "scene_type should be unchanged")
}

// ---------------------------------------------------------------------------
// BulkIndex
// ---------------------------------------------------------------------------

func TestIndexer_BulkIndex(t *testing.T) {
	svc, indexName, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()
	const n = 50
	docs := make([]*types.PhotoDocument, n)
	for i := range n {
		docs[i] = newTestDoc(fmt.Sprintf("/photos/img_%04d.jpg", i))
		docs[i].Description = fmt.Sprintf("Bulk photo %d", i)
		docs[i].Tags = []string{"bulk", fmt.Sprintf("tag_%d", i)}
	}

	err := svc.BulkIndex(ctx, docs, indexName)
	require.NoError(t, err, "BulkIndex should succeed")

	for _, doc := range docs {
		got, err := svc.GetPhoto(ctx, doc.Path, indexName)
		require.NoError(t, err, "GetPhoto should retrieve %s", doc.Path)
		assert.Equal(t, doc.Description, got.Description, "description mismatch for %s", doc.Path)
	}
}

// ---------------------------------------------------------------------------
// GetPhoto not found
// ---------------------------------------------------------------------------

func TestIndexer_GetPhoto_NotFound(t *testing.T) {
	svc, indexName, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()

	_, err := svc.GetPhoto(ctx, "/photos/nonexistent.jpg", indexName)
	require.Error(t, err, "GetPhoto should return error for nonexistent doc")
	assert.Contains(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// Queue on failure
// ---------------------------------------------------------------------------

func TestIndexer_QueueOnFailure(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	esClient, esCleanup := setupTestES(t)

	ctx := context.Background()
	indexName := fmt.Sprintf("test_queue_%d", time.Now().UnixNano())

	err := esClient.EnsureIndex(ctx, indexName, 0)
	require.NoError(t, err, "EnsureIndex should succeed")

	svc := NewIndexerService(esClient, 100)

	doc := newTestDoc("/photos/queued.jpg")
	err = svc.IndexPhoto(ctx, doc, indexName)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_ = svc.IndexPhoto(ctx, newTestDoc(fmt.Sprintf("/photos/queue_%d.jpg", i)), "nonexistent_index")
	}

	queuedDoc := newTestDoc("/photos/after_circuit_open.jpg")
	err = svc.IndexPhoto(ctx, queuedDoc, "nonexistent_index")
	require.NoError(t, err, "should return nil even when circuit is open")

	for i := 0; i < 5; i++ {
		err = svc.IndexPhoto(ctx, newTestDoc(fmt.Sprintf("/photos/recover_%d.jpg", i)), indexName)
		require.NoError(t, err, "writes to valid index should succeed")
	}

	got, err := svc.GetPhoto(ctx, "/photos/queued.jpg", indexName)
	require.NoError(t, err)
	assert.Equal(t, "/photos/queued.jpg", got.Path)

	got, err = svc.GetPhoto(ctx, "/photos/recover_0.jpg", indexName)
	require.NoError(t, err)
	assert.Equal(t, "/photos/recover_0.jpg", got.Path)

	esCleanup()
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestIndexer_UpdateStatus_NotFound(t *testing.T) {
	svc, indexName, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()

	err := svc.UpdateStatus(ctx, "/photos/ghost.jpg", types.StatusAnalyzed, indexName)
	require.NoError(t, err, "UpdateStatus on non-existent doc should not crash")
}

func TestIndexer_BulkIndex_Empty(t *testing.T) {
	svc, indexName, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()

	err := svc.BulkIndex(ctx, []*types.PhotoDocument{}, indexName)
	require.NoError(t, err, "empty bulk should succeed")
}

func TestIndexer_StopDrainsQueue(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	esClient, esCleanup := setupTestES(t)
	defer esCleanup()

	ctx := context.Background()
	indexName := fmt.Sprintf("test_stop_%d", time.Now().UnixNano())

	err := esClient.EnsureIndex(ctx, indexName, 0)
	require.NoError(t, err)

	svc := NewIndexerService(esClient, 10)

	doc := newTestDoc("/photos/stop_drain.jpg")
	for i := 0; i < 5; i++ {
		_ = svc.IndexPhoto(ctx, newTestDoc(fmt.Sprintf("/photos/fail_%d.jpg", i)), "nonexistent_index")
	}
	_ = svc.IndexPhoto(ctx, doc, "nonexistent_index")

	svc.Stop()
}

func TestIndexer_MultipleIndices(t *testing.T) {
	svc, indexName, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()

	idxA := indexName + "_a"
	idxB := indexName + "_b"

	err := svc.client.EnsureIndex(ctx, idxB, 0)
	require.NoError(t, err)

	docA := newTestDoc("/photos/a.jpg")
	docA.Description = "Doc in index A"
	err = svc.IndexPhoto(ctx, docA, idxA)
	require.NoError(t, err)

	docB := newTestDoc("/photos/b.jpg")
	docB.Description = "Doc in index B"
	err = svc.IndexPhoto(ctx, docB, idxB)
	require.NoError(t, err)

	gotA, err := svc.GetPhoto(ctx, "/photos/a.jpg", idxA)
	require.NoError(t, err)
	assert.Equal(t, "Doc in index A", gotA.Description)

	gotB, err := svc.GetPhoto(ctx, "/photos/b.jpg", idxB)
	require.NoError(t, err)
	assert.Equal(t, "Doc in index B", gotB.Description)
}

func TestIndexer_HighConfidence(t *testing.T) {
	svc, indexName, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()
	doc := newTestDoc("/photos/confident.jpg")
	doc.Confidence = 0.999
	doc.PeopleCount = 3
	doc.HasText = true
	doc.Colors = []types.ColorInfo{{Name: "红色", Hex: "#FF0000"}, {Name: "绿色", Hex: "#00FF00"}}

	err := svc.IndexPhoto(ctx, doc, indexName)
	require.NoError(t, err)

	got, err := svc.GetPhoto(ctx, "/photos/confident.jpg", indexName)
	require.NoError(t, err)
	assert.Equal(t, float64(0.999), got.Confidence)
	assert.Equal(t, 3, got.PeopleCount)
	assert.True(t, got.HasText)
	require.Len(t, got.Colors, 2)
	assert.Equal(t, "红色", got.Colors[0].Name)
	assert.Equal(t, "#FF0000", got.Colors[0].Hex)
	assert.Equal(t, "绿色", got.Colors[1].Name)
	assert.Equal(t, "#00FF00", got.Colors[1].Hex)
}
