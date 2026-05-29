// Package errors 定义 phosche 的统一应用错误类型，支持错误码、HTTP 状态码和详情信息。
package errors

import "fmt"

// AppError 是 phosche 的标准应用错误类型，包含错误码、描述、HTTP 状态码和可选的内部错误。
//   - Code：错误码，如 NOT_FOUND、VALIDATION_ERROR
//   - Message：面向用户的错误描述
//   - HTTPStatus：对应的 HTTP 状态码
//   - Details：附加详情信息（可选），如校验失败的字段列表
//   - Err：原始错误（可选），用于内部错误包装
type AppError struct {
	Code       string `json:"code"`                  // 错误码，如 NOT_FOUND、VALIDATION_ERROR
	Message    string `json:"message"`               // 面向用户的可读错误消息
	HTTPStatus int    `json:"-"`                     // HTTP 状态码，如 400、404、500、503
	Details    any    `json:"details,omitempty"`     // 可选的错误详细信息，如校验失败的字段列表
	Err        error  `json:"-"`                     // 可嵌套的原始错误，支持 errors.Is/As 链式判断
}

// Error 实现 error 接口，返回格式为 "错误码: 描述" 或 "错误码: 描述: 原始错误"。
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap 返回被包装的原始错误，支持 errors.Is 和 errors.As 链式查找。
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewNotFoundError 创建 HTTP 404 资源未找到错误（错误码 NOT_FOUND）。
func NewNotFoundError(msg string) *AppError {
	return &AppError{
		Code:       "NOT_FOUND",
		Message:    msg,
		HTTPStatus: 404,
	}
}

// NewValidationError 创建 HTTP 400 参数校验错误（错误码 VALIDATION_ERROR），可附带详细信息。
func NewValidationError(msg string, details any) *AppError {
	return &AppError{
		Code:       "VALIDATION_ERROR",
		Message:    msg,
		HTTPStatus: 400,
		Details:    details,
	}
}

// NewInternalError 创建 HTTP 500 内部服务器错误（错误码 INTERNAL_ERROR），包装原始错误。
func NewInternalError(err error) *AppError {
	return &AppError{
		Code:       "INTERNAL_ERROR",
		Message:    "an internal error occurred",
		HTTPStatus: 500,
		Err:        err,
	}
}

// NewServiceUnavailableError 创建 HTTP 503 服务不可用错误（错误码 SERVICE_UNAVAILABLE）。
func NewServiceUnavailableError(msg string) *AppError {
	return &AppError{
		Code:       "SERVICE_UNAVAILABLE",
		Message:    msg,
		HTTPStatus: 503,
	}
}
