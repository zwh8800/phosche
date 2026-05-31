package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	apperrors "github.com/zwh8800/phosche/internal/errors"
	"github.com/zwh8800/phosche/internal/types"
)

// photoDetailResponse 扩展 PhotoDocument，附加 photo_url 字段用于前端渲染图片。
type photoDetailResponse struct {
	*types.PhotoDocument
	PhotoURL string `json:"photo_url"`
}

// photoDetailHandler 获取单张照片详情，通过 URL 路径中的照片 ID 查询。
// 从 chi URL 参数中获取 URL 编码的照片路径，调用 Indexer.GetPhoto 从 ES 获取照片文档。
// 若返回 NOT_FOUND 错误，响应 404 状态码；成功时返回嵌入 photo_url 的照片详情。
func (s *Server) photoDetailHandler(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "*")
	if rawID == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Photo ID is required")
		return
	}

	id, err := url.PathUnescape(rawID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid photo ID")
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

	userEmail := UserEmailFromContext(r.Context())
	if doc.Email != "" && doc.Email != userEmail {
		writeJSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied")
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

// writeJSONError 写入结构化的 JSON 错误响应，格式为 {"error": {"code": "...", "message": "..."}}。
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
