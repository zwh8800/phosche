package api

import (
	"encoding/json"
	"net/http"

	"github.com/zwh8800/phosche/internal/types"
)

// searchHandler 处理全文搜索请求，解析 JSON 请求体并验证分页参数（page>=1，page_size 1-100）。
func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req types.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	resp, err := s.searchService.Search(r.Context(), s.IndexName, &req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "search failed"})
		return
	}

	json.NewEncoder(w).Encode(resp)
}
