# Phosche - 照片搜索服务

## TL;DR

> **Quick Summary**: 构建一个 Go + React 的个人照片搜索服务，自动监控目录中新增的图片，通过多模态 LLM (Ollama/OpenAI) 分析内容并索引到 Elasticsearch，提供时间线浏览和多维度搜索的 Web 界面。
>
> **Deliverables**:
> - Go 后端服务：目录监控、AI 图片分析、ES 索引与搜索、静态文件服务、REST API
> - React 前端 SPA：时间线浏览、全文搜索与多维度过滤、照片详情
> - Docker Compose 编排 + 单二进制部署双模式
> - 完整 TDD 测试覆盖
>
> **Estimated Effort**: Large
> **Parallel Execution**: YES - 5 waves (Wave 1 split into 1a + 1b)
> **Critical Path**: T1 → T3 → T11 → T15 → T16 → T21 → T28b → F1-F4

---

## Context

### Original Request
构建照片搜索服务（phosche）：
1. 根据配置监控一个/多个目录，新图片文件到达时触发 AI 扫描
2. AI 扫描使用多模态 LLM，支持 Ollama 和 OpenAI 协议
3. LLM 分析结果落库到 Elasticsearch 8.x
4. 提供 Web 页面，支持时间线浏览和多维度搜索

### Interview Summary

**Key Discussions**:
- **Web 框架**: chi — 轻量级，与标准库兼容，适合 REST API + 静态文件服务
- **前端方案**: React + Vite SPA — 前后端分离，交互丰富的搜索界面
- **Elasticsearch**: ES 8.x — 官方 Go 客户端最成熟，个人项目许可证无压力
- **LLM 协议**: Ollama + OpenAI 双协议，统一接口抽象，配置文件切换
- **图片格式**: JPEG, PNG, WebP, HEIC/HEIF（4 种格式）
- **文件监控**: 启动时全量扫描已有图片 + 持续监控新增，去重策略：路径 + mtime + size
- **数据存储**: 仅 ES，无关系型数据库（所有数据存 ES）
- **搜索功能**: 全文搜索 + 日期范围 + EXIF 信息 + AI 标签多维度过滤
- **图片访问**: Go 内置静态文件服务器，通过 `/photos/` 路径访问磁盘原图
- **测试策略**: TDD（从零搭建测试基础设施）
- **部署方式**: Docker Compose + 单二进制双模式
- **配置格式**: YAML
- **用户认证**: 无（个人/家庭使用场景）
- **LLM 提示词**: 通用分析提示词，配置文件可自定义

**Metis Review - Key Findings**:
- fsnotify 不支持原生递归监控 → 需实现 Walk + Add 循环，并处理运行时新建子目录
- HEIC 解码依赖 libheif → Linux/Docker 容器需安装系统库
- ES 8.x 默认启用 TLS 自签名证书 → 需配置选项禁用或信任 CA
- LLM 响应必须定义 JSON Schema → 分析结果结构化存储
- ES 不可用时的降级策略 → 内存有界队列 + 断路器模式
- 图片删除时的 ES 清理 → 定期孤儿文档清理任务

---

## Work Objectives

### Core Objective
构建一个端到端的个人照片搜索服务：自动发现→AI 理解→全文索引→Web 浏览与搜索

### Concrete Deliverables
- `cmd/phosche/main.go` — 主入口，优雅启停
- `internal/config/` — YAML 配置解析与验证
- `internal/watcher/` — 递归文件系统监控 + 初始扫描
- `internal/decoder/` — 多格式图片解码（JPEG/PNG/WebP/HEIC）
- `internal/analyzer/` — LLM 分析引擎（Ollama + OpenAI 双协议）
- `internal/indexer/` — ES 文档索引与更新
- `internal/search/` — ES 搜索查询构建器
- `internal/api/` — REST API（chi 路由 + 中间件）
- `internal/static/` — 静态文件服务（图片原图）
- `internal/pipeline/` — 处理流水线编排（watch→analyze→index）
- `web/` — React + Vite SPA 前端
- `config.yaml` — 示例配置文件
- `Dockerfile` — 单二进制构建
- `docker-compose.yaml` — 编排 ES + phosche

### Definition of Done
- [ ] `go test ./...` → 全部通过
- [ ] `go build -o phosche ./cmd/phosche/` → 成功
- [ ] `docker compose up` → 服务启动，健康检查通过
- [ ] 向监控目录放入 JPEG → ES 中出现已分析文档
- [ ] 通过 Web UI 搜索 → 返回匹配结果
- [ ] 时间线页面 → 显示所有已分析照片

### Must Have
- 递归目录监控 + 初始全量扫描
- Ollama + OpenAI 双 LLM 协议支持
- 结构化 LLM 响应（JSON Schema 定义）
- ES 全文搜索 + 日期范围 + EXIF + AI 标签过滤
- Web 时间线浏览（按日期分组，无限滚动）
- 图片原图静态文件服务
- YAML 配置文件，所有参数可配置
- 优雅启停（信号处理）
- ES 不可用时继续运行（断路器 + 有界队列）
- 损坏图片日志警告 + 跳过（不崩溃）

### Must NOT Have (Guardrails)
- 用户认证/登录系统
- 关系型数据库（SQLite/PostgreSQL）
- 缩略图生成或任何图片处理（转码/缩放/裁剪）
- 图片上传功能
- 视频文件支持
- RAW 相机格式（NEF/CR2/ARW/DNG）
- 内容哈希去重（仅路径 + mtime + size）
- 实时推送（WebSocket/SSE）
- 批量重新分析功能
- 搜索结果导出
- AI 提示词 Web UI 编辑
- 人脸识别（LLM 原生能力除外）
- 内容相似度/感知哈希去重

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed. No exception.

### Test Decision
- **Infrastructure exists**: NO（从零搭建）
- **Automated tests**: TDD（测试驱动开发）
- **Framework**: Go 标准 `testing` + `testify/assert` + `testcontainers-go`（ES 集成测试）
- **TDD 流程**: 每个 Task 遵循 RED（编写失败测试）→ GREEN（最小实现）→ REFACTOR

### QA Policy
每个 Task 必须包含 Agent-Executed QA Scenarios（见 TODO 模板）。
Evidence 保存到 `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`。

- **API/Backend**: `curl` 发送请求，断言状态码 + JSON 响应字段
- **CLI/进程**: `interactive_bash` (tmux) 运行进程，验证输出
- **Frontend/UI**: Playwright 打开浏览器，导航、交互、断言 DOM、截图
- **集成测试**: `go test` 运行 testcontainers ES 集成测试

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1a (Start Immediately — 脚手架, 1 task):
└── T1: 项目脚手架 (Go + React + slog 日志约定)

Wave 1b (After T1 — 基础设施, 8 tasks parallel):
├── T2: YAML 配置系统
├── T3: 共享类型定义 + 错误类型
├── T4: ES 客户端 + 索引映射
├── T5: LLM 客户端接口 (Ollama + OpenAI)
├── T6: 图片解码器 (JPEG/PNG/WebP/HEIC)
├── T7: 文件监控类型 + 接口
└── T13: 静态文件服务器

Wave 2 (After Wave 1b — 核心模块 + 前端脚手架, 6 tasks parallel):
├── T8: 递归 fsnotify 文件监控器
├── T9: 初始目录扫描器
├── T10: LLM 分析器 (提示词引擎 + 响应解析)
├── T11: ES 索引器服务
├── T12: ES 搜索查询构建器
├── T14: 处理流水线编排器 (watch→analyze→index)
└── T20: React 项目脚手架 + API 客户端 + Layout 骨架

Wave 3 (After Wave 2 — API, 5 tasks parallel):
├── T15: API 路由 + 中间件 + 健康检查
├── T16: 时间线 API 端点
├── T17: 搜索 API 端点
├── T18: 照片详情 API 端点
└── T19: 状态/统计/过滤器 API 端点

Wave 4 (After Wave 3 — 前端 UI, 4 tasks parallel):
├── T21: 时间线页面 (按日期分组 + 无限滚动)
├── T22: 搜索页面 (全文搜索 + 过滤器)
├── T23: 照片详情弹窗
└── T24: 响应式微调 + 404 + 错误边界

Wave 5 (After Wave 4 — 集成与部署, 5 tasks parallel):
├── T25: 主入口点 (cmd/phosche/main.go 完整装配)
├── T26: Docker Compose 编排
├── T27: Dockerfile + 单二进制构建
├── T28a: 后端端到端集成测试
└── T28b: 前端端到端集成测试

Wave FINAL (After ALL — 4 parallel reviews):
├── F1: Plan Compliance Audit (oracle)
├── F2: Code Quality Review (unspecified-high)
├── F3: Real Manual QA (unspecified-high + playwright)
└── F4: Scope Fidelity Check (deep)
→ Present results → Get explicit user okay
```

**Critical Path**: T1 → T3 → T11 → T15 → T16 → T21 → T28b → F1-F4
**Parallel Speedup**: ~78% faster than sequential
**Max Concurrent**: 8 (Wave 1b)

### Dependency Matrix

| Task | Depends On | Blocks | Wave |
|------|-----------|--------|------|
| T1 | — | T2-T7,T13,T20 | 1a |
| T2 | T1 | T8,T9,T13,T14,T25 | 1b |
| T3 | T1 | T8-T19,T25 | 1b |
| T4 | T1,T3 | T11,T12,T16-T19,T28a | 1b |
| T5 | T1,T3 | T10,T14 | 1b |
| T6 | T1 | T14 | 1b |
| T7 | T1,T3 | T8,T9 | 1b |
| T8 | T2,T3,T7 | T14 | 2 |
| T9 | T2,T7 | T14 | 2 |
| T10 | T3,T5 | T14 | 2 |
| T11 | T3,T4 | T14,T15,T16-T19 | 2 |
| T12 | T3,T4 | T15,T16-T18 | 2 |
| T13 | T1,T2 | T25 | 1b |
| T14 | T8,T9,T10,T11,T2 | T25 | 2 |
| T15 | T3,T11,T12 | T16-T20,T25 | 3 |
| T16 | T15,T11,T12 | T21,T28a,T28b | 3 |
| T17 | T15,T11,T12 | T22,T28a,T28b | 3 |
| T18 | T15,T11,T12 | T23 | 3 |
| T19 | T15,T11 | — | 3 |
| T20 | T1 | T21-T24 | 2 |
| T21 | T16,T20 | T28b | 4 |
| T22 | T17,T20 | T28b | 4 |
| T23 | T18,T20 | T24 | 4 |
| T24 | T20,T21,T22,T23 | — | 4 |
| T25 | T13,T14,T15 | T26-T28b | 5 |
| T26 | T25 | — | 5 |
| T27 | T25 | — | 5 |
| T28a | T16,T17,T25,T4 | — | 5 |
| T28b | T21,T22,T25 | — | 5 |

### Agent Dispatch Summary

- **Wave 1a**: 1 task — T1 → `quick`
- **Wave 1b**: 8 tasks — T2-T7, T13 → `quick`/`unspecified-high`
- **Wave 2**: 6 tasks — T8-T12 → `deep`/`unspecified-high`, T20 → `visual-engineering`
- **Wave 3**: 5 tasks — T15-T19 → `unspecified-high`
- **Wave 4**: 4 tasks — T21-T24 → `visual-engineering`
- **Wave 5**: 5 tasks — T25 → `deep`, T26-T27 → `quick`, T28a-T28b → `deep`
- **FINAL**: 4 tasks — F1 → `oracle`, F2-F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

> Implementation + Test = ONE Task. Never separate.
> EVERY task MUST have: Recommended Agent Profile + Parallelization info + QA Scenarios.
> **A task WITHOUT QA Scenarios is INCOMPLETE. No exceptions.**

- [x] 1. **项目脚手架 — Go 模块 + React 项目初始化**

  **What to do**:
  - 已有 `go.mod`（`github.com/zwh8800/phosche`），创建标准 Go 项目目录结构：`cmd/phosche/`, `internal/config/`, `internal/watcher/`, `internal/decoder/`, `internal/analyzer/`, `internal/indexer/`, `internal/search/`, `internal/api/`, `internal/static/`, `internal/pipeline/`
  - 在 `web/` 目录用 Vite 创建 React + TypeScript 项目（`npm create vite@latest web -- --template react-ts`）
  - 安装前端核心依赖：`react-router-dom`, `axios`, `@tanstack/react-query`
  - 创建 `cmd/phosche/main.go` 最小入口（打印 "phosche starting..."）
  - 创建 `config.example.yaml` 占位文件
  - 创建 `.gitignore`（Go + Node 标准忽略规则）
  - 确定日志方案：全项目统一使用 Go 标准库 `log/slog`，结构化日志。在 `cmd/phosche/main.go` 中初始化全局 slog logger（JSON handler，可配 log level）
  - TDD: 创建 `cmd/phosche/main_test.go` — 测试 main 包可编译且 `go build` 成功

  **Must NOT do**:
  - 不要安装额外的 Go 依赖（后续 Task 各自安装）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 纯脚手架操作，创建目录结构、初始化项目模板，无复杂逻辑
  - **Skills**: []
  - **Skills Evaluated but Omitted**: N/A（纯脚手架，无需专业技能）

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1（with T2-T7）
  - **Blocks**: T2, T3, T4, T5, T6, T7, T13, T20
  - **Blocked By**: None（can start immediately）

  **Acceptance Criteria**:
  - [ ] `go build ./cmd/phosche/` → 成功，生成二进制
  - [ ] `ls cmd/phosche/internal/` → 显示 10 个目录
  - [ ] `ls web/src/` → 显示 `App.tsx`, `main.tsx`
  - [ ] `ls config.example.yaml` → 文件存在
  - [ ] `log/slog` 在 main.go 中初始化，可编译通过

  **QA Scenarios**:

  ```
  Scenario: Go 项目可编译
    Tool: Bash (go build)
    Preconditions: Go 1.26.2 已安装
    Steps:
      1. go build -o /tmp/phosche ./cmd/phosche/
      2. ls -la /tmp/phosche
    Expected Result: 二进制文件存在且可执行（exit 0）
    Failure Indicators: 编译错误，exit code != 0
    Evidence: .sisyphus/evidence/task-1-build.txt

  Scenario: React 项目可构建
    Tool: Bash (npm)
    Preconditions: Node.js >= 18 已安装
    Steps:
      1. cd web && npm install
      2. npm run build
    Expected Result: dist/ 目录生成，包含 index.html + assets
    Failure Indicators: npm 错误，构建失败
    Evidence: .sisyphus/evidence/task-1-react-build.txt
  ```

  **Commit**: YES
  - Message: `chore: initialize project scaffolding (Go + React)`
  - Files: `cmd/`, `internal/`, `web/`, `config.example.yaml`, `.gitignore`

- [x] 2. **YAML 配置系统**

  **What to do**:
  - 安装 `gopkg.in/yaml.v3`
  - 在 `internal/config/config.go` 定义 `Config` 结构体，包含所有配置项：
    - `Watch.Directories []string` — 监控目录列表
    - `Watch.Recursive bool` — 是否递归子目录
    - `Watch.DebounceMs int` — 去抖间隔
    - `LLM.Provider string` — "ollama" 或 "openai"
    - `LLM.Ollama.BaseURL string` — Ollama API 地址
    - `LLM.Ollama.Model string` — 模型名
    - `LLM.OpenAI.APIKey string` — API Key
    - `LLM.OpenAI.BaseURL string` — API 地址
    - `LLM.OpenAI.Model string` — 模型名
    - `LLM.Prompt string` — 系统提示词
    - `LLM.MaxRetries int` — 最大重试次数
    - `LLM.Concurrency int` — 并发数
    - `LLM.TimeoutSeconds int` — 超时
    - `Elasticsearch.Addresses []string` — ES 地址
    - `Elasticsearch.Username string`
    - `Elasticsearch.Password string`
    - `Elasticsearch.InsecureSkipVerify bool` — 跳过 TLS 验证
    - `Elasticsearch.IndexName string` — 索引名
    - `Server.Host string`
    - `Server.Port int`
    - `Server.PhotoBasePath string` — 图片静态文件根路径
  - 实现 `LoadConfig(path string) (*Config, error)` — 读取 YAML 文件并验证必填字段
  - TDD: 先写测试 — 有效 YAML 正确解析，无效 YAML 返回错误，缺失必填字段返回验证错误

  **Must NOT do**:
  - 不要实现热重载（v1 仅启动时加载）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准的配置结构体 + YAML 解析，Go 基础操作
  - **Skills**: []
  - **Skills Evaluated but Omitted**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1（with T1, T3-T7）
  - **Blocks**: T8, T9, T14, T25
  - **Blocked By**: T1

  **Acceptance Criteria**:
  - [ ] `go test ./internal/config/` → PASS（至少 3 个测试：有效配置、缺失必填字段、无效 YAML）
  - [ ] `config.example.yaml` — 完整示例，所有字段有默认值

  **QA Scenarios**:

  ```
  Scenario: 有效 YAML 正确解析
    Tool: Bash (go test)
    Preconditions: config.example.yaml 存在且格式正确
    Steps:
      1. go test -v ./internal/config/ -run TestLoadConfig_Valid
      2. 验证返回的 Config 结构体字段值与 YAML 一致
    Expected Result: test PASS，Config.Host="0.0.0.0"，Config.Port=8080
    Failure Indicators: test FAIL，字段值不匹配
    Evidence: .sisyphus/evidence/task-2-valid-config.txt

  Scenario: 无效 YAML 返回错误
    Tool: Bash (go test)
    Preconditions: 已有测试文件
    Steps:
      1. go test -v ./internal/config/ -run TestLoadConfig_InvalidYAML
    Expected Result: test PASS，LoadConfig 返回 non-nil error
    Evidence: .sisyphus/evidence/task-2-invalid-yaml.txt
  ```

  **Commit**: YES
  - Message: `feat(config): implement YAML configuration system with validation`
  - Files: `internal/config/`, `config.example.yaml`

- [x] 3. **共享类型定义 + 错误类型**

  **What to do**:
  - 在 `internal/types/types.go` 定义所有共享类型：
    - `Photo` — Path, MTime, Size, Status(unanalyzed/analyzing/analyzed/failed), AnalyzedAt, EXIF
    - `EXIFInfo` — DateTimeOriginal, CameraModel, LensModel, FocalLength, Aperture, ISO, GPSLat, GPSLon
    - `AnalysisResult` — Description, Tags []string, Objects []string, SceneType, Colors []string, Confidence float64
    - `PhotoDocument` — 嵌入 Photo + AnalysisResult + EXIFInfo 的 ES 文档结构
    - `SearchRequest` — Query, DateFrom, DateTo, Tags, Objects, SceneType, CameraModel, Page, PageSize
    - `SearchResponse` — Hits []PhotoDocument, Total int64, Page, PageSize
    - `ProcessMessage` — 内部管道消息（Photo + 控制字段）
    - `JobStatus` — 处理任务状态枚举
  - 添加 JSON 和 ES 映射 tag（`json:"..." es:"..."`）
  - 在 `internal/errors/errors.go` 定义统一错误类型：
    - `AppError` 结构体：`Code string`, `Message string`, `HTTPStatus int`, `Details interface{}`
    - 预定义错误分类：`ErrNotFound`, `ErrValidation`, `ErrInternal`, `ErrServiceUnavailable`
    - 构造函数：`NewNotFoundError(msg)`, `NewValidationError(msg, details)`, `NewInternalError(err)`
  - API 统一错误响应格式：`{"error":{"code":"NOT_FOUND","message":"Photo not found"}}`
  - TDD: 编写序列化/反序列化往返测试

  **Must NOT do**:
  - 不要在 types 包中引入业务逻辑

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 纯类型定义 + JSON tag，无业务逻辑
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1（with T1-T2, T4-T7）
  - **Blocks**: T8-T19, T25
  - **Blocked By**: T1

  **Acceptance Criteria**:
  - [ ] `go test ./internal/types/` → PASS
  - [ ] `PhotoDocument` 可正确 JSON 序列化/反序列化
  - [ ] `SearchRequest` JSON 反序列化支持可选字段
  - [ ] `go test ./internal/errors/` → PASS
  - [ ] `AppError` 可正确 JSON 序列化

  **QA Scenarios**:

  ```
  Scenario: Photo 类型 JSON 往返正确
    Tool: Bash (go test)
    Preconditions: 类型定义已存在
    Steps:
      1. go test -v ./internal/types/ -run TestPhotoRoundTrip
      2. 验证序列化后反序列化，字段值一致
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-3-types-roundtrip.txt
  ```

  **Commit**: YES
  - Message: `feat(types): define shared type system for photo pipeline`
  - Files: `internal/types/`

- [x] 4. **Elasticsearch 客户端 + 索引映射**

  **What to do**:
  - 安装 `github.com/elastic/go-elasticsearch/v8`
  - 在 `internal/indexer/client.go` 实现 `NewESClient(cfg)` — 创建 ES 客户端，处理 TLS 配置（InsecureSkipVerify）
  - 在 `internal/indexer/mapping.go` 定义索引映射 JSON：
    - `description`, `tags`, `objects` → `text`（全文搜索）
    - `scene_type`, `camera_model`, `tags.keyword` → `keyword`（精确过滤）
    - `date_time_original` → `date`（日期范围）
    - `gps_location` → `geo_point`（可选，未来地理搜索）
    - `created_at` → `date`
    - `status` → `keyword`
  - 实现 `EnsureIndex(ctx, indexName)` — 如索引不存在则创建
  - Mapping 版本检查：在索引 mapping 中嵌入 `_meta.version` 字段。启动时 `EnsureIndex` 检查现有 mapping 版本，如不匹配则打日志警告（`slog.Warn("index mapping version mismatch, consider recreating index")`），不自动迁移
  - TDD: 使用 `testcontainers-go` 启动 ES 容器，测试客户端连接、索引创建

  **Must NOT do**:
  - 不要实现文档 CRUD（T11 做）
  - 不要实现搜索（T12 做）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 涉及 ES 客户端配置、TLS 处理、testcontainers 集成测试，中等复杂度
  - **Skills**: []
  - **Skills Evaluated but Omitted**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1（with T1-T3, T5-T7）
  - **Blocks**: T11, T12, T16-T19, T28a
  - **Blocked By**: T1, T3

  **Acceptance Criteria**:
  - [ ] `go test ./internal/indexer/` → PASS（集成测试需要 Docker）
  - [ ] `EnsureIndex` 幂等（多次调用不报错）
  - [ ] ES 不可用时返回明确错误（非 panic）
  - [ ] mapping 版本不匹配时打日志警告（非 panic）

  **QA Scenarios**:

  ```
  Scenario: ES 客户端连接到 testcontainers ES
    Tool: Bash (go test)
    Preconditions: Docker 已运行
    Steps:
      1. go test -v ./internal/indexer/ -run TestESClient_Connect
      2. 验证 Ping 返回 200
    Expected Result: test PASS，ES 集群健康状态 green/yellow
    Evidence: .sisyphus/evidence/task-4-es-connect.txt

  Scenario: 索引创建幂等
    Tool: Bash (go test)
    Preconditions: testcontainers ES 运行中
    Steps:
      1. go test -v ./internal/indexer/ -run TestEnsureIndex_Idempotent
      2. EnsureIndex 调用 2 次，无错误
    Expected Result: test PASS，索引存在且 mapping 正确
    Evidence: .sisyphus/evidence/task-4-index-idempotent.txt
  ```

  **Commit**: YES
  - Message: `feat(indexer): set up ES client with index mapping`
  - Files: `internal/indexer/`

- [x] 5. **LLM 客户端接口 — Ollama + OpenAI 双协议**

  **What to do**:
  - 在 `internal/analyzer/` 定义 `LLMClient` 接口：
    - `AnalyzeImage(ctx, imageData []byte, prompt string) (*AnalysisResult, error)`
  - 实现 `OllamaClient` — 调用 Ollama `/api/chat`：
    - 请求格式：`{"model":"...", "messages":[{"role":"user","content":"...","images":["base64..."]}], "stream":false, "format":"json"}`
    - 解析响应 JSON 为 `AnalysisResult`
  - 实现 `OpenAIClient` — 调用 OpenAI `/v1/chat/completions`：
    - 请求格式：`{"model":"...", "messages":[{"role":"user","content":[{"type":"text","text":"..."},{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,..."}}]}], "response_format":{"type":"json_object"}}`
  - 实现 `NewLLMClient(cfg)` — 根据 provider 返回对应实现
  - TDD: mock HTTP server 模拟 Ollama/OpenAI 响应，验证请求格式和响应解析

  **Must NOT do**:
  - 不要实现重试逻辑（T10 做）
  - 不要硬编码 prompt（从 config 读取）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 双协议实现需要理解两套 API 差异，HTTP mock 测试需要精心构造
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1（with T1-T4, T6-T7）
  - **Blocks**: T10, T14
  - **Blocked By**: T1, T3

  **Acceptance Criteria**:
  - [ ] `go test ./internal/analyzer/` → PASS
  - [ ] Ollama mock 测试：验证请求包含 `images` 字段和 base64 数据
  - [ ] OpenAI mock 测试：验证请求使用 `image_url` 格式和 `response_format`
  - [ ] 解析错误 LLM 响应返回明确 error

  **QA Scenarios**:

  ```
  Scenario: Ollama 客户端正确发送请求
    Tool: Bash (go test)
    Preconditions: mock HTTP server 已设置
    Steps:
      1. go test -v ./internal/analyzer/ -run TestOllamaClient_RequestFormat
       2. 验证请求 method=POST, path=/api/chat
      3. 验证请求体 model 字段存在, images 字段包含 base64 数据
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-5-ollama-request.txt

  Scenario: OpenAI 客户端正确发送请求
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/analyzer/ -run TestOpenAIClient_RequestFormat
      2. 验证请求 method=POST, path=/v1/chat/completions
      3. 验证 Authorization header 存在
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-5-openai-request.txt

  Scenario: 无效 JSON 响应返回错误
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/analyzer/ -run TestLLMClient_InvalidJSON
      2. mock server 返回 `{"not_valid"}`
    Expected Result: test PASS，error 不为 nil，包含 "parse" 或 "unmarshal"
    Evidence: .sisyphus/evidence/task-5-invalid-json.txt
  ```

  **Commit**: YES
  - Message: `feat(analyzer): implement LLM client with Ollama and OpenAI support`
  - Files: `internal/analyzer/`

- [x] 6. **图片解码器 — JPEG/PNG/WebP/HEIC**

  **What to do**:
  - 在 `internal/decoder/decoder.go` 定义 `ImageDecoder` 接口：`Decode(path string) (image.Image, string, error)`
  - JPEG/PNG：使用标准库 `image/jpeg`, `image/png`
  - WebP：使用 `golang.org/x/image/webp`
  - HEIC：使用纯 Go 方案（如 `github.com/nickalie/go-heif` 或 `github.com/strukturag/libheif/go`），避免 CGo 依赖。需调研库的维护状态和兼容性，如纯 Go 方案不可行则降级为 v2 feature
  - 实现 `DecodeImage(path)` — 根据文件扩展名选择解码器，返回 `image.Image` + MIME type
  - 需要提取 EXIF 数据：使用 `github.com/rwcarlsen/goexif/exif`
  - TDD: 为每种格式准备测试图片，验证解码成功 + EXIF 提取

  **Must NOT do**:
  - 不要做图片缩放、裁剪、转码

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: HEIC 解码需要处理 CGo/libheif 跨平台兼容性，EXIF 提取有字段多样性
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1（with T1-T5, T7）
  - **Blocks**: T8, T9, T14
  - **Blocked By**: T1

  **Acceptance Criteria**:
  - [ ] `go test ./internal/decoder/` → PASS
  - [ ] JPEG 测试：解码成功 + EXIF DateTimeOriginal 提取
  - [ ] PNG 测试：解码成功
  - [ ] WebP 测试：解码成功
  - [ ] HEIC 测试：解码成功（如纯 Go 方案不可行，标记为 v2 并跳过）
  - [ ] 损坏文件测试：返回 error（非 panic）

  **QA Scenarios**:

  ```
  Scenario: JPEG 解码 + EXIF 提取
    Tool: Bash (go test)
    Preconditions: testdata/sample.jpg 存在（含 EXIF）
    Steps:
      1. go test -v ./internal/decoder/ -run TestDecodeJPEG
      2. 验证返回 image.Image 非 nil
      3. 验证 EXIF DateTimeOriginal 非空
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-6-jpeg-decode.txt

  Scenario: 损坏文件不崩溃
    Tool: Bash (go test)
    Preconditions: 创建 0 字节文件
    Steps:
      1. go test -v ./internal/decoder/ -run TestDecodeCorrupt
      2. 验证返回 error，image.Image 为 nil
    Expected Result: test PASS，error 包含 "decode" 或 "invalid"
    Evidence: .sisyphus/evidence/task-6-corrupt.txt
  ```

  **Commit**: YES
  - Message: `feat(decoder): implement multi-format image decoder with EXIF extraction`
  - Files: `internal/decoder/`

- [x] 7. **文件监控类型 + 接口**

  **What to do**:
  - 在 `internal/watcher/` 定义接口和类型：
    - `FileEvent` — Path, Op(Create/Modify/Delete), Timestamp
    - `Watcher` 接口：`Watch(ctx, dirs []string, recursive bool) (<-chan FileEvent, error)`, `Close() error`
    - `Scanner` 接口：`Scan(ctx, dirs []string, existing map[string]int64) ([]string, error)` — 扫描目录返回新文件列表，existing 是已知文件(path→mtime)用于去重
    - `DedupFilter` — 基于 path + mtime + size 的去重
  - 创建 `existing.go` — `LoadExisting(paths []string) map[string]int64` 从磁盘加载已知文件状态
  - TDD: 定义接口后编写 mock 测试，验证接口契约

  **Must NOT do**:
  - 不要实现 fsnotify（T8 做）
  - 不要实现扫描逻辑（T9 做）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 接口定义 + 工具函数，无外部依赖
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1（with T1-T6）
  - **Blocks**: T8, T9
  - **Blocked By**: T1, T3

  **Acceptance Criteria**:
  - [ ] `go test ./internal/watcher/` → PASS
  - [ ] `DedupFilter` 测试：同一 path+mtime+size 的文件被过滤
  - [ ] `DedupFilter` 测试：mtime 变化的文件不被过滤
  - [ ] `LoadExisting` 测试：正确扫描目录返回文件列表

  **QA Scenarios**:

  ```
  Scenario: 去重过滤器正确过滤重复文件
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/watcher/ -run TestDedupFilter_Duplicate
      2. 创建 FileEvent{Path:"a.jpg", MTime:100, Size:1024} 两次
      3. 第二次应被过滤（不输出）
    Expected Result: test PASS，首次通过，重复被过滤
    Evidence: .sisyphus/evidence/task-7-dedup.txt

  Scenario: mtime 变化的文件不被过滤
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/watcher/ -run TestDedupFilter_Modified
      2. 第一次 FileEvent{Path:"a.jpg", MTime:100}
      3. 第二次 FileEvent{Path:"a.jpg", MTime:200}（mtime 变化）
    Expected Result: test PASS，两次都通过
    Evidence: .sisyphus/evidence/task-7-modified.txt
  ```

  **Commit**: YES
  - Message: `feat(watcher): define watcher interface and dedup types`
  - Files: `internal/watcher/`

- [x] 8. **递归 fsnotify 文件监控器**

  **What to do**:
  - 安装 `github.com/fsnotify/fsnotify`
  - 在 `internal/watcher/fsnotify.go` 实现 `FSNotifyWatcher`（实现 `Watcher` 接口）
  - 实现递归监控：`filepath.Walk` 遍历目录树，对每个子目录调用 `fsnotify.Watcher.Add()`
  - 运行时处理新子目录创建：监听到 `Create` 事件时判断是否为目录，如是则 `Add()` 该目录
  - 实现事件去抖（debounce）：短时间内同一文件的多次事件合并为一次（配置项 `Watch.DebounceMs`）
  - 文件过滤：仅处理配置的图片扩展名（.jpg/.jpeg/.png/.webp/.heic/.heif）
  - 通过 channel 发送 `FileEvent` 给下游消费者
  - TDD: 创建临时目录，写入文件，验证事件被正确捕获和去抖

  **Must NOT do**:
  - 不要做图片解码（T6 已有）
  - 不要做去重判断（Pipeline 层负责）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: fsnotify 递归监控涉及目录树遍历、运行时目录创建处理、事件去抖，逻辑较复杂

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2（with T9-T14）
  - **Blocks**: T14
  - **Blocked By**: T2, T3, T7

  **Acceptance Criteria**:
  - [ ] `go test ./internal/watcher/` → PASS（含 fsnotify 测试）
  - [ ] 递归监控测试：深层子目录中的文件创建被捕获
  - [ ] 运行时新目录创建测试：新子目录被自动加入监控
  - [ ] 去抖测试：100ms 内 5 次事件 → 仅输出 1 次
  - [ ] 非图片文件被忽略
  - [ ] 优雅关闭：`Close()` 后 channel 关闭，无 goroutine 泄漏

  **QA Scenarios**:

  ```
  Scenario: 文件创建产生事件
    Tool: Bash (go test)
    Preconditions: 临时监控目录已创建
    Steps:
      1. go test -v ./internal/watcher/ -run TestFSNotify_CreateEvent
      2. 在监控目录中 touch test.jpg
      3. 从 channel 读取事件，验证 Path 匹配
    Expected Result: test PASS，事件在 2s 内收到
    Evidence: .sisyphus/evidence/task-8-create-event.txt

  Scenario: 非图片文件被忽略
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/watcher/ -run TestFSNotify_IgnoreNonImage
      2. 在监控目录中 touch test.txt
      3. 2s 内 channel 无事件
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-8-ignore-nonimage.txt

  Scenario: 去抖合并多次事件
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/watcher/ -run TestFSNotify_Debounce
      2. 快速写入同一 test.jpg 3 次（50ms 间隔）
      3. 验证仅收到 1 个事件
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-8-debounce.txt
  ```

  **Commit**: YES
  - Message: `feat(watcher): implement recursive fsnotify file watcher with debounce`
  - Files: `internal/watcher/fsnotify.go`

- [x] 9. **初始目录扫描器**

  **What to do**:
  - 在 `internal/watcher/scanner.go` 实现 `DirectoryScanner`（实现 `Scanner` 接口）
  - 递归扫描配置的目录列表，收集所有支持的图片格式文件
  - 与去重映射对比：已有文件（path+mtime+size 匹配）跳过，新/修改文件加入返回列表
  - 按 mtime 降序排列，保证初始分析优先处理最新照片
  - 使用 `filepath.Walk` 流式处理，不将所有文件名加载到内存
  - 默认不跟随符号链接（配置项可开启）
  - TDD: 创建包含已知文件的临时目录，验证扫描正确、去重正确

  **Must NOT do**:
  - 不要触发 AI 分析（扫描器只返回文件列表）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 递归扫描 + 去重比较 + 排序，逻辑中等复杂度

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2（with T8, T10-T14）
  - **Blocks**: T14
  - **Blocked By**: T2, T7

  **Acceptance Criteria**:
  - [ ] `go test ./internal/watcher/` → PASS（含扫描测试）
  - [ ] 扫描测试目录返回正确的图片文件列表
  - [ ] 去重测试：已知文件不在结果中
  - [ ] 排序测试：结果按 mtime 降序
  - [ ] 空目录测试：返回空列表，无错误
  - [ ] 符号链接测试：默认不跟随

  **QA Scenarios**:

  ```
  Scenario: 扫描返回新文件
    Tool: Bash (go test)
    Preconditions: 临时目录有 3 JPEG、1 PNG、2 TXT
    Steps:
      1. go test -v ./internal/watcher/ -run TestScanner_NewFiles
      2. 传入空 existing 映射
    Expected Result: test PASS，返回 4 个图片文件，无 TXT
    Evidence: .sisyphus/evidence/task-9-scan-new.txt

  Scenario: 已知文件被跳过
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/watcher/ -run TestScanner_SkipExisting
      2. existing 映射包含其中一个 JPEG
    Expected Result: test PASS，返回 3 个文件（跳过了已有的）
    Evidence: .sisyphus/evidence/task-9-skip-existing.txt
  ```

  **Commit**: YES
  - Message: `feat(watcher): implement directory scanner with dedup`
  - Files: `internal/watcher/scanner.go`

- [x] 10. **LLM 分析器 — 提示词引擎 + 响应解析**

  **What to do**:
  - 在 `internal/analyzer/analyzer.go` 实现 `ImageAnalyzer`
  - 提示词构建：加载 `config.LLM.Prompt` 模板，注入图片 base64 数据
  - 默认提示词：分析图片中的物体、场景、颜色、氛围、人物特征（不要求识别人脸身份）
  - 要求 LLM 返回严格 JSON Schema：`{"description":"...","tags":["...","..."],"objects":["...","..."],"scene_type":"indoor|outdoor|unknown","colors":["..."],"people_count":0,"has_text":false}`
  - 响应解析：JSON 反序列化为 `AnalysisResult`，验证必填字段存在
  - 重试策略：指数退避（1s→2s→4s），最多 `MaxRetries` 次，仅对可重试错误（网络/5xx）重试
  - 图片预处理（LLM 专用，不修改原图）：大图（>2048px）在内存中等比缩放到合理尺寸，仅用于发送给 LLM，避免超出上下文窗口。缩放后的数据不写磁盘，不生成缩略图。
  - TDD: 对每个 LLM 客户端 mock，测试分析流程、重试、超时、大图压缩

  **Must NOT do**:
  - 不要让 LLM 识别人脸身份（仅计数）
  - 不要修改 LLM 客户端接口（T5 已有）
  - 不要将缩放后的图片写入磁盘（仅内存中处理，用于 LLM 请求）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 提示词工程 + Schema 验证 + 重试策略 + 图片预处理，多关注点

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2（with T8-T9, T11-T14）
  - **Blocks**: T14
  - **Blocked By**: T3, T5

  **Acceptance Criteria**:
  - [ ] `go test ./internal/analyzer/` → PASS
  - [ ] 有效 LLM 响应正确解析为 AnalysisResult
  - [ ] 缺少必填字段的响应返回 error（非 panic）
  - [ ] 重试测试：网络错误时重试 3 次后最终失败
  - [ ] 超时测试：超时后返回 error
  - [ ] 大图压缩测试：>2048px 图片被缩放

  **QA Scenarios**:

  ```
  Scenario: 有效 JSON 响应正确解析
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/analyzer/ -run TestAnalyzer_ValidResponse
      2. mock 返回: {"description":"a cat","tags":["cat"],"objects":["cat"],"scene_type":"indoor","colors":["white"],"people_count":0,"has_text":false}
    Expected Result: test PASS，AnalysisResult.Description="a cat"
    Evidence: .sisyphus/evidence/task-10-valid-response.txt

  Scenario: 重试后成功
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/analyzer/ -run TestAnalyzer_RetrySuccess
      2. mock 前 2 次 500，第 3 次 200 返回有效 JSON
    Expected Result: test PASS，最终成功，日志含 2 次重试
    Evidence: .sisyphus/evidence/task-10-retry.txt

  Scenario: 缺少必填字段返回错误
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/analyzer/ -run TestAnalyzer_MissingField
      2. mock 返回: {"description":"a cat"}（缺其他字段）
    Expected Result: test PASS，返回 error
    Evidence: .sisyphus/evidence/task-10-missing-field.txt
  ```

  **Commit**: YES
  - Message: `feat(analyzer): implement image analyzer with prompt engine and retry`
  - Files: `internal/analyzer/analyzer.go`

- [x] 11. **ES 索引器服务**

  **What to do**:
  - 在 `internal/indexer/indexer.go` 实现 `IndexerService`
  - `IndexPhoto(ctx, doc)` — upsert 到 ES（以 path hash 为 `_id`）
  - `UpdateStatus(ctx, path, status)` — 仅更新 status 字段，不覆盖分析结果
  - 并发安全：使用 ES `_seq_no` + `_primary_term` 乐观锁
  - `BulkIndex(ctx, docs)` — 批量索引，用于初始扫描
  - 断路器 + 重试：ES 不可用时入队到内存有界队列（容量可配），恢复后 flush
  - TDD: testcontainers ES 集成测试

  **Must NOT do**:
  - 不要实现搜索（T12 做）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: upsert + 乐观锁 + 断路器 + testcontainers，复杂度高

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2（with T8-T10, T12-T14）
  - **Blocks**: T14, T16-T19
  - **Blocked By**: T3, T4

  **Acceptance Criteria**:
  - [ ] `go test ./internal/indexer/` → PASS
  - [ ] upsert 测试：首次插入 + 再次更新均正确
  - [ ] 状态更新测试：仅 status 变化，description 不变
  - [ ] 批量索引测试：100 docs 批量写入成功
  - [ ] 断路器测试：ES 不可用时入队，恢复后 flush

  **QA Scenarios**:

  ```
  Scenario: upsert 正确更新
    Tool: Bash (go test)
    Preconditions: testcontainers ES 运行中
    Steps:
      1. go test -v ./internal/indexer/ -run TestIndexer_Upsert
      2. IndexPhoto(doc1) → ES 查询确认字段
      3. IndexPhoto(doc1 updated) → ES 查询确认字段已变
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-11-upsert.txt

  Scenario: 断路器恢复
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/indexer/ -run TestIndexer_CircuitBreaker
      2. 模拟 ES 断开 → IndexPhoto 入队 → 恢复连接 → flush 成功
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-11-circuit-breaker.txt
  ```

  **Commit**: YES
  - Message: `feat(indexer): implement ES indexing service with circuit breaker`
  - Files: `internal/indexer/indexer.go`

- [x] 12. **ES 搜索查询构建器**

  **What to do**:
  - 在 `internal/search/search.go` 实现 `SearchService`
  - `Search(ctx, req)` → ES DSL 查询：
    - 全文搜索: `multi_match` on `description`, `tags`, `objects`
    - 日期范围: `range` on `date_time_original`
    - 标签/物体/场景/相机: `terms`/`term` 过滤
    - 分页: `from` + `size`，排序: `date_time_original` desc
  - 结果高亮（可选）：高亮 description 匹配片段
  - TDD: testcontainers ES 中预索引数据，验证各类型搜索

  **Must NOT do**:
  - 不要实现语义/向量搜索

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 复杂 ES DSL 构建，多字段多条件组合

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2（with T8-T11, T13-T14）
  - **Blocks**: T16-T18
  - **Blocked By**: T3, T4

  **Acceptance Criteria**:
  - [ ] `go test ./internal/search/` → PASS
  - [ ] 全文搜索 "cat" 匹配正确
  - [ ] 日期范围过滤正确
  - [ ] 组合查询（query + 日期 + 场景）正确
  - [ ] 分页测试正确
  - [ ] 空结果返回空 hits

  **QA Scenarios**:

  ```
  Scenario: 全文搜索
    Tool: Bash (go test)
    Preconditions: testcontainers ES 有 3 个文档
    Steps:
      1. go test -v ./internal/search/ -run TestSearch_FullText
      2. Search({Query:"mountain"}) → hits > 0
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-12-fulltext.txt

  Scenario: 组合过滤
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/search/ -run TestSearch_Combined
      2. Search({Query:"sunset", DateFrom:"2024-06-01", SceneType:"outdoor"})
    Expected Result: test PASS，同时满足所有条件
    Evidence: .sisyphus/evidence/task-12-combined.txt
  ```

  **Commit**: YES
  - Message: `feat(search): implement ES search query builder with multi-filter`
  - Files: `internal/search/search.go`

- [x] 13. **静态文件服务器**

  **What to do**:
  - 在 `internal/static/server.go` 实现静态文件服务
  - 使用标准库 `http.FileServer`，映射 `/photos/...` → `PhotoBasePath/...`
  - 安全防护：防止路径遍历（`..` 逃逸），仅允许图片扩展名
  - 添加 `Cache-Control: public, max-age=86400` 头
  - TDD: `httptest` 验证文件服务、安全过滤

  **Must NOT do**:
  - 不要生成缩略图、不要转码

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准库静态文件服务 + 安全校验

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1b（with T2-T7）
  - **Blocks**: T25
  - **Blocked By**: T1, T2

  **Acceptance Criteria**:
  - [ ] `go test ./internal/static/` → PASS
  - [ ] GET /photos/test.jpg → 200, image/jpeg
  - [ ] GET /photos/../etc/passwd → 403/404
  - [ ] GET /photos/test.txt → 403
  - [ ] Cache-Control 头存在

  **QA Scenarios**:

  ```
  Scenario: 提供图片文件
    Tool: Bash (curl)
    Steps:
      1. 启动测试 HTTP server
      2. curl -s -o /dev/null -w "%{http_code}" http://localhost:PORT/photos/sample.jpg
    Expected Result: 200
    Evidence: .sisyphus/evidence/task-13-serve-image.txt

  Scenario: 路径遍历被阻止
    Tool: Bash (curl)
    Steps:
      1. curl -s -o /dev/null -w "%{http_code}" "http://localhost:PORT/photos/../../../etc/passwd"
    Expected Result: 403 或 404
    Evidence: .sisyphus/evidence/task-13-path-traversal.txt
  ```

  **Commit**: YES
  - Message: `feat(static): implement secure static file server for photos`
  - Files: `internal/static/server.go`

- [x] 14. **处理流水线编排器**

  **What to do**:
  - 在 `internal/pipeline/pipeline.go` 实现 `Pipeline`
  - 核心流程：watcher/scanner → decoder → analyzer → indexer
  - Worker pool: goroutine pool，并发数 = `LLM.Concurrency`
  - 有界队列 + 背压：队列满时阻塞 watcher 的 channel 读取（不丢弃事件），文件事件在 fsnotify 内部 buffer 排队，队列空闲后自动恢复
  - 状态机: unanalyzed → analyzing → analyzed/failed
  - 错误容忍：单张失败不影响后续
  - LLM 不可用降级：当 LLM 服务不可用时，标记照片 status=`pending_analysis`，入队到重试队列
  - 定期重试：后台 goroutine 每 5 分钟扫描 `pending_analysis` 队列，尝试重新分析（最大重试次数可配，默认 10 次）
  - 优雅关闭: SIGINT/SIGTERM → 停止接收 → 等待完成任务 → 关闭
  - TDD: mock 所有下游，验证编排逻辑

  **Must NOT do**:
  - 不要在 Pipeline 中实现业务逻辑

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: goroutine pool + 背压 + 状态机 + 优雅关闭

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2（with T8-T12, T20）
  - **Blocks**: T25
  - **Blocked By**: T8, T9, T10, T11, T2

  **Acceptance Criteria**:
  - [ ] `go test ./internal/pipeline/` → PASS
  - [ ] 正常流程: event → analyze → index
  - [ ] 错误容忍: analyzer 失败 → 标记 failed → 继续
  - [ ] 并发控制: concurrency=2 最多 2 个并发分析
  - [ ] 优雅关闭: ctx cancel → 无 goroutine 泄漏
  - [ ] LLM 不可用测试: analyzer 返回 error → status=pending_analysis → 重试后成功

  **QA Scenarios**:

  ```
  Scenario: 端到端处理
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/pipeline/ -run TestPipeline_E2E
      2. mock watcher 发送 1 个事件
      3. 验证 analyzer 被调用 1 次，indexer 被调用 1 次，status=analyzed
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-14-e2e.txt

  Scenario: 失败不阻塞
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/pipeline/ -run TestPipeline_Failure
      2. mock analyzer 第 1 张 error，第 2 张 success
      3. 验证第 1 张 status=failed，第 2 张正常处理
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-14-failure-tolerant.txt

  Scenario: LLM 不可用时降级
    Tool: Bash (go test)
    Steps:
      1. go test -v ./internal/pipeline/ -run TestPipeline_LLMUnavailable
      2. mock analyzer 返回 connection refused
      3. 验证照片 status=pending_analysis
      4. mock analyzer 恢复 → 重试后 status=analyzed
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-14-llm-unavailable.txt
  ```

  **Commit**: YES
  - Message: `feat(pipeline): implement processing pipeline with worker pool`
  - Files: `internal/pipeline/pipeline.go`

- [x] 15. **API 路由 + 中间件 + 健康检查**

  **What to do**:
  - 在 `internal/api/router.go` 使用 chi 构建路由
  - 中间件：`chi.Logger`, `chi.Recoverer`, `chi.Timeout(30s)`, CORS（允许前端跨域开发）
  - 健康检查端点：`GET /health` → `{"status":"ok","es":"connected","uptime":"...","photos_analyzed":0}`
  - 依赖注入：创建 `Server` 结构体，持有所有 service 的引用。使用 `HealthChecker` 接口获取 Pipeline 状态（可选，非阻塞）。
  - 子路由挂载：`/api/` 下挂载 photo/search/stats 子路由器
  - `/` 和 `/assets/*` 交由前端 SPA 的静态文件处理（dev 时 proxy 到 Vite）
  - TDD: `httptest` 测试健康检查、CORS 头

  **Must NOT do**:
  - 不要实现认证中间件

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: chi 路由 + 中间件链 + 子路由挂载 + 依赖注入

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3（with T16-T19）
  - **Blocks**: T16-T20, T25
  - **Blocked By**: T3, T11, T12

  **Acceptance Criteria**:
  - [ ] `go test ./internal/api/` → PASS
  - [ ] GET /health → 200, `{"status":"ok"}`
  - [ ] CORS 头存在（`Access-Control-Allow-Origin`）
  - [ ] 超时中间件生效（30s 后返回 504）

  **QA Scenarios**:

  ```
  Scenario: 健康检查
    Tool: Bash (curl)
    Steps:
      1. 启动测试 HTTP server
      2. curl -s http://localhost:PORT/health | jq
    Expected Result: 200, .status == "ok"
    Evidence: .sisyphus/evidence/task-15-health.txt

  Scenario: CORS 头
    Tool: Bash (curl)
    Steps:
      1. curl -sI http://localhost:PORT/health -H "Origin: http://localhost:5173"
    Expected Result: Access-Control-Allow-Origin: *
    Evidence: .sisyphus/evidence/task-15-cors.txt
  ```

  **Commit**: YES
  - Message: `feat(api): set up chi router with middleware and health check`
  - Files: `internal/api/router.go`

- [x] 16. **时间线 API 端点**

  **What to do**:
  - 在 `internal/api/photos.go` 实现 `GET /api/photos`
  - 查询参数：`date_from`, `date_to`, `status`, `page`, `page_size`
  - 调用 `SearchService` 按 `date_time_original` 降序查询
  - 响应格式：`{"photos":[...],"total":N,"page":1,"page_size":50}`
  - 每张照片返回：id, path, thumbnail_url（指向静态文件 `/photos/...`）, date, description, tags
  - 实现孤儿清理端点：`POST /api/photos/cleanup` — 扫描 ES 中引用已不存在的照片路径，删除对应 ES 文档
  - TDD: mock SearchService，验证分页、日期过滤

  **Must NOT do**:
  - 不要在 API 层做图片解码或分析

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: REST API 端点 + 查询参数解析 + 分页

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3（with T15, T17-T19）
  - **Blocks**: T21, T28a, T28b
  - **Blocked By**: T15, T11, T12

  **Acceptance Criteria**:
  - [ ] `go test ./internal/api/` → PASS
  - [ ] GET /api/photos → 200, 返回 JSON 数组
  - [ ] GET /api/photos?date_from=2024-01-01 → 仅返回该日期后的
  - [ ] GET /api/photos?page=2&page_size=10 → 返回第 2 页 10 条
  - [ ] 空结果 → `{"photos":[],"total":0}`

  **QA Scenarios**:

  ```
  Scenario: 时间线返回照片列表
    Tool: Bash (curl)
    Steps:
      1. curl -s http://localhost:PORT/api/photos | jq '.photos | length'
    Expected Result: 返回数组（可为空）
    Evidence: .sisyphus/evidence/task-16-timeline.txt

  Scenario: 分页正确
    Tool: Bash (curl)
    Steps:
      1. curl -s "http://localhost:PORT/api/photos?page=1&page_size=5" | jq '.page_size'
    Expected Result: 5
    Evidence: .sisyphus/evidence/task-16-pagination.txt
  ```

  **Commit**: YES
  - Message: `feat(api): implement photo timeline endpoint with pagination`
  - Files: `internal/api/photos.go`

- [x] 17. **搜索 API 端点**

  **What to do**:
  - 在 `internal/api/search.go` 实现 `POST /api/search`
  - 请求体：`SearchRequest` JSON（query, date_from, date_to, tags, objects, scene_type, camera_model, page, page_size）
  - 调用 `SearchService.Search()`，返回 `SearchResponse`
  - 查询参数验证：page ≥ 1, page_size ∈ [1, 100]
  - TDD: mock SearchService，验证查询参数传递、错误响应

  **Must NOT do**:
  - 不要在 API 层构建 ES 查询

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: POST 端点 + 请求体验证 + 多条件搜索

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3（with T15-T16, T18-T19）
  - **Blocks**: T22, T28a, T28b
  - **Blocked By**: T15, T11, T12

  **Acceptance Criteria**:
  - [ ] `go test ./internal/api/` → PASS
  - [ ] POST /api/search with `{"query":"cat"}` → 200
  - [ ] POST /api/search with `{"page_size":200}` → 400（超出上限）
  - [ ] POST /api/search with invalid JSON → 400
  - [ ] 响应含 `hits`, `total`, `page`

  **QA Scenarios**:

  ```
  Scenario: 搜索返回结果
    Tool: Bash (curl)
    Steps:
      1. curl -s -X POST http://localhost:PORT/api/search \
           -H "Content-Type: application/json" \
           -d '{"query":"mountain","date_from":"2024-01-01"}'
      2. 验证 .total >= 0
    Expected Result: 200，响应格式正确
    Evidence: .sisyphus/evidence/task-17-search.txt

  Scenario: 无效参数返回 400
    Tool: Bash (curl)
    Steps:
      1. curl -s -X POST http://localhost:PORT/api/search \
           -H "Content-Type: application/json" \
           -d '{"page_size":999}'
    Expected Result: 400，错误消息含 "page_size"
    Evidence: .sisyphus/evidence/task-17-invalid.txt
  ```

  **Commit**: YES
  - Message: `feat(api): implement search endpoint with validation`
  - Files: `internal/api/search.go`

- [x] 18. **照片详情 API 端点**

  **What to do**:
  - 在 `internal/api/photo_detail.go` 实现 `GET /api/photos/{id}`
  - 返回完整的 `PhotoDocument`：图片元数据 + EXIF + AI 分析结果
  - 图片 URL：拼接 `photo_url` 字段（`/photos/{relative_path}`）
  - TDD: mock indexer，验证返回完整文档

  **Must NOT do**:
  - 不要返回图片二进制数据（通过静态文件服务访问）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单条 GET 端点，简单查询 + 响应组装

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3（with T15-T17, T19）
  - **Blocks**: T23
  - **Blocked By**: T15, T11, T12

  **Acceptance Criteria**:
  - [ ] `go test ./internal/api/` → PASS
  - [ ] GET /api/photos/{id} → 200, 返回完整文档
  - [ ] 不存在的 id → 404

  **QA Scenarios**:

  ```
  Scenario: 获取照片详情
    Tool: Bash (curl)
    Steps:
      1. curl -s http://localhost:PORT/api/photos/some-id | jq '.description'
    Expected Result: 200，description 字段存在
    Evidence: .sisyphus/evidence/task-18-detail.txt

  Scenario: 不存在的 ID
    Tool: Bash (curl)
    Steps:
      1. curl -s -o /dev/null -w "%{http_code}" http://localhost:PORT/api/photos/nonexistent
    Expected Result: 404
    Evidence: .sisyphus/evidence/task-18-notfound.txt
  ```

  **Commit**: YES
  - Message: `feat(api): implement photo detail endpoint`
  - Files: `internal/api/photo_detail.go`

- [x] 19. **状态/统计/过滤器 API 端点**

  **What to do**:
  - 在 `internal/api/stats.go` 实现 `GET /api/stats`
  - 返回：照片总数、各状态计数（unanalyzed/analyzing/analyzed/failed）、分析速度（最近 N 分钟处理数）
  - 使用 ES aggregation 查询统计
  - 在 `internal/api/filters.go` 实现 `GET /api/filters`
  - 返回可用过滤器的聚合值：tags top 50, scene_type 枚举, camera_model top 20
  - 使用 ES aggregation 查询（terms aggregation）
  - TDD: mock 索引数据，验证聚合结果

  **Must NOT do**:
  - 不要实现复杂的仪表板

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单聚合端点

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3（with T15-T18）
  - **Blocks**: —
  - **Blocked By**: T15, T11

  **Acceptance Criteria**:
  - [ ] `go test ./internal/api/` → PASS
  - [ ] GET /api/stats → 200, 含 total, by_status, recent_throughput
  - [ ] GET /api/filters → 200, 含 tags, scenes, cameras 字段

  **QA Scenarios**:

  ```
  Scenario: 获取统计
    Tool: Bash (curl)
    Steps:
      1. curl -s http://localhost:PORT/api/stats | jq '.total'
    Expected Result: 200，total 为数字
    Evidence: .sisyphus/evidence/task-19-stats.txt

  Scenario: 获取过滤器选项
    Tool: Bash (curl)
    Steps:
      1. curl -s http://localhost:PORT/api/filters | jq '.tags | length'
    Expected Result: 200，tags 为数组
    Evidence: .sisyphus/evidence/task-19-filters.txt
  ```

  **Commit**: YES
  - Message: `feat(api): implement stats endpoint`
  - Files: `internal/api/stats.go`

- [x] 20. **React 项目脚手架 + API 客户端**

  **What to do**:
  - 在 `web/` 搭建 React + TypeScript + Vite 项目（脚手架由 T1 创建）
  - 补充依赖：`vitest`, `msw`（mock service worker 用于 API 测试）
  - 创建 `src/api/client.ts` — axios 实例，配置 baseURL、错误拦截
  - 创建 `src/api/photos.ts` — `fetchPhotos()`, `searchPhotos()`, `fetchPhotoDetail()`, `fetchStats()`
  - 创建 `src/types.ts` — 前端类型定义（对齐后端 types）
  - 配置 Vite proxy：开发时将 `/api`、`/photos`、`/health` 代理到 Go 后端
  - 创建基础布局：`App.tsx` + `Layout.tsx` + React Router 路由配置
  - TDD: 使用 vitest + msw mock 测试 API 客户端
  - 安装 Tailwind CSS（`tailwindcss` + `postcss` + `autoprefixer`）并配置
  - 创建基础 Layout 骨架：`Layout.tsx` 包含顶部导航栏（Logo + 时间线/搜索链接）和主内容区域，页面组件在 Layout 内渲染

  **Must NOT do**:
  - 不要实现具体的页面 UI（T21-T24 做）

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: React 脚手架 + API 客户端 + 路由，前端基础架构
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2（with T8-T14）
  - **Blocks**: T21-T24
  - **Blocked By**: T1

  **Acceptance Criteria**:
  - [ ] `cd web && npm run build` → 成功
  - [ ] `cd web && npx vitest run` → PASS
  - [ ] Vite dev server 启动成功（npm run dev）
  - [ ] API 客户端 mock 测试通过
  - [ ] Tailwind CSS 配置正确，`npm run build` 无错误
  - [ ] Layout 组件渲染，导航链接可见

  **QA Scenarios**:

  ```
  Scenario: React 项目构建
    Tool: Bash (npm)
    Steps:
      1. cd web && npm install && npm run build
      2. 验证 dist/index.html 存在
    Expected Result: 构建成功
    Evidence: .sisyphus/evidence/task-20-react-build.txt

  Scenario: API 客户端 mock 测试
    Tool: Bash (vitest)
    Steps:
      1. cd web && npx vitest run
    Expected Result: 所有测试 PASS
    Evidence: .sisyphus/evidence/task-20-api-client-test.txt
  ```

  **Commit**: YES
  - Message: `feat(web): scaffold React project with API client and routing`
  - Files: `web/`

- [x] 21. **时间线页面 — 按日期分组 + 无限滚动**

  **What to do**:
  - 在 `web/src/pages/Timeline.tsx` 实现时间线页面
  - 使用 `@tanstack/react-query` 的 `useInfiniteQuery` 加载照片（分页）
  - 按日期分组：同一天的照片归入一组，显示日期标题
  - 照片卡片：缩略图（原图 CSS 缩略，不生成新文件）+ 简要描述 + 标签
  - 无限滚动：IntersectionObserver 检测底部，加载下一页
  - 加载状态：Skeleton 骨架屏；空状态提示
  - 响应式：桌面 4 列，平板 3 列，手机 1 列

  **Must NOT do**:
  - 不要生成缩略图文件（CSS 限制图片尺寸即可）

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4（with T22-T24）
  - **Blocks**: T28b
  - **Blocked By**: T16, T20

  **QA Scenarios**:

  ```
  Scenario: 时间线显示照片
    Tool: Playwright
    Steps:
      1. page.goto("http://localhost:PORT/")
      2. 等待 .photo-card 出现（timeout: 10s）
      3. 验证 .photo-card 数量 > 0
    Expected Result: 照片卡片可见
    Evidence: .sisyphus/evidence/task-21-timeline.png

  Scenario: 空状态
    Tool: Playwright
    Steps:
      1. page.goto("http://localhost:PORT/")
      2. 验证 "还没有照片" 文案
    Expected Result: 空状态提示
    Evidence: .sisyphus/evidence/task-21-empty.png

  Scenario: 无限滚动
    Tool: Playwright
    Steps:
      1. page.goto("http://localhost:PORT/")
      2. 记录初始 .photo-card 数量
      3. 滚动到底部，等待数量增加
    Expected Result: 照片数量增加
    Evidence: .sisyphus/evidence/task-21-scroll.png
  ```

  **Commit**: YES
  - Message: `feat(web): implement timeline page with infinite scroll`
  - Files: `web/src/pages/Timeline.tsx`

- [x] 22. **搜索页面 — 全文搜索 + 过滤器**

  **What to do**:
  - 在 `web/src/pages/Search.tsx` 实现搜索页面
  - 搜索栏 + 过滤器面板（日期范围、场景类型、标签多选、物体多选）
  - 搜索结果：与时间线相同的卡片布局
  - URL 同步：搜索参数同步到 URL query string
  - Loading + 空结果状态
  - 过滤器选项从 ES 获取已有值

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4（with T21, T23-T24）
  - **Blocks**: T28b
  - **Blocked By**: T17, T20

  **QA Scenarios**:

  ```
  Scenario: 搜索 "cat"
    Tool: Playwright
    Steps:
      1. page.goto("http://localhost:PORT/search")
      2. 输入 "cat"，点击搜索
      3. 验证 .photo-card 出现
    Expected Result: 显示匹配结果
    Evidence: .sisyphus/evidence/task-22-search.png

  Scenario: 无结果
    Tool: Playwright
    Steps:
      1. 搜索 "xyznonexistent123"
      2. 验证 "未找到" 文案
    Expected Result: 无结果提示
    Evidence: .sisyphus/evidence/task-22-empty.png
  ```

  **Commit**: YES
  - Message: `feat(web): implement search page with multi-filter`
  - Files: `web/src/pages/Search.tsx`

- [x] 23. **照片详情弹窗**

  **What to do**:
  - 在 `web/src/components/PhotoDetail.tsx` 实现详情模态框
  - 显示大图 + AI 描述 + 标签 + 物体 + 场景 + EXIF
  - 键盘：ESC 关闭，左右箭头切换
  - URL 路由：`/photos/{id}` 直接访问
  - Loading + 错误状态

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4（with T21-T22, T24）
  - **Blocks**: —
  - **Blocked By**: T18, T20

  **QA Scenarios**:

  ```
  Scenario: 打开详情
    Tool: Playwright
    Steps:
      1. page.goto("http://localhost:PORT/")
      2. 点击第一个 .photo-card
      3. 验证 .photo-detail-modal 可见
    Expected Result: 弹窗显示完整信息
    Evidence: .sisyphus/evidence/task-23-detail.png

  Scenario: ESC 关闭
    Tool: Playwright
    Steps:
      1. 打开详情 → 按 Escape
      2. 验证 .photo-detail-modal 不可见
    Expected Result: 弹窗关闭
    Evidence: .sisyphus/evidence/task-23-esc.png
  ```

  **Commit**: YES
  - Message: `feat(web): implement photo detail modal`
  - Files: `web/src/components/PhotoDetail.tsx`

- [x] 24. **响应式微调 + 404 + 错误边界**

  **What to do**:
  - 完善 `web/src/components/Layout.tsx` 的响应式行为：移动端汉堡菜单、路由高亮
  - 404 页面（`NotFound.tsx`）
  - 全局错误边界（`ErrorBoundary.tsx`）
  - 响应式微调：确保各页面在桌面/平板/手机端布局正确

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4（with T21-T23）
  - **Blocks**: —
  - **Blocked By**: T20, T21, T22, T23

  **QA Scenarios**:

  ```
  Scenario: 导航跳转
    Tool: Playwright
    Steps:
      1. page.goto("http://localhost:PORT/")
      2. 点击 "搜索" → URL 变为 /search
    Expected Result: 导航正确
    Evidence: .sisyphus/evidence/task-24-nav.png
  ```

  **Commit**: YES
  - Message: `feat(web): implement global layout with responsive navigation`
  - Files: `web/src/components/Layout.tsx`

- [x] 25. **主入口点 — 完整装配**

  **What to do**:
  - 完成 `cmd/phosche/main.go`：读取配置 → 初始化所有模块 → 装配 Pipeline → 启动 HTTP server → 等待信号 → 优雅关闭
  - 信号处理：`signal.NotifyContext` 监听 SIGINT/SIGTERM
  - 优雅关闭顺序：先停 Pipeline（停止接收新任务），再停 HTTP server（等待现有请求完成），最后关闭 ES 连接
  - 前端静态文件服务：生产模式将 `web/dist/` 嵌入（使用 `embed` 包）或从文件系统提供
  - 开发模式：配置 `DevMode: true` 时不嵌入前端，由 Vite dev server 独立运行
  - 启动日志：打印配置摘要（端口、ES 地址、LLM provider、监控目录列表）
  - TDD: 集成测试验证启动、健康检查、信号关闭

  **Must NOT do**:
  - 不要在 main.go 中写业务逻辑

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 全部模块装配 + 优雅关闭 + 前端嵌入 + dev/prod 模式

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5（with T26-T28b）
  - **Blocks**: T26, T27, T28a, T28b
  - **Blocked By**: T13, T14, T15

  **QA Scenarios**:

  ```
  Scenario: 服务启动 + 健康检查
    Tool: Bash + curl
    Steps:
      1. go build -o /tmp/phosche ./cmd/phosche/
      2. CONFIG_PATH=config.example.yaml /tmp/phosche &
      3. sleep 2
      4. curl -s http://localhost:8080/health | jq .status
      5. kill %1
    Expected Result: "ok"
    Evidence: .sisyphus/evidence/task-25-startup.txt

  Scenario: 优雅关闭
    Tool: Bash
    Steps:
      1. 启动服务
      2. kill -TERM PID
      3. 验证进程在 5s 内退出
      4. 验证日志含 "shutting down"
    Expected Result: 进程正常退出，exit 0
    Evidence: .sisyphus/evidence/task-25-shutdown.txt
  ```

  **Commit**: YES
  - Message: `feat(main): wire up all modules with graceful shutdown`
  - Files: `cmd/phosche/main.go`

- [x] 26. **Docker Compose 编排**

  **What to do**:
  - 创建 `docker-compose.yaml`：
    - `elasticsearch` 服务：官方 `docker.elastic.co/elasticsearch/elasticsearch:8.x` 镜像
      - 环境变量：`discovery.type=single-node`, `xpack.security.enabled=false`
      - 健康检查：`curl -s http://localhost:9200/_cluster/health`
      - 数据持久化：`esdata` volume
    - `phosche` 服务：Go 二进制（或 Dockerfile 构建）
      - 依赖 `elasticsearch`（`depends_on` + `condition: service_healthy`）
      - 端口映射：`8080:8080`
      - 挂载配置：`./config.yaml:/app/config.yaml:ro`
      - 挂载照片目录：`./photos:/photos:ro`（示例）
    - 可选：`ollama` 服务（如使用本地 LLM）
  - 创建 `.env.example` 文件
  - 验证：`docker compose up` 后两个服务均健康

  **Must NOT do**:
  - 不要在 docker-compose 中硬编码敏感信息

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: YAML 编排文件 + 标准 Docker Compose 配置

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5（with T25, T27-T28b）
  - **Blocks**: —
  - **Blocked By**: T25

  **QA Scenarios**:

  ```
  Scenario: Docker Compose 启动
    Tool: Bash (docker compose)
    Preconditions: Docker 已安装并运行
    Steps:
      1. docker compose up -d
      2. sleep 30（等待 ES 启动）
      3. docker compose ps
      4. 验证所有服务 "Up" 或 "healthy"
    Expected Result: 所有服务运行中
    Evidence: .sisyphus/evidence/task-26-compose.txt
  ```

  **Commit**: YES
  - Message: `feat(deploy): add Docker Compose configuration`
  - Files: `docker-compose.yaml`, `.env.example`

- [x] 27. **Dockerfile + 单二进制构建**

  **What to do**:
  - 创建多阶段 `Dockerfile`：
    - Stage 1 (build): 使用 `golang:1.26-alpine` 编译 Go 二进制
      - 需要 `libheif-dev` 用于 HEIC 解码（CGo）
    - Stage 2 (frontend): 使用 `node:20-alpine` 构建 React（`npm run build`）
    - Stage 3 (runtime): 使用 `alpine:latest`
      - 安装 `libheif` 运行时库
      - 复制 Go 二进制 + `web/dist/` + 默认 `config.yaml`
      - `ENTRYPOINT ["/app/phosche"]`
  - 创建 `Makefile`（可选）：
    - `make build` — 编译 Go 二进制
    - `make build-frontend` — 构建前端
    - `make docker-build` — 构建 Docker 镜像
  - 验证：`docker build -t phosche .` + `docker run phosche`

  **Must NOT do**:
  - 不要使用过于复杂的构建脚本

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准多阶段 Dockerfile + Makefile

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5（with T25-T26, T28a-T28b）
  - **Blocks**: —
  - **Blocked By**: T25

  **QA Scenarios**:

  ```
  Scenario: Docker 构建成功
    Tool: Bash (docker)
    Preconditions: Docker 已安装
    Steps:
      1. docker build -t phosche:test .
      2. 验证镜像存在
    Expected Result: 构建成功，镜像大小合理
    Evidence: .sisyphus/evidence/task-27-build.txt

  Scenario: 单二进制构建
    Tool: Bash (go build)
    Steps:
      1. go build -o /tmp/phosche ./cmd/phosche/
      2. ls -la /tmp/phosche
      3. file /tmp/phosche
    Expected Result: 可执行文件存在
    Evidence: .sisyphus/evidence/task-27-binary.txt
  ```

  **Commit**: YES
  - Message: `feat(deploy): add Dockerfile and Makefile`
  - Files: `Dockerfile`, `Makefile`

- [x] 28a. **后端端到端集成测试**

  **What to do**:
  - 在 `internal/integration/` 创建端到端测试
  - 使用 `testcontainers-go` 启动 ES 容器
  - 创建临时目录（`t.TempDir()`），放入测试 JPEG（含 EXIF）
  - 启动完整的 phosche 服务（使用测试配置）
  - 测试流程：
    1. 服务启动 → 健康检查通过
    2. 初始扫描发现测试图片
    3. mock LLM 分析（注入预设 analysis result）
    4. 轮询 `/api/photos` 直到照片状态变为 `analyzed`（超时 30s）
    5. 验证 `/api/search` 能搜索到该照片
    6. 验证 `/api/photos/{id}` 返回完整数据
  - TDD: 这是最终的集成验收测试

  **Must NOT do**:
  - 不要测试真实 LLM（mock 替代）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 全栈集成测试，涉及 testcontainers + mock LLM + 文件监控 + ES + API

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5
  - **Blocks**: —
  - **Blocked By**: T16, T17, T25, T4

  **QA Scenarios**:

  ```
  Scenario: 后端端到端流程
    Tool: Bash (go test)
    Preconditions: Docker 已运行
    Steps:
      1. go test -v -run TestEndToEnd ./internal/integration/ -timeout 120s
      2. 验证 service 启动
      3. 验证 photo discovered → analyzed → searchable
    Expected Result: test PASS
    Evidence: .sisyphus/evidence/task-28a-e2e.txt
  ```

  **Commit**: YES
  - Message: `test(integration): add backend end-to-end integration test`
  - Files: `internal/integration/`

- [x] 28b. **前端端到端集成测试**

  **What to do**:
  - 在 `web/e2e/` 创建 Playwright E2E 测试
  - 启动完整的 phosche 服务（后端 + 前端）
  - 测试流程：
    1. 打开时间线页面 → 页面正常渲染
    2. 搜索功能 → 输入关键词 → 显示结果
    3. 点击照片 → 详情弹窗打开
    4. 导航 → 时间线/搜索页面切换正常
  - TDD: Playwright 测试脚本

  **Must NOT do**:
  - 不要测试真实 LLM（使用 mock）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Playwright E2E 测试，涉及前后端集成

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 5
  - **Blocks**: —
  - **Blocked By**: T21, T22, T25

  **QA Scenarios**:

  ```
  Scenario: 前端 E2E 完整流程
    Tool: Playwright
    Steps:
      1. 启动服务
      2. 打开 http://localhost:8080/
      3. 验证时间线页面渲染
      4. 导航到搜索页面
      5. 搜索关键词
      6. 点击结果 → 详情弹窗
    Expected Result: 所有步骤无报错
    Evidence: .sisyphus/evidence/task-28b-frontend-e2e.png
  ```

  **Commit**: YES
  - Message: `test(e2e): add frontend end-to-end integration test`
  - Files: `web/e2e/`

---

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.
>
> **Do NOT auto-proceed after verification. Wait for user's explicit approval before marking work complete.**
> **Never mark F1-F4 as checked before getting user's okay.** Rejection or user feedback → fix → re-run → present again → wait for okay.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, curl endpoint, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in `.sisyphus/evidence/`. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `gofmt -d .` + `go test ./...`. Review all changed files for: `interface{}` instead of `any`, empty error handling, `println` in prod, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names (data/result/item/temp).
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [ ] F3. **Real Manual QA** — `unspecified-high` (+ `playwright` skill)
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test cross-task integration (features working together, not isolation). Test edge cases: empty state, invalid input, rapid actions. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination: Task N touching Task M's files. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **T1-T7 (Wave 1)**: 7 separate commits, one per task — `feat(scaffold): initialize Go module and React project` etc.
- **T8-T14 (Wave 2)**: 7 separate commits — `feat(watcher): implement recursive fsnotify file watcher` etc.
- **T15-T19 (Wave 3)**: 5 commits — API layer grouped by endpoint
- **T20-T24 (Wave 4)**: 5 commits — Frontend grouped by page
- **T25-T28b (Wave 5)**: 5 commits — Integration and deployment (T28 split into T28a + T28b)

---

## Success Criteria

### Verification Commands
```bash
# 1. Build & Test
go build -o phosche ./cmd/phosche/  # Expected: exit 0, binary exists
go test ./...                        # Expected: PASS, no failures
go vet ./...                         # Expected: no output

# 2. Service Health
curl http://localhost:8080/health    # Expected: {"status":"ok"}

# 3. Photo Indexing (after adding test JPEG)
curl http://localhost:8080/api/photos | jq '.photos | length'  # Expected: > 0

# 4. Search
curl -X POST http://localhost:8080/api/search \
  -H "Content-Type: application/json" \
  -d '{"query":"test","date_from":"2024-01-01"}' \
  | jq '.hits.total.value'          # Expected: number

# 5. Frontend
curl http://localhost:8080/          # Expected: HTML with React mount point

# 6. Docker
docker compose up -d                 # Expected: services healthy
docker compose ps                    # Expected: all "Up"
```

### Final Checklist
- [ ] All 8 "Must Have" items present
- [ ] All 12 "Must NOT Have" items absent
- [ ] All tests pass (`go test ./...`)
- [ ] Docker Compose 一键启动
- [ ] 单二进制可执行
- [ ] 配置文件完整可解析
- [ ] 所有 QA scenarios 执行通过
- [ ] F1-F4 全部 APPROVE
- [ ] 用户显式确认 "okay"
