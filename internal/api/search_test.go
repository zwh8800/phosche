package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zwh8800/phosche/internal/types"
)

type mockSearcher struct {
	resp *types.SearchResponse
	err  error
}

func (m *mockSearcher) Search(ctx context.Context, indexName string, req *types.SearchRequest, userEmail string) (*types.SearchResponse, error) {
	return m.resp, m.err
}

func (m *mockSearcher) GetFilters(_ context.Context, _ string, _ string) (*types.FiltersResponse, error) {
	return nil, nil
}
func (m *mockSearcher) GetStats(_ context.Context, _ string, _ string) (*types.StatsResponse, error) {
	return &types.StatsResponse{}, nil
}
func (m *mockSearcher) FindSimilar(_ context.Context, _ string, _ string, _ []float32, _ string) (*types.RecommendationResponse, error) {
	return &types.RecommendationResponse{}, nil
}
func (m *mockSearcher) FindNearby(_ context.Context, _ string, _ string, _, _ float64, _ string) (*types.RecommendationResponse, error) {
	return &types.RecommendationResponse{}, nil
}

func TestSearch_Success(t *testing.T) {
	mock := &mockSearcher{
		resp: &types.SearchResponse{
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

	body := `{"query":"sunset","page":1,"page_size":20}`
	resp, err := http.Post(ts.URL+"/api/search", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result types.SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Total)
	assert.Equal(t, 1, result.Page)
}

func TestSearch_Success_DefaultsPageAndSize(t *testing.T) {
	mock := &mockSearcher{
		resp: &types.SearchResponse{
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

	body := `{"query":"sunset"}`
	resp, err := http.Post(ts.URL+"/api/search", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSearch_Validation_PageTooSmall(t *testing.T) {
	srv := &Server{}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	body := `{"page":-1,"page_size":20}`
	resp, err := http.Post(ts.URL+"/api/search", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp map[string]string
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Contains(t, errResp["error"], "page")
}

func TestSearch_Validation_PageSizeTooLarge(t *testing.T) {
	srv := &Server{}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	body := `{"page":1,"page_size":200}`
	resp, err := http.Post(ts.URL+"/api/search", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSearch_InvalidJSON(t *testing.T) {
	srv := &Server{}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	body := `not-json`
	resp, err := http.Post(ts.URL+"/api/search", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSearch_ServiceError(t *testing.T) {
	mock := &mockSearcher{
		err: fmt.Errorf("opensearch unavailable"),
	}
	srv := &Server{searchService: mock, IndexName: "test-index"}
	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	body := `{"page":1,"page_size":20}`
	resp, err := http.Post(ts.URL+"/api/search", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
