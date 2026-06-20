package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeaderAuth_WithXTokenUserEmailHeader(t *testing.T) {
	tests := []struct {
		name          string
		headerValue   string
		expectedEmail string
	}{
		{
			name:          "valid email header",
			headerValue:   "user@example.com",
			expectedEmail: "user@example.com",
		},
		{
			name:          "empty header value",
			headerValue:   "",
			expectedEmail: "",
		},
		{
			name:          "missing header",
			headerValue:   "",
			expectedEmail: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotEmail string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotEmail = UserEmailFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			handler := HeaderAuth(next)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.name != "missing header" {
				req.Header.Set("X-Token-User-Email", tt.headerValue)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if gotEmail != tt.expectedEmail {
				t.Errorf("email = %q, want %q", gotEmail, tt.expectedEmail)
			}
		})
	}
}

func TestHeaderAuth_RequestPassesThrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := HeaderAuth(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestUserEmailFromContext_NoValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	email := UserEmailFromContext(req.Context())
	if email != "" {
		t.Errorf("email = %q, want empty string", email)
	}
}
