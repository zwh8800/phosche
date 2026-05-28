package api

import (
	"net/http"
)

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
