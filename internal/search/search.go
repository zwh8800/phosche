package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/zwh8800/phosche/internal/indexer"
	"github.com/zwh8800/phosche/internal/types"
)

// Searcher defines the interface for photo search operations.
type Searcher interface {
	Search(ctx context.Context, indexName string, req *types.SearchRequest) (*types.SearchResponse, error)
}

// SearchService provides full-text search and filtered queries against the
// Elasticsearch photo index.
type SearchService struct {
	client *indexer.ESClient
}

// NewSearchService creates a SearchService backed by the given ES client.
func NewSearchService(client *indexer.ESClient) *SearchService {
	return &SearchService{client: client}
}

// Search executes a full-text search with optional filters and pagination.
func (s *SearchService) Search(ctx context.Context, indexName string, req *types.SearchRequest) (*types.SearchResponse, error) {
	query := s.buildQuery(req)
	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	searchReq := esapi.SearchRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(bodyBytes),
	}

	resp, err := searchReq.Do(ctx, s.client.Client())
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search returned %s: %s", resp.Status(), string(b))
	}

	return s.parseSearchResponse(resp.Body, req)
}

// GetFilters returns aggregated filter values (tags, scene types, camera models)
// for use in the frontend filter UI.
func (s *SearchService) GetFilters(ctx context.Context, indexName string) (*types.FiltersResponse, error) {
	query := map[string]any{
		"size": 0,
		"aggs": map[string]any{
			"tags": map[string]any{
				"terms": map[string]any{"field": "tags.keyword", "size": 50},
			},
			"scene_types": map[string]any{
				"terms": map[string]any{"field": "scene_type", "size": 20},
			},
			"cameras": map[string]any{
				"terms": map[string]any{"field": "camera_model", "size": 20},
			},
		},
	}

	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal filters query: %w", err)
	}

	searchReq := esapi.SearchRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(bodyBytes),
	}

	resp, err := searchReq.Do(ctx, s.client.Client())
	if err != nil {
		return nil, fmt.Errorf("filters request: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("filters returned %s: %s", resp.Status(), string(b))
	}

	return s.parseFiltersResponse(resp.Body)
}

func (s *SearchService) buildQuery(req *types.SearchRequest) map[string]any {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	query := map[string]any{
		"from": (page - 1) * pageSize,
		"size": pageSize,
		"sort": []any{
			map[string]any{"date_time_original": map[string]any{"order": "desc"}},
		},
		"highlight": map[string]any{
			"fields": map[string]any{
				"description": map[string]any{},
			},
		},
	}

	var must []any
	var filter []any

	if req.Query != "" {
		must = append(must, map[string]any{
			"multi_match": map[string]any{
				"query":  req.Query,
				"fields": []string{"description", "tags", "objects"},
			},
		})
	}

	dateRange := map[string]any{}
	if req.DateFrom != "" {
		dateRange["gte"] = req.DateFrom
	}
	if req.DateTo != "" {
		dateRange["lte"] = req.DateTo
	}
	if len(dateRange) > 0 {
		filter = append(filter, map[string]any{
			"range": map[string]any{
				"date_time_original": dateRange,
			},
		})
	}

	if len(req.Tags) > 0 {
		filter = append(filter, map[string]any{
			"terms": map[string]any{
				"tags.keyword": req.Tags,
			},
		})
	}

	if len(req.Objects) > 0 {
		filter = append(filter, map[string]any{
			"terms": map[string]any{
				"objects.keyword": req.Objects,
			},
		})
	}

	if req.SceneType != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{
				"scene_type": req.SceneType,
			},
		})
	}

	if req.Status != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{
				"status": req.Status,
			},
		})
	}

	if req.CameraModel != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{
				"camera_model": req.CameraModel,
			},
		})
	}

	if len(must) > 0 || len(filter) > 0 {
		boolQuery := map[string]any{}
		if len(must) > 0 {
			boolQuery["must"] = must
		}
		if len(filter) > 0 {
			boolQuery["filter"] = filter
		}
		query["query"] = map[string]any{"bool": boolQuery}
	} else {
		query["query"] = map[string]any{"match_all": map[string]any{}}
	}

	return query
}

type esSearchResult struct {
	Hits struct {
		Total struct {
			Value    int64  `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []struct {
			Source types.PhotoDocument `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type esAggsResult struct {
	Aggregations struct {
		Tags struct {
			Buckets []aggBucket `json:"buckets"`
		} `json:"tags"`
		SceneTypes struct {
			Buckets []aggBucket `json:"buckets"`
		} `json:"scene_types"`
		Cameras struct {
			Buckets []aggBucket `json:"buckets"`
		} `json:"cameras"`
	} `json:"aggregations"`
}

type aggBucket struct {
	Key string `json:"key"`
}

func (s *SearchService) parseSearchResponse(body io.Reader, req *types.SearchRequest) (*types.SearchResponse, error) {
	var result esSearchResult
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	total := result.Hits.Total.Value

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	var totalPages int
	if pageSize > 0 && total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(pageSize)))
	} else if total == 0 {
		totalPages = 0
	} else {
		totalPages = 1
	}

	hits := make([]types.PhotoDocument, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		hits = append(hits, hit.Source)
	}

	return &types.SearchResponse{
		Hits:       hits,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetStats returns aggregate photo statistics (total count, counts by status,
// and count of recently created photos).
func (s *SearchService) GetStats(ctx context.Context, indexName string) (*types.StatsResponse, error) {
	query := map[string]any{
		"size":             0,
		"track_total_hits": true,
		"aggs": map[string]any{
			"by_status": map[string]any{
				"terms": map[string]any{"field": "status", "size": 10},
			},
			"recent": map[string]any{
				"filter": map[string]any{
					"range": map[string]any{
						"created_at": map[string]any{"gte": "now-1h"},
					},
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal stats query: %w", err)
	}

	searchReq := esapi.SearchRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(bodyBytes),
	}

	resp, err := searchReq.Do(ctx, s.client.Client())
	if err != nil {
		return nil, fmt.Errorf("stats request: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("stats returned %s: %s", resp.Status(), string(b))
	}

	return s.parseStatsResponse(resp.Body)
}

type esStatsResult struct {
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
	} `json:"hits"`
	Aggregations struct {
		ByStatus struct {
			Buckets []aggBucketWithCount `json:"buckets"`
		} `json:"by_status"`
		Recent struct {
			DocCount int64 `json:"doc_count"`
		} `json:"recent"`
	} `json:"aggregations"`
}

type aggBucketWithCount struct {
	Key      string `json:"key"`
	DocCount int64  `json:"doc_count"`
}

func (s *SearchService) parseStatsResponse(body io.Reader) (*types.StatsResponse, error) {
	var result esStatsResult
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode stats response: %w", err)
	}

	byStatus := map[types.JobStatus]int64{
		types.StatusUnanalyzed:      0,
		types.StatusAnalyzing:       0,
		types.StatusAnalyzed:        0,
		types.StatusFailed:          0,
		types.StatusPendingAnalysis: 0,
	}

	for _, b := range result.Aggregations.ByStatus.Buckets {
		byStatus[types.JobStatus(b.Key)] = b.DocCount
	}

	return &types.StatsResponse{
		Total:       result.Hits.Total.Value,
		ByStatus:    byStatus,
		RecentCount: result.Aggregations.Recent.DocCount,
	}, nil
}

func (s *SearchService) parseFiltersResponse(body io.Reader) (*types.FiltersResponse, error) {
	var result esAggsResult
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode filters response: %w", err)
	}

	tags := make([]string, 0, len(result.Aggregations.Tags.Buckets))
	for _, b := range result.Aggregations.Tags.Buckets {
		tags = append(tags, b.Key)
	}

	scenes := make([]string, 0, len(result.Aggregations.SceneTypes.Buckets))
	for _, b := range result.Aggregations.SceneTypes.Buckets {
		scenes = append(scenes, b.Key)
	}

	cameras := make([]string, 0, len(result.Aggregations.Cameras.Buckets))
	for _, b := range result.Aggregations.Cameras.Buckets {
		cameras = append(cameras, b.Key)
	}

	return &types.FiltersResponse{
		Tags:       tags,
		SceneTypes: scenes,
		Cameras:    cameras,
	}, nil
}
