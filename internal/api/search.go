package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/zwh8800/phosche/internal/types"
)

// searchHandler 处理全文搜索请求，解析 JSON 请求体并验证分页参数（page>=1，page_size 1-100）。
func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req types.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("search request body decode failed", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	if req.Page < 1 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "page must be at least 1"})
		return
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "page_size must be between 1 and 100"})
		return
	}

	slog.Debug("searchHandler dispatching",
		"query", req.Query,
		"page", req.Page,
		"page_size", req.PageSize,
		"user_email", UserEmailFromContext(r.Context()),
		"has_tags", len(req.Tags) > 0,
		"has_objects", len(req.Objects) > 0,
		"scene_type", req.SceneType,
		"status", req.Status,
	)

	resp, err := s.searchService.Search(r.Context(), s.IndexName, &req, UserEmailFromContext(r.Context()))
	if err != nil {
		slog.Error("searchHandler: searchService.Search returned error",
			"index", s.IndexName,
			"query", req.Query,
			"page", req.Page,
			"page_size", req.PageSize,
			"error", err.Error(),
			"error_type", err.Error()[:min(200, len(err.Error()))],
		)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "search failed",
			"details": err.Error(),
		})
		return
	}

	slog.Debug("searchHandler success",
		"hits_count", len(resp.Hits),
		"hits_total", resp.Total,
		"total_pages", resp.TotalPages,
	)

	json.NewEncoder(w).Encode(resp)
}
