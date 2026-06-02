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

func TestFilters_Success(t *testing.T) {
	srv := &Server{
		searchService: &mockSearchService{
			filtersFunc: func(_ context.Context, _ string) (*types.FiltersResponse, error) {
				return &types.FiltersResponse{
					Tags:       []string{"landscape", "portrait", "cat"},
					SceneTypes: []string{"outdoor", "indoor"},
					Countries:  []string{"China"},
					Provinces:  []string{"Beijing"},
					Cities:     []string{"Beijing"},
					Districts:  []string{"Dongcheng"},
					Statuses:   []string{"analyzed"},
				}, nil
			},
		},
		IndexName: "test_index",
	}

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

	assert.Equal(t, []string{"landscape", "portrait", "cat"}, body.Tags)
	assert.Equal(t, []string{"outdoor", "indoor"}, body.SceneTypes)
	assert.Equal(t, []string{"China"}, body.Countries)
	assert.Equal(t, []string{"Beijing"}, body.Provinces)
	assert.Equal(t, []string{"Beijing"}, body.Cities)
	assert.Equal(t, []string{"Dongcheng"}, body.Districts)
	assert.Equal(t, []string{"analyzed"}, body.Statuses)
}

func TestFilters_ServiceError(t *testing.T) {
	srv := &Server{
		searchService: &mockSearchService{
			filtersFunc: func(_ context.Context, _ string) (*types.FiltersResponse, error) {
				return nil, assert.AnError
			},
		},
		IndexName: "test_index",
	}

	router := NewRouter(srv)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/filters")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body["error"], "search service unavailable")
}
