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

func TestStats_Success(t *testing.T) {
	srv := &Server{
		searchService: &mockSearchService{
			statsFunc: func(_ context.Context, _ string) (*types.StatsResponse, error) {
				return &types.StatsResponse{
					Total: 100,
					ByStatus: map[types.JobStatus]int64{
						types.StatusAnalyzed:        80,
						types.StatusAnalyzing:       5,
						types.StatusFailed:          10,
						types.StatusPendingAnalysis: 5,
						types.StatusUnanalyzed:      0,
					},
					RecentCount: 12,
				}, nil
			},
		},
		IndexName: "test_index",
	}

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

	assert.Equal(t, int64(100), body.Total)
	assert.Equal(t, int64(80), body.ByStatus[types.StatusAnalyzed])
	assert.Equal(t, int64(5), body.ByStatus[types.StatusAnalyzing])
	assert.Equal(t, int64(10), body.ByStatus[types.StatusFailed])
	assert.Equal(t, int64(5), body.ByStatus[types.StatusPendingAnalysis])
	assert.Equal(t, int64(0), body.ByStatus[types.StatusUnanalyzed])
	assert.Equal(t, int64(12), body.RecentCount)
}

func TestStats_ServiceError(t *testing.T) {
	srv := &Server{
		searchService: &mockSearchService{
			statsFunc: func(_ context.Context, _ string) (*types.StatsResponse, error) {
				return nil, assert.AnError
			},
		},
		IndexName: "test_index",
	}

	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body["error"], "search service unavailable")
}
