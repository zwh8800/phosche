package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	apperrors "github.com/zwh8800/phosche/internal/errors"
)

// similarPhotosHandler handles GET /api/photos/{id}/similar
// Returns up to 3 photos with similar embedding vectors.
func (s *Server) similarPhotosHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Photo ID is required")
		return
	}

	// Fetch source photo
	doc, err := s.Indexer.GetPhotoByID(r.Context(), id, s.IndexName)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) && appErr.Code == "NOT_FOUND" {
			writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Photo not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get photo")
		return
	}

	// Access control check
	userEmail := UserEmailFromContext(r.Context())
	if doc.Email != "" && doc.Email != userEmail {
		writeJSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied")
		return
	}

	// Call search service
	resp, err := s.searchService.FindSimilar(r.Context(), s.IndexName, id, doc.Embedding, userEmail)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to find similar photos")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// nearbyPhotosHandler handles GET /api/photos/{id}/nearby
// Returns up to 3 photos geographically close to the source photo.
func (s *Server) nearbyPhotosHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Photo ID is required")
		return
	}

	// Fetch source photo
	doc, err := s.Indexer.GetPhotoByID(r.Context(), id, s.IndexName)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) && appErr.Code == "NOT_FOUND" {
			writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Photo not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get photo")
		return
	}

	// Access control check
	userEmail := UserEmailFromContext(r.Context())
	if doc.Email != "" && doc.Email != userEmail {
		writeJSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied")
		return
	}

	// Get GPS coordinates from EXIF
	var lat, lon float64
	if doc.EXIF != nil {
		lat = doc.EXIF.GPSLat
		lon = doc.EXIF.GPSLon
	}

	// Call search service
	resp, err := s.searchService.FindNearby(r.Context(), s.IndexName, id, lat, lon, userEmail)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to find nearby photos")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
