# internal/errors

统一应用错误类型。所有业务错误通过 `AppError` 表达，支持错误码、HTTP 状态码和错误链。

## 核心类型

`AppError` 结构体：`Code`（错误码）+ `Message`（用户描述）+ `HTTPStatus` + `Details`（可选）+ `Err`（原始错误）。

实现 `error` 和 `Unwrap()` 接口，支持 `errors.Is`/`errors.As` 链式查找。

## 工厂方法

| 方法 | 错误码 | HTTP | 用途 |
|------|--------|------|------|
| `NewNotFoundError(msg)` | NOT_FOUND | 404 | 资源不存在 |
| `NewValidationError(msg, details)` | VALIDATION_ERROR | 400 | 参数校验失败 |
| `NewInternalError(err)` | INTERNAL_ERROR | 500 | 内部错误，包装原始 error |
| `NewServiceUnavailableError(msg)` | SERVICE_UNAVAILABLE | 503 | 外部服务不可用 |

## 使用方式

```go
// 创建
return errors.NewNotFoundError("photo not found: " + path)

// 判断（在 API handler 中）
var appErr *errors.AppError
if errors.As(err, &appErr) {
    http.Error(w, appErr.Message, appErr.HTTPStatus)
}
```

## 约定

- 错误码是字符串常量（`"NOT_FOUND"` 等），**未定义为导出常量** — 比较时使用字符串字面量
- API handler 通过 `errors.As` 提取 `AppError` 后直接用 `HTTPStatus` 作为响应码
- `Details` 字段用于校验错误的字段级反馈（JSON 序列化到响应体）
