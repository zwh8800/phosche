package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/zwh8800/phosche/internal/decoder"
	"github.com/zwh8800/phosche/internal/types"
)

// handleMigrateTimezone 处理时区迁移请求。
// 遍历所有已分析的照片，重新提取 EXIF 并更新时区信息。
// 返回 202 Accepted 立即响应，后台异步执行迁移。
func (s *Server) handleMigrateTimezone(w http.ResponseWriter, r *http.Request) {
	go s.runTimezoneMigration()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "migration started",
	})
}

// runTimezoneMigration 执行时区迁移：遍历所有文档，重新提取 EXIF 并更新。
func (s *Server) runTimezoneMigration() {
	ctx := context.Background()
	updated := 0
	skipped := 0
	errors := 0

	slog.Info("timezone migration started")

	err := s.Indexer.ScrollAll(ctx, s.IndexName, func(doc *types.PhotoDocument) error {
		if doc.EXIF == nil || doc.EXIF.DateTimeOriginal == "" {
			skipped++
			return nil
		}

		newExif, err := decoder.ExtractEXIF(doc.Path)
		if err != nil {
			slog.Warn("migration: extract EXIF failed", "path", doc.Path, "error", err)
			errors++
			return nil
		}
		if newExif == nil {
			skipped++
			return nil
		}

		if newExif.DateTimeOriginal == doc.EXIF.DateTimeOriginal {
			skipped++
			return nil
		}

		if err := s.Indexer.UpdateEXIF(ctx, doc.Path, newExif, s.IndexName); err != nil {
			slog.Warn("migration: update EXIF failed", "path", doc.Path, "error", err)
			errors++
			return nil
		}

		updated++
		if updated%100 == 0 {
			slog.Info("migration progress", "updated", updated, "skipped", skipped, "errors", errors)
		}
		return nil
	})

	if err != nil {
		slog.Error("migration scroll failed", "error", err)
	}

	slog.Info("timezone migration completed", "updated", updated, "skipped", skipped, "errors", errors)
}
