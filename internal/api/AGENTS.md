# internal/api

REST API 层，基于 chi 路由器实现所有 HTTP 接口。

## 目录结构

- `router.go` — 路由注册、中间件栈配置、健康检查
- `search.go` — 全文搜索端点（POST /api/search）
- `photos.go` — 照片时间线查询（GET /api/photos）+ 孤儿文档清理
- `photo_detail.go` — 单张照片详情（GET /api/photos/*）
- `filters.go` — 筛选选项聚合（GET /api/filters）
- `stats.go` — 统计信息（GET /api/stats）
- `jwt.go` — JWT 认证中间件（从 cookie 提取 email）
- `*_test.go` — 对应测试文件

## 接口

- `PhotoSearcher` — 搜索服务接口（Search、GetFilters、GetStats）
- `Indexer` — 索引服务接口（GetPhoto、DeletePhoto）

## 中间件栈

注册顺序：Logger → Recoverer → Timeout(30s) → CORS → JWTAuth

JWTAuth 从 `access_token` cookie 提取 email 注入 context，不验证签名。

## 认证

所有请求都会经过 JWTAuth，通过 `UserEmailFromContext(r.Context())` 获取用户 email。
未认证时 email 为空字符串，只返回公开照片。

## 路由

所有 API 端点以 `/api/` 为前缀，`/health` 和 `/photos/*` 除外。

## 错误处理

使用 `internal/errors.AppError` 类型。Handler 通过 `errors.As` 提取 `AppError`，用 `HTTPStatus` 作为响应码。

## 测试

- 使用 `httptest.NewServer` + 手写 mock struct（函数指针字段）
- Mock 定义在 `mock_test.go`：`mockSearchService`、`mockIndexer` 等
- QA 套件在 `qa_test.go`：命名场景 + 通过计数 + emoji 日志
- 错误码通过 `errors.As` 提取 `AppError`，用字符串字面量比较（如 `"NOT_FOUND"`）
