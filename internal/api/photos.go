package api

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/zwh8800/phosche/internal/types"
)

// handleGetPhotos 处理照片时间线查询，支持分页、日期范围和状态过滤。
// 从 URL 查询参数中解析以下可选参数：
//   - page (int, 默认 1) — 页码
//   - page_size (int, 默认 50) — 每页照片数量
//   - date_from (string, 格式 YYYY-MM-DD) — 起始拍摄日期过滤
//   - date_to (string, 格式 YYYY-MM-DD) — 结束拍摄日期过滤
//   - status (string) — 按处理状态过滤（unanalyzed/analyzing/analyzed/failed/pending_analysis）
// 构造 SearchRequest 后委托给 searchService.Search 执行搜索，返回分页的时间线结果。
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

// handleCleanup 扫描 ES 索引中的所有照片，删除文件系统中已不存在的孤儿文档。
// 工作流程：
//  1. 以 page_size=10000 扫描 ES 索引中所有已索引的照片文档
//  2. 对每个文档，使用 os.Stat 检查对应文件是否仍存在于文件系统中
//  3. 若文件已不存在，调用 Indexer.DeletePhoto 删除该 ES 文档
//  4. 返回被删除的孤儿文档总数
// 响应格式：{"deleted": <count>}
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
