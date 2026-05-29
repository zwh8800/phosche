package api

import (
	"net/http"
)

// statsHandler 返回照片库统计数据：总数、各状态分布、最近1小时新增数。
func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := s.searchService.GetStats(r.Context(), s.IndexName)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "search service unavailable: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
