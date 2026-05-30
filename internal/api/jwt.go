package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey string

const userEmailKey contextKey = "userEmail"

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

func UserEmailFromContext(ctx context.Context) string {
	email, _ := ctx.Value(userEmailKey).(string)
	return email
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
