package embedding

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// EmbeddingError 是 Embedding 客户端的统一错误类型，包含错误分类信息。
// 使用方式：
//
//	var eei *embedding.EmbeddingError
//	if errors.As(err, &eei) {
//	    if eei.Retryable { ... }
//	}
type EmbeddingError struct {
	// Provider 标识错误来源："ollama" 或 "openai"
	Provider string

	// StatusCode HTTP 响应状态码。0 表示非 HTTP 错误（如网络错误）
	StatusCode int

	// Message 可读的错误描述
	Message string

	// Retryable 标记此错误是否可以通过重试解决
	Retryable bool

	// Cause 原始错误（可选）
	Cause error
}

func (e *EmbeddingError) Error() string {
	msg := fmt.Sprintf("[%s] %s", e.Provider, e.Message)
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *EmbeddingError) Unwrap() error {
	return e.Cause
}

// IsRetryable 判断 Embedding 错误是否可重试。
// 错误分类规则：
//   - EmbeddingError：根据其 Retryable 字段
//   - context.Canceled/DeadlineExceeded：不可重试
//   - net.Error（超时、DNS 等）：可重试
//   - url.Error：解包后递归判断
//   - 其他未识别：设为可重试（保守策略）
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// context canceled/deadline — 不可重试
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// EmbeddingError — 使用其分类
	var eei *EmbeddingError
	if errors.As(err, &eei) {
		return eei.Retryable
	}

	// 网络错误 — 可重试
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// URL 错误 — 解包后递归判断
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return IsRetryable(urlErr.Err)
	}

	// HTTP 状态码检查（通过字符串匹配作为兜底）
	errStr := err.Error()
	if strings.Contains(errStr, "status 4") {
		return false
	}
	if strings.Contains(errStr, "status 5") {
		return true
	}

	// 保守策略：未识别的错误类型允许重试
	return true
}

// IsEmbeddingError 检查错误是否为 EmbeddingError。
func IsEmbeddingError(err error) bool {
	var eei *EmbeddingError
	return errors.As(err, &eei)
}

// GetEmbeddingError 从错误链中提取 EmbeddingError。
func GetEmbeddingError(err error) *EmbeddingError {
	var eei *EmbeddingError
	errors.As(err, &eei)
	return eei
}

// NewRetryableError 创建一个可重试的 EmbeddingError。
func NewRetryableError(provider string, statusCode int, message string) *EmbeddingError {
	return &EmbeddingError{
		Provider:   provider,
		StatusCode: statusCode,
		Message:    message,
		Retryable:  true,
	}
}

// NewNonRetryableError 创建一个不可重试的 EmbeddingError。
func NewNonRetryableError(provider string, statusCode int, message string) *EmbeddingError {
	return &EmbeddingError{
		Provider:   provider,
		StatusCode: statusCode,
		Message:    message,
		Retryable:  false,
	}
}
