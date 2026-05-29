# Architectural Decisions - Phosche

## Session 1 (2026-05-28)
- **Heap**: Go + chi + React/Vite/Tailwind + ES 8.x
- **LLM**: Dual protocol (Ollama /api/chat, OpenAI /v1/chat/completions)
- **Dedup**: path + mtime + size (not content hash)
- **Storage**: ES only, no relational DB
- **Auth**: None (personal use)
- **Images**: Go static file server via /photos/
- **Logging**: slog (structured, JSON handler for prod)
- **Errors**: Unified AppError type with code/message/HTTP status

## F1: Plan Compliance Audit (2026-05-29)

### Must Have Verification (10/10 — ALL PASS)

| # | Requirement | Verified File(s) | Status |
|---|------------|------------------|--------|
| 1 | 递归目录监控 + 初始全量扫描 | watcher/fsnotify.go:72-86, scanner.go:52-107 | ✅ |
| 2 | Ollama + OpenAI 双 LLM 协议 | analyzer/ollama.go, analyzer/openai.go, analyzer/client.go:31-39 | ✅ |
| 3 | 结构化 LLM 响应 (JSON Schema) | analyzer/analyzer.go:23,137-153; ollama.go:55; openai.go:97-99 | ✅ |
| 4 | ES 全文搜索 + 日期范围 + EXIF + AI 标签过滤 | search/search.go:127-189 (multi_match, range, terms, term) | ✅ |
| 5 | Web 时间线浏览（按日期分组，无限滚动） | web/src/pages/Timeline.tsx:94-145 (useInfiniteQuery + IntersectionObserver + date grouping) | ✅ |
| 6 | 图片原图静态文件服务 | static/server.go:24-61 (path traversal prevention, ext allowlist, Cache-Control) | ✅ |
| 7 | YAML 配置文件，所有参数可配置 | config/config.go:10-132; config.example.yaml | ✅ |
| 8 | 优雅启停（信号处理） | app/run.go:125-139 (signal.NotifyContext + graceful HTTP shutdown) | ✅ |
| 9 | ES 不可用时继续运行（断路器 + 有界队列） | indexer/indexer.go:52-64,349-461 (circuit breaker, bounded chan, drain) | ✅ |
| 10 | 损坏图片日志警告 + 跳过（不崩溃） | decoder/decoder.go:66 (slog.Warn); pipeline/pipeline.go:203-207 | ✅ |

### Must NOT Have Verification (12/12 — ALL CLEAN)

| # | Prohibited Item | Search Scope | Verdict |
|---|----------------|-------------|---------|
| 1 | 用户认证/登录系统 | *.go — no auth middleware, no login handlers. Only OpenAI API key header (not user auth). | ✅ CLEAN |
| 2 | 关系型数据库 | go.mod — zero SQL dependencies. Codebase — no database/sql, GORM, sqlx, etc. | ✅ CLEAN |
| 3 | 缩略图生成 | Only in-memory scaling for LLM (analyzer.go:99-135) — explicitly allowed per T10 spec. No persistent thumbnails. | ✅ CLEAN |
| 4 | 图片上传功能 | Zero upload handlers, multipart parsing, or form-file endpoints. | ✅ CLEAN |
| 5 | 视频文件支持 | isImageFile() only: .jpg/.jpeg/.png/.webp/.heic/.heif (existing.go:10-17). No video extensions. | ✅ CLEAN |
| 6 | RAW 相机格式 | isImageFile + decoder only support 6 image extensions. No RAW format handlers. | ✅ CLEAN |
| 7 | 内容哈希去重 | Dedup = path + mtime + size (types.go:49-65). SHA-256 only for ES _id, not dedup. | ✅ CLEAN |
| 8 | 实时推送 (WebSocket/SSE) | Zero WebSocket/SSE dependencies or handlers. | ✅ CLEAN |
| 9 | 批量重新分析 | Retry loop is individual item retry (pipeline.go:256-301), not batch re-analysis. | ✅ CLEAN |
| 10 | 搜索结果导出 | No export/download/CSV/JSON-download endpoints. | ✅ CLEAN |
| 11 | AI 提示词 Web UI 编辑 | No prompt editing UI. Prompt configurable only via YAML config file. | ✅ CLEAN |
| 12 | 人脸识别 | `people_count` via LLM native capability only. No cv/face-recognition libraries. | ✅ CLEAN |

### Tasks Status (28/28)
All 28 tasks in the plan have `[x]` checkmark in the plan file.

### VERDICT: APPROVE
All Must Have items are present and correctly implemented.
All Must NOT Have items are absent.
Zero violations found.
