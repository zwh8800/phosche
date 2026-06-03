// Package search 提供基于 OpenSearch 的照片搜索服务，支持全文检索、多维过滤和聚合统计。
//
// 核心功能：
//   - 全文搜索：multi_match 跨 description/tags/objects/text 字段
//   - 混合检索：BM25 + kNN 通过 OpenSearch search pipeline 实现服务端 RRF
//   - 多维过滤：日期范围、标签、物体、场景类型、相机型号、处理状态
//   - 排序策略：有查询词时按 _score（相关性），无查询词时按拍摄时间倒序
//   - 聚合统计：文档总数、按状态分组计数、筛选选项聚合（tags/scene/camera）
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"github.com/zwh8800/phosche/internal/indexer"
	"github.com/zwh8800/phosche/internal/types"
)

// pipelineName is the OpenSearch search pipeline for native RRF hybrid search.
const pipelineName = "phosche-rrf-pipeline"

// Embedder 是文本向量化的抽象接口。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// EmbeddingCache 是查询 embedding 缓存的接口。
type EmbeddingCache interface {
	Get(text string) ([]float32, bool)
	Set(text string, embedding []float32)
}

// HybridConfig 是混合检索参数。
type HybridConfig struct {
	RRFWindowSize    int
	RRFRankConstant  int
	KNNK             int
	KNNNumCandidates int
}

// SearchOption 是 SearchService 的函数式配置选项。
type SearchOption func(*SearchService)

// WithEmbedder 配置混合检索所需的 embedding 组件。
func WithEmbedder(e Embedder, cache EmbeddingCache, cfg HybridConfig) SearchOption {
	return func(s *SearchService) {
		s.embedder = e
		s.embeddingCache = cache
		s.hybridCfg = cfg
	}
}

// buildEmailFilter 构建基于 email 的访问过滤条件。
// 匹配规则：文档无 email 字段 或 字段值为空 或（userEmail 非空时）字段值等于 userEmail。
func buildEmailFilter(userEmail string) map[string]any {
	should := []any{
		map[string]any{"bool": map[string]any{"must_not": map[string]any{"exists": map[string]any{"field": "email"}}}},
		map[string]any{"term": map[string]any{"email": ""}},
	}
	if userEmail != "" {
		should = append(should, map[string]any{"term": map[string]any{"email": userEmail}})
	}
	return map[string]any{"bool": map[string]any{"should": should, "minimum_should_match": 1}}
}

// Searcher 定义照片搜索操作的接口，供测试 mock 使用。
type Searcher interface {
	Search(ctx context.Context, indexName string, req *types.SearchRequest, userEmail string) (*types.SearchResponse, error)
	GetFilters(ctx context.Context, indexName string, userEmail string) (*types.FiltersResponse, error)
	GetStats(ctx context.Context, indexName string, userEmail string) (*types.StatsResponse, error)
}

// SearchService 提供基于 OpenSearch 的全文搜索和条件过滤查询。
type SearchService struct {
	client         *indexer.OSClient
	embedder       Embedder
	embeddingCache EmbeddingCache
	hybridCfg      HybridConfig
}

// NewSearchService 创建 SearchService 实例。
// 当配置了 embedder 时，自动创建 OpenSearch search pipeline 用于服务端 RRF。
func NewSearchService(client *indexer.OSClient, opts ...SearchOption) *SearchService {
	s := &SearchService{client: client}
	for _, opt := range opts {
		opt(s)
	}
	if s.embedder != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.EnsureSearchPipeline(ctx); err != nil {
			slog.Error("failed to ensure search pipeline", "error", err)
		}
	}
	return s
}

// EnsureSearchPipeline creates the RRF search pipeline if it doesn't exist.
// Called during startup when embedder is configured. Must be called after OpenSearch is available.
func (s *SearchService) EnsureSearchPipeline(ctx context.Context) error {
	if s.embedder == nil {
		return nil
	}

	body := map[string]any{
		"description": "phosche hybrid search with BM25 + kNN RRF",
		"phase_results_processors": []any{
			map[string]any{
				"score-ranker-processor": map[string]any{
					"combination": map[string]any{
						"technique":     "rrf",
						"rank_constant": s.hybridCfg.RRFRankConstant,
					},
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal pipeline body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "/_search/pipeline/"+pipelineName, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create pipeline request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Client().Client.Perform(req)
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}
	defer resp.Body.Close()

	// 200 = created, 409 = already exists (idempotent), other = error
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pipeline creation returned %s: %s", resp.Status, string(b))
	}

	slog.Info("search pipeline ensured", "name", pipelineName, "rank_constant", s.hybridCfg.RRFRankConstant)
	return nil
}

func (s *SearchService) getEmbedding(ctx context.Context, text string) ([]float32, bool) {
	if s.embeddingCache != nil {
		if cached, ok := s.embeddingCache.Get(text); ok {
			return cached, true
		}
	}

	embeddings, err := s.embedder.Embed(ctx, []string{text})
	if err != nil {
		slog.Warn("query embedding failed, falling back to BM25", "error", err)
		return nil, false
	}
	if len(embeddings) == 0 {
		return nil, false
	}

	if s.embeddingCache != nil {
		s.embeddingCache.Set(text, embeddings[0])
	}

	return embeddings[0], true
}

// Search 执行全文搜索+条件过滤查询。
//
// 流程：
//  1. 有查询词且 embedder 可用时，调用 searchHybrid（BM25 + kNN 混合检索，服务端 RRF）
//  2. 否则调用 searchBM25（纯 BM25 全文检索或过滤）
//
// userEmail 用于基于 email 的访问过滤，限制搜索结果仅包含用户有权访问的文档。
func (s *SearchService) Search(ctx context.Context, indexName string, req *types.SearchRequest, userEmail string) (*types.SearchResponse, error) {
	slog.Debug("search request received",
		"index", indexName,
		"query", req.Query,
		"page", req.Page,
		"page_size", req.PageSize,
		"date_from", req.DateFrom,
		"date_to", req.DateTo,
		"tags", req.Tags,
		"objects", req.Objects,
		"scene_type", req.SceneType,
		"country", req.Country,
		"province", req.Province,
		"city", req.City,
		"district", req.District,
		"status", req.Status,
		"has_embedder", s.embedder != nil,
	)

	if req.Query != "" && s.embedder != nil {
		queryVec, ok := s.getEmbedding(ctx, req.Query)
		if ok {
			return s.searchHybrid(ctx, indexName, req, queryVec, userEmail)
		}
	}
	return s.searchBM25(ctx, indexName, req, userEmail)
}

// searchHybrid 执行 BM25 + kNN 混合检索，通过 OpenSearch search pipeline 实现服务端 RRF。
func (s *SearchService) searchHybrid(ctx context.Context, indexName string, req *types.SearchRequest, queryVec []float32, userEmail string) (*types.SearchResponse, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	from := (page - 1) * pageSize

	filters := s.buildFilters(req, userEmail)

	// BM25 sub-query
	bm25Query := map[string]any{
		"bool": map[string]any{
			"must": []any{
				map[string]any{
					"multi_match": map[string]any{
						"query":  req.Query,
						"fields": []string{"description", "tags", "objects", "text", "address", "formatted_address"},
					},
				},
			},
			"filter": filters,
		},
	}

	// KNN sub-query (OpenSearch knn query clause)
	k := s.hybridCfg.KNNK
	if k <= 0 {
		k = pageSize * 2
	}
	knnQuery := map[string]any{
		"knn": map[string]any{
			"embedding": map[string]any{
				"vector": queryVec,
				"k":      k,
			},
		},
	}

	// Hybrid query combining BM25 and kNN sub-queries
	// RRF score fusion is applied server-side by the search pipeline
	query := map[string]any{
		"from": from,
		"size": pageSize,
		"query": map[string]any{
			"hybrid": map[string]any{
				"queries":          []any{bm25Query, knnQuery},
				"pagination_depth": from + pageSize,
			},
		},
		"highlight": map[string]any{
			"fields": map[string]any{
				"description": map[string]any{},
			},
		},
	}

	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal hybrid query: %w", err)
	}

	slog.Debug("hybrid search request details",
		"index", indexName,
		"page", page,
		"from", from,
		"size", pageSize,
		"k", k,
		"vector_dim", len(queryVec),
		"filter_count", len(filters),
		"pipeline", pipelineName,
		"query_len", len(bodyBytes),
		"query_body", truncateJSON(bodyBytes, 800),
	)

	resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
		Params: opensearchapi.SearchParams{
			SearchPipeline: pipelineName,
		},
	})
	if err != nil {
		// Dump full OpenSearch error (includes status, type, reason, root_cause, caused_by)
		status := 0
		if resp != nil && resp.Inspect().Response != nil {
			status = resp.Inspect().Response.StatusCode
		}
		slog.Error("hybrid search OpenSearch request failed",
			"index", indexName,
			"page", page,
			"from", from,
			"size", pageSize,
			"k", k,
			"vector_dim", len(queryVec),
			"filter_count", len(filters),
			"pipeline", pipelineName,
			"http_status", status,
			"error", err.Error(),
			"error_type", fmt.Sprintf("%T", err),
			"query_len", len(bodyBytes),
			"query_body", truncateJSON(bodyBytes, 2000),
		)
		return nil, fmt.Errorf("hybrid search: %w", err)
	}

	slog.Debug("hybrid search OpenSearch response OK",
		"index", indexName,
		"took_ms", resp.Took,
		"hits_total", resp.Hits.Total.Value,
		"hits_count", len(resp.Hits.Hits),
		"shards_total", resp.Shards.Total,
		"shards_failed", resp.Shards.Failed,
		"shards_successful", resp.Shards.Successful,
	)

	return s.buildSearchResponse(resp, req)
}

// searchBM25 执行纯 BM25 全文搜索或过滤查询。
func (s *SearchService) searchBM25(ctx context.Context, indexName string, req *types.SearchRequest, userEmail string) (*types.SearchResponse, error) {
	query := s.buildQuery(req, userEmail)
	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	slog.Debug("BM25 search", "index", indexName, "query", truncateJSON(bodyBytes, 500), "page", req.Page, "page_size", req.PageSize)

	resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
	})
	if err != nil {
		status := 0
		if resp != nil && resp.Inspect().Response != nil {
			status = resp.Inspect().Response.StatusCode
		}
		slog.Error("BM25 search OpenSearch request failed",
			"index", indexName,
			"page", req.Page,
			"page_size", req.PageSize,
			"http_status", status,
			"error", err.Error(),
			"error_type", fmt.Sprintf("%T", err),
			"query", truncateJSON(bodyBytes, 2000),
		)
		return nil, fmt.Errorf("BM25 search: %w", err)
	}

	slog.Debug("BM25 search OpenSearch response OK",
		"index", indexName,
		"took_ms", resp.Took,
		"hits_total", resp.Hits.Total.Value,
		"hits_count", len(resp.Hits.Hits),
	)

	return s.buildSearchResponse(resp, req)
}

// GetFilters 执行词项聚合（terms aggregation），获取前端筛选面板所需的可选项列表。
//
// GetFilters 返回筛选 UI 所需的聚合数据（标签、场景类型、国家、省、市、区、状态）。
//   - tags: 最多 50 个热门标签
//   - scene_types: 最多 20 个场景类型
//   - countries/provinces/cities/districts: 最多 50 个地理选项
//   - statuses: 最多 10 个状态选项
//
// 返回的 FiltersResponse 用于填充搜索页面的下拉筛选器。
func (s *SearchService) GetFilters(ctx context.Context, indexName string, userEmail string) (*types.FiltersResponse, error) {
	slog.Debug("get filters", "index", indexName)
	query := map[string]any{
		"size":  0,
		"query": buildEmailFilter(userEmail),
		"aggs": map[string]any{
			"tags": map[string]any{
				"terms": map[string]any{"field": "tags.keyword", "size": 50},
			},
			"scene_types": map[string]any{
				"terms": map[string]any{"field": "scene_type", "size": 20},
			},
			"countries": map[string]any{
				"terms": map[string]any{"field": "country.keyword", "size": 50},
			},
			"provinces": map[string]any{
				"terms": map[string]any{"field": "province.keyword", "size": 50},
			},
			"cities": map[string]any{
				"terms": map[string]any{"field": "city.keyword", "size": 50},
			},
			"districts": map[string]any{
				"terms": map[string]any{"field": "district.keyword", "size": 50},
			},
			"statuses": map[string]any{
				"terms": map[string]any{"field": "status", "size": 10},
			},
		},
	}

	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal filters query: %w", err)
	}

	resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("filters request: %w", err)
	}

	if len(resp.Aggregations) == 0 {
		return &types.FiltersResponse{}, nil
	}
	return s.parseFiltersResponse(resp.Aggregations)
}

// buildQuery 是核心查询构造器，将 SearchRequest 转换为 OpenSearch 查询 DSL。
//
// 构建规则（按顺序）：
//  1. 分页默认值：page=1, pageSize=20
//  2. 排序策略：
//     - 有搜索词时：_score（相关性）优先，其次 date_time_original desc，最后 mtime desc
//     - 无搜索词时：date_time_original desc（缺失值排最后），mtime desc
//  3. 高亮：description 字段的 BM25 检索时启用
//  4. 全文搜索：multi_match 跨 description、tags、objects、text 字段
//  5. 日期范围过滤：date_time_original 的 gte/lte
//  6. 词项过滤：tags.keyword、objects.keyword 的 terms 查询
//  7. 精确匹配：scene_type、status、country/province/city/district 的 term 查询
//
// 8. 组合：must + filter 子句包装为 bool 查询；无任何条件时使用 match_all
// 9. Email 访问过滤：始终添加 email 过滤条件，限制可见范围
func (s *SearchService) buildQuery(req *types.SearchRequest, userEmail string) map[string]any {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	// 排序策略：有 query 时按 _score 优先（相关性），无 query 时按拍摄时间倒序。
	// 使用 script sort：优先用 exif.date_time_original（拍摄时间），缺失时用 mtime（文件修改时间）
	sort := []any{
		map[string]any{
			"_script": map[string]any{
				"type": "number",
				"script": map[string]any{
					"source": "if (doc['exif.date_time_original'].size() > 0) { return doc['exif.date_time_original'].value.toInstant().toEpochMilli(); } return doc['mtime'].value * 1000;",
				},
				"order": "desc",
			},
		},
		map[string]any{"mtime": map[string]any{"order": "desc"}},
	}
	if req.Query != "" {
		sort = append([]any{"_score"}, sort...)
	}

	query := map[string]any{
		"from": (page - 1) * pageSize,
		"size": pageSize,
		"sort": sort,
		"highlight": map[string]any{
			"fields": map[string]any{
				"description": map[string]any{},
			},
		},
	}

	var must []any
	var filter []any

	// Email 访问过滤：始终添加，限制文档可见范围
	filter = append(filter, buildEmailFilter(userEmail))

	// 全文搜索：multi_match 跨 description/tags/objects/text/formatted_address 六个字段
	if req.Query != "" {
		must = append(must, map[string]any{
			"multi_match": map[string]any{
				"query":  req.Query,
				"fields": []string{"description", "tags", "objects", "text", "address", "formatted_address"},
			},
		})
	}

	// 日期范围过滤（gte: date_from, lte: date_to）
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
				"exif.date_time_original": dateRange,
			},
		})
	}

	// 标签过滤（terms 查询 tags.keyword 实现多选）
	if len(req.Tags) > 0 {
		filter = append(filter, map[string]any{
			"terms": map[string]any{
				"tags.keyword": req.Tags,
			},
		})
	}

	// 物体过滤（terms 查询 objects.keyword）
	if len(req.Objects) > 0 {
		filter = append(filter, map[string]any{
			"terms": map[string]any{
				"objects.keyword": req.Objects,
			},
		})
	}

	// 场景类型过滤（term 精确匹配）
	if req.SceneType != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{
				"scene_type": req.SceneType,
			},
		})
	}

	// 状态过滤（term 精确匹配）
	if req.Status != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{
				"status": req.Status,
			},
		})
	}

	// 国家过滤（term 精确匹配）
	if req.Country != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{
				"country.keyword": req.Country,
			},
		})
	}

	// 省份过滤（term 精确匹配）
	if req.Province != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{
				"province.keyword": req.Province,
			},
		})
	}

	// 城市过滤（term 精确匹配）
	if req.City != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{
				"city.keyword": req.City,
			},
		})
	}

	// 区/县过滤（term 精确匹配）
	if req.District != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{
				"district.keyword": req.District,
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

func (s *SearchService) buildFilters(req *types.SearchRequest, userEmail string) []any {
	var filter []any

	filter = append(filter, buildEmailFilter(userEmail))

	dateRange := map[string]any{}
	if req.DateFrom != "" {
		dateRange["gte"] = req.DateFrom
	}
	if req.DateTo != "" {
		dateRange["lte"] = req.DateTo
	}
	if len(dateRange) > 0 {
		filter = append(filter, map[string]any{
			"range": map[string]any{"exif.date_time_original": dateRange},
		})
	}

	if len(req.Tags) > 0 {
		filter = append(filter, map[string]any{
			"terms": map[string]any{"tags.keyword": req.Tags},
		})
	}

	if len(req.Objects) > 0 {
		filter = append(filter, map[string]any{
			"terms": map[string]any{"objects.keyword": req.Objects},
		})
	}

	if req.SceneType != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"scene_type": req.SceneType},
		})
	}

	if req.Status != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"status": req.Status},
		})
	}

	if req.Country != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"country.keyword": req.Country},
		})
	}

	if req.Province != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"province.keyword": req.Province},
		})
	}

	if req.City != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"city.keyword": req.City},
		})
	}

	if req.District != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"district.keyword": req.District},
		})
	}

	return filter
}

// aggResult 是聚合结果的通用结构，包含 buckets 列表。
type aggResult struct {
	Buckets []aggBucket `json:"buckets"`
}

// aggBucket 表示词项聚合（terms aggregation）中的一个桶，Key 为聚合值。
type aggBucket struct {
	Key string `json:"key"`
}

// aggBucketWithCount 表示带文档计数的词项聚合桶（Key + DocCount）。
type aggBucketWithCount struct {
	Key      string `json:"key"`
	DocCount int64  `json:"doc_count"`
}

// buildSearchResponse converts typed OpenSearch SearchResp to types.SearchResponse.
func (s *SearchService) buildSearchResponse(resp *opensearchapi.SearchResp, req *types.SearchRequest) (*types.SearchResponse, error) {
	total := int64(resp.Hits.Total.Value)

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

	hits := make([]types.PhotoDocument, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		var doc types.PhotoDocument
		if err := json.Unmarshal(hit.Source, &doc); err != nil {
			slog.Warn("unmarshal search hit failed", "doc_id", hit.ID, "error", err)
			continue
		}
		doc.Embedding = nil
		hits = append(hits, doc)
	}

	return &types.SearchResponse{
		Hits:       hits,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetStats 返回照片库的聚合统计信息。
//
// 聚合内容：
//   - by_status: terms aggregation on status 字段，按处理状态分组计数
//   - recent: filter aggregation，统计最近 1 小时内创建的文档数
//   - track_total_hits: true，精确统计文档总数（非近似值）
func (s *SearchService) GetStats(ctx context.Context, indexName string, userEmail string) (*types.StatsResponse, error) {
	slog.Debug("get stats", "index", indexName)
	query := map[string]any{
		"size":             0,
		"track_total_hits": true,
		"query":            buildEmailFilter(userEmail),
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

	resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("stats request: %w", err)
	}

	return s.parseStatsResponse(resp)
}

// parseStatsResponse 解析 OpenSearch 统计响应，构建按状态分组的计数映射。
//
// 处理逻辑：
//   - 初始化所有 5 种状态为 0（确保未出现的状态也返回 0 而非缺失）
//   - 遍历 by_status buckets 填充实际计数
//   - 提取 recent filter 的 doc_count 作为最近新增数
func (s *SearchService) parseStatsResponse(resp *opensearchapi.SearchResp) (*types.StatsResponse, error) {
	total := int64(resp.Hits.Total.Value)

	var aggs struct {
		ByStatus struct {
			Buckets []aggBucketWithCount `json:"buckets"`
		} `json:"by_status"`
		Recent struct {
			DocCount int64 `json:"doc_count"`
		} `json:"recent"`
	}
	if err := json.Unmarshal(resp.Aggregations, &aggs); err != nil {
		return nil, fmt.Errorf("decode stats aggs: %w", err)
	}

	byStatus := map[types.JobStatus]int64{
		types.StatusUnanalyzed:      0,
		types.StatusAnalyzing:       0,
		types.StatusAnalyzed:        0,
		types.StatusFailed:          0,
		types.StatusPendingAnalysis: 0,
	}

	for _, b := range aggs.ByStatus.Buckets {
		byStatus[types.JobStatus(b.Key)] = b.DocCount
	}

	return &types.StatsResponse{
		Total:       total,
		ByStatus:    byStatus,
		RecentCount: aggs.Recent.DocCount,
	}, nil
}

// parseFiltersResponse 解析 OpenSearch 聚合响应，提取 tags/scene_types/countries/provinces/cities/districts/statuses 的 bucket key 列表。
// 返回的字符串切片按文档计数降序排列（OpenSearch terms aggregation 默认行为），
// 前端可直接用于填充下拉筛选器的选项列表。
func (s *SearchService) parseFiltersResponse(aggsRaw json.RawMessage) (*types.FiltersResponse, error) {
	var result struct {
		Tags        aggResult `json:"tags"`
		SceneTypes  aggResult `json:"scene_types"`
		Countries   aggResult `json:"countries"`
		Provinces   aggResult `json:"provinces"`
		Cities      aggResult `json:"cities"`
		Districts   aggResult `json:"districts"`
		Statuses    aggResult `json:"statuses"`
	}
	if err := json.Unmarshal(aggsRaw, &result); err != nil {
		return nil, fmt.Errorf("decode filters aggs: %w", err)
	}

	tags := make([]string, 0, len(result.Tags.Buckets))
	for _, b := range result.Tags.Buckets {
		tags = append(tags, b.Key)
	}

	scenes := make([]string, 0, len(result.SceneTypes.Buckets))
	for _, b := range result.SceneTypes.Buckets {
		scenes = append(scenes, b.Key)
	}

	countries := make([]string, 0, len(result.Countries.Buckets))
	for _, b := range result.Countries.Buckets {
		countries = append(countries, b.Key)
	}

	provinces := make([]string, 0, len(result.Provinces.Buckets))
	for _, b := range result.Provinces.Buckets {
		provinces = append(provinces, b.Key)
	}

	cities := make([]string, 0, len(result.Cities.Buckets))
	for _, b := range result.Cities.Buckets {
		cities = append(cities, b.Key)
	}

	districts := make([]string, 0, len(result.Districts.Buckets))
	for _, b := range result.Districts.Buckets {
		districts = append(districts, b.Key)
	}

	statuses := make([]string, 0, len(result.Statuses.Buckets))
	for _, b := range result.Statuses.Buckets {
		statuses = append(statuses, b.Key)
	}

	return &types.FiltersResponse{
		Tags:       tags,
		SceneTypes: scenes,
		Countries:  countries,
		Provinces:  provinces,
		Cities:     cities,
		Districts:  districts,
		Statuses:   statuses,
	}, nil
}

// truncateJSON 截断 JSON 字节数组用于日志输出，避免超长查询体淹没日志。
func truncateJSON(b []byte, maxLen int) string {
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "..."
}
