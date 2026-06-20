package api

import (
	"context"
	"net/http"
)

type contextKey string

const userEmailKey contextKey = "userEmail"

// HeaderAuth 是一个 HTTP 中间件，从请求的 X-Token-User-Email header 中提取用户邮箱并注入 context。
// 适合上游网关已校验 JWT 并将用户信息注入 header 的场景。
// 如果 header 不存在或为空，请求将继续处理（不中断）。
func HeaderAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := r.Header.Get("X-Token-User-Email")
		if email != "" {
			ctx := context.WithValue(r.Context(), userEmailKey, email)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// UserEmailFromContext 从请求 context 中提取由认证中间件（HeaderAuth 或 JWTAuth）注入的用户邮箱。
// 若未找到则返回空字符串，调用方应检查空值。
func UserEmailFromContext(ctx context.Context) string {
	email, _ := ctx.Value(userEmailKey).(string)
	return email
}
