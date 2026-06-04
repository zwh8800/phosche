# Phosche — 个人照片搜索服务

> 自动监控照片目录，通过多模态 AI 分析照片内容，构建可全文搜索的个人照片库。

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)
![License](https://img.shields.io/badge/License-MIT-blue)
![OpenSearch](https://img.shields.io/badge/OpenSearch-2.x-005571?logo=opensearch)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react)
![Docker](https://img.shields.io/badge/Docker-zwh8800%2Fphosche-2496ED?logo=docker)
[![CI/CD](https://github.com/zwh8800/phosche/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/zwh8800/phosche/actions/workflows/docker-publish.yml)

---

## 功能特性

- **自动文件监控** — 使用 fsnotify 递归监控指定目录，新照片出现后自动触发处理流水线
- **多模态 AI 分析** — 支持 Ollama 本地模型（llama3.2-vision）和 OpenAI API（GPT-4o），分析照片内容并提取描述、标签、场景类型、颜色、人数、文本等信息
- **结构化响应** — LLM 返回 JSON 格式的结构化分析结果，包含 description、tags、objects、scene_type、colors、people_count、has_text、confidence 等字段
- **逆地理编码** — 自动提取照片 EXIF 中的 GPS 坐标，通过高德地图 API 获取可读地址（省/市/区/街道），存入 OpenSearch 支持地点搜索
- **OpenSearch 全文搜索** — 基于 OpenSearch 2.x 构建索引，支持中文 IK 分词，可按关键词、日期范围、标签、对象、场景类型、相机型号、拍摄地点等多维筛选
- **时间线浏览** — Web 界面按日期分组展示照片，支持无限滚动加载
- **照片详情页** — 查看单张照片的 EXIF 元数据（相机型号、镜头、光圈、ISO、GPS、拍摄地址等）和 AI 分析结果
- **数据统计** — 实时统计照片总数及各处理状态的数量分布
- **缩略图与缓存** — 自动生成 400px 缩略图和 HEIC→JPEG 全尺寸缓存，加速页面加载；支持 `?thumb=1`（缩略图）和 `?convert=1`（HEIC 转 JPEG）查询参数
- **私有目录访问控制** — 可配置私有照片目录，仅授权用户（通过 JWT Cookie 中的 email）可查看，未授权请求返回 403
- **目录排除** — 支持排除指定目录（如回收站、临时目录），避免扫描无关文件
- **跳过初始扫描** — 可选跳过启动时的全量目录扫描，仅监控新增文件
- **JWT 认证** — 从 `access_token` cookie 提取用户邮箱，用于私有目录访问控制
- **断路器保护** — OpenSearch 不可用时自动打开断路器，内存有界队列缓冲写入请求，恢复后自动排水
- **LLM 降级与重试** — LLM 不可用时将照片标记为 pending_analysis，每隔 5 分钟自动重试，最多重试 10 次
- **错误隔离** — 损坏图片优雅跳过（仅记录日志），不导致服务崩溃
- **优雅关闭** — SIGINT/SIGTERM 触发优雅关闭：停止接收新任务，等待当前处理完成，释放所有资源
- **多格式图片解码** — 支持 JPEG、PNG、WebP、HEIC/HEIF 格式，自动提取 EXIF 元数据
- **路径遍历防护** — 静态文件服务严格校验请求路径，防止目录穿越攻击
- **混合检索** — 支持 BM25 全文检索 + kNN 向量检索的混合模式（通过 OpenSearch search pipeline RRF），需配置 embedding 服务
- **相似照片推荐** — 基于 embedding 向量相似度推荐相关照片（`/api/photos/{id}/similar`）
- **附近照片推荐** — 基于 GPS 坐标推荐地理位置附近的照片（`/api/photos/{id}/nearby`）
- **时区迁移** — 支持批量更新已索引照片的 EXIF 时区信息（`/api/migrate-timezone`）
- **PWA 支持** — 前端支持渐进式 Web 应用，可安装到桌面，自动检测更新并提示刷新
- **Docker Compose 一键部署** — 内置编排文件，可一键启动 OpenSearch + phosche + 可选 Ollama

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端语言 | Go 1.26 |
| HTTP 路由 | chi/v5 |
| 结构化日志 | log/slog |
| 搜索引擎 | OpenSearch 2.x（opensearch-go 客户端 + IK 中文分词 + kNN 向量检索） |
| AI 分析 | Ollama（本地）/ OpenAI（云端），OpenAI 兼容协议统一接口 |
| 文本向量化 | Ollama embedding / OpenAI embedding，支持 BM25 + kNN 混合检索（RRF） |
| 逆地理编码 | 高德地图 REST API（GPS 坐标 → 可读地址） |
| 图片解码 | 标准库 + golang.org/x/image/webp + gen2brain/heic |
| EXIF 提取 | dsoprea/go-exif/v3 |
| 图片缓存 | 自动缩略图生成 + HEIC→JPEG 转换缓存 |
| 文件监控 | fsnotify |
| 前端框架 | React 19 + TypeScript 6 |
| 构建工具 | Vite 8 |
| 样式方案 | Tailwind CSS 4 |
| 状态管理 | TanStack React Query 5 |
| HTTP 客户端 | axios |
| 路由 | react-router-dom 7 |
| PWA | vite-plugin-pwa |
| 容器化 | Docker（多阶段构建）+ Docker Compose |
| 测试 | Go 测试框架 + testcontainers-go + Playwright |

---

## 快速开始

### 前置要求

- Go 1.26 或更高版本（手动启动时需要）
- Node.js 18 或更高版本（构建前端时需要）
- Docker 和 Docker Compose（推荐，一键启动方式）
- OpenSearch 2.x（手动启动时需要）
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

最低配置需要修改 `watch.directories`（照片目录路径，同时作为照片文件根目录）和 `opensearch.addresses`。

### 启动方式

#### 方式一：Docker Compose（推荐）

```bash
# 修改 config.yaml 中的配置，确保路径与容器内挂载一致
vim config.yaml

# 启动 OpenSearch 和 phosche
docker compose up -d

# 如果使用本地 Ollama，加上 ollama profile
docker compose --profile ollama up -d
```

Docker Compose 会依次启动 OpenSearch（等待健康检查通过）和 phosche 服务。图片目录通过 volume 挂载到容器中。

#### 方式二：手动启动

1. 确保 OpenSearch 已启动并可访问
2. 配置 config.yaml，确保 `opensearch.addresses` 指向正确的 OpenSearch 地址
3. 构建前端并启动服务：

```bash
# 构建前端静态资源（开发模式下可跳过）
make build-frontend

# 启动服务
go run . -config config.yaml
```

服务默认监听 `0.0.0.0:8080`。开发模式下（`dev_mode: true`），前端由 Vite 独立运行，后端不提供 SPA 静态文件。

#### 方式三：单独使用 Docker 启动

适用于 OpenSearch 已单独部署（或使用云服务）的场景，通过 `docker run` 启动 phosche 容器。

**1. 启动 OpenSearch（含 IK 中文分词插件）：**

phosche 依赖 IK 分词插件进行中文全文搜索。任选以下任一方式启动 OpenSearch：

**方式 A：使用项目提供的 Dockerfile 构建（推荐）**

构建包含 IK 插件的自定义镜像，然后启动容器：

```bash
docker build -t phosche-opensearch -f Dockerfile.opensearch .
docker run -d \
  --name opensearch \
  -p 9200:9200 \
  -e "discovery.type=single-node" \
  -e "DISABLE_SECURITY_PLUGIN=true" \
  -e "OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m" \
  -v osdata:/usr/share/opensearch/data \
  phosche-opensearch
```

**方式 B：基于官方镜像直接运行（一行命令）**

使用 `bash -c` 接管默认 entrypoint，在启动主服务前一次性安装 IK 插件（已安装过的会跳过），无需二次重启：

```bash
docker run -d \
  --name opensearch \
  -p 9200:9200 \
  -e "discovery.type=single-node" \
  -e "DISABLE_SECURITY_PLUGIN=true" \
  -e "OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m" \
  -v osdata:/usr/share/opensearch/data \
  opensearchproject/opensearch:2.19.5 \
  bash -c '
    if [ ! -d /usr/share/opensearch/plugins/analysis-ik ]; then
      opensearch-plugin install --batch https://get.infini.cloud/opensearch/analysis-ik/2.19.5
    fi
    /usr/share/opensearch/opensearch-docker-entrypoint.sh opensearch
  '
```

关键路径说明（与 ES 版等价）：

| 项 | 值 |
|---|---|
| Entry point 脚本 | `/usr/share/opensearch/opensearch-docker-entrypoint.sh` |
| 默认 CMD | `opensearch`（ES 侧对应 `eswrapper`） |
| 插件 CLI | `opensearch-plugin`（PATH 包含 `$OPENSEARCH_HOME/bin`） |
| IK 安装目录 | `/usr/share/opensearch/plugins/analysis-ik` |
| IK 安装源 | `https://get.infini.cloud/opensearch/analysis-ik/2.19.5`（INFINI Labs 官方；2.19.5 不可用，INFINI 未为该版本发布 IK） |

> 注意：OpenSearch **不能**使用 Elasticsearch 的 IK 包（URL 前缀 `elasticsearch/` 与 `opensearch/` 区分），必须用 INFINI Labs 发布的 `opensearch` 版本，即上面命令中使用的 URL。

验证 OpenSearch 和 IK 插件是否正常：

```bash
curl http://localhost:9200/_cat/plugins
# 应输出: xxxxx analysis-ik 2.19.5
```

**2. 获取 phosche 镜像：**

```bash
# 方式 A：从 Docker Hub 拉取（推荐）
docker pull zwh8800/phosche:latest

# 方式 B：本地构建
docker build -t phosche .
```

**3. 准备配置文件：**

```bash
cp config.example.yaml config.yaml
vim config.yaml
```

关键配置项（与 Docker Compose 方式不同，这里的 OpenSearch 地址指向外部服务）：

```yaml
opensearch:
  addresses:
    - https://your-opensearch-cloud.example.com:9200   # 外部 OpenSearch 地址
  username: "your-username"                     # 如启用了认证
  password: "your-password"
  insecure_skip_verify: false

server:
  dev_mode: false    # 关闭开发模式，使用内嵌前端
  log_level: info    # 日志级别：debug/info/warn/error

llm:
  provider: openai  # 当前仅支持 openai（含 Ollama 等兼容协议）
  openai:
    base_url: https://api.openai.com/v1  # 本地 Ollama 用 http://localhost:11434/v1
    model: gpt-4o                        # 本地 Ollama 用 llama3.2-vision
    api_key: "sk-..."                    # 本地 Ollama 留空即可

env:
  amap_key: ""       # 高德地图 API Key（用于逆地理编码，可选）
```

**4. 启动容器：**

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
| `-v /path/to/photos:/photos:ro` | 挂载照片目录（只读），路径需与 `watch.directories` 配置一致 |
| `-e CONFIG_PATH=/app/config.yaml` | 指定配置文件路径 |
| `phosche` | 镜像名称 |

**访问服务：** `http://localhost:8080`

**访问本地 Ollama（可选）：**

如果 Ollama 运行在宿主机上，需要让容器能够访问宿主机的网络：

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

然后在 `config.yaml` 中将 Ollama 地址设为 `http://host.docker.internal:11434/v1`：

```yaml
llm:
  provider: openai
  openai:
    base_url: http://host.docker.internal:11434/v1
    model: llama3.2-vision
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
| `directories` | `[]string` | 必填 | 监控的照片目录路径列表，同时用于文件监控和照片文件服务 |
| `recursive` | `bool` | `true` | 是否递归监控子目录 |
| `debounce_ms` | `int` | `500` | 文件事件的去抖间隔（毫秒），同一文件在间隔内的多次修改会被合并 |
| `min_dir_depth` | `int` | `1` | 最小监控目录深度 |
| `exclude_dirs` | `[]string` | `[]` | 排除的目录名列表，支持前缀匹配和目录名匹配（如 `"#recycle"` 匹配任意路径中的该目录） |
| `private_dirs` | `map[string][]string` | `{}` | 私有目录配置，key 为目录前缀路径，value 为授权用户邮箱列表 |
| `skip_initial_scan` | `bool` | `false` | 跳过启动时的全量目录扫描，仅监控新增文件 |

### `llm` — AI 分析

LLM 使用 OpenAI 兼容协议统一接入，本地 Ollama 和云端 OpenAI 共用同一套配置。

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `provider` | `string` | 必填 | LLM 提供商，当前仅支持 `"openai"`（含 Ollama 等兼容协议） |
| `openai.base_url` | `string` | 必填 | API 地址。本地 Ollama：`http://localhost:11434/v1`；云端 OpenAI：`https://api.openai.com/v1` |
| `openai.model` | `string` | 必填 | 模型名称。如 `llama3.2-vision`（Ollama）、`gpt-4o`（OpenAI） |
| `openai.api_key` | `string` | `""` | API 密钥（可选，使用本地 Ollama 时留空即可） |
| `openai.response_format` | `string` | `""` | 响应格式：`json_object`（默认）、`json_schema`（严格 schema）、`text`（纯文本引导）。LM Studio 等仅支持 `json_schema` |
| `openai.max_tokens` | `int` | `0` | 最大输出 token 数（兼容 LMStudio/Ollama），0 表示不设置（由服务端决定） |
| `openai.max_completion_tokens` | `int` | `0` | 最大完成 token 数（含 reasoning token，兼容 OpenAI o1/o3 系列），0 表示不设置 |
| `max_retries` | `int` | `3` | 分析失败时的最大重试次数 |
| `concurrency` | `int` | `2` | 同时进行的 AI 分析任务数 |
| `timeout_seconds` | `int` | `60` | 单次 AI 分析请求的超时时间（秒） |
| `output_language` | `string` | `"zh"` | 输出语言，影响分析结果描述的语言 |

### `opensearch` — 搜索引擎

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `addresses` | `[]string` | 必填 | OpenSearch 节点地址列表 |
| `username` | `string` | `""` | OpenSearch 认证用户名 |
| `password` | `string` | `""` | OpenSearch 认证密码 |
| `insecure_skip_verify` | `bool` | `false` | 是否跳过 TLS 证书验证 |
| `index_name` | `string` | `"phosche"` | OpenSearch 索引名称 |

### `server` — HTTP 服务

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `host` | `string` | `"0.0.0.0"` | 监听地址 |
| `port` | `int` | `8080` | 监听端口 |
| `dev_mode` | `bool` | `true` | 开发模式开关，开启时不提供 SPA 静态文件 |
| `log_level` | `string` | `"info"` | 日志级别，可选 `debug`、`info`、`warn`、`error` |
| `cache_dir` | `string` | `""` | 照片缓存目录（缩略图 + HEIC 转 JPEG），空表示不缓存（实时生成） |
| `timezone` | `string` | `"Asia/Shanghai"` | 照片拍摄时区，用于解析无时区信息的 EXIF 时间（如 `Asia/Shanghai`、`America/New_York`） |

### `env` — 外部服务

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `amap_key` | `string` | `""` | 高德地图 API Key，用于逆地理编码（GPS 坐标转地址）。为空时禁用逆地理编码 |

### `embedding` — 文本向量化（可选）

用于混合检索（BM25 + kNN），默认禁用。

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `enabled` | `bool` | `false` | 是否启用 embedding。false 时完全跳过向量化，只用 BM25 搜索 |
| `provider` | `string` | `"ollama"` | 向量化后端，可选 `"ollama"` 或 `"openai"` |
| `ollama.base_url` | `string` | `http://localhost:11434` | Ollama 服务地址 |
| `ollama.model` | `string` | `bge-m3` | embedding 模型名称（如 bge-m3、qwen3-embedding:4b） |
| `ollama.dimensions` | `int` | `1024` | 向量维度，必须与模型实际输出一致 |
| `openai.api_key` | `string` | `""` | OpenAI API 密钥 |
| `openai.base_url` | `string` | `https://api.openai.com/v1` | API 地址 |
| `openai.model` | `string` | `text-embedding-3-small` | 模型名称 |
| `openai.dimensions` | `int` | `1024` | 向量维度（OpenAI 支持 Matryoshka 截断，可设小于模型原始维度） |
| `hybrid.rrf_rank_constant` | `int` | `60` | RRF rank_constant，控制排名差异的权重衰减速度 |
| `max_retries` | `int` | `2` | 单次 embedding 请求的重试次数（5xx/网络错误才重试，4xx 不重试） |
| `timeout_seconds` | `int` | `15` | 单次 embedding 请求超时（秒） |
| `query_cache.size` | `int` | `512` | 查询 embedding 的 LRU 缓存条目数 |
| `query_cache.ttl_minutes` | `int` | `60` | 缓存过期时间（分钟） |
| `required` | `bool` | `false` | embedding 失败时是否阻塞文档入库。false = 失败后文档照常入库（无向量），只被 BM25 召回 |

---

## API 文档

所有 API 端点以 `/api/` 为前缀（健康检查除外），返回 JSON 格式响应。

### 认证

API 使用基于 Cookie 的 JWT 认证。中间件从 `access_token` cookie 中解析 JWT payload 提取 `email` 字段，用于私有目录的访问控制。

- 认证是可选的，未认证时 email 为空字符串，只能查看公开照片
- 私有目录中的照片仅授权用户（配置在 `watch.private_dirs` 中的邮箱列表）可查看
- JWT 仅提取 email，不验证签名（适合前置网关已验证的场景）

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

扫描 OpenSearch 索引中所有照片文档，删除文件系统中已不存在的记录。

**响应示例：**
```json
{
  "deleted": 5
}
```

### 照片详情

```
GET /api/photos/{path}
```

`{path}` 是照片的相对文件路径（URL 编码），如 `2024/01/IMG_0001.jpg`。

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
  "geo": {
    "country": "中国",
    "province": "北京市",
    "city": "北京市",
    "district": "东城区",
    "township": "东华门街道",
    "formatted_address": "北京市东城区东华门街道天安门广场"
  },
  "description": "一张在公园里拍摄的照片，阳光明媚，草地上有几个人在野餐",
  "tags": ["公园", "野餐", "阳光", "草地"],
  "objects": ["人", "草地", "树木"],
  "scene_type": "outdoor",
  "colors": [
    {"name": "绿色", "hex": "#22C55E"},
    {"name": "蓝色", "hex": "#3B82F6"},
    {"name": "白色", "hex": "#F8FAFC"}
  ],
  "people_count": 3,
  "has_text": false,
  "confidence": 0.95,
  "photo_url": "/photos/photos/2024/01/IMG_0001.jpg"
}
```

### 相似照片推荐

```
GET /api/photos/{id}/similar
```

基于 embedding 向量相似度推荐相关照片。需要启用 embedding 功能。`{id}` 是照片的文档 ID（SHA-256 哈希）。

**响应：** 同搜索响应格式，返回相似度最高的照片列表。

### 附近照片推荐

```
GET /api/photos/{id}/nearby
```

基于 GPS 坐标推荐地理位置附近的照片。`{id}` 是照片的文档 ID（SHA-256 哈希）。

**响应：** 同搜索响应格式，返回距离最近的照片列表。

### 时区迁移

```
POST /api/migrate-timezone
```

批量更新已索引照片的 EXIF 时区信息。遍历所有已分析的照片，重新提取 EXIF 并更新。返回 202 Accepted 立即响应，后台异步执行迁移。

**响应示例：**
```json
{
  "status": "migration started"
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
      "geo": {
        "country": "中国",
        "province": "海南省",
        "city": "三亚市",
        "formatted_address": "海南省三亚市亚龙湾"
      },
      "description": "海滩日落景色，天空呈现橙色和紫色",
      "tags": ["海滩", "日落", "天空"],
      "objects": ["大海", "天空", "云"],
      "scene_type": "outdoor",
      "colors": [
        {"name": "橙色", "hex": "#F97316"},
        {"name": "紫色", "hex": "#A855F7"}
      ],
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
GET /photos/{path}?thumb=1
GET /photos/{path}?convert=1
```

提供照片文件的静态访问服务。`{path}` 是相对于 `watch.directories` 的文件路径。

- 仅允许图片扩展名：`.jpg`、`.jpeg`、`.png`、`.webp`、`.heic`、`.heif`
- 严格防止路径遍历攻击
- 响应头包含 `Cache-Control: public, max-age=86400`

**查询参数：**

| 参数 | 类型 | 说明 |
|------|------|------|
| `thumb` | `string` | 设为 `1` 时返回 400px 宽的 JPEG 缩略图。优先从缓存读取，缓存缺失时实时生成 |
| `convert` | `string` | 设为 `1` 时，HEIC/HEIF 格式转为 JPEG 返回（优先缓存），非 HEIC 格式直接返回原始文件 |

> 缩略图和 HEIC 转换结果会写入 `server.cache_dir` 目录缓存，后续请求直接读取缓存文件。

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
│   │   ├── openai.go          # OpenAI 兼容协议实现（/v1/chat/completions，Ollama 与 OpenAI 共用）
│   │   └── analyzer.go        # 图片分析器（图片预处理、重试逻辑、结果校验）
│   │
│   ├── api/                   # REST API 层
│   │   ├── router.go          # chi 路由配置、中间件、健康检查
│   │   ├── photos.go          # 时间线列表 + 孤儿文档清理
│   │   ├── photo_detail.go    # 单张照片详情 + 相似/附近推荐
│   │   ├── search.go          # 全文搜索端点
│   │   ├── filters.go         # 筛选选项端点
│   │   ├── stats.go           # 统计信息端点
│   │   ├── migrate.go         # 时区迁移端点（异步后台执行）
│   │   └── jwt.go             # JWT 认证中间件（从 cookie 提取 email）
│   │
│   ├── app/                   # 应用装配与生命周期
│   │   └── run.go             # 依赖注入、组件启动、优雅关闭
│   │
│   ├── cache/                 # 照片缓存
│   │   └── cache.go           # 缩略图 + HEIC 转 JPEG 缓存生成
│   │
│   ├── config/                # 配置管理
│   │   ├── config.go          # YAML 加载、默认值、校验
│   │   └── config_test.go     # 配置测试
│   │
│   ├── decoder/               # 图片解码
│   │   ├── decoder.go         # 多格式解码（JPEG/PNG/WebP/HEIC）+ EXIF 提取
│   │   └── decoder_test.go    # 解码测试
│   │
│   ├── embedder/              # 索引侧 embedding（文档向量化）
│   │   ├── client.go          # EmbeddingClient 接口 + 工厂方法（Ollama/OpenAI）
│   │   ├── service.go         # EmbeddingService（重试 + 缓存）
│   │   ├── cache.go           # LRU 查询 embedding 缓存
│   │   ├── text.go            # BuildEmbeddingText（从 PhotoDocument 构建文本）
│   │   ├── ollama.go          # Ollama embedding 客户端
│   │   └── openai.go          # OpenAI embedding 客户端
│   │
│   ├── embedding/             # 搜索侧 embedding（查询向量化）
│   │   ├── client.go          # Embedder 接口 + 工厂方法
│   │   ├── ollama.go          # Ollama embedder 实现
│   │   ├── openai.go          # OpenAI embedder 实现
│   │   ├── batch.go           # 批处理 + 重试逻辑
│   │   └── errors.go          # EmbeddingError 类型（可重试/不可重试）
│   │
│   ├── errors/                # 统一错误类型
│   │   └── errors.go          # AppError（NOT_FOUND、VALIDATION_ERROR 等）
│   │
│   ├── geocoder/              # 逆地理编码
│   │   └── geocoder.go        # 高德地图 API 客户端（GPS → 地址）
│   │
│   ├── indexer/               # OpenSearch 索引服务
│   │   ├── client.go          # OpenSearch 客户端封装（连接、TLS、健康检查）
│   │   ├── mapping.go         # OpenSearch 索引映射
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
│   │   ├── search.go          # OpenSearch 查询构建器（BM25 + kNN 混合检索、全文搜索、过滤器、聚合统计）
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
│   │   ├── __tests__/           # 前端单元测试（vitest + msw）
│   │   ├── components/        # 全局组件
│   │   │   ├── Layout.tsx     # 页面布局（导航栏）
│   │   │   ├── ErrorBoundary.tsx  # 错误边界
│   │   │   ├── PhotoDetail.tsx    # 照片详情组件
│   │   │   └── ReloadPrompt.tsx     # PWA 更新提示
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
├── docker-compose.yaml        # Docker Compose 编排（OpenSearch + phosche + Ollama）
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
# 启动 OpenSearch（使用 Docker）
docker run -d -p 9200:9200 -e "discovery.type=single-node" -e "DISABLE_SECURITY_PLUGIN=true" opensearchproject/opensearch:2.19.5

# 构建前端
make build-frontend

# 启动服务（开发模式，需要 Vite 前端独立运行）
make run

# 在另一个终端启动前端开发服务器
cd web && npm run dev
```

开发模式下，前端的 Vite 开发服务器会代理 API 请求到后端。后端 `dev_mode: true` 时不提供 SPA 静态文件。

```bash
# 仅启动后端（不构建前端）
go run ./cmd/phosche/ -config config.yaml
```

### 构建

```bash
# 构建 Go 二进制
make build

# 构建前端
make build-frontend

# 构建 Docker 镜像
make docker-build

# 清理构建产物
make clean
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

集成测试使用 testcontainers-go，会自动启动 OpenSearch 容器进行端到端验证。

### 代码规范

- 后端 Go 代码遵循标准 `go fmt` 和 `go vet` 规范
- 前端 TypeScript 代码使用 ESLint 检查
- 提交前请确保所有测试通过

### CI/CD

项目使用 GitHub Actions 自动构建和发布 Docker 镜像。

**触发条件：** 推送 `v*` 格式的 tag（如 `v1.0.0`）时自动触发。

**工作流程：**
1. Checkout 代码
2. 设置 QEMU + Buildx（多架构支持）
3. 登录 Docker Hub
4. 构建 `linux/amd64` 架构镜像
5. 推送到 Docker Hub

**Docker 镜像：** [`zwh8800/phosche`](https://hub.docker.com/r/zwh8800/phosche)

**Tag 策略：**
| Tag | 示例 | 说明 |
|-----|------|------|
| `latest` | `zwh8800/phosche:latest` | 稳定版本（非预发布 tag 时更新） |
| `{major}.{minor}` | `zwh8800/phosche:1.0` | 主版本.次版本 |
| `{major}` | `zwh8800/phosche:1` | 主版本 |
| `{version}` | `zwh8800/phosche:1.0.0` | 完整语义版本 |

**发布步骤：**

```bash
# 1. 确保代码已提交
git add . && git commit -m "..."

# 2. 打 tag
git tag v1.0.0

# 3. 推送 tag（触发 CI）
git push origin v1.0.0
```

**配置 GitHub Secrets：**

在仓库 Settings → Secrets and variables → Actions 中添加：

| Secret | 说明 |
|--------|------|
| `DOCKER_USERNAME` | Docker Hub 用户名 |
| `DOCKER_PASSWORD` | Docker Hub 密码或 Access Token |

> 建议使用 Docker Hub Access Token 而非密码，在 [Docker Hub Security](https://hub.docker.com/settings/security) 页面创建。

---

## 架构说明

### 数据处理流水线

```
  ┌─────────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌────────┐    ┌──────────────┐
  │ 目录扫描 /   │    │          │    │          │    │ 逆地理    │    │          │    │        │    │              │
  │ 文件监控     │───▶│ 图片解码 │───▶│ AI 分析  │───▶│ 编码     │───▶│ 向量化   │───▶│OpenSearch│───▶│ Web 展示/搜索│
  │ (fsnotify)  │    │          │    │ (LLM)    │    │(高德API) │    │(可选)    │    │  索引   │    │              │
  └─────────────┘    └──────────┘    └──────────┘    └──────────┘    └──────────┘    └────────┘    └──────────────┘
```

详细的处理流程：

1. **文件发现** — 启动时执行全量目录扫描，发现已有照片；之后通过 fsnotify 实时监听文件系统的 Create/Write 事件
2. **去重与去抖** — DedupFilter 基于 `path + mtime + size` 三重校验去重；Debounce 机制合并短时间内的多次文件写入事件
3. **图片解码** — 根据文件扩展名选择对应的解码器（JPEG/PNG/WebP/HEIC），同时提取 EXIF 元数据（拍摄时间、相机型号、光圈、ISO、GPS 等）
4. **AI 分析** — 图片缩放到最大 2048 像素（保持宽高比），编码为 JPEG 后发送给 LLM。Ollama 和 OpenAI 均使用 OpenAI 兼容协议（data URL）。LLM 返回 JSON 格式的结构化分析结果。EXIF 和逆地理编码信息会附加到 LLM 提示词中作为上下文
5. **向量化** — 将 AI 分析结果（描述、标签、物体、地点等）构建成文本，通过 embedding 服务转换为向量。支持 Ollama 和 OpenAI 两个 provider。此步骤可选，失败时文档仍可入库（仅被 BM25 召回）
6. **OpenSearch 索引** — 将照片元数据、EXIF 信息、AI 分析结果、逆地理编码信息和向量合并为 PhotoDocument，索引到 OpenSearch。使用路径的 SHA-256 哈希作为文档 ID
7. **缓存生成** — 分析完成后自动生成 400px 缩略图和 HEIC→JPEG 全尺寸缓存，存储到 `server.cache_dir` 目录
8. **Web 展示** — React SPA 提供时间线浏览和全文搜索界面，通过 REST API 与后端交互。搜索支持纯 BM25 和 BM25 + kNN 混合检索两种模式

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
- **analyzed** — 分析成功，结果已索引到 OpenSearch
- **pending_analysis** — LLM 连接失败（网络错误或服务不可用），进入重试队列，每 5 分钟自动重试，最多 10 次
- **failed** — 处理失败（图片损坏、格式不支持、LLM 返回错误等不可恢复的错误）

### 高可用机制

- **断路器模式** — IndexerService 监控 OpenSearch 写入失败次数（阈值 3 次），失败达到阈值时打开断路器，后续写入进入内存队列（容量 100），不再阻塞处理流水线。断路器每 5 秒检查 OpenSearch 健康状态，恢复后自动排水队列
- **LLM 降级** — 检测到 LLM 连接错误时，照片进入 pending_analysis 队列，不阻塞流水线，后台定时重试
- **缓存加速** — 照片分析完成后自动生成缩略图和 HEIC→JPEG 缓存文件，Web 请求时优先读取缓存，避免重复解码和转换
- **逆地理编码降级** — 高德 API 不可用时仅记录警告日志，照片仍正常处理，GeoInfo 字段留空
- **优雅关闭** — 收到 SIGINT/SIGTERM 后，停止接收新的文件事件，等待所有进行中的分析和索引任务完成（最多 5 分钟），最后关闭 HTTP 服务器

---

## 许可证

MIT License

Copyright (c) 2024 zwh8800

特此免费授予任何获得本软件副本和相关文档文件（以下简称"软件"）的人不受限制地处理本软件的权利，包括但不限于使用、复制、修改、合并、发布、分发、再许可和/或出售本软件副本的权利，并允许被提供本软件的人在满足以下条件的情况下这样做：

上述版权声明和本许可声明应包含在所有副本或本软件的重要部分中。

本软件按"原样"提供，不提供任何形式的明示或暗示的保证，包括但不限于对适销性、特定用途的适用性和非侵权性的保证。在任何情况下，作者或版权持有人均不对任何索赔、损害或其他责任负责，无论是在合同、侵权或其他方面，由本软件或本软件的使用或其他交易引起或与之相关。
