package api

import (
	"net/http"
)

func (s *Server) filtersHandler(w http.ResponseWriter, r *http.Request) {
	filters, err := s.searchService.GetFilters(r.Context(), s.IndexName)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "search service unavailable: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, filters)
}
