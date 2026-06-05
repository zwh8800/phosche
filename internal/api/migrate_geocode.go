package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/zwh8800/phosche/internal/types"
)

func (s *Server) handleMigrateGeocode(w http.ResponseWriter, r *http.Request) {
	go s.runGeocodeMigration()
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "geocode migration started",
	})
}

func (s *Server) runGeocodeMigration() {
	ctx := context.Background()
	updated := 0
	skipped := 0
	errors := 0

	slog.Info("geocode migration started")

	err := s.Indexer.ScrollAll(ctx, s.IndexName, func(doc *types.PhotoDocument) error {
		// Skip if no GPS coordinates
		if doc.Location == nil || (doc.Location.Lat == 0 && doc.Location.Lon == 0) {
			skipped++
			return nil
		}
		// Skip if already has geocoding data
		if doc.FormattedAddress != "" {
			skipped++
			return nil
		}
		// Skip if no geocoder configured
		if s.Geocoder == nil {
			skipped++
			return nil
		}

		geoInfo, geoErr := s.Geocoder.ReverseGeocode(ctx, doc.Location.Lat, doc.Location.Lon)
		if geoErr != nil {
			slog.Warn("migration: reverse geocode failed", "path", doc.Path, "error", geoErr)
			errors++
			return nil
		}
		if geoInfo == nil || geoInfo.FormattedAddress == "" {
			skipped++
			return nil
		}

		if err := s.Indexer.UpdateGeo(ctx, doc.Path, geoInfo, s.IndexName); err != nil {
			slog.Warn("migration: update geo failed", "path", doc.Path, "error", err)
			errors++
			return nil
		}

		updated++
		if updated%100 == 0 {
			slog.Info("geocode migration progress", "updated", updated, "skipped", skipped, "errors", errors)
		}
		return nil
	})

	if err != nil {
		slog.Error("geocode migration scroll failed", "error", err)
	}

	slog.Info("geocode migration completed", "updated", updated, "skipped", skipped, "errors", errors)
}
