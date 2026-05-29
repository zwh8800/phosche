# Phosche — 个人照片搜索服务

> 自动监控照片目录，通过多模态 AI 分析照片内容，构建可全文搜索的个人照片库。

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)
![License](https://img.shields.io/badge/License-MIT-blue)
![Elasticsearch](https://img.shields.io/badge/Elasticsearch-8.x-005571?logo=elasticsearch)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react)

---

## 功能特性

- **自动文件监控** — 使用 fsnotify 递归监控指定目录，新照片出现后自动触发处理流水线
- **多模态 AI 分析** — 支持 Ollama 本地模型（llama3.2-vision）和 OpenAI API（GPT-4o），分析照片内容并提取描述、标签、场景类型、颜色、人数、文本等信息
- **结构化响应** — LLM 返回 JSON 格式的结构化分析结果，包含 description、tags、objects、scene_type、colors、people_count、has_text、confidence 等字段
- **Elasticsearch 全文搜索** — 基于 ES 8.x 构建索引，支持关键词搜索、日期范围过滤、标签/对象/场景类型/相机型号筛选
- **时间线浏览** — Web 界面按日期分组展示照片，支持无限滚动加载
- **照片详情页** — 查看单张照片的 EXIF 元数据（相机型号、镜头、光圈、ISO、GPS 等）和 AI 分析结果
- **数据统计** — 实时统计照片总数及各处理状态的数量分布
- **断路器保护** — ES 不可用时自动打开断路器，内存有界队列缓冲写入请求，恢复后自动排水
- **LLM 降级与重试** — LLM 不可用时将照片标记为 pending_analysis，每隔 5 分钟自动重试，最多重试 10 次
- **错误隔离** — 损坏图片优雅跳过（仅记录日志），不导致服务崩溃
- **优雅关闭** — SIGINT/SIGTERM 触发优雅关闭：停止接收新任务，等待当前处理完成，释放所有资源
- **多格式图片解码** — 支持 JPEG、PNG、WebP、HEIC/HEIF 格式，自动提取 EXIF 元数据
- **路径遍历防护** — 静态文件服务严格校验请求路径，防止目录穿越攻击
- **Docker Compose 一键部署** — 内置编排文件，可一键启动 ES + phosche + 可选 Ollama

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端语言 | Go 1.26 |
| HTTP 路由 | chi/v5 |
| 结构化日志 | log/slog |
| 搜索引擎 | Elasticsearch 8.x（官方 go-elasticsearch 客户端） |
| AI 分析 | Ollama（本地）/ OpenAI（云端），双协议统一接口 |
| 图片解码 | 标准库 + golang.org/x/image/webp + gen2brain/heic |
| EXIF 提取 | rwcarlsen/goexif |
| 文件监控 | fsnotify |
| 前端框架 | React 19 + TypeScript 6 |
| 构建工具 | Vite 8 |
| 样式方案 | Tailwind CSS 4 |
| 状态管理 | TanStack React Query 5 |
| HTTP 客户端 | axios |
| 路由 | react-router-dom 7 |
| 容器化 | Docker（多阶段构建）+ Docker Compose |
| 测试 | Go 测试框架 + testcontainers-go + Playwright |

---

## 快速开始

### 前置要求

- Go 1.26 或更高版本（手动启动时需要）
- Node.js 18 或更高版本（构建前端时需要）
- Docker 和 Docker Compose（推荐，一键启动方式）
- Elasticsearch 8.x（手动启动时需要）
- Ollama（可选，使用本地 AI 分析时需要）

### 获取代码

```bash
git clone https://github.com/zwh8800/phosche.git
cd phosche
```

### 配置

```bash
cp config.example.yaml config.yaml
# 根据你的环境编辑 config.yaml
```

最低配置需要修改 `watch.directories`（照片目录路径）、`server.photo_base_path`（图片文件根目录）和 `elasticsearch.addresses`。

### 启动方式

#### 方式一：Docker Compose（推荐）

```bash
# 修改 config.yaml 中的配置，确保路径与容器内挂载一致
vim config.yaml

# 启动 Elasticsearch 和 phosche
docker compose up -d

# 如果使用本地 Ollama，加上 ollama profile
docker compose --profile ollama up -d
```

Docker Compose 会依次启动 Elasticsearch（等待健康检查通过）和 phosche 服务。图片目录通过 volume 挂载到容器中。

#### 方式二：手动启动

1. 确保 Elasticsearch 已启动并可访问
2. 配置 config.yaml，确保 `elasticsearch.addresses` 指向正确的 ES 地址
3. 构建前端并启动服务：

```bash
# 构建前端静态资源（开发模式下可跳过）
make build-frontend

# 启动服务
go run ./cmd/phosche/ -config config.yaml
```

服务默认监听 `0.0.0.0:8080`。开发模式下（`dev_mode: true`），前端由 Vite 独立运行，后端不提供 SPA 静态文件。

#### 方式三：单独使用 Docker 启动

适用于 Elasticsearch 已单独部署（或使用云服务）的场景，通过 `docker run` 启动 phosche 容器。

**1. 构建镜像：**

```bash
docker build -t phosche .
```

**2. 准备配置文件：**

```bash
cp config.example.yaml config.yaml
vim config.yaml
```

关键配置项（与 Docker Compose 方式不同，这里的 ES 地址指向外部服务）：

```yaml
elasticsearch:
  addresses:
    - https://your-es-cloud.example.com:9200   # 外部 ES 地址
  username: "your-username"                     # 如启用了认证
  password: "your-password"
  insecure_skip_verify: false

server:
  dev_mode: false    # 关闭开发模式，使用内嵌前端

llm:
  provider: openai   # 或 ollama（见下方网络说明）
```

**3. 启动容器：**

```bash
docker run -d \
  --name phosche \
  -p 8080:8080 \
  -v "$(pwd)/config.yaml:/app/config.yaml:ro" \
  -v /path/to/your/photos:/photos:ro \
  -e CONFIG_PATH=/app/config.yaml \
  phosche
```

**参数说明：**

| 参数 | 说明 |
|------|------|
| `-d` | 后台运行 |
| `--name phosche` | 容器名称 |
| `-p 8080:8080` | 端口映射（主机:容器） |
| `-v ./config.yaml:/app/config.yaml:ro` | 挂载配置文件（只读） |
| `-v /path/to/photos:/photos:ro` | 挂载照片目录（只读），路径需与 `server.photo_base_path` 和 `watch.directories` 配置一致 |
| `-e CONFIG_PATH=/app/config.yaml` | 指定配置文件路径 |
| `phosche` | 镜像名称 |

**访问服务：** `http://localhost:8080`

**访问本地 Ollama（可选）：**

如果 LLM provider 配置为 `ollama` 且 Ollama 运行在宿主机上，需要让容器能够访问宿主机的网络：

```bash
# macOS / Windows（Docker Desktop）
docker run -d \
  --name phosche \
  --add-host host.docker.internal:host-gateway \
  -p 8080:8080 \
  -v "$(pwd)/config.yaml:/app/config.yaml:ro" \
  -v /path/to/your/photos:/photos:ro \
  -e CONFIG_PATH=/app/config.yaml \
  phosche
```

然后在 `config.yaml` 中将 Ollama 地址改为 `http://host.docker.internal:11434`：

```yaml
llm:
  provider: ollama
  ollama:
    base_url: http://host.docker.internal:11434
```

> **Linux 宿主机**：使用 `--network host` 替代端口映射和 `--add-host`，Ollama 地址填 `http://localhost:11434`。

**常用管理命令：**

```bash
# 查看日志
docker logs -f phosche

# 停止容器
docker stop phosche

# 重启容器（修改配置后）
docker restart phosche

# 删除容器
docker rm phosche
```

---

## 配置说明

配置文件为 YAML 格式，完整配置项如下：

### `watch` — 文件监控

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `directories` | `[]string` | 必填 | 监控的照片目录路径列表 |
| `recursive` | `bool` | `true` | 是否递归监控子目录 |
| `debounce_ms` | `int` | `500` | 文件事件的去抖间隔（毫秒），同一文件在间隔内的多次修改会被合并 |
| `min_dir_depth` | `int` | `1` | 最小监控目录深度 |

### `llm` — AI 分析

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `provider` | `string` | `"ollama"` | LLM 提供商，可选 `"ollama"` 或 `"openai"` |
| `ollama.base_url` | `string` | `"http://localhost:11434"` | Ollama 服务地址 |
| `ollama.model` | `string` | `"llama3.2-vision"` | Ollama 视觉模型名称 |
| `openai.api_key` | `string` | `""` | OpenAI API 密钥 |
| `openai.base_url` | `string` | `"https://api.openai.com/v1"` | OpenAI API 地址（可替换为兼容接口） |
| `openai.model` | `string` | `"gpt-4o"` | OpenAI 模型名称 |
| `prompt` | `string` | 内置默认提示词 | 发送给 LLM 的提示词，要求返回 JSON 格式的结构化分析结果 |
| `max_retries` | `int` | `3` | 分析失败时的最大重试次数 |
| `concurrency` | `int` | `2` | 同时进行的 AI 分析任务数 |
| `timeout_seconds` | `int` | `60` | 单次 AI 分析请求的超时时间（秒） |
| `output_language` | `string` | `"zh"` | 输出语言，影响分析结果描述的语言 |

### `elasticsearch` — 搜索引擎

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `addresses` | `[]string` | 必填 | Elasticsearch 节点地址列表 |
| `username` | `string` | `""` | ES 认证用户名 |
| `password` | `string` | `""` | ES 认证密码 |
| `insecure_skip_verify` | `bool` | `false` | 是否跳过 TLS 证书验证 |
| `index_name` | `string` | `"phosche"` | ES 索引名称 |

### `server` — HTTP 服务

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `host` | `string` | `"0.0.0.0"` | 监听地址 |
| `port` | `int` | `8080` | 监听端口 |
| `photo_base_path` | `string` | 必填 | 照片文件在文件系统中的根目录，用于静态文件服务 |
| `dev_mode` | `bool` | `true` | 开发模式开关，开启时不提供 SPA 静态文件 |

---

## API 文档

所有 API 端点以 `/api/` 为前缀（健康检查除外），返回 JSON 格式响应。

### 健康检查

```
GET /health
```

**响应示例：**
```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

### 照片时间线

```
GET /api/photos?page=1&page_size=50&date_from=2024-01-01&date_to=2024-12-31&status=analyzed
```

**查询参数：**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `page` | `int` | `1` | 页码，从 1 开始 |
| `page_size` | `int` | `50` | 每页数量 |
| `date_from` | `string` | 可选 | 起始日期，格式 `YYYY-MM-DD` |
| `date_to` | `string` | 可选 | 结束日期，格式 `YYYY-MM-DD` |
| `status` | `string` | 可选 | 按状态过滤：`unanalyzed`、`analyzing`、`analyzed`、`failed`、`pending_analysis` |

**响应：** 同搜索响应格式。

### 清理孤儿文档

```
POST /api/photos/cleanup
```

扫描 ES 索引中所有照片文档，删除文件系统中已不存在的记录。

**响应示例：**
```json
{
  "deleted": 5
}
```

### 照片详情

```
GET /api/photos/{id}
```

`{id}` 是照片路径的 SHA-256 哈希值。

**响应示例：**
```json
{
  "id": "abc123...",
  "path": "/photos/2024/01/IMG_0001.jpg",
  "mtime": 1704067200,
  "size": 3842048,
  "status": "analyzed",
  "analyzed_at": 1704153600,
  "created_at": 1704067200,
  "exif": {
    "date_time_original": "2024-01-01T10:30:00Z",
    "camera_model": "iPhone 15 Pro",
    "lens_model": "iPhone 15 Pro back camera 6.86mm f/1.78",
    "focal_length": "6.9mm",
    "aperture": "f/1.8",
    "iso": 100,
    "gps_lat": 39.9042,
    "gps_lon": 116.4074
  },
  "description": "一张在公园里拍摄的照片，阳光明媚，草地上有几个人在野餐",
  "tags": ["公园", "野餐", "阳光", "草地"],
  "objects": ["人", "草地", "树木"],
  "scene_type": "outdoor",
  "colors": ["绿色", "蓝色", "白色"],
  "people_count": 3,
  "has_text": false,
  "confidence": 0.95,
  "photo_url": "/photos//photos/2024/01/IMG_0001.jpg"
}
```

### 全文搜索

```
POST /api/search
```

**请求体：**

```json
{
  "query": "海滩日落",
  "date_from": "2024-01-01",
  "date_to": "2024-12-31",
  "tags": ["旅行", "风景"],
  "objects": ["大海", "沙滩"],
  "scene_type": "outdoor",
  "camera_model": "iPhone 15 Pro",
  "status": "analyzed",
  "page": 1,
  "page_size": 20
}
```

**请求字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `query` | `string` | 全文搜索关键词，匹配描述、标签和检测到的物体 |
| `date_from` | `string` | 起始拍摄日期 |
| `date_to` | `string` | 结束拍摄日期 |
| `tags` | `[]string` | 标签过滤列表 |
| `objects` | `[]string` | 检测到的物体过滤列表 |
| `scene_type` | `string` | 场景类型过滤：`indoor`、`outdoor`、`unknown` |
| `camera_model` | `string` | 相机型号过滤 |
| `status` | `string` | 处理状态过滤 |
| `page` | `int` | 页码（默认 1） |
| `page_size` | `int` | 每页数量（默认 20，最大 100） |

**响应示例：**

```json
{
  "hits": [
    {
      "id": "abc123...",
      "path": "/photos/vacation/beach.jpg",
      "mtime": 1704067200,
      "size": 2048000,
      "status": "analyzed",
      "analyzed_at": 1704153600,
      "created_at": 1704067200,
      "exif": { ... },
      "description": "海滩日落景色，天空呈现橙色和紫色",
      "tags": ["海滩", "日落", "天空"],
      "objects": ["大海", "天空", "云"],
      "scene_type": "outdoor",
      "colors": ["橙色", "紫色", "蓝色"],
      "people_count": 0,
      "has_text": false
    }
  ],
  "total": 42,
  "page": 1,
  "page_size": 20,
  "total_pages": 3
}
```

### 统计信息

```
GET /api/stats
```

**响应示例：**
```json
{
  "total": 1000,
  "by_status": {
    "unanalyzed": 10,
    "analyzing": 2,
    "analyzed": 950,
    "failed": 20,
    "pending_analysis": 18
  },
  "recent_count": 5
}
```

### 筛选选项

```
GET /api/filters
```

返回所有可用的标签、场景类型和相机型号列表，用于前端搜索筛选项。

**响应示例：**
```json
{
  "tags": ["旅行", "风景", "人像", "美食", "建筑"],
  "scene_types": ["indoor", "outdoor", "unknown"],
  "cameras": ["iPhone 15 Pro", "Canon EOS R5", "Sony A7III"]
}
```

### 图片静态文件

```
GET /photos/{path}
```

提供照片文件的静态访问服务。`{path}` 是相对于 `server.photo_base_path` 的文件路径。

- 仅允许图片扩展名：`.jpg`、`.jpeg`、`.png`、`.webp`、`.heic`、`.heif`
- 严格防止路径遍历攻击
- 响应头包含 `Cache-Control: public, max-age=86400`

---

## 项目结构

```
phosche/
├── cmd/phosche/              # 主程序入口
│   ├── main.go               # 启动入口，解析命令行参数
│   └── main_test.go           # 入口测试
│
├── internal/                  # 核心内部包
│   ├── analyzer/              # AI 分析层
│   │   ├── client.go          # LLM 客户端接口 + 工厂方法
│   │   ├── ollama.go          # Ollama 协议实现（/api/chat）
│   │   ├── openai.go          # OpenAI 协议实现（/v1/chat/completions）
│   │   └── analyzer.go        # 图片分析器（图片预处理、重试逻辑、结果校验）
│   │
│   ├── api/                   # REST API 层
│   │   ├── router.go          # chi 路由配置、中间件、健康检查
│   │   ├── photos.go          # 时间线列表 + 孤儿文档清理
│   │   ├── photo_detail.go    # 单张照片详情
│   │   ├── search.go          # 全文搜索端点
│   │   ├── filters.go         # 筛选选项端点
│   │   └── stats.go           # 统计信息端点
│   │
│   ├── app/                   # 应用装配与生命周期
│   │   └── run.go             # 依赖注入、组件启动、优雅关闭
│   │
│   ├── config/                # 配置管理
│   │   ├── config.go          # YAML 加载、默认值、校验
│   │   └── config_test.go     # 配置测试
│   │
│   ├── decoder/               # 图片解码
│   │   ├── decoder.go         # 多格式解码（JPEG/PNG/WebP/HEIC）+ EXIF 提取
│   │   └── decoder_test.go    # 解码测试
│   │
│   ├── errors/                # 统一错误类型
│   │   └── errors.go          # AppError（NOT_FOUND、VALIDATION_ERROR 等）
│   │
│   ├── indexer/               # ES 索引服务
│   │   ├── client.go          # ES 客户端封装（连接、TLS、健康检查）
│   │   ├── mapping.go         # ES 索引映射
│   │   ├── indexer.go         # 索引服务（CRUD、断路器、重试队列）
│   │   └── indexer_test.go    # 索引服务测试
│   │
│   ├── integration/           # 端到端集成测试
│   │   └── e2e_test.go        # 基于 testcontainers 的 E2E 测试
│   │
│   ├── pipeline/              # 处理流水线
│   │   ├── pipeline.go        # 主流水线编排（扫描→监控→解码→分析→索引→重试）
│   │   └── pipeline_test.go   # 流水线测试
│   │
│   ├── search/                # 搜索查询构建
│   │   ├── search.go          # ES 查询构建器（全文搜索、过滤器、聚合统计）
│   │   └── search_test.go     # 搜索测试
│   │
│   ├── static/                # 静态文件服务
│   │   ├── server.go          # 照片文件服务（路径遍历防护、扩展名白名单）
│   │   └── server_test.go     # 静态服务测试
│   │
│   ├── types/                 # 共享类型定义
│   │   ├── types.go           # 照片、EXIF、分析结果、搜索请求/响应等类型
│   │   └── types_test.go      # 类型测试
│   │
│   └── watcher/               # 文件监控
│       ├── types.go           # Watcher/Scanner 接口 + 去重过滤器
│       ├── fsnotify.go        # fsnotify 实现（递归监控、去抖）
│       ├── scanner.go         # 目录扫描器
│       └── existing.go        # 已存在文件管理
│
├── web/                       # 前端 SPA
│   ├── src/
│   │   ├── api/               # API 客户端（axios 封装）
│   │   │   ├── client.ts      # 通用 HTTP 客户端
│   │   │   └── photos.ts      # 照片相关 API 调用
│   │   ├── components/        # 全局组件
│   │   │   ├── Layout.tsx     # 页面布局（导航栏）
│   │   │   ├── ErrorBoundary.tsx  # 错误边界
│   │   │   └── PhotoDetail.tsx    # 照片详情组件
│   │   ├── pages/             # 路由页面
│   │   │   ├── Timeline.tsx   # 时间线浏览（无限滚动）
│   │   │   ├── Search.tsx     # 多条件搜索页
│   │   │   ├── PhotoDetail.tsx # 照片详情页
│   │   │   └── NotFound.tsx   # 404 页面
│   │   ├── types.ts           # 前端类型定义
│   │   ├── App.tsx            # 根组件 + 路由配置
│   │   ├── main.tsx           # 入口
│   │   └── index.css          # 全局样式（Tailwind）
│   │
│   ├── e2e/                   # Playwright E2E 测试
│   └── package.json           # 前端依赖
│
├── config.example.yaml        # 配置文件示例
├── docker-compose.yaml        # Docker Compose 编排（ES + phosche + Ollama）
├── Dockerfile                 # 多阶段构建（Go 编译 → 前端构建 → 运行镜像）
├── Makefile                   # 构建命令集合
├── .env.example               # 环境变量示例
├── embed.go                   # 前端静态资源嵌入（//go:embed）
├── go.mod / go.sum            # Go 模块依赖
└── README.md                  # 本文档
```

---

## 开发指南

### 本地开发

```bash
# 启动 Elasticsearch（使用 Docker）
docker run -d -p 9200:9200 -e "discovery.type=single-node" -e "xpack.security.enabled=false" docker.elastic.co/elasticsearch/elasticsearch:8.17.0

# 启动服务（开发模式，需要 Vite 前端独立运行）
make run

# 在另一个终端启动前端开发服务器
cd web && npm run dev
```

开发模式下，前端的 Vite 开发服务器会代理 API 请求到后端。后端 `dev_mode: true` 时不提供 SPA 静态文件。

### 构建

```bash
# 构建 Go 二进制
make build

# 构建前端
make build-frontend

# 构建 Docker 镜像
make docker-build
```

### 测试

```bash
# 运行所有单元测试
make test

# 带竞态检测运行测试
make test-race

# 运行前端测试
cd web && npm test

# 运行 E2E 测试
cd web && npx playwright test
```

集成测试使用 testcontainers-go，会自动启动 ES 容器进行端到端验证。

### 代码规范

- 后端 Go 代码遵循标准 `go fmt` 和 `go vet` 规范
- 前端 TypeScript 代码使用 ESLint 检查
- 提交前请确保所有测试通过

---

## 架构说明

### 数据处理流水线

```
  ┌─────────────┐    ┌──────────┐    ┌──────────┐    ┌────────┐    ┌──────────────┐
  │ 目录扫描 /   │    │          │    │          │    │        │    │              │
  │ 文件监控     │───▶│ 图片解码 │───▶│ AI 分析  │───▶│ES 索引 │───▶│ Web 展示/搜索│
  │ (fsnotify)  │    │          │    │ (LLM)    │    │        │    │              │
  └─────────────┘    └──────────┘    └──────────┘    └────────┘    └──────────────┘
```

详细的处理流程：

1. **文件发现** — 启动时执行全量目录扫描，发现已有照片；之后通过 fsnotify 实时监听文件系统的 Create/Write 事件
2. **去重与去抖** — DedupFilter 基于 `path + mtime + size` 三重校验去重；Debounce 机制合并短时间内的多次文件写入事件
3. **图片解码** — 根据文件扩展名选择对应的解码器（JPEG/PNG/WebP/HEIC），同时提取 EXIF 元数据（拍摄时间、相机型号、光圈、ISO、GPS 等）
4. **AI 分析** — 图片缩放到最大 2048 像素（保持宽高比），编码为 JPEG 后发送给 LLM。支持 Ollama（base64 图片）和 OpenAI（data URL）两种协议。LLM 返回 JSON 格式的结构化分析结果
5. **ES 索引** — 将照片元数据、EXIF 信息和 AI 分析结果合并为 PhotoDocument，索引到 Elasticsearch。使用路径的 SHA-256 哈希作为文档 ID
6. **Web 展示** — React SPA 提供时间线浏览和全文搜索界面，通过 REST API 与后端交互

### 照片状态机

每张照片在处理生命周期中经历以下状态转换：

```
                  ┌──────────────────────────────────────────────┐
                  │                                              │
                  ▼                                              │
  ┌──────────┐   ┌──────────┐   ┌────────────────┐   ┌───────┐  │
  │          │   │          │   │   LLM 连接失败   │   │       │  │
  │unanalyzed│──▶│analyzing │───▶────────────────▶│pending│──┘  │
  │          │   │          │   │                 │analysis│     │
  └──────────┘   └──────────┘   │                 └────────┘     │
                                │    (5分钟间隔重试，              │
                  ┌──────────┐  │     最多10次)                  │
                  │          │  │                                │
                  │ analyzed │◀─┘   ┌──────────┐                │
                  │          │      │  other   │                │
                  └──────────┘      │  error   │                │
                                    └──────────┘                │
                                        │                       │
                                        ▼                       │
                                  ┌──────────┐                  │
                                  │          │                  │
                                  │  failed  │                  │
                                  │          │                  │
                                  └──────────┘                  │
```

- **unanalyzed** — 初始状态，照片刚被发现尚未处理
- **analyzing** — 正在执行 AI 分析
- **analyzed** — 分析成功，结果已索引到 ES
- **pending_analysis** — LLM 连接失败（网络错误或服务不可用），进入重试队列，每 5 分钟自动重试，最多 10 次
- **failed** — 处理失败（图片损坏、格式不支持、LLM 返回错误等不可恢复的错误）

### 高可用机制

- **断路器模式** — IndexerService 监控 ES 写入失败次数（阈值 3 次），失败达到阈值时打开断路器，后续写入进入内存队列（容量 100），不再阻塞处理流水线。断路器每 5 秒检查 ES 健康状态，恢复后自动排水队列
- **LLM 降级** — 检测到 LLM 连接错误时，照片进入 pending_analysis 队列，不阻塞流水线，后台定时重试
- **优雅关闭** — 收到 SIGINT/SIGTERM 后，停止接收新的文件事件，等待所有进行中的分析和索引任务完成（最多 5 分钟），最后关闭 HTTP 服务器

---

## 许可证

MIT License

Copyright (c) 2024 zwh8800

特此免费授予任何获得本软件副本和相关文档文件（以下简称"软件"）的人不受限制地处理本软件的权利，包括但不限于使用、复制、修改、合并、发布、分发、再许可和/或出售本软件副本的权利，并允许被提供本软件的人在满足以下条件的情况下这样做：

上述版权声明和本许可声明应包含在所有副本或本软件的重要部分中。

本软件按"原样"提供，不提供任何形式的明示或暗示的保证，包括但不限于对适销性、特定用途的适用性和非侵权性的保证。在任何情况下，作者或版权持有人均不对任何索赔、损害或其他责任负责，无论是在合同、侵权或其他方面，由本软件或本软件的使用或其他交易引起或与之相关。
