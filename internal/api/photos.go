package api

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/zwh8800/phosche/internal/types"
)

func (s *Server) handleGetPhotos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize = 50
	}

	req := &types.SearchRequest{
		DateFrom: q.Get("date_from"),
		DateTo:   q.Get("date_to"),
		Status:   q.Get("status"),
		Page:     page,
		PageSize: pageSize,
	}

	resp, err := s.searchService.Search(r.Context(), s.IndexName, req)
	if err != nil {
		slog.Error("photo search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCleanup(w http.ResponseWriter, r *http.Request) {
	req := &types.SearchRequest{
		Page:     1,
		PageSize: 10000,
	}

	resp, err := s.searchService.Search(r.Context(), s.IndexName, req)
	if err != nil {
		slog.Error("cleanup search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search failed during cleanup")
		return
	}

	deleted := 0
	for _, doc := range resp.Hits {
		if _, statErr := os.Stat(doc.Path); os.IsNotExist(statErr) {
			if delErr := s.Indexer.DeletePhoto(r.Context(), doc.Path, s.IndexName); delErr != nil {
				slog.Warn("cleanup delete failed", "path", doc.Path, "error", delErr)
				continue
			}
			deleted++
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted})
}
