// Package search 提供基于 Elasticsearch 的照片搜索服务，支持全文检索、
// 多维过滤和聚合统计。
//
// 核心功能：
//   - 全文搜索：multi_match 跨 description/tags/objects/text 字段
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
	"sort"
	"time"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/zwh8800/phosche/internal/indexer"
	"github.com/zwh8800/phosche/internal/types"
)

// Embedder 是文本向量化的抽象接口。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// EmbeddingCache 是查询 embedding 缓存的接口。
type EmbeddingCache interface {
	Get(text string) ([]float32, bool)
	Set(text string, embedding []float32)
}

// HybridConfig 是应用层 RRF 混合检索参数。
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
}

// SearchService 提供基于 Elasticsearch 的全文搜索和条件过滤查询。
type SearchService struct {
	client         *indexer.ESClient
	embedder       Embedder
	embeddingCache EmbeddingCache
	hybridCfg      HybridConfig
}

// NewSearchService 创建 SearchService 实例。
func NewSearchService(client *indexer.ESClient, opts ...SearchOption) *SearchService {
	s := &SearchService{client: client}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

	if req.CameraModel != "" {
		filter = append(filter, map[string]any{
			"term": map[string]any{"camera_model": req.CameraModel},
		})
	}

	return filter
}

// Search 执行全文搜索+条件过滤查询。
//
// 流程：
//  1. 调用 buildQuery 构建 ES 查询 DSL
//  2. 序列化为 JSON，通过 esapi.SearchRequest 发送
//  3. 调用 parseSearchResponse 解析 ES 响应
//
// userEmail 用于基于 email 的访问过滤，限制搜索结果仅包含用户有权访问的文档。
// 调试日志会输出截断后的查询 JSON（最多 500 字符）和结果命中数。
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
		"camera_model", req.CameraModel,
		"status", req.Status,
		"has_embedder", s.embedder != nil,
	)

	if req.Query != "" && s.embedder != nil {
		queryVec, ok := s.getEmbedding(ctx, req.Query)
		if ok {
			slog.Debug("hybrid search enabled, using BM25 + kNN RRF",
				"index", indexName,
				"query", req.Query,
				"vector_dim", len(queryVec),
			)
			return s.searchHybrid(ctx, indexName, req, queryVec, userEmail)
		}
		slog.Debug("embedding unavailable, falling back to BM25",
			"index", indexName,
			"query", req.Query,
		)
	}
	return s.searchFallbackToBM25(ctx, indexName, req, userEmail)
}

// GetFilters 执行词项聚合（terms aggregation），获取前端筛选面板所需的可选项列表。
//
// 聚合字段：
//   - tags.keyword: 最多 50 个标签（按文档计数降序）
//   - scene_type: 最多 20 个场景类型
//   - camera_model: 最多 20 个相机型号
//
// 返回的 FiltersResponse 用于填充搜索页面的下拉筛选器。
func (s *SearchService) GetFilters(ctx context.Context, indexName string, userEmail string) (*types.FiltersResponse, error) {
	slog.Debug("ES get filters", "index", indexName)
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

// buildQuery 是核心查询构造器，将 SearchRequest 转换为 ES 查询 DSL。
//
// 构建规则（按顺序）：
//  1. 分页默认值：page=1, pageSize=20
//  2. 排序策略：
//     - 有搜索词时：_score（相关性）优先，其次 date_time_original desc，最后 mtime desc
//     - 无搜索词时：date_time_original desc（缺失值排最后），mtime desc
//  3. 高亮：description 字段启用 ES 高亮
//  4. 全文搜索：multi_match 跨 description、tags、objects、text 四个字段
//  5. 日期范围过滤：date_time_original 的 gte/lte
//  6. 词项过滤：tags.keyword、objects.keyword 的 terms 查询
//  7. 精确匹配：scene_type、status、camera_model 的 term 查询
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

	// 相机型号过滤（term 精确匹配）
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

// esSearchResult 是 ES 搜索响应的 JSON 结构，包含命中总数和文档 _source。
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

// esAggsResult 对应 ES 聚合查询（GetFilters）返回的 JSON 结构。
// 包含 tags、scene_types、cameras 三个词项聚合的 buckets。
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

// aggBucket 表示词项聚合（terms aggregation）中的一个桶，Key 为聚合值。
type aggBucket struct {
	Key string `json:"key"`
}

// parseSearchResponse 从 ES 响应体解码搜索结果，填充分页元数据。
//
// 总页数计算：ceil(total / pageSize)，特殊处理 total=0 时总页数为 0。
// 使用 SearchRequest 中的 page/pageSize（已由 buildQuery 应用默认值），
// 确保响应中的分页信息与请求一致。
// parseSearchResponse 解析 ES 搜索响应并构建分页结果。
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
		doc := hit.Source
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

// GetStats 返回照片汇总统计信息。
//
// 聚合内容：
//   - by_status: terms aggregation on status 字段，按处理状态分组计数
//   - recent: filter aggregation，统计最近 1 小时内创建的文档数
//   - track_total_hits: true，精确统计文档总数（非近似值）
func (s *SearchService) GetStats(ctx context.Context, indexName string, userEmail string) (*types.StatsResponse, error) {
	slog.Debug("ES get stats", "index", indexName)
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

// esStatsResult 是 ES 统计查询（GetStats）响应的 JSON 结构。
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

// aggBucketWithCount 表示带文档计数的词项聚合桶（Key + DocCount）。
type aggBucketWithCount struct {
	Key      string `json:"key"`
	DocCount int64  `json:"doc_count"`
}

// parseStatsResponse 解析 ES 统计响应，构建按状态分组的计数映射。
//
// 处理逻辑：
//   - 初始化所有 5 种状态为 0（确保未出现的状态也返回 0 而非缺失）
//   - 遍历 by_status buckets 填充实际计数
//   - 提取 recent filter 的 doc_count 作为最近新增数
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

// parseFiltersResponse 解析 ES 聚合响应，提取 tags/scene_types/cameras 的 bucket key 列表。
// 返回的字符串切片按文档计数降序排列（ES terms aggregation 默认行为），
// 前端可直接用于填充下拉筛选器的选项列表。
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

// truncateJSON 截断 JSON 字节数组用于日志输出，避免超长查询体淹没日志。
func truncateJSON(b []byte, maxLen int) string {
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "..."
}

// scoredDoc 是 RRF 融合后的文档，持有最终融合分数。
type scoredDoc struct {
	id     string
	score  float64
	source types.PhotoDocument
}

// msearchHit 是 _msearch 响应中的单个命中。
type msearchHit struct {
	ID     string              `json:"_id"`
	Source types.PhotoDocument `json:"_source"`
}

// msearchResponse 是 _msearch 响应的 JSON 结构。
type msearchResponse struct {
	Responses []struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []msearchHit `json:"hits"`
		} `json:"hits"`
		Error *json.RawMessage `json:"error,omitempty"`
	} `json:"responses"`
}

// searchHybrid 执行 _msearch（BM25 + kNN），RRF 融合结果后分页返回。
func (s *SearchService) searchHybrid(ctx context.Context, indexName string, req *types.SearchRequest, queryVector []float32, userEmail string) (*types.SearchResponse, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	from := (page - 1) * pageSize

	rankWindowSize := calculateRankWindowSize(from, pageSize)

	msearchBody := s.buildHybridQuery(indexName, req, queryVector, userEmail, rankWindowSize)

	slog.Debug("hybrid msearch request",
		"index", indexName,
		"query", req.Query,
		"rank_window_size", rankWindowSize,
		"rank_constant", s.hybridCfg.RRFRankConstant,
		"from", from,
		"page_size", pageSize,
		"body_size", len(msearchBody),
		"body", string(msearchBody),
	)

	start := time.Now()
	bm25Hits, knnHits, bm25Total, knnTotal, err := s.executeHybridSearch(ctx, indexName, msearchBody)
	if err != nil {
		return nil, err
	}
	esDuration := time.Since(start)

	slog.Debug("hybrid msearch completed",
		"index", indexName,
		"bm25_hits", len(bm25Hits),
		"bm25_total", bm25Total,
		"knn_hits", len(knnHits),
		"knn_total", knnTotal,
		"es_duration_ms", esDuration.Milliseconds(),
	)

	start = time.Now()
	fused := s.reciprocalRankFusion(bm25Hits, knnHits, s.hybridCfg.RRFRankConstant)
	fusionDuration := time.Since(start)
	pageHits := paginateResults(fused, from, pageSize)

	total := max(bm25Total, knnTotal)

	slog.Debug("hybrid RRF fusion completed",
		"index", indexName,
		"query", req.Query,
		"bm25_candidates", len(bm25Hits),
		"knn_candidates", len(knnHits),
		"fused_unique_docs", len(fused),
		"page_results", len(pageHits),
		"total", total,
		"fusion_duration_us", fusionDuration.Microseconds(),
	)

	if len(fused) > 0 {
		slog.Debug("hybrid RRF top results",
			"index", indexName,
			"query", req.Query,
			"top_doc_id", fused[0].id,
			"top_doc_score", fmt.Sprintf("%.6f", fused[0].score),
		)
		if len(fused) > 1 {
			slog.Debug("hybrid RRF runner-up",
				"index", indexName,
				"query", req.Query,
				"second_doc_id", fused[1].id,
				"second_doc_score", fmt.Sprintf("%.6f", fused[1].score),
			)
		}
	}

	var totalPages int
	if pageSize > 0 && total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(pageSize)))
	} else if total == 0 {
		totalPages = 0
	} else {
		totalPages = 1
	}

	hits := make([]types.PhotoDocument, 0, len(pageHits))
	for _, doc := range pageHits {
		d := doc.source
		d.Embedding = nil
		hits = append(hits, d)
	}

	return &types.SearchResponse{
		Hits:       hits,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// searchFallbackToBM25 当 embedding 不可用时回退到纯 BM25 搜索。
func (s *SearchService) searchFallbackToBM25(ctx context.Context, indexName string, req *types.SearchRequest, userEmail string) (*types.SearchResponse, error) {
	query := s.buildQuery(req, userEmail)
	bodyBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	slog.Debug("ES search", "index", indexName, "query", string(bodyBytes), "page", req.Page, "page_size", req.PageSize)

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
		errMsg := string(b)
		slog.Error("ES search failed",
			"status", resp.Status(),
			"error", errMsg,
			"query", string(bodyBytes),
		)
		return nil, fmt.Errorf("search returned %s: %s", resp.Status(), errMsg)
	}

	result, err := s.parseSearchResponse(resp.Body, req)
	if err == nil {
		slog.Debug("ES search result", "total", result.Total, "hits", len(result.Hits))
	}
	return result, err
}

// buildHybridQuery 构建 _msearch NDJSON 请求体，包含 BM25 和 kNN 两个子查询。
func (s *SearchService) buildHybridQuery(indexName string, req *types.SearchRequest, queryVector []float32, userEmail string, rankWindowSize int) []byte {
	var buf bytes.Buffer

	bm25Query := s.buildBM25SubQuery(req, userEmail, rankWindowSize)
	knnQuery := s.buildKNNSubQuery(queryVector, userEmail, req, rankWindowSize)

	headerBytes, _ := json.Marshal(map[string]any{"index": indexName})
	bm25Bytes, _ := json.Marshal(bm25Query)
	knnBytes, _ := json.Marshal(knnQuery)

	buf.Write(headerBytes)
	buf.WriteByte('\n')
	buf.Write(bm25Bytes)
	buf.WriteByte('\n')
	buf.Write(headerBytes)
	buf.WriteByte('\n')
	buf.Write(knnBytes)
	buf.WriteByte('\n')

	return buf.Bytes()
}

// buildBM25SubQuery 构建 BM25 multi_match 子查询（不含 sort/from/size 外层分页）。
func (s *SearchService) buildBM25SubQuery(req *types.SearchRequest, userEmail string, rankWindowSize int) map[string]any {
	filters := s.buildFilters(req, userEmail)

	return map[string]any{
		"size": rankWindowSize,
		"query": map[string]any{
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
		},
		"highlight": map[string]any{
			"fields": map[string]any{
				"description": map[string]any{},
			},
		},
		"_source": map[string]any{
			"excludes": []string{"embedding", "embedding_version", "embedded_at"},
		},
	}
}

// buildKNNSubQuery 构建基于 script_score 余弦相似度的 kNN 子查询。
func (s *SearchService) buildKNNSubQuery(queryVector []float32, userEmail string, req *types.SearchRequest, rankWindowSize int) map[string]any {
	filters := s.buildFilters(req, userEmail)

	return map[string]any{
		"size": rankWindowSize,
		"query": map[string]any{
			"script_score": map[string]any{
				"query": map[string]any{
					"bool": map[string]any{
						"filter": filters,
					},
				},
				"script": map[string]any{
					"source": "cosineSimilarity(params.query_vector, 'embedding') + 1.0",
					"params": map[string]any{
						"query_vector": queryVector,
					},
				},
			},
		},
		"_source": map[string]any{
			"excludes": []string{"embedding", "embedding_version", "embedded_at"},
		},
	}
}

// executeHybridSearch 发送 _msearch 请求并解析 BM25 和 kNN 两个子查询的结果。
// 任一子查询失败时记录警告并以空结果继续，两者均失败时返回错误。
func (s *SearchService) executeHybridSearch(ctx context.Context, indexName string, msearchBody []byte) (
	bm25Hits []msearchHit, knnHits []msearchHit, bm25Total int64, knnTotal int64, err error,
) {
	resp, err := s.client.Client().Msearch(
		bytes.NewReader(msearchBody),
		s.client.Client().Msearch.WithContext(ctx),
		s.client.Client().Msearch.WithIndex(indexName),
	)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("msearch request: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		slog.Error("hybrid msearch failed",
			"index", indexName,
			"status", resp.Status(),
			"error", string(b),
		)
		return nil, nil, 0, 0, fmt.Errorf("msearch returned %s: %s", resp.Status(), string(b))
	}

	var msearchResp msearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&msearchResp); err != nil {
		slog.Error("hybrid msearch response decode failed",
			"index", indexName,
			"error", err,
		)
		return nil, nil, 0, 0, fmt.Errorf("decode msearch response: %w", err)
	}

	if len(msearchResp.Responses) < 2 {
		return nil, nil, 0, 0, fmt.Errorf("msearch returned %d responses, expected 2", len(msearchResp.Responses))
	}

	bm25Resp := msearchResp.Responses[0]
	knnResp := msearchResp.Responses[1]

	bm25Failed := bm25Resp.Error != nil
	knnFailed := knnResp.Error != nil

	if bm25Failed && knnFailed {
		return nil, nil, 0, 0, fmt.Errorf("both sub-queries failed: bm25=%s, knn=%s",
			string(*bm25Resp.Error), string(*knnResp.Error))
	}

	if bm25Failed {
		slog.Warn("BM25 sub-query failed in hybrid search, continuing with kNN only",
			"error", string(*bm25Resp.Error))
		knnHits = knnResp.Hits.Hits
		knnTotal = knnResp.Hits.Total.Value
		return nil, knnHits, 0, knnTotal, nil
	}

	if knnFailed {
		slog.Warn("kNN sub-query failed in hybrid search, continuing with BM25 only",
			"error", string(*knnResp.Error))
		bm25Hits = bm25Resp.Hits.Hits
		bm25Total = bm25Resp.Hits.Total.Value
		return bm25Hits, nil, bm25Total, 0, nil
	}

	bm25Hits = bm25Resp.Hits.Hits
	bm25Total = bm25Resp.Hits.Total.Value
	knnHits = knnResp.Hits.Hits
	knnTotal = knnResp.Hits.Total.Value

	return bm25Hits, knnHits, bm25Total, knnTotal, nil
}

// reciprocalRankFusion 实现 RRF 算法：score(d) = Σ 1/(k + rank(d))，rank 从 1 开始。
func (s *SearchService) reciprocalRankFusion(bm25Hits, knnHits []msearchHit, rankConstant int) []scoredDoc {
	scores := make(map[string]float64)
	sources := make(map[string]types.PhotoDocument)

	for i, hit := range bm25Hits {
		scores[hit.ID] += 1.0 / float64(rankConstant+i+1)
		sources[hit.ID] = hit.Source
	}

	for i, hit := range knnHits {
		scores[hit.ID] += 1.0 / float64(rankConstant+i+1)
		if _, exists := sources[hit.ID]; !exists {
			sources[hit.ID] = hit.Source
		}
	}

	result := make([]scoredDoc, 0, len(scores))
	for id, score := range scores {
		result = append(result, scoredDoc{id: id, score: score, source: sources[id]})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].score == result[j].score {
			return result[i].id < result[j].id
		}
		return result[i].score > result[j].score
	})
	return result
}

// paginateResults 对 RRF 融合后的有序文档切片执行分页。
func paginateResults(docs []scoredDoc, from, pageSize int) []scoredDoc {
	if from >= len(docs) {
		return nil
	}
	end := min(from+pageSize, len(docs))
	return docs[from:end]
}

// calculateRankWindowSize 计算 rank window size：from + pageSize，钳制到 [50, 1000]。
func calculateRankWindowSize(from, pageSize int) int {
	size := from + pageSize
	if size < 50 {
		return 50
	}
	if size > 1000 {
		return 1000
	}
	return size
}
