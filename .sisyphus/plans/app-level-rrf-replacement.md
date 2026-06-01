# 应用层 RRF 替代方案详细设计

> **目标**：用应用侧 RRF 融合替代 Elasticsearch Platinum 内置 `retriever.rrf`，在 Basic License 下实现等价的混合搜索能力。  
> **状态**：待审核  
> **影响范围**：`internal/search/search.go`（主要改动）、`internal/search/search_test.go`（新增测试）

---

## 一、背景与动机

### 1.1 当前实现问题

当前 `buildRRFQuery` 使用 ES Platinum 的 `retriever.rrf` API：

```go
"retriever": map[string]any{
    "rrf": map[string]any{
        "retrievers": []any{standardRetriever, knnRetriever},
        "rank_constant": 60,
        "rank_window_size": rankWindowSize,
    },
}
```

**问题**：`retriever.rrf` 是 Elasticsearch Platinum License 的付费功能，Basic License 无法使用。

### 1.2 目标

- ✅ 完全兼容现有 API 接口（SearchRequest / SearchResponse 不变）
- ✅ 搜索结果质量与内置 RRF 等价（相同算法数学公式）
- ✅ 性能可接受（两次并行查询 vs 单次内置融合）
- ✅ 保持现有过滤、分页、高亮、Email 访问控制等功能
- ✅ 优雅降级（embedding 失败时回退纯 BM25）

---

## 二、架构设计

### 2.1 整体流程

```
SearchService.Search()
    ├─ 检查是否需要 RRF（req.Query != "" && cfg.Embedder != nil）
    │
    ├─ 生成查询向量（getEmbedding）
    │   ├─ 缓存命中 → 直接使用
    │   └─ 缓存未命中 → Embedder.Embed() → 存入缓存
    │
    ├─ 并行执行两个 ES 查询（使用 msearch API）
    │   ├─ BM25 Query：multi_match + filters
    │   └─ kNN Query：dense_vector cosine similarity + filters
    │
    ├─ 应用层 RRF 融合（reciprocalRankFusion）
    │   ├─ 提取两份结果的 _id + score
    │   ├─ 去重（_id 为 key）
    │   ├─ 计算 RRF 分数
    │   └─ 按分数降序排序
    │
    └─ 分页截取 + 构建响应
        ├─ 取 [from, from+pageSize)
        ├─ 回填完整文档（从原始查询结果中查找）
        └─ 返回 SearchResponse
```

### 2.2 组件划分

| 组件 | 职责 | 输入 | 输出 |
|------|------|------|------|
| `buildHybridQuery` | 构建 BM25 + kNN msearch 请求体 | SearchRequest, queryVector | []byte (msearch body) |
| `executeHybridSearch` | 执行 msearch 并解析结果 | msearch body, indexName | bm25Hits, knnHits |
| `reciprocalRankFusion` | RRF 融合算法 | bm25Hits, knnHits, k, rankConstant | []scoredDoc (融合后的候选列表) |
| `paginateResults` | 分页截取 + 文档回填 | scoredDocs, from, pageSize | []PhotoDocument |

### 2.3 关键数据结构

```go
// scoredDoc 表示一个带分数的候选文档
type scoredDoc struct {
    id       string  // 文档 _id
    score    float64 // RRF 累积分数
    source   types.PhotoDocument // 完整文档（从任一查询结果中取出）
    highlight map[string][]string // 高亮结果（从 BM25 结果中取出）
}

// msearchResponse 对应 _msearch API 的响应格式
type msearchResponse struct {
    Responses []struct {
        Hits struct {
            Total struct {
                Value    int64  `json:"value"`
                Relation string `json:"relation"`
            } `json:"total"`
            Hits []struct {
                ID         string                   `json:"_id"`
                Score      float64                  `json:"_score"`
                Source     types.PhotoDocument      `json:"_source"`
                Highlight  map[string][]string      `json:"highlight"`
            } `json:"hits"`
        } `json:"hits"`
        Error map[string]any `json:"error"` // 子查询错误（如果发生）
    } `json:"responses"`
}
```

---

## 三、详细设计

### 3.1 msearch 请求构建

**目标**：将两个独立查询合并为一次 HTTP 请求到 `/_msearch` 端点。

```go
func (s *SearchService) buildHybridQuery(req *types.SearchRequest, queryVector []float32, userEmail string) ([]byte, error) {
    rankWindowSize := s.calculateRankWindowSize(req)
    filters := s.buildFilters(req, userEmail)
    
    // 构建 BM25 查询（取第一个 rankWindowSize 条）
    bm25Query := s.buildBM25SubQuery(req, filters, rankWindowSize)
    
    // 构建 kNN 查询（取 rankWindowSize 条，k = rankWindowSize）
    knnQuery := s.buildKNNSubQuery(queryVector, filters, rankWindowSize)
    
    // 合并为 msearch 格式：每行一个查询，用换行分隔
    var body bytes.Buffer
    
    // BM25 部分：header + body
    body.WriteString(`{"index":"` + req.IndexName + `"}` + "\n")
    bm25Body, _ := json.Marshal(bm25Query)
    body.Write(bm25Body)
    body.WriteString("\n")
    
    // kNN 部分：header + body
    body.WriteString(`{"index":"` + req.IndexName + `"}` + "\n")
    knnBody, _ := json.Marshal(knnQuery)
    body.Write(knnBody)
    body.WriteString("\n")
    
    return body.Bytes(), nil
}
```

**关键参数**：
- BM25 查询 `size: rankWindowSize`
- kNN 查询 `k: rankWindowSize, num_candidates: rankWindowSize * 10`（kNN 需要更多候选才能找到最优）

### 3.2 BM25 子查询结构

```json
{
  "size": <rankWindowSize>,
  "query": {
    "bool": {
      "must": [
        {
          "multi_match": {
            "query": "<user_query>",
            "fields": ["description", "tags", "objects", "text", "address", "formatted_address"]
          }
        }
      ],
      "filter": [<email_filter>, <date_range>, <tags>, <objects>, <scene_type>, <camera_model>]
    }
  },
  "highlight": {
    "fields": {
      "description": {}
    }
  },
  "_source": { "excludes": ["embedding", "embedding_version", "embedded_at"] }
}
```

**与当前 `buildQuery` 的区别**：
- 移除 `sort` 参数（RRF 融合后自己排序）
- 移除 `from/size` 分页逻辑（融合后截取）
- 添加 `_source.excludes` 排除 embedding 相关字段（减少网络传输）

### 3.3 kNN 子查询结构

```json
{
  "size": <rankWindowSize>,
  "query": {
    "script_score": {
      "query": {
        "bool": {
          "filter": [<email_filter>, <date_range>, <tags>, <objects>, <scene_type>, <camera_model>]
        }
      },
      "script": {
        "source": "cosineSimilarity(params.query_vector, 'embedding') + 1.0",
        "params": {
          "query_vector": [<vector>]
        }
      }
    }
  },
  "_source": { "excludes": ["embedding", "embedding_version", "embedded_at"] }
}
```

**注意**：这里使用 `script_score` 而非 `knn` 查询，原因：
- `knn` 查询不支持在 `bool` 的 `filter` 子句中（它有自己的 filter 参数）
- `script_score` 可以完美复用现有的 filter 构造逻辑
- 性能对比：`knn` 略快（近似最近邻），但 `script_score` 对于 rankWindowSize ≤ 1000 的场景完全足够

**替代方案（更优性能）**：如果未来需要 kNN 查询的原生性能，可以改为：
```json
{
  "size": <rankWindowSize>,
  "knn": {
    "field": "embedding",
    "query_vector": [<vector>],
    "k": <rankWindowSize>,
    "num_candidates": <rankWindowSize * 10>,
    "filter": {
      "bool": {
        "filter": [<all_filters>]
      }
    }
  }
}
```

### 3.4 RRF 融合算法

```go
// reciprocalRankFusion 执行应用层 RRF 融合算法
// 公式：score(d) = Σ_i 1/(k + rank_i(d))
// 其中 k 是常数（推荐 60），rank 从 1 开始（非 0）
func (s *SearchService) reciprocalRankFusion(
    bm25Hits []msearchHit,
    knnHits []msearchHit,
    rankConstant int,
) []scoredDoc {
    scores := make(map[string]float64)
    sources := make(map[string]types.PhotoDocument)
    highlights := make(map[string]map[string][]string)
    
    // 处理 BM25 结果：rank 从 1 开始递增
    for i, hit := range bm25Hits {
        rank := i + 1
        scores[hit.ID] += 1.0 / float64(rankConstant + rank)
        sources[hit.ID] = hit.Source
        if len(hit.Highlight) > 0 {
            highlights[hit.ID] = hit.Highlight
        }
    }
    
    // 处理 kNN 结果：rank 从 1 开始递增
    for i, hit := range knnHits {
        rank := i + 1
        scores[hit.ID] += 1.0 / float64(rankConstant + rank)
        sources[hit.ID] = hit.Source  // 覆盖 source（理论上两边一样，但用 kNN 的作为后备）
        if len(hit.Highlight) > 0 {
            highlights[hit.ID] = hit.Highlight
        }
    }
    
    // 转换为列表并排序（按 RRF 分数降序）
    result := make([]scoredDoc, 0, len(scores))
    for id, score := range scores {
        result = append(result, scoredDoc{
            id:        id,
            score:     score,
            source:    sources[id],
            highlight: highlights[id],
        })
    }
    
    sort.Slice(result, func(i, j int) bool {
        if result[i].score == result[j].score {
            // 分数相同时按 ID 字典序保证稳定性
            return result[i].id < result[j].id
        }
        return result[i].score > result[j].score
    })
    
    return result
}
```

**关键设计决策**：
- **rank 从 1 开始**（非 0）：RRF 原论文 (Cormack et al., 2009) 推荐，与 ES 内置一致
- **rankConstant = 60**：原论文推荐值，平衡了 top 结果之间的分数差距
- **分数相同时的排序**：按 document ID 字典序，保证结果稳定可重复

### 3.5 分页截取

```go
// paginateResults 从融合后的列表中截取当前页的结果
func (s *SearchService) paginateResults(
    scoredDocs []scoredDoc,
    from int,
    pageSize int,
) []types.PhotoDocument {
    
    // 边界：from 超过总数
    if from >= len(scoredDocs) {
        return []types.PhotoDocument{}
    }
    
    // 边界：from + pageSize 超过总数
    end := from + pageSize
    if end > len(scoredDocs) {
        end = len(scoredDocs)
    }
    
    page := scoredDocs[from:end]
    
    // 回填文档并清理高亮（如果 kNN 高亮了其他字段）
    result := make([]types.PhotoDocument, len(page))
    for i, doc := range page {
        d := doc.source
        d.Highlight = doc.highlight
        result[i] = d
    }
    
    return result
}
```

**total 计算**：
```go
// total = max(bm25Total, knnTotal)
// 原因：RRF 是并集（任一匹配即为有效），理论上 ≥ max
// 但 RRF rank_window_size 限制了候选范围，取 max 更稳定
total := maxInt64(bm25Response.Hits.Total.Value, knnResponse.Hits.Total.Value)
```

### 3.6 rankWindowSize 自动计算

```go
// calculateRankWindowSize 确定子查询应取多少条结果
// 公式：rankWindowSize = from + pageSize
// 确保融合窗口足以覆盖当前页
func (s *SearchService) calculateRankWindowSize(req *types.SearchRequest) int {
    page := req.Page
    if page <= 0 {
        page = 1
    }
    pageSize := req.PageSize
    if pageSize <= 0 {
        pageSize = 20
    }
    from := (page - 1) * pageSize
    
    // rankWindowSize 必须 >= from + pageSize
    rankWindowSize := from + pageSize
    
    // 上限保护：避免深分页时过大的查询
    if rankWindowSize > 1000 {
        rankWindowSize = 1000
    }
    
    // 下限保护：即使第一页也至少取合理数量的候选
    if rankWindowSize < 50 {
        rankWindowSize = 50
    }
    
    return rankWindowSize
}
```

**参数约束**：
- **下限 50**：即使 page=1, pageSize=20，也取 50 条候选（保证融合质量）
- **上限 1000**：防止 page=50, pageSize=100 时取 5000 条（ES from+size 默认上限也是 10000）

---

## 四、边界情况处理

### 4.1 查询失败降级

#### 情况 A：BM25 查询失败，kNN 成功

```go
func (s *SearchService) executeHybridSearch(ctx context.Context, msearchBody []byte, indexName string) ([]msearchHit, []msearchHit, error) {
    resp, err := s.client.Client().Msearch(bytes.NewReader(msearchBody), s.client.Client().Msearch.WithIndex(indexName))
    // 发送 msearch
    
    if resp.IsError() {
        // 整个 msearch 失败（网络错误等）→ 返回错误
        return nil, nil, fmt.Errorf("msearch failed: %s", resp.Status())
    }
    
    var mResp msearchResponse
    json.NewDecoder(resp.Body).Decode(&mResp)
    
    // 检查子查询错误
    if len(mResp.Responses) < 2 {
        return nil, nil, fmt.Errorf("msearch expected 2 responses, got %d", len(mResp.Responses))
    }
    
    bm25Resp := mResp.Responses[0]
    knnResp := mResp.Responses[1]
    
    // BM25 失败，kNN 成功 → 降级为纯 kNN
    if bm25Resp.Error != nil {
        slog.Warn("BM25 sub-query failed, falling back to kNN only", 
            "error", bm25Resp.Error)
        return []msearchHit{}, knnResp.Hits.Hits, nil
    }
    
    // kNN 失败，BM25 成功 → 降级为纯 BM25
    if knnResp.Error != nil {
        slog.Warn("kNN sub-query failed, falling back to BM25 only", 
            "error", knnResp.Error)
        return bm25Resp.Hits.Hits, []msearchHit{}, nil
    }
    
    // 都成功 → 正常融合
    return bm25Resp.Hits.Hits, knnResp.Hits.Hits, nil
}
```

#### 情况 B：Embedding 生成失败

保持现有逻辑，在 `Search()` 方法开头检查：

```go
if req.Query != "" && s.embedder != nil {
    queryVec, ok := s.getEmbedding(ctx, req.Query)
    if !ok {
        // embedding 失败 → 降级为纯 BM25（现有 buildQuery）
        return s.searchFallbackToBM25(ctx, indexName, req, userEmail)
    }
    return s.searchHybrid(ctx, indexName, req, userEmail, queryVec)
}
return s.searchFallbackToBM25(ctx, indexName, req, userEmail)
```

### 4.2 空结果处理

| 场景 | BM25 | kNN | 处理方式 |
|------|------|-----|----------|
| A | 0 hits | 0 hits | 返回空结果，total=0 |
| B | 0 hits | >0 hits | RRF 只使用 kNN 结果（rank 从 1 开始） |
| C | >0 hits | 0 hits | RRF 只使用 BM25 结果（rank 从 1 开始） |
| D | >0 hits | >0 hits | 正常 RRF 融合 |

```go
func (s *SearchService) reciprocalRankFusion(bm25Hits, knnHits []msearchHit, rankConstant int) []scoredDoc {
    // 空结果时自然处理：循环不执行，scores 为空，返回空列表
    
    if len(bm25Hits) == 0 && len(knnHits) == 0 {
        return []scoredDoc{}
    }
    
    // 只有一方有结果时，另一方贡献分数=0
    // 逻辑不变，自然处理
    // ...
}
```

### 4.3 去重逻辑

**RRF 天然去重**：算法使用 `_id` 作为 map 的 key，同一文档被多次命中时分数相加，不会产生重复结果。

```go
scores := make(map[string]float64)  // _id → 累积分数

// BM25 第一轮命中：scores["abc123"] = 1/61
// kNN 第二轮命中同一文档：scores["abc123"] += 1/61 → 2/61（自然去重）
```

**source 文档的处理**：
```go
sources := make(map[string]types.PhotoDocument)
highlights := make(map[string]map[string][]string)

// 两个查询都会返回 source，理论上内容相同
// 使用 BM25 的 source（优先，因为它可能包含高亮）
for _, hit := range bm25Hits {
    sources[hit.ID] = hit.Source
    highlights[hit.ID] = hit.Highlight
}

// kNN 的 source 仅在 BM25 未命中时使用（作为后备）
for _, hit := range knnHits {
    if _, exists := sources[hit.ID]; !exists {
        sources[hit.ID] = hit.Source
    }
    // kNN 通常没有高亮，所以不覆盖 highlights
}
```

### 4.4 高亮处理

- **BM25 高亮**：完整保留，作为 RRF 结果的 highlight 字段
- **kNN 高亮**：kNN 通常不使用高亮（cosine similarity 无匹配词），如果有高亮则合并

```go
// 如果 kNN 也返回了高亮（理论上少见），合并到现有高亮
if len(hit.Highlight) > 0 {
    if existing, ok := highlights[hit.ID]; ok {
        // 合并两个查询的高亮结果
        for field, snippets := range hit.Highlight {
            if _, exists := existing[field]; !exists {
                existing[field] = snippets
            }
        }
    } else {
        highlights[hit.ID] = hit.Highlight
    }
}
```

### 4.5 分数相同时的排序稳定性

RRF 可能出现分数相同的情况（例如两个文档分别在 BM25 rank 3 和 kNN rank 37，另一个在 rank 4 和 rank 36，分数可能相同）。

**解决**：排序时按 ID 字典序作为 tiebreaker：

```go
sort.Slice(result, func(i, j int) bool {
    if result[i].score == result[j].score {
        return result[i].id < result[j].id  // 字典序，保证稳定
    }
    return result[i].score > result[j].score
})
```

**理由**：ES 内置 RRF 也使用 ID 作为 tiebreaker（基于 _doc 的底层顺序），这里使用字典序（字符串序）保证结果可重复。

### 4.6 Email 访问控制

两个子查询必须使用完全相同的 filter 集合，包括 email 过滤：

```go
filters := s.buildFilters(req, userEmail)  // 包含 email filter

// BM25 查询使用这些 filter
bm25Query["query"]["bool"]["filter"] = filters

// kNN 查询也使用这些 filter（通过 script_score 的 bool.filter）
knnQuery["query"]["script_score"]["query"]["bool"]["filter"] = filters
```

### 4.7 分页边界

#### 情况 A：from >= rankWindowSize

```go
if from >= len(scoredDocs) {
    return []types.PhotoDocument{}  // 空页，返回空
}
```

**示例**：page=10, pageSize=20 → from=180，但 rankWindowSize=200，正常处理

#### 情况 B：from + pageSize > rankWindowSize

```go
end := from + pageSize
if end > len(scoredDocs) {
    end = len(scoredDocs)
}
// 可能返回少于 pageSize 条结果（正常现象，最后一页）
```

**示例**：page=5, pageSize=20 → from=80, rankWindowSize=100，可能只有 95 个候选 → 返回 15 条（最后一页正常）

### 4.8 过滤条件一致性

确保两个查询的过滤条件完全一致，避免结果偏斜：

```go
filters := s.buildFilters(req, userEmail)
// filters 包含：email filter, dateRange, tags, objects, sceneType, cameraModel

// BM25 使用 filters
bm25Query["query"]["bool"]["filter"] = filters

// kNN 使用 filters
knnQuery["query"]["script_score"]["query"]["bool"]["filter"] = filters
```

### 4.9 向量查询的 num_candidates 计算

对于 kNN 查询，`num_candidates` 应显著大于 `k`（保证搜索充分）：

```go
numCandidates := rankWindowSize * 10  // 10倍扩展候选范围
if numCandidates > 10000 {
    numCandidates = 10000  // ES 默认上限
}
```

**对于 script_score**：不需要 num_candidates（它扫描所有匹配 filter 的文档），但如果未来改用 `knn` 查询则需要。

---

## 五、性能考虑

### 5.1 网络开销

| 方案 | 网络请求数 | 每次请求体大小 |
|------|-----------|--------------|
| 当前内置 RRF | 1 | ~2KB |
| 应用层 RRF (msearch) | 1 | ~4KB（两个查询合并在一次请求） |
| 应用层 RRF (两次独立请求) | 2 | ~2KB × 2 |

**选择**：使用 `_msearch` API，单次请求并行执行两个子查询，网络开销与内置 RRF 几乎相同。

### 5.2 ES 服务端开销

| 方案 | BM25 查询数 | kNN 查询数 | 融合计算 |
|------|------------|-----------|---------|
| 当前内置 RRF | 1 | 1 | ES 内部 |
| 应用层 RRF | 1 | 1 | Go 应用侧 |

**差异**：
- ES 内部融合比应用侧融合快几十微秒（可忽略）
- 应用侧融合需要传输 ~rankWindowSize 个文档 ID（~几 KB 额外传输）
- **总体性能差异 <10%**，可忽略

### 5.3 应用侧复杂度

```
O(rankWindowSize) 去重 + O(rankWindowSize log rankWindowSize) 排序
```

对于 rankWindowSize ≤ 1000，排序耗时 <1ms（可忽略）。

### 5.4 Embedding 缓存

当前已有的 `embeddingCache` 逻辑保持不变：

```go
queryVec, ok := s.getEmbedding(ctx, req.Query)
// 缓存命中直接使用，避免重复向量化
// 缓存未命中调用 Embedder.Embed()，然后存入缓存
```

---

## 六、测试策略

### 6.1 单元测试

#### 测试 1：RRF 融合算法正确性

```go
func TestReciprocalRankFusion(t *testing.T) {
    // 场景 A：两份非空结果，有交集
    bm25 := []msearchHit{
        {ID: "a", Score: 10.5, Source: ...},
        {ID: "b", Score: 9.2, Source: ...},
        {ID: "c", Score: 8.1, Source: ...},
    }
    knn := []msearchHit{
        {ID: "d", Score: 0.95, Source: ...},
        {ID: "a", Score: 0.92, Source: ...},  // 与 BM25 交集
        {ID: "e", Score: 0.88, Source: ...},
    }
    
    result := s.reciprocalRankFusion(bm25, knn, 60)
    
    // 验证：a 的分数应该最高（两次命中相加）
    // a: 1/(60+1) + 1/(60+2) = 1/61 + 1/62 ≈ 0.0325
    // b: 1/(60+2) ≈ 0.0161（仅 BM25）
    // d: 1/(60+1) ≈ 0.0161（仅 kNN）
    assert.Equal(t, "a", result[0].id)  // a 应该排第一
    assert.InDelta(t, 0.0325, result[0].score, 0.0001)
    assert.Len(t, result, 5)  // 去重后 5 个文档
}
```

#### 测试 2：空结果处理

```go
func TestReciprocalRankFusion_EmptyResults(t *testing.T) {
    // 场景 B：BM25 空，kNN 有结果
    bm25 := []msearchHit{}
    knn := []msearchHit{{ID: "x", Score: 0.95}}
    
    result := s.reciprocalRankFusion(bm25, knn, 60)
    assert.Len(t, result, 1)
    assert.Equal(t, "x", result[0].id)
    assert.InDelta(t, 1.0/61.0, result[0].score, 0.0001)
    
    // 场景 C：kNN 空，BM25 有结果
    bm25 = []msearchHit{{ID: "y", Score: 8.5}}
    knn = []msearchHit{}
    
    result = s.reciprocalRankFusion(bm25, knn, 60)
    assert.Len(t, result, 1)
    assert.Equal(t, "y", result[0].id)
    
    // 场景 D：都空
    result = s.reciprocalRankFusion([]msearchHit{}, []msearchHit{}, 60)
    assert.Len(t, result, 0)
}
```

#### 测试 3：分页边界

```go
func TestPaginateResults_Boundary(t *testing.T) {
    docs := make([]scoredDoc, 100)  // 模拟 100 个融合结果
    for i := range docs {
        docs[i] = scoredDoc{id: fmt.Sprintf("doc_%d", i), score: float64(100-i)}
    }
    
    // 正常分页（第 1 页）
    result := s.paginateResults(docs, 0, 20)
    assert.Len(t, result, 20)
    assert.Equal(t, "doc_0", result[0].id)
    
    // 中间页
    result = s.paginateResults(docs, 40, 20)
    assert.Len(t, result, 20)
    assert.Equal(t, "doc_40", result[0].id)
    
    // 最后一页（不足 pageSize）
    result = s.paginateResults(docs, 85, 20)
    assert.Len(t, result, 15)  // 只有 15 个
    
    // from 超过总数（超出范围）
    result = s.paginateResults(docs, 100, 20)
    assert.Len(t, result, 0)
}
```

#### 测试 4：rankWindowSize 计算

```go
func TestCalculateRankWindowSize(t *testing.T) {
    cases := []struct {
        page, pageSize int
        expected       int
    }{
        {1, 20, 50},      // 下限保护
        {3, 20, 60},      // 正常计算
        {10, 50, 500},    // 较大分页
        {50, 100, 1000},  // 上限保护
        {0, 20, 50},      // 无效页码修正
    }
    
    for _, tc := range cases {
        req := &types.SearchRequest{Page: tc.page, PageSize: tc.pageSize}
        result := s.calculateRankWindowSize(req)
        assert.Equal(t, tc.expected, result, 
            "page=%d, pageSize=%d", tc.page, tc.pageSize)
    }
}
```

#### 测试 5：高亮处理

```go
func TestHighlightMerging(t *testing.T) {
    // BM25 有高亮，kNN 无高亮
    bm25 := []msearchHit{{
        ID: "a", 
        Source: ...,
        Highlight: map[string][]string{
            "description": {"这是<em>测试</em>描述"},
        },
    }}
    knn := []msearchHit{{ID: "a", Source: ...}}  // 无高亮
    
    result := s.reciprocalRankFusion(bm25, knn, 60)
    assert.Len(t, result, 1)
    assert.Contains(t, result[0].highlight, "description")
    assert.Equal(t, []string{"这是<em>测试</em>描述"}, result[0].highlight["description"])
}
```

### 6.2 集成测试

**目标**：验证完整流程，包括真实 ES 交互。

```go
func TestHybridSearch_Integration(t *testing.T) {
    // 使用 testcontainers-go 启动 ES
    // 准备测试文档（带有 embedding 字段）
    // 执行混合搜索，验证：
    //   1. RRF 融合正确（结果顺序合理）
    //   2. 分页正确（第 1 页、深分页）
    //   3. 过滤条件生效
    //   4. 高亮保留
    //   5. Email 访问控制
}
```

### 6.3 现有测试的影响

**不需要修改的测试**：
- `TestSearch`（不依赖 embedding 的纯 BM25 搜索）
- `TestGetFilters`（聚合查询，不受影响）
- `TestGetStats`（统计查询，不受影响）

**需要审查的测试**：
- 现有的 RRF 相关测试（如果有） → 需要重写或移除

**新增的测试**：
- `TestReciprocalRankFusion`（算法正确性）
- `TestHybridSearchQueryBuilding`（msearch 请求构建）
- `TestHybridSearchFallback`（降级逻辑）
- `TestHybridSearchIntegration`（端到端）

---

## 七、接口兼容性

### 7.1 不变的部分

| 组件 | 状态 |
|------|------|
| `SearchRequest`（types/types.go） | ✅ 不变 |
| `SearchResponse`（types/types.go） | ✅ 不变 |
| `Searcher` 接口（search/search.go） | ✅ 不变 |
| `Embedder` 接口（search/search.go） | ✅ 不变 |
| `EmbeddingCache` 接口（search/search.go） | ✅ 不变 |
| `HybridConfig`（search/search.go） | ✅ 不变 |
| `WithEmbedder` 选项（search/search.go） | ✅ 不变 |

### 7.2 移除的部分

- `buildRRFQuery` 方法（重构为 `buildHybridQuery` + `buildBM25SubQuery` + `buildKNNSubQuery`）

### 7.3 新增的部分

- `buildHybridQuery`（构建 msearch 请求）
- `buildBM25SubQuery`（BM25 子查询构造）
- `buildKNNSubQuery`（kNN 子查询构造）
- `executeHybridSearch`（执行 msearch）
- `reciprocalRankFusion`（RRF 融合）
- `paginateResults`（分页截取）
- `searchHybrid`（完整混合搜索流程）
- `searchFallbackToBM25`（降级流程）

---

## 八、错误处理

### 8.1 错误分类

| 错误类型 | 处理方式 |
|----------|---------|
| Embedding 生成失败 | 降级为纯 BM25（现有逻辑） |
| msearch 整请求失败 | 返回 error（网络问题等） |
| BM25 子查询失败 | 降级为纯 kNN（警告日志） |
| kNN 子查询失败 | 降级为纯 BM25（警告日志） |
| 两子查询都失败 | 返回 error |
| 解析响应失败 | 返回 error |

### 8.2 错误日志

```go
slog.Warn("BM25 sub-query failed, falling back to kNN only", 
    "error", bm25Resp.Error,
    "index", indexName)

slog.Debug("hybrid search completed", 
    "bm25_hits", len(bm25Hits),
    "knn_hits", len(knnHits),
    "fused_candidates", len(scoredDocs),
    "page", req.Page,
    "page_size", req.PageSize)
```

### 8.3 用户可见的错误消息

保持与当前一致的错误格式，使用 `internal/errors` 包：

```go
// 如果两个子查询都失败
return nil, errors.New(errors.ErrTypeServerError, "search failed: %v", err)

// 如果 embedding 生成失败（自动降级，用户看不到）
slog.Warn("embedding generation failed, falling back to BM25", ...)
```

---

## 九、代码组织

### 9.1 文件结构

```
internal/search/
├── search.go              # 现有文件，主要改动
│   ├── Search()           # 保持不变（入口）
│   ├── searchHybrid()     # 新增：完整混合搜索流程
│   ├── searchFallbackToBM25()  # 新增：降级流程
│   ├── buildHybridQuery() # 新增：构建 msearch 请求
│   ├── buildBM25SubQuery() # 新增：BM25 子查询
│   ├── buildKNNSubQuery() # 新增：kNN 子查询
│   ├── executeHybridSearch() # 新增：执行 msearch
│   ├── reciprocalRankFusion() # 新增：RRF 融合
│   ├── paginateResults()  # 新增：分页截取
│   └── calculateRankWindowSize() # 新增：rank window 计算
├── search_test.go         # 现有测试，新增测试用例
└── hybrid_rrf_test.go     # 新增：RRF 相关专项测试文件
```

### 9.2 函数调用图

```
Search()
├── [if embedding failed] searchFallbackToBM25()
│   └── buildQuery() + ES search (现有逻辑)
│
└── [hybrid path] searchHybrid()
    ├── buildHybridQuery()
    │   ├── buildBM25SubQuery()
    │   │   └── buildFilters() (复用现有)
    │   └── buildKNNSubQuery()
    │       └── buildFilters() (复用现有)
    ├── executeHybridSearch() → _msearch
    ├── reciprocalRankFusion()
    └── paginateResults()
        └── buildSearchResponse() (复用现有 parseSearchResponse 逻辑)
```

---

## 十、配置管理

### 10.1 现有配置

保持不变（`HybridConfig`）：

```go
type HybridConfig struct {
    RRFWindowSize    int    // rank_constant（默认 60，原论文推荐）
    RRFRankConstant  int    // rank_constant（默认 60）
    KNNK             int    // kNN 的 k 参数（保留，未来优化用）
    KNNNumCandidates int    // kNN 的 num_candidates（保留，未来优化用）
}
```

### 10.2 配置验证

```go
// 在 app/run.go 中初始化 HybridConfig 时
if cfg.Embedding.RRFRankConstant <= 0 {
    cfg.Embedding.RRFRankConstant = 60
}
if cfg.Embedding.RRFWindowSize <= 0 {
    cfg.Embedding.RRFWindowSize = 100  // 默认窗口大小
}
```

---

## 十一、监控与可观测性

### 11.1 日志记录

```go
slog.Debug("hybrid search started",
    "index", indexName,
    "query", truncateJSON(bodyBytes, 500),
    "page", req.Page,
    "page_size", req.PageSize)

slog.Debug("hybrid search completed",
    "bm25_hits", len(bm25Hits),
    "knn_hits", len(knnHits),
    "fused_candidates", len(scoredDocs),
    "page", req.Page,
    "page_size", req.PageSize)

slog.Warn("BM25 sub-query failed, falling back to kNN only", ...)
slog.Warn("kNN sub-query failed, falling back to BM25 only", ...)
```

### 11.2 性能指标（可选，未来）

如果需要监控 RRF 性能，可以记录时间：

```go
startTime := time.Now()
bm25Hits, knnHits, err := s.executeHybridSearch(ctx, msearchBody, indexName)
executeDuration := time.Since(startTime)

fusionStart := time.Now()
scoredDocs := s.reciprocalRankFusion(bm25Hits, knnHits, rankConstant)
fusionDuration := time.Since(fusionStart)

slog.Debug("hybrid search performance",
    "execute_duration_ms", executeDuration.Milliseconds(),
    "fusion_duration_us", fusionDuration.Microseconds())
```

---

## 十二、风险与缓解

| 风险 | 原因 | 缓解措施 |
|------|------|---------|
| 性能下降 | 两次 ES 查询 vs 单次内置融合 | 使用 `_msearch` 单次请求，实际开销 <10% |
| 结果质量差异 | 应用侧融合可能与内置 RRF 实现细节不同 | RRF 公式与原论文一致，使用 rank=1 递增，与 ES 行为一致 |
| 深分页性能 | rankWindowSize = from+pageSize，深分页时较大 | 上限保护：rankWindowSize ≤ 1000 |
| 子查询错误传播 | 一个子查询失败可能影响结果 | 优雅降级：BM25 失败→kNN，kNN 失败→BM25 |
| source 字段不一致 | 两个查询返回的 source 可能略有差异 | 优先使用 BM25 的 source（包含高亮），kNN 作为后备 |

---

## 十三、实现顺序

### 阶段 1：核心 RRF 实现（约 80 行代码）

1. 新增 `reciprocalRankFusion()` 函数
2. 新增 `buildHybridQuery()` 构建 msearch 请求
3. 新增 `executeHybridSearch()` 执行 msearch
4. 新增 `searchHybrid()` 完整流程
5. 修改 `Search()` 调用 `searchHybrid()`

### 阶段 2：边界处理（约 40 行代码）

1. 实现分页截取 `paginateResults()`
2. 实现 rankWindowSize 计算 `calculateRankWindowSize()`
3. 实现降级逻辑 `searchFallbackToBM25()`
4. 处理高亮合并

### 阶段 3：测试与验证（约 100 行测试代码）

1. 编写 RRF 算法单元测试
2. 编写空结果处理测试
3. 编写分页边界测试
4. 编写集成测试（与现有测试框架配合）
5. 运行全部测试，确保无回归

### 阶段 4：清理与优化

1. 移除原有的 `buildRRFQuery()` 方法
2. 更新相关注释
3. 添加性能监控日志
4. 代码审查

---

## 十四、验收标准

- [ ] 所有现有测试通过（无回归）
- [ ] 新增 RRF 算法单元测试通过（边界情况覆盖）
- [ ] 混合搜索功能正常（手动测试）
- [ ] 降级功能正常（embedding 失败时回退 BM25）
- [ ] 分页正确（第 1 页、中间页、最后一页）
- [ ] 高亮保留（BM25 高亮正常显示）
- [ ] Email 访问控制正常（私有目录过滤）
- [ ] 过滤条件生效（标签、日期、场景等）
- [ ] 性能可接受（与内置 RRF 差距 <10%）

---

## 十五、未来优化（不在本次范围内）

> 以下优化可在验证基础功能后考虑

1. **改用原生 kNN 查询**：将 `script_score` 替换为 `knn` 查询，提升语义搜索性能
2. **缓存 kNN 结果**：缓存频繁查询的 kNN 结果，减少 ES 查询次数
3. **可配置权重**：为 BM25 和 kNN 提供独立的权重参数（非 RRF，而是线性融合）
4. **结果多样性**：引入 MMR（Maximal Marginal Relevance）避免结果过于相似
5. **并行 msearch**：将 msearch 拆分为两个独立请求并行发送（某些场景下更快）

---

## 十六、参考资源

- **RRF 原论文**：Cormack, Clarke, & Butt (2009) - [Reciprocal Rank Fusion](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf)
- **ES 内置 RRF 文档**（付费功能）：https://www.elastic.co/guide/en/elasticsearch/reference/current/rrf.html
- **ES _msearch API**：https://www.elastic.co/guide/en/elasticsearch/reference/current/search-multi-search.html
- **ES script_score 查询**：https://www.elastic.co/guide/en/elasticsearch/reference/current/query-dsl-script-score-query.html

---

## 十七、审核清单

请审核以下关键设计决策：

- [ ] **方案选择**：应用层 RRF（而非线性加权融合）
- [ ] **msearch vs 两次独立请求**：选择 msearch 合并为一次 HTTP 请求
- [ ] **rank 从 1 开始**：与原论文和 ES 内置行为一致
- [ ] **rankConstant = 60**：原论文推荐值
- [ ] **rankWindowSize 上限 1000**：与 ES from+size 默认上限一致
- [ ] **去重策略**：使用 `_id` 作为唯一键
- [ ] **source 优先级**：BM25 优先（含高亮），kNN 作为后备
- [ ] **降级策略**：embedding 失败→BM25，BM25 失败→kNN，kNN 失败→BM25
- [ ] **分数 tiebreaker**：按 document ID 字典序
- [ ] **total 计算**：max(bm25Total, knnTotal)
