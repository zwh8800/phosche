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
	"github.com/zwh8800/phosche/internal/util"
	"github.com/zwh8800/phosche/internal/types"
)

// pipelineName 是 OpenSearch 原生 RRF 混合检索的搜索管道名称。
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
	RRFRankConstant int
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
	FindSimilar(ctx context.Context, indexName string, photoID string, embedding []float32, userEmail string) (*types.RecommendationResponse, error)
	FindNearby(ctx context.Context, indexName string, photoID string, lat, lon float64, userEmail string) (*types.RecommendationResponse, error)
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

// EnsureSearchPipeline 创建 RRF 搜索管道（如果不存在）。
// 在启动时调用（embedder 已配置的情况下），需要 OpenSearch 已可用。
// 使用 PUT /_search/pipeline/{name} 幂等创建，200 表示创建成功，409 表示已存在。
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

	// 200 = 创建成功，409 = 已存在（幂等），其他状态码 = 错误
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
// 混合检索触发条件：req.Query 非空 且 embedder 已配置 且 embedding 计算成功。
// 降级策略：embedder 未配置 → 纯 BM25；embedding 计算失败 → 日志警告并回退到纯 BM25。
//
// 流程：
//  1. 有查询词且 embedder 可用时，调用 searchHybrid（BM25 + kNN 混合检索，服务端 RRF）
//  2. 否则调用 searchBM25（纯 BM25 全文检索或过滤）
//
// 参数：
//   - ctx: 请求上下文
//   - indexName: OpenSearch 索引名称
//   - req: 搜索请求（查询词、分页、过滤条件等）
//   - userEmail: 当前用户邮箱，用于基于 email 的访问过滤，限制搜索结果仅包含用户有权访问的文档
//
// 返回值：
//   - *types.SearchResponse: 包含 hits、total、page、page_size、total_pages
//   - error: OpenSearch 请求失败或序列化错误
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
//
// RRF（Reciprocal Rank Fusion）算法原理：
//   - 分别计算 BM25 和 kNN 两个子查询的排名
//   - 最终分数 = Σ 1/(rank_constant + rank_i)，rank_i 为文档在各子查询中的排名
//   - rank_constant 默认 60，控制排名差异的权重衰减速度（值越大，头部与尾部的分数差异越小）
//   - 该融合在 OpenSearch 服务端通过 search pipeline 的 phase_results_processor 完成
//
// 参数：
//   - queryVec: 查询文本的 embedding 向量，由 getEmbedding 计算
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

	// BM25 子查询：基于关键词的全文检索，使用 multi_match 跨字段匹配
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

	// KNN 子查询（OpenSearch knn 查询子句）：基于向量相似度的近邻检索
	// filter 确保 province/country/date 等筛选条件在 kNN 检索中同样生效
	k := pageSize * 2
	knnQuery := map[string]any{
		"knn": map[string]any{
			"embedding": map[string]any{
				"vector": queryVec,
				"k":      k,
				"filter": map[string]any{
					"bool": map[string]any{
						"filter": filters,
					},
				},
			},
		},
	}

	// 混合查询：组合 BM25 和 kNN 子查询，通过 search pipeline 实现服务端 RRF 融合
	// RRF（Reciprocal Rank Fusion）算法：score = Σ 1/(k + rank_i)，k 为 rank_constant
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
		"query_body", util.TruncateBytes(bodyBytes, 800),
	)

	resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
		Params: opensearchapi.SearchParams{
			SearchPipeline: pipelineName,
		},
	})
	if err != nil {
		// 输出完整的 OpenSearch 错误信息（包含 status、type、reason、root_cause、caused_by）
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
			"query_body", util.TruncateBytes(bodyBytes, 2000),
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

	slog.Debug("BM25 search", "index", indexName, "query", util.TruncateBytes(bodyBytes, 500), "page", req.Page, "page_size", req.PageSize)

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
			"query", util.TruncateBytes(bodyBytes, 2000),
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

// buildQuery 是核心查询构造器，将 SearchRequest 转换为 OpenSearch 查询 DSL。
// 仅用于纯 BM25 模式（searchBM25），混合检索模式使用 searchHybrid 独立构建查询。
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

	filter := s.buildFilters(req, userEmail)

	// 全文搜索：multi_match 跨 description/tags/objects/text/formatted_address 六个字段
	if req.Query != "" {
		must = append(must, map[string]any{
			"multi_match": map[string]any{
				"query":  req.Query,
				"fields": []string{"description", "tags", "objects", "text", "address", "formatted_address"},
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
			"term": map[string]any{"country": req.Country},
		})
	}

	if req.Province != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"province": req.Province},
		})
	}

	if req.City != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"city": req.City},
		})
	}

	if req.District != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"district": req.District},
		})
	}

	return filter
}

// buildSearchResponse 将 OpenSearch 类型化响应转换为标准搜索响应格式。
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
				"terms": map[string]any{"field": "country", "size": 50},
			},
			"provinces": map[string]any{
				"terms": map[string]any{"field": "province", "size": 50},
			},
			"cities": map[string]any{
				"terms": map[string]any{"field": "city", "size": 50},
			},
			"districts": map[string]any{
				"terms": map[string]any{"field": "district", "size": 50},
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

// collectNonEmptyKeys 从词项聚合结果中过滤掉空字符串桶。
func collectNonEmptyKeys(buckets []aggBucket) []string {
	keys := make([]string, 0, len(buckets))
	for _, b := range buckets {
		if b.Key != "" {
			keys = append(keys, b.Key)
		}
	}
	return keys
}

// parseFiltersResponse 解析 OpenSearch 聚合响应，提取 tags/scene_types/countries/provinces/cities/districts/statuses 的 bucket key 列表。
// 返回的字符串切片按文档计数降序排列（OpenSearch terms aggregation 默认行为），
// 前端可直接用于填充下拉筛选器的选项列表。
func (s *SearchService) parseFiltersResponse(aggsRaw json.RawMessage) (*types.FiltersResponse, error) {
	var result struct {
		Tags       aggResult `json:"tags"`
		SceneTypes aggResult `json:"scene_types"`
		Countries  aggResult `json:"countries"`
		Provinces  aggResult `json:"provinces"`
		Cities     aggResult `json:"cities"`
		Districts  aggResult `json:"districts"`
		Statuses   aggResult `json:"statuses"`
	}
	if err := json.Unmarshal(aggsRaw, &result); err != nil {
		return nil, fmt.Errorf("decode filters aggs: %w", err)
	}

	tags := collectNonEmptyKeys(result.Tags.Buckets)
	scenes := collectNonEmptyKeys(result.SceneTypes.Buckets)
	countries := collectNonEmptyKeys(result.Countries.Buckets)
	provinces := collectNonEmptyKeys(result.Provinces.Buckets)
	cities := collectNonEmptyKeys(result.Cities.Buckets)
	districts := collectNonEmptyKeys(result.Districts.Buckets)
	statuses := collectNonEmptyKeys(result.Statuses.Buckets)

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

// FindSimilar 查找与指定照片视觉相似的照片，基于 kNN 向量检索。
//
// 算法：使用 hybrid 搜索管道（BM25 + kNN），BM25 使用 match_all 常量分数（不参与排序），
// kNN 根据 embedding 向量相似度检索最近邻。RRF 融合后 kNN 主导排序结果。
//
// 参数：
//   - photoID: 源照片文档 ID（SHA-256 哈希），用于排除自身
//   - embedding: 源照片的 embedding 向量（空向量时直接返回空结果）
//   - userEmail: 访问控制用邮箱
//
// 返回值：
//   - *types.RecommendationResponse: 包含最相似的 3 张照片（排除自身）
//   - error: OpenSearch 请求失败或序列化错误
func (s *SearchService) FindSimilar(ctx context.Context, indexName string, photoID string, embedding []float32, userEmail string) (*types.RecommendationResponse, error) {
	if len(embedding) == 0 {
		return &types.RecommendationResponse{Photos: []types.PhotoDocument{}, Total: 0}, nil
	}

	accessFilter := []any{
		buildEmailFilter(userEmail),
		map[string]any{
			"bool": map[string]any{
				"must_not": map[string]any{
					"term": map[string]any{"_id": photoID},
				},
			},
		},
	}

	// BM25 子查询：带访问过滤的 match_all（常量分数，kNN 通过 RRF 主导排序）
	bm25Query := map[string]any{
		"bool": map[string]any{
			"filter": accessFilter,
		},
	}

	// KNN 子查询：embedding 向量搜索，k=4 时取 top-3（排除自身后）
	knnQuery := map[string]any{
		"knn": map[string]any{
			"embedding": map[string]any{
				"vector": embedding,
				"k":      4,
				"filter": map[string]any{
					"bool": map[string]any{
						"filter": []any{
							buildEmailFilter(userEmail),
							map[string]any{
								"bool": map[string]any{
									"must_not": map[string]any{
										"term": map[string]any{"_id": photoID},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	query := map[string]any{
		"size": 3,
		"query": map[string]any{
			"hybrid": map[string]any{
				"queries": []any{bm25Query, knnQuery},
			},
		},
	}

	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal find-similar query: %w", err)
	}

	slog.Debug("find similar request",
		"index", indexName,
		"photo_id", photoID,
		"embedding_dim", len(embedding),
		"pipeline", pipelineName,
		"query_body", util.TruncateBytes(bodyBytes, 800),
	)

	resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
		Params: opensearchapi.SearchParams{
			SearchPipeline: pipelineName,
		},
	})
	if err != nil {
		status := 0
		if resp != nil && resp.Inspect().Response != nil {
			status = resp.Inspect().Response.StatusCode
		}
		slog.Error("find similar OpenSearch request failed",
			"index", indexName,
			"photo_id", photoID,
			"http_status", status,
			"pipeline", pipelineName,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("find similar: %w", err)
	}

	slog.Debug("find similar OpenSearch response OK",
		"index", indexName,
		"photo_id", photoID,
		"took_ms", resp.Took,
		"hits_total", resp.Hits.Total.Value,
		"hits_count", len(resp.Hits.Hits),
	)

	return s.buildRecommendationResponse(resp), nil
}

// FindNearby 查找与指定照片地理位置相近的照片，基于 Haversine 脚本计算距离。
//
// Haversine 公式：d = 2R·arcsin(√(sin²(Δlat/2) + cos(lat1)·cos(lat2)·sin²(Δlon/2)))
// 其中 R=6371km 为地球半径，结果单位为公里。该公式在 OpenSearch 的 painless 脚本中执行。
//
// 搜索策略：
//   - 使用 script filter 限制 5km 半径内的文档
//   - 使用 script sort 按 Haversine 距离升序排列（最近的在前）
//   - 仅返回 status=analyzed 的照片（确保有 GPS 数据和完整分析结果）
//
// 参数：
//   - photoID: 源照片文档 ID，用于排除自身
//   - lat/lon: 源照片的 GPS 坐标（纬度/经度）
//   - userEmail: 访问控制用邮箱
//
// 返回值：
//   - *types.RecommendationResponse: 包含距离最近的 3 张照片（排除自身）
//   - error: OpenSearch 请求失败或序列化错误。lat 和 lon 均为 0 时直接返回空结果。
func (s *SearchService) FindNearby(ctx context.Context, indexName string, photoID string, lat, lon float64, userEmail string) (*types.RecommendationResponse, error) {
	if lat == 0 && lon == 0 {
		return &types.RecommendationResponse{Photos: []types.PhotoDocument{}, Total: 0}, nil
	}

	haversineScript := map[string]any{
		"source": "double R = 6371; double lat1 = doc['exif.gps_lat'].value * Math.PI / 180; double lat2 = params.lat * Math.PI / 180; double dLat = (params.lat - doc['exif.gps_lat'].value) * Math.PI / 180; double dLon = (params.lon - doc['exif.gps_lon'].value) * Math.PI / 180; double a = Math.sin(dLat/2) * Math.sin(dLat/2) + Math.cos(lat1) * Math.cos(lat2) * Math.sin(dLon/2) * Math.sin(dLon/2); double c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a)); return R * c;",
		"lang":   "painless",
		"params": map[string]any{"lat": lat, "lon": lon},
	}

	distanceFilterScript := map[string]any{
		"source": "double R = 6371; double lat1 = doc['exif.gps_lat'].value * Math.PI / 180; double lat2 = params.lat * Math.PI / 180; double dLat = (params.lat - doc['exif.gps_lat'].value) * Math.PI / 180; double dLon = (params.lon - doc['exif.gps_lon'].value) * Math.PI / 180; double a = Math.sin(dLat/2) * Math.sin(dLat/2) + Math.cos(lat1) * Math.cos(lat2) * Math.sin(dLon/2) * Math.sin(dLon/2); double c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a)); return R * c <= params.maxDist;",
		"lang":   "painless",
		"params": map[string]any{"lat": lat, "lon": lon, "maxDist": 5.0},
	}

	query := map[string]any{
		"size": 3,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					buildEmailFilter(userEmail),
					map[string]any{"term": map[string]any{"status": "analyzed"}},
					map[string]any{
						"bool": map[string]any{
							"must_not": map[string]any{
								"term": map[string]any{"_id": photoID},
							},
						},
					},
					map[string]any{"exists": map[string]any{"field": "exif.gps_lat"}},
					map[string]any{"script": map[string]any{"script": distanceFilterScript}},
				},
			},
		},
		"sort": []any{
			map[string]any{
				"_script": map[string]any{
					"type":   "number",
					"script": haversineScript,
					"order":  "asc",
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal find-nearby query: %w", err)
	}

	slog.Debug("find nearby request",
		"index", indexName,
		"photo_id", photoID,
		"lat", lat,
		"lon", lon,
		"query_body", util.TruncateBytes(bodyBytes, 500),
	)

	resp, err := s.client.Client().Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
	})
	if err != nil {
		status := 0
		if resp != nil && resp.Inspect().Response != nil {
			status = resp.Inspect().Response.StatusCode
		}
		slog.Error("find nearby OpenSearch request failed",
			"index", indexName,
			"photo_id", photoID,
			"lat", lat,
			"lon", lon,
			"http_status", status,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("find nearby: %w", err)
	}

	slog.Debug("find nearby OpenSearch response OK",
		"index", indexName,
		"photo_id", photoID,
		"took_ms", resp.Took,
		"hits_total", resp.Hits.Total.Value,
		"hits_count", len(resp.Hits.Hits),
	)

	return s.buildRecommendationResponse(resp), nil
}

// buildRecommendationResponse 将 OpenSearch 响应转换为推荐响应格式。
// 返回前清空 embedding 字段以避免传输大量向量数据。
func (s *SearchService) buildRecommendationResponse(resp *opensearchapi.SearchResp) *types.RecommendationResponse {
	hits := make([]types.PhotoDocument, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		var doc types.PhotoDocument
		if err := json.Unmarshal(hit.Source, &doc); err != nil {
			slog.Warn("unmarshal recommendation hit failed", "doc_id", hit.ID, "error", err)
			continue
		}
		doc.Embedding = nil
		hits = append(hits, doc)
	}

	return &types.RecommendationResponse{
		Photos: hits,
		Total:  len(hits),
	}
}
