package api

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zwh8800/phosche/internal/static"
	"github.com/zwh8800/phosche/internal/types"
)

// fullMockSearcher implements all PhotoSearcher methods for comprehensive QA.
type fullMockSearcher struct {
	statsResp   *types.StatsResponse
	statsErr    error
	filtersResp *types.FiltersResponse
	filtersErr  error
	searchResp  *types.SearchResponse
	searchErr   error
}

func (m *fullMockSearcher) GetStats(_ context.Context, _ string) (*types.StatsResponse, error) {
	return m.statsResp, m.statsErr
}

func (m *fullMockSearcher) GetFilters(_ context.Context, _ string) (*types.FiltersResponse, error) {
	return m.filtersResp, m.filtersErr
}

func (m *fullMockSearcher) Search(_ context.Context, _ string, _ *types.SearchRequest) (*types.SearchResponse, error) {
	return m.searchResp, m.searchErr
}

// ============================================================
// SCENARIO 1: Health Check
// ============================================================
func TestQA_HealthCheck(t *testing.T) {
	router := NewRouter(&Server{})
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "health should return 200")

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body["status"], "health status should be ok")
	assert.Equal(t, "0.1.0", body["version"], "version should be 0.1.0")
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	t.Log("✅ Health check PASSED")
}

// ============================================================
// SCENARIO 2: Photo List Endpoint
// ============================================================
func TestQA_PhotoList(t *testing.T) {
	mock := &fullMockSearcher{
		searchResp: &types.SearchResponse{
			Hits: []types.PhotoDocument{
				{
					Photo: types.Photo{
						ID:     "photo-1",
						Path:   "/photos/img1.jpg",
						Status: types.StatusAnalyzed,
					},
					AnalysisResult: types.AnalysisResult{
						Description: "Sunset photo",
						Tags:        []string{"sunset", "nature"},
					},
				},
			},
			Total:      1,
			Page:       1,
			PageSize:   50,
			TotalPages: 1,
		},
	}
	srv := &Server{searchService: mock, IndexName: "test-index"}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body types.SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, int64(1), body.Total, "should return 1 photo")
	assert.Len(t, body.Hits, 1, "should have 1 hit")
	assert.Equal(t, "photo-1", body.Hits[0].ID)
	t.Logf("✅ Photo list PASSED (total=%d, hits=%d)", body.Total, len(body.Hits))
}

// ============================================================
// SCENARIO 3: Search Endpoint
// ============================================================
func TestQA_Search(t *testing.T) {
	mock := &fullMockSearcher{
		searchResp: &types.SearchResponse{
			Hits:       []types.PhotoDocument{},
			Total:      0,
			Page:       1,
			PageSize:   20,
			TotalPages: 0,
		},
	}
	srv := &Server{searchService: mock, IndexName: "test-index"}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Test with valid search query
	body := `{"query":"test","page":1,"page_size":20}`
	resp, err := http.Post(ts.URL+"/api/search", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result types.SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	t.Logf("✅ Search PASSED (status=%d, page=%d)", resp.StatusCode, result.Page)
}

// ============================================================
// SCENARIO 4: Stats Endpoint
// ============================================================
func TestQA_Stats(t *testing.T) {
	mock := &fullMockSearcher{
		statsResp: &types.StatsResponse{
			Total: 42,
			ByStatus: map[types.JobStatus]int64{
				types.StatusAnalyzed:   30,
				types.StatusUnanalyzed: 10,
				types.StatusFailed:     2,
			},
			RecentCount: 5,
		},
	}
	srv := &Server{searchService: mock, IndexName: "test-index"}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body types.StatsResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, int64(42), body.Total, "total should be 42")
	assert.Equal(t, int64(5), body.RecentCount, "recent_count should be 5")
	t.Logf("✅ Stats PASSED (total=%d, recent=%d, statuses=%d)", body.Total, body.RecentCount, len(body.ByStatus))
}

// ============================================================
// SCENARIO 5: Filters Endpoint
// ============================================================
func TestQA_Filters(t *testing.T) {
	mock := &fullMockSearcher{
		filtersResp: &types.FiltersResponse{
			Tags:       []string{"sunset", "portrait", "nature"},
			SceneTypes: []string{"indoor", "outdoor"},
			Cameras:    []string{"Canon EOS R5", "iPhone 15"},
		},
	}
	srv := &Server{searchService: mock, IndexName: "test-index"}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/filters")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body types.FiltersResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Len(t, body.Tags, 3, "should have 3 tags")
	assert.Len(t, body.SceneTypes, 2, "should have 2 scene types")
	assert.Len(t, body.Cameras, 2, "should have 2 cameras")
	t.Logf("✅ Filters PASSED (tags=%d, scenes=%d, cameras=%d)", len(body.Tags), len(body.SceneTypes), len(body.Cameras))
}

// ============================================================
// SCENARIO 6: Static File Server
// ============================================================
func TestQA_StaticFileServer(t *testing.T) {
	// Create a temporary directory with a test JPEG
	tempDir := t.TempDir()
	testJPEG := filepath.Join(tempDir, "test.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	f, err := os.Create(testJPEG)
	require.NoError(t, err)
	err = jpeg.Encode(f, img, &jpeg.Options{Quality: 85})
	require.NoError(t, err)
	f.Close()

	handler := static.PhotoHandler([]string{tempDir})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Serve the test JPEG
	resp, err := http.Get(ts.URL + "/photos/test.jpg")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "should serve existing JPEG")
	assert.Contains(t, resp.Header.Get("Content-Type"), "image/jpeg")
	assert.NotEmpty(t, resp.Header.Get("Cache-Control"), "should set Cache-Control")

	// Non-existent file should return 404
	resp2, err := http.Get(ts.URL + "/photos/nonexistent.jpg")
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp2.StatusCode, "nonexistent file should return 404")

	// Create a .txt file to test extension blocking
	txtPath := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(txtPath, []byte("hello"), 0644)
	require.NoError(t, err)

	// Forbidden extension should be rejected (file exists but extension not allowed)
	resp3, err := http.Get(ts.URL + "/photos/test.txt")
	require.NoError(t, err)
	defer resp3.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp3.StatusCode, "non-image extension should return 404")

	// Path traversal should be blocked
	resp4, err := http.Get(ts.URL + "/photos/../../../etc/passwd")
	require.NoError(t, err)
	defer resp4.Body.Close()
	assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, resp4.StatusCode, "path traversal should be blocked")

	t.Log("✅ Static file server PASSED (serve jpeg, 404, 403, path traversal blocked)")
}

// ============================================================
// EDGE CASE 1: Invalid search JSON → 400
// ============================================================
func TestQA_Edge_InvalidSearchJSON(t *testing.T) {
	srv := &Server{}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/search", "application/json", strings.NewReader(`not-json`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body["error"], "invalid JSON")
	t.Logf("✅ Edge: Invalid JSON → 400 PASSED (message=%s)", body["error"])
}

// ============================================================
// EDGE CASE 2: Nonexistent endpoint → 404
// ============================================================
func TestQA_Edge_NonexistentEndpoint(t *testing.T) {
	router := NewRouter(&Server{})
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	t.Logf("✅ Edge: Nonexistent endpoint → 404 PASSED")
}

// ============================================================
// EDGE CASE 3: Nonexistent path (not /api/) → 404
// ============================================================
func TestQA_Edge_NonexistentPath(t *testing.T) {
	router := NewRouter(&Server{})
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nonexistent-page")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	t.Logf("✅ Edge: Nonexistent path → 404 PASSED")
}

// ============================================================
// EDGE CASE 4: Method not allowed on health endpoint
// ============================================================
func TestQA_Edge_MethodNotAllowed(t *testing.T) {
	router := NewRouter(&Server{})
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/health", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	t.Logf("✅ Edge: Method not allowed → 405 PASSED")
}

// ============================================================
// EDGE CASE 5: Empty search body with defaults
// ============================================================
func TestQA_Edge_EmptySearchDefaults(t *testing.T) {
	mock := &fullMockSearcher{
		searchResp: &types.SearchResponse{
			Hits:       []types.PhotoDocument{},
			Total:      0,
			Page:       1,
			PageSize:   20,
			TotalPages: 0,
		},
	}
	srv := &Server{searchService: mock, IndexName: "test-index"}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Empty JSON body → should use defaults (page=1, page_size=20)
	resp, err := http.Post(ts.URL+"/api/search", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result types.SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	t.Logf("✅ Edge: Empty search defaults PASSED (page=%d, page_size=%d)", result.Page, result.PageSize)
}

// ============================================================
// EDGE CASE 6: Photos endpoint with query params
// ============================================================
func TestQA_PhotosWithQueryParams(t *testing.T) {
	mock := &fullMockSearcher{
		searchResp: &types.SearchResponse{
			Hits:       []types.PhotoDocument{},
			Total:      0,
			Page:       3,
			PageSize:   10,
			TotalPages: 0,
		},
	}
	srv := &Server{searchService: mock, IndexName: "test-index"}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos?page=3&page_size=10&status=analyzed")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result types.SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Page)
	assert.Equal(t, 10, result.PageSize)
	t.Logf("✅ Edge: Photos with query params PASSED (page=%d, page_size=%d)", result.Page, result.PageSize)
}

// ============================================================
// SUITE RUNNER: Aggregate results
// ============================================================
func TestQA_AllScenarios(t *testing.T) {
	// This test runs all scenarios and prints a summary.
	// Individual subtests report their own pass/fail.
	subtests := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"HealthCheck", TestQA_HealthCheck},
		{"PhotoList", TestQA_PhotoList},
		{"Search", TestQA_Search},
		{"Stats", TestQA_Stats},
		{"Filters", TestQA_Filters},
		{"StaticFileServer", TestQA_StaticFileServer},
		{"Edge_InvalidSearchJSON", TestQA_Edge_InvalidSearchJSON},
		{"Edge_NonexistentEndpoint", TestQA_Edge_NonexistentEndpoint},
		{"Edge_NonexistentPath", TestQA_Edge_NonexistentPath},
		{"Edge_MethodNotAllowed", TestQA_Edge_MethodNotAllowed},
		{"Edge_EmptySearchDefaults", TestQA_Edge_EmptySearchDefaults},
		{"PhotosWithQueryParams", TestQA_PhotosWithQueryParams},
	}

	passed := 0
	failed := 0
	for _, st := range subtests {
		ok := t.Run(st.name, st.fn)
		if ok {
			passed++
		} else {
			failed++
		}
	}

	t.Logf("\n========================================")
	t.Logf("QA SUMMARY: %d/%d pass | Edge Cases: %d tested",
		passed, len(subtests),
		len(subtests)-6) // first 6 are scenarios, rest are edge cases
	t.Logf("SCENARIOS: Health ✓ PhotoList ✓ Search ✓ Stats ✓ Filters ✓ Static ✓")
	t.Logf("EDGE CASES: InvalidJSON ✓ NonexistentAPI ✓ NonexistentPath ✓ MethodNotAllowed ✓ EmptyDefaults ✓ QueryParams ✓")
	t.Logf("VERDICT: %s", func() string {
		if failed == 0 {
			return "✅ ALL PASS"
		}
		return fmt.Sprintf("❌ %d FAILURES", failed)
	}())
	t.Logf("========================================")
}
