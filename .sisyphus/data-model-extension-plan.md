# Phosche 数据模型扩展实现计划

## 概述
扩展 Phosche 数据模型，增加 `text` 字段（图片文字提取），扩展 `scene_type` 枚举值（3种→7种），并优化搜索排序规则。

## 需求详情

### 1. 增加 text 字段
- **目的**：让 LLM 提取图片中的文字内容
- **影响范围**：
  - LLM 输出结构
  - ES Mapping
  - Go 类型定义
  - 搜索逻辑（全文搜索字段）

### 2. 扩展 scene_type 枚举
- **当前**：`indoor`, `outdoor`, `unknown`（3种）
- **目标**：`indoor`, `outdoor`, `underwater`, `aerial`, `studio`, `night`, `unknown`（7种）
- **影响范围**：
  - LLM Prompt
  - 前端筛选选项

### 3. 优化搜索排序
- **当前**：固定排序（date_time_original desc, mtime desc）
- **目标**：有搜索关键词时，优先按相关性评分排序
- **影响范围**：
  - 搜索查询构建逻辑

---

## 实现步骤

### Phase 1: 后端类型定义（优先级：高）

#### Task 1.1: 更新 AnalysisResult 结构体
**文件**：`internal/types/types.go`

**修改内容**：
```go
type AnalysisResult struct {
    Description string   `json:"description" es:"text"`
    Tags        []string `json:"tags" es:"text"`
    Objects     []string `json:"objects" es:"text"`
    SceneType   string   `json:"scene_type" es:"keyword"`
    Colors      []string `json:"colors" es:"keyword"`
    PeopleCount int      `json:"people_count" es:"integer"`
    HasText     bool     `json:"has_text" es:"boolean"`
    Text        string   `json:"text" es:"text"`  // 新增
    Confidence  float64  `json:"confidence,omitempty" es:"double"`
}
```

**验证**：
- 编译通过
- 现有测试通过

---

### Phase 2: ES Mapping 更新（优先级：高）

#### Task 2.1: 更新索引映射
**文件**：`internal/indexer/mapping.go`

**修改内容**：
在 `properties` 中增加：
```go
"text": map[string]any{
    "type": "text",
    "analyzer": "standard",
},
```

**验证**：
- 删除旧索引
- 启动服务，检查新索引创建成功
- 使用 `GET /phosche/_mapping` 验证 `text` 字段存在

---

### Phase 3: 搜索逻辑优化（优先级：高）

#### Task 3.1: 扩展全文搜索字段
**文件**：`internal/search/search.go`

**修改内容**：
在 `buildQuery` 函数的 `multi_match` 中增加 `text` 字段：
```go
"multi_match": map[string]any{
    "query":  req.Query,
    "fields": []string{"description", "tags", "objects", "text"},  // 增加 "text"
},
```

#### Task 3.2: 实现相关性排序
**文件**：`internal/search/search.go`

**修改内容**：
在 `buildQuery` 函数中，根据是否有搜索关键词动态调整排序：
```go
sort := []any{
    map[string]any{"date_time_original": map[string]any{"order": "desc", "missing": "_last"}},
    map[string]any{"mtime": map[string]any{"order": "desc"}},
}

// 有搜索关键词时，优先按相关性排序
if req.Query != "" {
    sort = append([]any{"_score"}, sort...)
}

query := map[string]any{
    "from": (page - 1) * pageSize,
    "size": pageSize,
    "sort": sort,
    // ...
}
```

**验证**：
- 无搜索关键词时，按时间排序
- 有搜索关键词时，按相关性排序
- 搜索包含文字的图片，能匹配到 `text` 字段

---

### Phase 4: LLM Prompt 更新（优先级：中）

#### Task 4.1: 更新配置文件
**文件**：`config.yaml`

**修改内容**：
```yaml
llm:
  prompt: |
    分析这张图片，返回 JSON 格式：
    {
      "description": "图片描述（50-100字）",
      "tags": ["标签1", "标签2"],
      "objects": ["物体1", "物体2"],
      "scene_type": "场景类型",
      "colors": ["颜色1", "颜色2"],
      "people_count": 人数,
      "has_text": true/false,
      "text": "图片中的文字内容，无文字则为空字符串"
    }
    
    scene_type 必须是以下之一：
    - indoor: 室内场景
    - outdoor: 室外场景
    - underwater: 水下场景
    - aerial: 航拍/无人机视角
    - studio: 影棚/专业拍摄环境
    - night: 夜景/低光环境
    - unknown: 无法判断
```

**验证**：
- 重启服务
- 上传新图片，检查 LLM 返回包含 `text` 字段
- 检查 `scene_type` 使用新的枚举值

---

### Phase 5: 前端更新（优先级：低）

#### Task 5.1: 更新搜索页面显示 text 字段
**文件**：`web/src/pages/Search.tsx`

**修改内容**：
在搜索结果卡片中显示 `text` 字段（如果有内容）：
```tsx
{photo.text && (
  <div className="text-sm text-gray-600 mt-2">
    <span className="font-semibold">文字：</span>
    {photo.text}
  </div>
)}
```

#### Task 5.2: 更新场景类型筛选选项
**文件**：`web/src/pages/Search.tsx`

**修改内容**：
更新场景类型下拉菜单选项：
```tsx
<select>
  <option value="">全部场景</option>
  <option value="indoor">室内</option>
  <option value="outdoor">室外</option>
  <option value="underwater">水下</option>
  <option value="aerial">航拍</option>
  <option value="studio">影棚</option>
  <option value="night">夜景</option>
  <option value="unknown">未知</option>
</select>
```

**验证**：
- 前端编译通过
- 搜索结果能显示 `text` 字段
- 场景类型筛选包含所有 7 个选项

---

## 数据迁移策略

### 现有数据处理
- **无需迁移**：ES 会自动处理新字段
- **旧文档**：`text` 字段为空字符串，`scene_type` 保持原值
- **新文档**：包含完整的 `text` 字段和新的 `scene_type` 枚举

### 重新分析选项
如果需要为现有照片提取文字：
1. 删除 ES 索引：`DELETE /phosche`
2. 重启服务，触发全量重新分析
3. 或手动标记部分照片为 `pending_analysis`，触发增量分析

---

## 测试计划

### 单元测试
1. **类型定义测试**
   - 验证 `AnalysisResult` 包含 `Text` 字段
   - 验证 JSON 序列化/反序列化正确

2. **搜索逻辑测试**
   - 无搜索关键词时，验证按时间排序
   - 有搜索关键词时，验证按相关性排序
   - 搜索包含文字的图片，验证能匹配 `text` 字段

### 集成测试
1. **LLM 集成测试**
   - 上传包含文字的图片
   - 验证 LLM 返回 `text` 字段
   - 验证 `scene_type` 使用新枚举值

2. **ES 集成测试**
   - 验证新索引包含 `text` 字段
   - 验证搜索能匹配 `text` 字段

### 手动测试
1. **端到端测试**
   - 上传 5 张不同类型的图片（室内、室外、夜景、航拍、包含文字）
   - 验证 LLM 分析结果正确
   - 验证搜索功能正常
   - 验证排序规则正确

---

## 验证清单

- [ ] `internal/types/types.go` 编译通过
- [ ] `internal/indexer/mapping.go` 编译通过
- [ ] `internal/search/search.go` 编译通过
- [ ] 所有现有测试通过
- [ ] ES 索引包含 `text` 字段
- [ ] 搜索能匹配 `text` 字段
- [ ] 有搜索关键词时按相关性排序
- [ ] LLM 返回包含 `text` 字段
- [ ] `scene_type` 使用新枚举值
- [ ] 前端编译通过
- [ ] 前端能显示 `text` 字段
- [ ] 前端场景类型筛选包含 7 个选项

---

## 风险与注意事项

### 风险
1. **LLM 模型不支持文字提取**：某些模型可能无法准确提取图片中的文字
   - **缓解**：测试多个模型，选择效果最好的
   - **备选**：如果效果不佳，可以将 `text` 字段设为可选

2. **ES 索引重建**：如果需要重新分析现有照片，会消耗大量 LLM API 调用
   - **缓解**：提供增量分析选项，只重新分析部分照片

3. **前端兼容性**：旧版本前端可能无法显示新字段
   - **缓解**：前端增加字段存在性检查

### 注意事项
1. **向后兼容**：所有改动保持向后兼容，旧数据不受影响
2. **性能影响**：增加 `text` 字段会略微增加 ES 存储和搜索开销
3. **LLM 成本**：提取文字会增加 LLM API 调用成本

---

## 时间估算

| Phase | 任务数 | 预计时间 |
|-------|--------|----------|
| Phase 1: 后端类型定义 | 1 | 10 分钟 |
| Phase 2: ES Mapping | 1 | 10 分钟 |
| Phase 3: 搜索逻辑 | 2 | 20 分钟 |
| Phase 4: LLM Prompt | 1 | 10 分钟 |
| Phase 5: 前端更新 | 2 | 20 分钟 |
| 测试与验证 | - | 30 分钟 |
| **总计** | **7** | **~100 分钟** |

---

## 下一步

1. 确认此计划是否符合预期
2. 开始实施 Phase 1（后端类型定义）
3. 按顺序完成所有 Phase
4. 执行测试计划
5. 部署到生产环境
