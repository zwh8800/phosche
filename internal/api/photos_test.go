package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zwh8800/phosche/internal/types"
)

type mockCleaner struct {
	deletedPaths []string
	err          error
}

func (m *mockCleaner) GetPhoto(ctx context.Context, path string, indexName string) (*types.PhotoDocument, error) {
	return nil, nil
}

func (m *mockCleaner) DeletePhoto(ctx context.Context, path string, indexName string) error {
	m.deletedPaths = append(m.deletedPaths, path)
	return m.err
}

func newSearchMock(result *types.SearchResponse, err error) *mockSearchService {
	return &mockSearchService{
		statsFunc: func(_ context.Context, _ string) (*types.StatsResponse, error) {
			return nil, nil
		},
		filtersFunc: func(_ context.Context, _ string) (*types.FiltersResponse, error) {
			return nil, nil
		},
		searchFunc: func(_ context.Context, _ string, _ *types.SearchRequest) (*types.SearchResponse, error) {
			return result, err
		},
	}
}

func newTestServer(svc *mockSearchService) *httptest.Server {
	return httptest.NewServer(NewRouter(NewServer(svc, &mockCleaner{}, "photos")))
}

func TestGetPhotos_Success(t *testing.T) {
	mock := newSearchMock(&types.SearchResponse{
		Hits: []types.PhotoDocument{
			{Photo: types.Photo{ID: "1", Path: "/photos/a.jpg"}},
			{Photo: types.Photo{ID: "2", Path: "/photos/b.jpg"}},
		},
		Total:      2,
		Page:       1,
		PageSize:   50,
		TotalPages: 1,
	}, nil)

	ts := newTestServer(mock)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body types.SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Len(t, body.Hits, 2)
	assert.Equal(t, int64(2), body.Total)
}

func TestGetPhotos_Pagination(t *testing.T) {
	var capturedReq *types.SearchRequest
	mock := &mockSearchService{
		statsFunc: func(_ context.Context, _ string) (*types.StatsResponse, error) {
			return nil, nil
		},
		filtersFunc: func(_ context.Context, _ string) (*types.FiltersResponse, error) {
			return nil, nil
		},
		searchFunc: func(_ context.Context, _ string, req *types.SearchRequest) (*types.SearchResponse, error) {
			capturedReq = req
			return &types.SearchResponse{
				Hits:       []types.PhotoDocument{},
				Total:      100,
				Page:       2,
				PageSize:   5,
				TotalPages: 20,
			}, nil
		},
	}

	ts := httptest.NewServer(NewRouter(NewServer(mock, &mockCleaner{}, "photos")))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos?page=2&page_size=5")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body types.SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, 5, body.PageSize)
	assert.Equal(t, 2, body.Page)

	require.NotNil(t, capturedReq)
	assert.Equal(t, 2, capturedReq.Page)
	assert.Equal(t, 5, capturedReq.PageSize)
}

func TestGetPhotos_EmptyResult(t *testing.T) {
	mock := newSearchMock(&types.SearchResponse{
		Hits:       []types.PhotoDocument{},
		Total:      0,
		Page:       1,
		PageSize:   50,
		TotalPages: 0,
	}, nil)

	ts := newTestServer(mock)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body types.SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Empty(t, body.Hits)
	assert.Equal(t, int64(0), body.Total)
}

func TestGetPhotos_DateFilter(t *testing.T) {
	var capturedReq *types.SearchRequest
	mock := &mockSearchService{
		statsFunc: func(_ context.Context, _ string) (*types.StatsResponse, error) {
			return nil, nil
		},
		filtersFunc: func(_ context.Context, _ string) (*types.FiltersResponse, error) {
			return nil, nil
		},
		searchFunc: func(_ context.Context, _ string, req *types.SearchRequest) (*types.SearchResponse, error) {
			capturedReq = req
			return &types.SearchResponse{
				Hits:       []types.PhotoDocument{},
				Total:      0,
				Page:       1,
				PageSize:   50,
				TotalPages: 0,
			}, nil
		},
	}

	ts := httptest.NewServer(NewRouter(NewServer(mock, &mockCleaner{}, "photos")))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos?date_from=2024-01-01&date_to=2024-12-31")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	require.NotNil(t, capturedReq)
	assert.Equal(t, "2024-01-01", capturedReq.DateFrom)
	assert.Equal(t, "2024-12-31", capturedReq.DateTo)
}

func TestGetPhotos_DefaultParams(t *testing.T) {
	var capturedReq *types.SearchRequest
	mock := &mockSearchService{
		statsFunc: func(_ context.Context, _ string) (*types.StatsResponse, error) {
			return nil, nil
		},
		filtersFunc: func(_ context.Context, _ string) (*types.FiltersResponse, error) {
			return nil, nil
		},
		searchFunc: func(_ context.Context, _ string, req *types.SearchRequest) (*types.SearchResponse, error) {
			capturedReq = req
			return &types.SearchResponse{
				Hits:       []types.PhotoDocument{},
				Total:      0,
				Page:       1,
				PageSize:   50,
				TotalPages: 0,
			}, nil
		},
	}

	ts := httptest.NewServer(NewRouter(NewServer(mock, &mockCleaner{}, "photos")))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	require.NotNil(t, capturedReq)
	assert.Equal(t, 1, capturedReq.Page)
	assert.Equal(t, 50, capturedReq.PageSize)
}
