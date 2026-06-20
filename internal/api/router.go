// Package api 提供 phosche 的 REST API 层，基于 chi 路由器实现所有 HTTP 接口。
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/zwh8800/phosche/internal/geocoder"
	"github.com/zwh8800/phosche/internal/types"
)

// PhotoSearcher 定义 API 层依赖的照片搜索操作接口。
type PhotoSearcher interface {
	Search(ctx context.Context, indexName string, req *types.SearchRequest, userEmail string) (*types.SearchResponse, error)
	GetFilters(ctx context.Context, indexName string, userEmail string) (*types.FiltersResponse, error)
	GetStats(ctx context.Context, indexName string, userEmail string) (*types.StatsResponse, error)
	FindSimilar(ctx context.Context, indexName string, photoID string, embedding []float32, userEmail string) (*types.RecommendationResponse, error)
	FindNearby(ctx context.Context, indexName string, photoID string, lat, lon float64, userEmail string) (*types.RecommendationResponse, error)
}

// Indexer 定义 API 层依赖的照片索引操作接口。
type Indexer interface {
	GetPhoto(ctx context.Context, path string, indexName string) (*types.PhotoDocument, error)
	GetPhotoByID(ctx context.Context, id string, indexName string) (*types.PhotoDocument, error)
	DeletePhoto(ctx context.Context, path string, indexName string) error
	UpdateEXIF(ctx context.Context, path string, exif *types.EXIFInfo, indexName string) error
	UpdateGeo(ctx context.Context, path string, geo *types.GeoInfo, indexName string) error
	ScrollAll(ctx context.Context, indexName string, callback func(*types.PhotoDocument) error) error
}

// Server 是 API 层的核心结构体，持有搜索服务（searchService，私有）和索引服务（Indexer，公开）的引用。
type Server struct {
	searchService PhotoSearcher
	Indexer       Indexer
	IndexName     string
	Geocoder      geocoder.Geocoder
}

// NewServer 创建并初始化 Server 实例，注入搜索服务、索引服务、索引名称和逆地理编码器。
func NewServer(svc PhotoSearcher, idx Indexer, indexName string, geocoder geocoder.Geocoder) *Server {
	return &Server{
		searchService: svc,
		Indexer:       idx,
		IndexName:     indexName,
		Geocoder:      geocoder,
	}
}

// debugHeaders 是一个 HTTP 中间件，在 debug 日志级别打印每个请求的完整 header 信息。
// 仅当日志级别设为 debug 时才会输出，不影响生产环境性能。
func debugHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := make(map[string][]string, len(r.Header))
		maps.Copy(headers, r.Header)
		slog.Debug("request headers",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"remote_addr", r.RemoteAddr,
			"host", r.Host,
			"headers", headers,
		)
		next.ServeHTTP(w, r)
	})
}

// NewRouter 创建并配置 chi 路由器，注册所有中间件和路由。
//
// 中间件栈（按注册顺序执行）：
//   - middleware.Logger — 记录每个 HTTP 请求的方法、路径、状态码和耗时
//   - middleware.Recoverer — 捕获 panic，返回 500 状态码，防止服务崩溃
//   - middleware.Timeout(30s) — 每个请求最多执行 30 秒，超时自动中断
//   - cors.Handler — 配置跨域访问（允许所有来源，支持 GET/POST/PUT/DELETE/OPTIONS）
//   - HeaderAuth — 从 X-Token-User-Email header 提取用户邮箱注入 context
//
// 路由表：
//   GET  /health        — 健康检查，返回服务状态和版本号
//   GET  /api/photos    — 照片时间线列表，支持分页和日期/状态过滤
//   POST /api/photos/cleanup — 清理 ES 中文件系统中已不存在的孤儿文档
//   GET  /api/filters   — 获取标签、场景类型、相机型号等筛选选项
//   GET  /api/stats     — 获取照片总数、各状态分布和近期新增数量等统计信息
//   POST /api/search    — 全文搜索照片，支持关键词、日期、标签等多维过滤
//   GET  /api/photos/*  — 单张照片详情，路径参数为 URL 编码的照片路径
func NewRouter(srv *Server) chi.Router {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.Logger)
	r.Use(debugHeaders)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Token-User-Email"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(HeaderAuth)

	// Health check
	r.Get("/health", healthHandler)

	// API subrouter
	r.Route("/api", func(r chi.Router) {
		r.Get("/photos", srv.handleGetPhotos)
		r.Post("/photos/cleanup", srv.handleCleanup)
		r.Get("/filters", srv.filtersHandler)
		r.Get("/stats", srv.statsHandler)
		r.Post("/search", srv.searchHandler)
		r.Get("/photos/{id}", srv.photoDetailHandler)
		r.Get("/photos/{id}/similar", srv.similarPhotosHandler)
		r.Get("/photos/{id}/nearby", srv.nearbyPhotosHandler)
		r.Post("/migrate-timezone", srv.handleMigrateTimezone)
		r.Post("/migrate-geocode", srv.handleMigrateGeocode)
	})

	return r
}

// healthHandler 返回服务健康检查响应（status: ok + version: 0.1.0）。
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": "0.1.0",
	})
}

// writeJSON 将任意值序列化为 JSON 并写入 HTTP 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError 写入标准的 JSON 错误响应，格式为 {"error": "message"}。
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
