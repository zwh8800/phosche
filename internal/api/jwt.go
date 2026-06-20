package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// JWTAuth 是一个 HTTP 中间件，从请求的 access_token cookie 中提取用户邮箱并注入 context。
// 不验证 JWT 签名——适合前置网关已验证身份的场景。
// 如果 cookie 不存在或格式无效，请求将继续处理（不中断）。
//
// Deprecated: 当前中间件栈已改用 HeaderAuth（从 X-Token-User-Email header 读取 email）。
// JWTAuth 保留作为备用，不再注册到路由中间件栈。
func JWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("access_token")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		email := parseJWTEmail(cookie.Value)
		if email != "" {
			ctx := context.WithValue(r.Context(), userEmailKey, email)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

func parseJWTEmail(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Email
}
