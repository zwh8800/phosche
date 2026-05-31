# AGENTS.md — Phosche

个人照片搜索服务：监控目录 → 多模态 AI 分析（Ollama/OpenAI）→ Elasticsearch 索引 → React SPA 展示。

## 快速命令

```bash
# 开发模式（仅后端，需 ES 已运行）
make run

# 开发模式（前端热重载，另开终端）
cd web && npm run dev

# 构建
make build-frontend && make build

# 测试
make test              # 全部 Go 单元测试
make test-race         # 带竞态检测
cd web && npm test     # 前端单元测试（vitest）
cd web && npx playwright test  # E2E 测试

# Docker
docker compose up -d                        # ES + phosche
docker compose --profile ollama up -d       # + 本地 Ollama
```

## 架构

### 两个入口点（关键）

- **`main.go`（根目录）** — 生产模式。通过 `//go:embed` 嵌入 `web/dist`，单二进制同时提供 API + SPA。
- **`cmd/phosche/main.go`** — 开发模式。传递 `nil` 给 distFS，前端由 Vite dev server 提供。

两者都调用 `app.Run(distFS, configPath)`。`distFS` 参数决定是否使用嵌入的 SPA。

### 处理流水线

```
watch (fsnotify) → decode (JPEG/PNG/WebP/HEIC) → analyze (LLM) → geocode (Amap) → index (ES) → serve (React SPA)
```

编排在 `internal/pipeline/` 中，每个阶段是独立包，通过清晰接口交互。

### 内部包

| 包 | 用途 |
|---|---|
| `analyzer` | LLM 客户端接口 + Ollama/OpenAI 实现。工厂方法：`NewLLMClient()` |
| `api` | chi 路由，全部 REST handler。接口：`PhotoSearcher`、`Indexer` |
| `app` | 依赖注入、生命周期、优雅关闭 |
| `cache` | 缩略图/原图缓存生成 |
| `config` | YAML 加载、默认值、校验 |
| `decoder` | 多格式图片解码 + EXIF 提取 |
| `errors` | 统一 `AppError` 类型（NOT_FOUND、VALIDATION_ERROR 等） |
| `geocoder` | 逆地理编码（高德 API） |
| `indexer` | ES 客户端封装、断路器、有界写入队列 |
| `integration` | E2E 测试（testcontainers-go，自动启动 ES 容器） |
| `pipeline` | 阶段编排、并发控制 |
| `search` | ES 查询构建器（全文、过滤、聚合） |
| `static` | 照片文件服务，路径遍历防护 |
| `types` | 共享类型：PhotoDocument、EXIF、SearchRequest/Response |
| `watcher` | fsnotify 监控 + 目录扫描 + 去重过滤器 |

### 照片状态机

```
unanalyzed → analyzing → analyzed（成功）
unanalyzed → analyzing → pending_analysis（LLM 不可用，每 5 分钟重试，最多 10 次）
unanalyzed → analyzing → failed（不可恢复错误）
```

## 配置

- 复制 `config.example.yaml` → `config.yaml`（已 gitignore）
- 必填字段：`watch.directories`、`elasticsearch.addresses`、`llm.provider`
- 配置加载逻辑：`internal/config/config.go`，带默认值和校验
- 环境变量覆盖：设置 `CONFIG_PATH` 可覆盖默认 `config.yaml` 路径
- `env` 配置项：
  ```yaml
  env:
    amap_key: ""  # 高德地图 API Key（用于逆地理编码）
  ```

## 前端（web/）

- React 19 + TypeScript 6 + Vite 8 + Tailwind CSS 4
- 状态管理：TanStack React Query 5
- 路由：react-router-dom 7
- HTTP 客户端：axios
- Vite 开发服务器代理 `/api`、`/photos`、`/health` → `localhost:8080`
- 测试：vitest（单元）、Playwright（E2E）

## 测试注意事项

- 集成测试（`internal/integration/`）使用 testcontainers-go — 需要 Docker 运行
- `make test` 运行全部 Go 测试（含集成测试），首次会拉取 ES 镜像
- 前端测试单独运行：`cd web && npm test`
- E2E 测试需要后端运行：先 `make run`，再 `cd web && npx playwright test`

## 约定

- Go 标准格式化（`go fmt`）— 无自定义 linter 配置
- 所有 API 端点以 `/api/` 为前缀，`/health` 和 `/photos/*` 除外
- 照片 ID 是文件路径的 SHA-256 哈希
- 结构化日志：`log/slog`（JSON 格式）
- EXIF 提取：dsoprea/go-exif/v3（JPEG/PNG），go-heic-exif-extractor（HEIC 兜底）
- 逆地理编码：高德 API，格式化地址存入 ES
- 缓存命名：`{photoID}_thumb.jpg` / `{photoID}_full.jpg`
- `cache/` 目录已 gitignore（`internal/cache/` 生成的缩略图）
