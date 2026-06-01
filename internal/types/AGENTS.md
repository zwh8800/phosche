# internal/types

共享领域类型定义，被 pipeline、indexer、search、api、decoder 等多个包引用。

## 核心类型

| 类型 | 用途 |
|------|------|
| `PhotoDocument` | ES 索引文档，内嵌 `Photo` + `AnalysisResult` + `GeoInfo`，扁平化到 ES |
| `Photo` | 照片元数据（ID、Path、MTime、Size、Status、EXIF、Email） |
| `AnalysisResult` | LLM 分析结果（description、tags、objects、scene_type、colors、people_count） |
| `GeoInfo` | 逆地理编码（province、city、district、formatted_address） |
| `EXIFInfo` | EXIF 元数据（DateTimeOriginal、CameraModel、GPS、ISO 等） |
| `SearchRequest` / `SearchResponse` | 搜索请求/响应 |
| `StatsResponse` / `FiltersResponse` | 统计和筛选聚合响应 |
| `JobStatus` | 照片处理状态枚举 |
| `FileEvent` | 文件系统事件（Path、Op、MTime、Size） |

## 照片状态机

```
unanalyzed → analyzing → analyzed（成功）
unanalyzed → analyzing → pending_analysis（LLM 不可用，每 5 分钟重试，最多 10 次）
unanalyzed → analyzing → failed（不可恢复错误）
```

状态常量：`StatusUnanalyzed`、`StatusAnalyzing`、`StatusAnalyzed`、`StatusFailed`、`StatusPendingAnalysis`。

## ES 映射约定

结构体字段的 `es` 标签定义 ES 映射类型：`text`（全文检索）、`keyword`（精确匹配）、`date`、`integer`、`double`、`nested`、`boolean`。

`indexer/mapping.go` 根据这些标签动态构建 ES 索引映射。

## 约定

- `PhotoDocument` 通过内嵌结构体实现扁平化 — 所有字段在 ES 中处于同一层级
- 照片 ID 是文件路径的 SHA-256 哈希（在 indexer、pipeline、cache 中分别计算）
- `FileOp` 枚举：`OpCreate`、`OpModify`、`OpDelete`
- JSON 标签使用 `snake_case` + `omitempty`
