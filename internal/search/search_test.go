package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zwh8800/phosche/internal/config"
	"github.com/zwh8800/phosche/internal/indexer"
	"github.com/zwh8800/phosche/internal/types"
)

const testIndex = "test_photos_search"

type testDoc struct {
	ID                string   `json:"id"`
	Description       string   `json:"description"`
	Tags              []string `json:"tags"`
	Objects           []string `json:"objects"`
	SceneType         string   `json:"scene_type"`
	CameraModel       string   `json:"camera_model"`
	DateTimeOriginal  string   `json:"date_time_original"`
	Status            string   `json:"status"`
}

var testDocs = []testDoc{
	{
		ID: "1", Description: "A beautiful mountain landscape at sunrise",
		Tags: []string{"nature", "mountain"}, Objects: []string{"mountain", "tree"},
		SceneType: "outdoor", CameraModel: "Canon EOS R5",
		DateTimeOriginal: "2024-06-15T10:00:00Z", Status: "analyzed",
	},
	{
		ID: "2", Description: "Beautiful sunset over the ocean",
		Tags: []string{"nature", "sunset"}, Objects: []string{"ocean", "sun"},
		SceneType: "outdoor", CameraModel: "Sony A7IV",
		DateTimeOriginal: "2024-06-20T18:00:00Z", Status: "analyzed",
	},
	{
		ID: "3", Description: "Indoor portrait with studio lighting",
		Tags: []string{"portrait", "indoor"}, Objects: []string{"person"},
		SceneType: "indoor", CameraModel: "Canon EOS R5",
		DateTimeOriginal: "2024-07-01T14:00:00Z", Status: "analyzed",
	},
	{
		ID: "4", Description: "Mountain hiking trail in autumn",
		Tags: []string{"nature", "mountain", "hiking"}, Objects: []string{"mountain", "trail"},
		SceneType: "outdoor", CameraModel: "Fujifilm X-T5",
		DateTimeOriginal: "2024-05-01T08:00:00Z", Status: "analyzed",
	},
	{
		ID: "5", Description: "City skyline at night",
		Tags: []string{"city", "night"}, Objects: []string{"building", "sky"},
		SceneType: "outdoor", CameraModel: "Sony A7IV",
		DateTimeOriginal: "2024-08-15T22:00:00Z", Status: "analyzed",
	},
}

func dockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func setupSearchTest(t *testing.T) (*SearchService, func()) {
	t.Helper()

	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "docker.elastic.co/elasticsearch/elasticsearch:8.17.0",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":         "single-node",
			"xpack.security.enabled": "false",
			"ES_JAVA_OPTS":           "-Xms512m -Xmx512m",
		},
		WaitingFor: wait.ForHTTP("/").WithPort("9200/tcp").WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start ES container")

	cleanup := func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer termCancel()
		if err := container.Terminate(termCtx); err != nil {
			t.Logf("failed to terminate ES container: %v", err)
		}
	}

	mappedPort, err := container.MappedPort(ctx, "9200")
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	address := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	cfg := config.ESConfig{Addresses: []string{address}}
	esClient, err := indexer.NewESClient(cfg)
	require.NoError(t, err)

	err = esClient.EnsureIndex(ctx, testIndex, 0)
	require.NoError(t, err)

	bulkBody := buildBulkBody(testDocs)
	bulkResp, err := esClient.Client().Bulk(bytes.NewReader(bulkBody),
		esClient.Client().Bulk.WithContext(ctx),
	)
	require.NoError(t, err)
	bulkResp.Body.Close()
	if bulkResp.IsError() {
		t.Fatalf("bulk index failed: %s", bulkResp.Status())
	}

	refreshResp, err := esClient.Client().Indices.Refresh(
		esClient.Client().Indices.Refresh.WithIndex(testIndex),
	)
	require.NoError(t, err)
	refreshResp.Body.Close()

	return NewSearchService(esClient), cleanup
}

func buildBulkBody(docs []testDoc) []byte {
	var buf bytes.Buffer
	for _, doc := range docs {
		meta := map[string]any{
			"index": map[string]any{
				"_index": testIndex,
				"_id":    doc.ID,
			},
		}
		metaBytes, _ := json.Marshal(meta)
		buf.Write(metaBytes)
		buf.WriteByte('\n')

		docBytes, _ := json.Marshal(doc)
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func collectIDs(hits []types.PhotoDocument) []string {
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.ID
	}
	return ids
}

func TestSearch_FullText(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	resp, err := svc.Search(context.Background(), testIndex, &types.SearchRequest{
		Query:    "mountain",
		Page:     1,
		PageSize: 10,
	}, "")
	require.NoError(t, err)
	assert.Greater(t, resp.Total, int64(0), "should find mountain docs")
	assert.Contains(t, collectIDs(resp.Hits), "1", "doc 1 has mountain")
	assert.Contains(t, collectIDs(resp.Hits), "4", "doc 4 has mountain")
}

func TestSearch_DateRange(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	resp, err := svc.Search(context.Background(), testIndex, &types.SearchRequest{
		DateFrom: "2024-06-01",
		DateTo:   "2024-06-30",
		Page:     1,
		PageSize: 10,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.Total, "only 2 docs in June 2024")

	ids := collectIDs(resp.Hits)
	assert.Contains(t, ids, "1", "doc 1 is in June")
	assert.Contains(t, ids, "2", "doc 2 is in June")
}

func TestSearch_CombinedFilters(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	resp, err := svc.Search(context.Background(), testIndex, &types.SearchRequest{
		Query:     "sunset",
		DateFrom:  "2024-01-01",
		SceneType: "outdoor",
		Page:      1,
		PageSize:  10,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Total, "only doc 2 matches all conditions")

	ids := collectIDs(resp.Hits)
	assert.Contains(t, ids, "2", "doc 2 is the sunset outdoor doc")
}

func TestSearch_Pagination(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	resp, err := svc.Search(context.Background(), testIndex, &types.SearchRequest{
		Page:     1,
		PageSize: 2,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, int64(5), resp.Total, "total should be 5")
	assert.Len(t, resp.Hits, 2, "page 1 should return 2 docs")
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 2, resp.PageSize)
	assert.Equal(t, 3, resp.TotalPages, "ceil(5/2)=3")
}

func TestSearch_EmptyResults(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	resp, err := svc.Search(context.Background(), testIndex, &types.SearchRequest{
		Query:    "nonexistent123xyz",
		Page:     1,
		PageSize: 10,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.Total, "should find no docs")
	assert.Empty(t, resp.Hits)
}

func TestGetFilters(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	filters, err := svc.GetFilters(context.Background(), testIndex, "")
	require.NoError(t, err)

	assert.NotEmpty(t, filters.Tags, "should have aggregated tags")
	tagSet := make(map[string]bool)
	for _, t := range filters.Tags {
		tagSet[t] = true
	}
	assert.True(t, tagSet["nature"], "nature tag should be present")
	assert.True(t, tagSet["mountain"], "mountain tag should be present")

	assert.NotEmpty(t, filters.SceneTypes, "should have aggregated scene types")
	for _, s := range filters.SceneTypes {
		assert.True(t, s == "outdoor" || s == "indoor", "unexpected scene type: %s", s)
	}

	assert.NotEmpty(t, filters.Cameras, "should have aggregated cameras")
	camSet := make(map[string]bool)
	for _, c := range filters.Cameras {
		camSet[c] = true
	}
	assert.True(t, camSet["Canon EOS R5"] || camSet["Sony A7IV"] || camSet["Fujifilm X-T5"],
		"should have camera models, got: %v", filters.Cameras)
}

func TestSearch_DefaultsHandling(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	resp, err := svc.Search(context.Background(), testIndex, &types.SearchRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, int64(5), resp.Total, "empty request should match all")
	assert.Equal(t, 1, resp.Page, "default page should be 1")
	assert.Equal(t, 20, resp.PageSize, "default page size should be 20")
	assert.Len(t, resp.Hits, 5, "all 5 docs should be returned")
}

func TestSearch_InvalidIndex(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	_, err := svc.Search(context.Background(), "nonexistent_index", &types.SearchRequest{
		Page:     1,
		PageSize: 10,
	}, "")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "search returned"),
		"should return ES error, got: %v", err)
}

func TestSearch_TagsFilter(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	resp, err := svc.Search(context.Background(), testIndex, &types.SearchRequest{
		Tags:     []string{"mountain"},
		Page:     1,
		PageSize: 10,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.Total, "2 docs have 'mountain' tag")

	ids := collectIDs(resp.Hits)
	assert.Contains(t, ids, "1")
	assert.Contains(t, ids, "4")
}

func TestSearch_CameraModelFilter(t *testing.T) {
	svc, cleanup := setupSearchTest(t)
	defer cleanup()

	resp, err := svc.Search(context.Background(), testIndex, &types.SearchRequest{
		CameraModel: "Sony A7IV",
		Page:        1,
		PageSize:    10,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.Total, "2 docs with Sony A7IV")

	ids := collectIDs(resp.Hits)
	assert.Contains(t, ids, "2")
	assert.Contains(t, ids, "5")
}
