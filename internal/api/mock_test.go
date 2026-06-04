package api

import (
	"context"

	"github.com/zwh8800/phosche/internal/types"
)

type mockSearchService struct {
	statsFunc   func(ctx context.Context, indexName string) (*types.StatsResponse, error)
	filtersFunc func(ctx context.Context, indexName string) (*types.FiltersResponse, error)
	searchFunc  func(ctx context.Context, indexName string, req *types.SearchRequest) (*types.SearchResponse, error)
}

func (m *mockSearchService) GetStats(ctx context.Context, indexName string, userEmail string) (*types.StatsResponse, error) {
	return m.statsFunc(ctx, indexName)
}

func (m *mockSearchService) GetFilters(ctx context.Context, indexName string, userEmail string) (*types.FiltersResponse, error) {
	return m.filtersFunc(ctx, indexName)
}

func (m *mockSearchService) Search(ctx context.Context, indexName string, req *types.SearchRequest, userEmail string) (*types.SearchResponse, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, indexName, req)
	}
	return &types.SearchResponse{}, nil
}

func (m *mockSearchService) FindSimilar(_ context.Context, _ string, _ string, _ []float32, _ string) (*types.RecommendationResponse, error) {
	return &types.RecommendationResponse{}, nil
}

func (m *mockSearchService) FindNearby(_ context.Context, _ string, _ string, _, _ float64, _ string) (*types.RecommendationResponse, error) {
	return &types.RecommendationResponse{}, nil
}
