package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/zwh8800/phosche/internal/types"
)

// PhotoSearcher defines the search interface used by photo endpoints.
type PhotoSearcher interface {
	Search(ctx context.Context, indexName string, req *types.SearchRequest) (*types.SearchResponse, error)
	GetFilters(ctx context.Context, indexName string) (*types.FiltersResponse, error)
	GetStats(ctx context.Context, indexName string) (*types.StatsResponse, error)
}

// Indexer defines the photo indexer operations the API layer depends on.
type Indexer interface {
	GetPhoto(ctx context.Context, path string, indexName string) (*types.PhotoDocument, error)
	DeletePhoto(ctx context.Context, path string, indexName string) error
}

type Server struct {
	searchService PhotoSearcher
	Indexer       Indexer
	IndexName     string
}

func NewServer(svc PhotoSearcher, idx Indexer, indexName string) *Server {
	return &Server{
		searchService: svc,
		Indexer:       idx,
		IndexName:     indexName,
	}
}

// NewRouter creates and configures the chi router with all middleware and routes.
func NewRouter(srv *Server) chi.Router {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
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

	// API subrouter
	r.Route("/api", func(r chi.Router) {
		r.Get("/photos", srv.handleGetPhotos)
		r.Post("/photos/cleanup", srv.handleCleanup)
		r.Get("/filters", srv.filtersHandler)
		r.Get("/stats", srv.statsHandler)
		r.Post("/search", srv.searchHandler)
		r.Get("/photos/*", srv.photoDetailHandler)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
