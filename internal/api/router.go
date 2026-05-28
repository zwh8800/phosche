package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Server holds all service dependencies for API endpoints.
type Server struct {
	// These will be filled in by subsequent tasks (T16-T19)
}

// NewRouter creates and configures the chi router with all middleware and routes.
func NewRouter() chi.Router {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.Logger)                     // request logging
	r.Use(middleware.Recoverer)                  // panic recovery
	r.Use(middleware.Timeout(30 * time.Second))  // request timeout
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/health", healthHandler)

	// API subrouter (T16-T19 will add endpoints here)
	r.Route("/api", func(r chi.Router) {
		// Placeholder — sub-tasks add routes here
	})

	return r
}

// healthHandler responds with a basic health check.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": "0.1.0",
	})
}
