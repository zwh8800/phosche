package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	apperrors "github.com/zwh8800/phosche/internal/errors"
	"github.com/zwh8800/phosche/internal/types"
)

type photoDetailResponse struct {
	*types.PhotoDocument
	PhotoURL string `json:"photo_url"`
}

func (s *Server) photoDetailHandler(w http.ResponseWriter, r *http.Request) {
	id, err := url.PathUnescape(chi.URLParam(r, "*"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Photo ID is required")
		return
	}
	id = strings.TrimPrefix(id, "/")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Photo ID is required")
		return
	}

	doc, err := s.Indexer.GetPhoto(r.Context(), id, s.IndexName)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) && appErr.Code == "NOT_FOUND" {
			writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Photo not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get photo")
		return
	}

	resp := photoDetailResponse{
		PhotoDocument: doc,
		PhotoURL:      "/photos/" + doc.Path,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
