# AGENTS.md — Phosche

个人照片搜索服务：监控目录 → 多模态 AI 分析（Ollama/OpenAI）→ OpenSearch 索引 → React SPA 展示。

## 交互规范

- **始终使用中文回复用户**，无论用户使用何种语言提问
- 代码注释、变量命名保持英文（遵循编程规范）
- 技术术语首次出现时可附英文原文，如：断路器（Circuit Breaker）

## 快速命令

```bash
# 开发模式（仅后端，需 ES 已运行）
make run

# 开发模式（前端热重载，另开终端）
cd web && npm run dev

# 构建
make build-frontend && make build    # 生产构建需先构建前端（go:embed 嵌入 web/dist）
make docker-build                    # Docker 多阶段构建自动完成前端+Go

# 测试
make test              # 全部 Go 单元测试（含集成测试，需 Docker）
make test-race         # 带竞态检测
cd web && npm test     # 前端单元测试（vitest）
cd web && npx playwright test  # E2E 测试（需后端运行：先 make run）
cd web && npm run lint # ESLint 检查

# Docker
docker compose up -d                        # ES + phosche
docker compose --profile ollama up -d       # + 本地 Ollama
```

## 架构

### 两个入口点（关键）

- **`main.go`（根目录）** — 生产模式。通过 `//go:embed` 嵌入 `web/dist`，单二进制同时提供 API + SPA。
- **`cmd/phosche/main.go`** — 开发模式。传递 `nil` 给 distFS，前端由 Vite dev server 提供。

两者都调用 `app.Run(distFS, configPath)`。`distFS` 参数决定是否使用嵌入的 SPA。

注意：`make build` 和 `make run` 使用 `cmd/phosche/`（开发入口），Dockerfile 中 `go build .` 使用根目录（生产入口，嵌入 SPA）。

### 处理流水线

```
watch (fsnotify) → decode (JPEG/PNG/WebP/HEIC) → analyze (LLM) → geocode (Amap) → index (ES) → serve (React SPA)
```

编排在 `internal/pipeline/` 中，每个阶段通过接口交互。`internal/app/run.go` 负责依赖注入和生命周期管理。

### 照片状态机

```
unanalyzed → analyzing → analyzed（成功）
unanalyzed → analyzing → pending_analysis（LLM 不可用，每 5 分钟重试，最多 10 次）
unanalyzed → analyzing → failed（不可恢复错误）
```

### 内部包

| 包 | 用途 |
|---|---|
| `analyzer` | LLM 客户端接口 + OpenAI 兼容实现（Ollama/OpenAI 共用）。工厂：`NewLLMClient()` |
| `api` | chi 路由，全部 REST handler。接口：`PhotoSearcher`、`Indexer` |
| `app` | 依赖注入、组件启动顺序、优雅关闭 |
| `cache` | 缩略图/原图缓存生成 |
| `config` | YAML 加载、默认值、校验 |
| `decoder` | 多格式图片解码 + EXIF 提取（dsoprea/go-exif/v3，HEIC 用 go-heic-exif-extractor 兜底） |
| `embedder` | **pipeline 侧**文档向量化（Ollama/OpenAI），含 LRU 缓存和文本模板 |
| `embedding` | **search 侧**查询向量化（独立包），含批处理和重试逻辑。不要和 `embedder` 混淆 |
| `errors` | 统一 `AppError` 类型（NOT_FOUND、VALIDATION_ERROR 等） |
| `geocoder` | 逆地理编码（高德 API） |
| `indexer` | ES 客户端封装、断路器（3 次失败触发，5 秒健康检查）、有界写入队列（容量 100）、**mapping 版本迁移框架**（`migration.go`） |
| `integration` | E2E 测试（testcontainers-go，自动启动 ES 容器） |
| `pipeline` | 阶段编排、并发控制、pending_analysis 重试循环 |
| `search` | ES 查询构建器（BM25 + kNN 混合检索通过 OpenSearch search pipeline RRF） |
| `static` | 照片文件服务，路径遍历防护 |
| `types` | 共享类型：PhotoDocument、EXIF、SearchRequest/Response |
| `watcher` | fsnotify 监控 + 目录扫描 + 去重过滤器（path+mtime+size） |

### 接口边界

| 接口 | 定义位置 | 实现 |
|---|---|---|
| `pipeline.Analyzer` | pipeline/pipeline.go | `analyzer.ImageAnalyzer` |
| `pipeline.Indexer` | pipeline/pipeline.go | `indexer.IndexerService` |
| `pipeline.Embedder` | pipeline/pipeline.go | `embedder.EmbeddingService` |
| `api.PhotoSearcher` | api/router.go | `search.SearchService` |
| `api.Indexer` | api/router.go | `indexer.IndexerService` |
| `analyzer.LLMClient` | analyzer/client.go | `analyzer.OpenAIClient` |
| `watcher.Watcher` | watcher/types.go | `watcher.FSNotifyWatcher` |
| `watcher.Scanner` | watcher/types.go | `watcher.DirectoryScanner` |

### API 端点

所有端点以 `/api/` 为前缀（`/health` 和 `/photos/*` 除外）。

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/health` | 健康检查 |
| GET | `/api/photos` | 时间线列表（分页、日期/状态过滤） |
| GET | `/api/photos/{id}` | 单张照片详情 |
| GET | `/api/photos/{id}/similar` | 相似照片推荐（基于 embedding） |
| GET | `/api/photos/{id}/nearby` | 附近照片推荐（基于 GPS） |
| POST | `/api/photos/cleanup` | 清理孤儿文档 |
| POST | `/api/search` | 全文搜索 |
| GET | `/api/filters` | 筛选选项聚合 |
| GET | `/api/stats` | 统计信息 |
| POST | `/api/migrate-timezone` | 时区迁移（异步后台执行） |
| GET | `/photos/{path}` | 照片静态文件（`?thumb=1` 缩略图，`?convert=1` HEIC 转 JPEG） |

中间件栈（注册顺序）：Logger → Recoverer → Timeout(30s) → CORS → JWTAuth

JWTAuth 从 `X-Token-User-Email` header 提取 email 注入 context，上游网关已校验 JWT 并注入此 header。JWTAuth（从 cookie 解析 JWT）已弃用，保留备用。

## 配置

- 复制 `config.example.yaml` → `config.yaml`（已 gitignore）
- 必填字段：`watch.directories`、`opensearch.addresses`、`llm.provider`、`llm.openai.base_url`、`llm.openai.model`
- 配置加载逻辑：`internal/config/config.go`，带默认值和校验
- 环境变量覆盖：设置 `CONFIG_PATH` 可覆盖默认 `config.yaml` 路径
- `server.timezone`：EXIF 无时区信息时使用的默认时区（如 `Asia/Shanghai`）
- `llm.openai.response_format`：`json_object`（默认）/ `json_schema`（严格 schema）/ `text`（纯文本引导）
- `embedding` 配置：可选的混合检索（BM25 + kNN），支持 Ollama/OpenAI 两个 provider

## 前端（web/）

- React 19 + TypeScript 6 + Vite 8 + Tailwind CSS 4
- 状态管理：TanStack React Query 5
- 路由：react-router-dom 7
- HTTP 客户端：axios
- PWA：vite-plugin-pwa（prompt 更新策略，Workbox 离线缓存）
- Vite 开发服务器代理 `/api`、`/photos`、`/health` → `localhost:8080`
- 测试：vitest + msw（单元）、Playwright（E2E）
- TypeScript：`verbatimModuleSyntax: true`（必须显式 `import type`）

## 测试注意事项

- 集成测试（`internal/integration/`、`internal/indexer/`、`internal/search/`）使用 testcontainers-go — 需要 Docker 运行
- `make test` 运行全部 Go 测试（含集成测试），首次会拉取 OpenSearch 2.19.5 镜像
- 前端测试单独运行：`cd web && npm test`
- E2E 测试需要后端运行：先 `make run`，再 `cd web && npx playwright test`
- Go mock 模式：手写接口实现（函数注入或固定返回值），不使用 mock 框架
- 前端 HTTP mock：msw (Mock Service Worker) 拦截 axios 请求

## 约定

- Go 标准格式化（`go fmt`）— 无自定义 linter 配置
- 所有 API 端点以 `/api/` 为前缀，`/health` 和 `/photos/*` 除外
- 照片 ID 是文件路径的 SHA-256 哈希
- 结构化日志：`log/slog`（JSON 格式，可配置级别）
- EXIF 提取：dsoprea/go-exif/v3（JPEG/PNG），go-heic-exif-extractor（HEIC 兜底）
- 逆地理编码：高德 API，格式化地址存入 ES
- 缓存命名：`{photoID}_thumb.jpg` / `{photoID}_full.jpg`
- `cache/` 目录已 gitignore（`internal/cache/` 生成的缩略图）
- Go 版本：1.26.2（`.tool-versions` 指定 asdf）

## Mapping 版本迁移

OpenSearch 索引的 mapping 版本采用**逐版本增量迁移**策略，不再删除重建索引（避免数据丢失）。

### 核心文件

- `internal/indexer/mapping.go` — `mappingVersion` 常量（当前为整数 `9`），`EnsureIndex()` 启动时检测版本并触发迁移
- `internal/indexer/migration.go` — 迁移框架：`Migration` 类型、`migrations` 全局注册表、`runMigrations()` 逐版本执行逻辑

### 迁移流程

启动时 `EnsureIndex()` 检测线上索引的 `_meta.version`：
- 版本匹配 → 无操作
- 版本落后 → 从 `currentVersion+1` 到 `MappingVersion()` 逐版本执行迁移
- 版本超前 → 记录错误日志，降级启动（不支持降级）
- `_meta.version` 缺失 → 视为版本 0，从版本 1 开始迁移

每个迁移步骤：`applyMapping()`（PUT mapping API 添加新字段）→ `Migrate()`（可选数据回填）→ `updateMetaVersion()`（更新 `_meta.version` 标记完成）

迁移失败时记录错误但不中断启动（降级模式），`_meta.version` 未更新的步骤下次启动会重试。

### 新增 mapping 版本

两步：

1. 在 `mapping.go` 中将 `mappingVersion` 递增（如 `9` → `10`），并在 `buildIndexMapping()` 中添加新字段
2. 在 `migration.go` 中注册迁移脚本：

```go
func init() {
    RegisterMigration(Migration{
        Version: 10,
        Mapping: map[string]any{
            "multimodal_embedding": map[string]any{
                "type":       "knn_vector",
                "dimension":  1024,
                "space_type": "cosinesimil",
            },
        },
        Migrate: func(ctx context.Context, client *OSClient, indexName string, cfg *config.Config) error {
            // 迁移脚本自行根据 cfg 创建所需的外部依赖
            embClient, _ := embedder.NewEmbeddingClient(embedder.EmbeddingClientConfig{...})
            // 遍历所有照片，调用模型获取向量并更新
            return client.ScrollAll(ctx, indexName, func(doc *types.PhotoDocument) error {
                vec, _ := embClient.Embed(ctx, ...)
                // Update document with new field via Update API
                ...
            })
        },
    })
}
```

`Mapping` 为 `nil` 表示仅数据回填无新字段；`Migrate` 为 `nil` 表示仅 mapping 变更无需回填。

## Git 提交规范

**必须使用中文**编写提交信息，格式采用 Conventional Commits 规范：

```
<type>(<scope>): <subject>

<body>

<footer>
```

- **type**: `feat` / `fix` / `refactor` / `docs` / `style` / `test` / `chore` / `perf`
- **scope**: 影响范围（可选），如 `analyzer`、`config`、`api`、`web` 等
- **subject**: 简短描述（必填），使用中文

### 示例

```
feat(analyzer): 新增批量分析接口，支持并发处理多张图片

- 实现 /api/batch-analyze 端点
- 最大并发数由 config.batch_concurrency 控制（默认 5）
- 返回结果包含每张图片的处理状态

Closes #123
```

```
fix(search): 修复按日期范围搜索时结果不准确的问题

调整时间戳比较逻辑，使用 Unix 时间戳而非格式化字符串
```

```
refactor(config): 简化配置加载流程

- 移除冗余的 validateSchema 函数
- 统一使用 go-playground/validator 进行校验
```

```
chore(docker): 优化 Docker 镜像大小，从 1.2GB 缩减至 450MB
```
