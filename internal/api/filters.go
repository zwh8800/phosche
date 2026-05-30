package api

import (
	"net/http"
)

// filtersHandler 返回前端筛选 UI 所需的聚合数据（标签、场景类型、相机型号）。
func (s *Server) filtersHandler(w http.ResponseWriter, r *http.Request) {
	filters, err := s.searchService.GetFilters(r.Context(), s.IndexName, UserEmailFromContext(r.Context()))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "search service unavailable: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, filters)
}
