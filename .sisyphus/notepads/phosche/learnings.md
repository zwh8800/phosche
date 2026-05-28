## T5: LLM Client Interface - Ollama + OpenAI

### Decisions
- Used `LLMClientConfig` (decoupled from `config.LLMConfig`) for the factory — avoids circular imports with config package
- Ollama response parsing handles both string-encoded JSON and object JSON in `message.content` — Ollama may return the analysis result as a JSON string within the content field

### Patterns
- Mock servers via `net/http/httptest` for both providers
- Both clients share the same `LLMClient` interface → easy to swap or add new providers
- Request bodies use typed structs with JSON tags rather than `map[string]interface{}` for safety
- `http.NewRequestWithContext` used for context-aware cancellation

### Results
- 4 source files: client.go, ollama.go, openai.go, client_test.go
- 12 tests, all PASS
- Zero LSP errors (only `interface{}` → `any` hints in test file)
- No external dependencies beyond stdlib

## T4: Elasticsearch 客户端 + 索引映射

### Implementation Notes
- Used `ESClient` struct wrapping `*elasticsearch.Client` with a `*slog.Logger` field for structured logging
- Exposed `NewESClientWithLogger` constructor for testability — allows capturing slog output in tests
- `NewESClient` defaults to `slog.Default()` for production use
- TLS config via `http.Transport.TLSClientConfig.InsecureSkipVerify` — only set when `cfg.InsecureSkipVerify` is true
- Ping ES via `client.Info()` in constructor — returns error (never panics)
- Index mapping uses nested `map[string]any` for clear structure, with `textWithKeyword` shared var for tags/objects multifields
- `_meta.version` stored inside `mappings` per Elasticsearch convention
- Version mismatch: only `slog.Warn()`, no auto-migration

### Test Infrastructure
- `exec.LookPath("docker")` for Docker availability check — simple and reliable enough
- `wait.ForHTTP("/").WithPort("9200/tcp")` with 2min startup timeout for ES readiness
- `ES_JAVA_OPTS=-Xms512m -Xmx512m` to limit memory in test containers
- `captureLogger` helper uses `slog.NewTextHandler` with `LevelWarn` for verifying log output
- VersionMismatch test creates its own container (isolated from other tests) because it needs a raw ES client and a logger-wrapped client
- Non-Docker tests (InvalidAddress, WithTLS) run regardless of Docker availability

### Dependencies
- `github.com/elastic/go-elasticsearch/v8` v8.19.6
- `github.com/testcontainers/testcontainers-go` v0.42.0
- `github.com/testcontainers/testcontainers-go/modules/elasticsearch` v0.42.0

## T9: DirectoryScanner Implementation

### Patterns & Conventions
- Reused `isImageFile()` and `supportedExts` map from `existing.go` — no duplication of the extension list
- `DirectoryScanner` is a zero-value struct — no state needed, just the `Scan` method
- Symlink detection: `info.Mode()&os.ModeSymlink != 0` in `filepath.Walk` callback; return `filepath.SkipDir` for directories, `nil` for files
- Used `os.Lstat` for root dir check to avoid following symlinked roots
- Context cancellation checked at two levels: before each directory iteration, and inside the walk callback for each entry
- Sort by mtime descending using `sort.Slice` with `scanEntry` struct

### Test Patterns
- Package `watcher_test` (external test package) — same as existing `watcher_test.go`
- Reused `createFile` helper from `watcher_test.go` (same package)
- For sort order test: used `os.Chtimes` to set specific mtimes
- For context cancellation: created 50 files + pre-cancelled context; both "partial results" and "error" outcomes are acceptable per spec

## T10: ImageAnalyzer — Prompt Engine + Retry + Image Preprocessing

### Decisions
- `isRetryable()` checks `*url.Error` before `net.Error` because `*url.Error` implements `net.Error` → reversed order avoids unreachable case (`SA4020`)
- 4xx detection via `strings.Contains(errStr, "status 4")` on client error messages — parses existing error format from ollama.go/openai.go without modifying the client interface
- `preprocessImage()` always re-encodes to JPEG at quality 85 for LLM transport, even when no resize is needed
- Context deadline from `timeout` is applied once for the entire Analyze call (all retry attempts share it), not per-attempt

### Patterns
- Mock LLMClient with `mockLLMClient` + pre-configured `mockCall` slice — clean separation of test scenarios (retry success, exhausted, 4xx, etc.)
- `capturingMockClient` captures image data for resize verification without a real LLM
- `blockingMockClient` blocks on `<-ctx.Done()` to simulate slow LLM for timeout tests
- Tests always pass valid JPEG bytes via `makeTestJPEG()` helper since `preprocessImage` decodes all input

### Results
- 2 files: analyzer.go, analyzer_test.go
- 7 new tests, all PASS; 19 total in package
- Zero LSP errors/warnings
- Build passes: `go build ./...`

## T8: FSNotifyWatcher — Recursive fsnotify File Monitor

### Implementation Notes
- `FSNotifyWatcher` implements `Watcher` interface from `watcher/types.go`
- Reuses `isImageFile()` and `supportedExts` from `existing.go` — no extension list duplication
- `WatcherConfig{DebounceMs int}` — defaults to 500ms if ≤ 0, matching config default
- `fsnotify.Watcher` created in `Watch()`, closed in goroutine defer (not in Close())

### Debounce Design
- Map `pending map[string]*pendingItem` protected by `timersMu sync.Mutex`
- `pendingItem` holds latest `types.FileEvent` + `*time.Timer`
- On event: stop existing timer, update event, start new timer
- Timer callback: deletes from pending, sends to buffered channel (cap 256)
- `atomic.Bool` `closed` flag checked before send — prevents sending after Close()
- `time.AfterFunc` chosen over manual timer loop for simplicity

### Directory Watching
- `filepath.Walk` adds all existing subdirs (skip errors, log warn)
- On `fsnotify.Create` for directories: `fw.Add(e.Name)` — runtime subdir tracking
- 200ms sleep in test between dir creation and file creation to allow watcher setup

### Test Findings
- `os.Chtimes` does NOT trigger fsnotify events on macOS (kqueue backend) — must create/write files after watch starts
- External test package (`watcher_test`) consistent with existing test files
- Debounce test uses 80ms intervals with 300ms debounce window — timing works on macOS
- 5s deadline for recursive test to allow generous event collection window

### Stats
- 2 source files: fsnotify.go, fsnotify_test.go
- 6 tests, all PASS
- Zero LSP errors, zero build errors
- Added dependency: `github.com/fsnotify/fsnotify` v1.10.1

## T11: IndexerService — ES Indexer with Circuit Breaker + Bounded Queue

### Implementation Notes
- `IndexerService` wraps `*ESClient` (same package), adds circuit breaker + bounded retry queue
- Document `_id` = `sha256(path)` hex-encoded — deterministic, collision-resistant
- All write ops (IndexPhoto, UpdateStatus, BulkIndex) return `nil` on ES failure — caller sees success
- `Refresh: "true"` on all writes for immediate read consistency (suitable for photo library throughput)
- `esapi.UpdateRequest` with `{"doc":{"status":...}}` for partial status updates — doesn't overwrite other fields
- `esutil.BulkIndexer` for BulkIndex — thread-safe with mutex-protected failure counter
- `BulkIndexerConfig{NumWorkers: 4, FlushBytes: 5MB, FlushInterval: 30s}` for reasonable bulk throughput

### Circuit Breaker Design
- States: CLOSED (normal), OPEN (failing). 3 consecutive failures → OPEN.
- `recordWriteResult(bool)` increments/resets counter and toggles circuit state under write lock
- `isCircuitOpen()` check at top of each write method — if open, queue immediately (no ES call)
- Background goroutine: every 5s, if circuit is OPEN, ping ES via `client.Info()`
- On ping success: CLOSE circuit, drain queue via `drainQueue()` (non-blocking select loop)
- `Stop()` closes `done` chan → goroutine calls `drainQueue()` before returning
- Queue full → `slog.Warn("queue full, dropping document")` — graceful degradation

### Test Strategy
- Reused `setupTestES(t)` from `client_test.go` for testcontainer setup
- Unique index names per test via `time.Now().UnixNano()` suffix
- `TestIndexer_QueueOnFailure`: uses nonexistent index name to trigger ES errors → circuit opens → no crash, nil return
- Docker-dependent tests properly skip with `t.Skip("Docker not available")`
- Non-Docker unit tests (InvalidAddress, WithTLS) pass regardless

### Gotchas
- `go-elasticsearch v8` `Client.Info()` takes no context arg — removed unused `ctx` from `pingES()`
- `esapi.UpdateRequest` has `DocumentID` field (not `ID`) — matches go-elasticsearch v8 convention
- `esapi.GetRequest` response body: `{"_source": {...}}` — must decode into wrapper struct

### Results
- 2 files: indexer.go (448 lines), indexer_test.go (310 lines)
- 11 new tests, all PASS (8 skip on no-Docker, 3 unit tests pass always)
- Zero LSP errors, zero vet warnings
- `go build ./...` passes

## T14: Pipeline Orchestrator — Worker Pool + Bounded Queue + Retry

### Design Decisions
- Defined `Analyzer` and `Indexer` interfaces locally in pipeline package (Go idiom: consume interfaces where needed) — `*analyzer.ImageAnalyzer` and `*indexer.IndexerService` satisfy them automatically
- Worker contexts: each item gets a fresh `context.WithTimeout(context.Background(), 5min)` — avoids cancelled-context issues during graceful shutdown drain phase
- `IsLLMConnectionError` exported (capital I) for testability — detects `net.Error` in the error chain via `errors.As` unwrapping
- Pending retry map stored as `Pipeline.pending` struct field (not goroutine-local) — allows `updateErrorStatus` to add items and `processPath` to remove on success

### Pipeline Flow
1. `scanExisting` → sends paths to inputCh (bounded channel)
2. `forwardEvents` goroutine → forwards watcher FileEvent paths to inputCh (skips delete)
3. N worker goroutines → read from inputCh, call `processPath` per item
4. Retry goroutine → periodic ticker; iterates pending map; retries items up to maxPendingRetries
5. Graceful shutdown: `ctx.Done` → `watcher.Close()` → `fwWg.Wait()` → `close(inputCh)` → `workersWg.Wait()` → `retryWg.Wait()` → `indexer.Stop()`

### Test Patterns
- `runPipeline(t, p)` helper returns `(cancel, wait)` — runs pipeline in goroutine, `wait()` blocks until Run returns
- `require.Eventually` for async verification (no brittle `time.Sleep` polling)
- Real temp JPEG files created via `image.NewRGBA + jpeg.Encode` — enables real `decoder.DecodeImage` tests
- Concurrency test: mock analyzer blocks on channel → verify maxConcurrent → release → verify all processed
- No defer+wrapper double-wait pattern (avoided `cancel/wait` being called twice)

### Gotchas
- Go 1.26 `:=` short variable declarations don't allow self-reference in closures within struct literals → use `var x *T; x = &T{...}` pattern
- Worker must use fresh context per item (not pipeline ctx) to survive graceful shutdown drain phase — otherwise in-flight analysis gets "context canceled"
- `IndexPhoto` doesn't call `UpdateStatus` — the document's `Status` field IS the status update; `UpdateStatus` only used for intermediate states (analyzing, pending_analysis, failed)

### Results
- 2 files: pipeline.go (302 lines), pipeline_test.go (523 lines)
- 9 tests (8 pipeline + 1 IsLLMConnectionError), all PASS
- Zero LSP errors; only Go 1.26 modernization hints
- `go test ./internal/pipeline/` → PASS (0.968s)

## T18: Photo Detail API — GET /api/photos/{id}

### Route Design
- Used chi catch-all `*` pattern (`/photos/*`) for path-based IDs containing slashes — `chi.URLParam(r, "*")` captures everything after the prefix
- `url.PathUnescape` decodes URL-encoded paths (e.g., `%2F` → `/`)
- `strings.TrimPrefix(id, "/")` removes leading slash from captured path

### Handler Pattern
- `writeJSONError` helper returns structured error: `{"error":{"code":"...","message":"..."}}`
- `errors.As(err, &appErr)` to unwrap `*AppError` and check `Code == "NOT_FOUND"` for 404
- Non-matching errors → 500 with generic INTERNAL_ERROR message
- Response includes all PhotoDocument fields (flattened via struct embedding) + `photo_url`

### Response Structure
```go
type photoDetailResponse struct {
    *types.PhotoDocument  // embeds Photo + AnalysisResult → all fields flattened
    PhotoURL string       `json:"photo_url"`  // "/photos/" + doc.Path
}
```

### Mock Pattern
- `mockIndexer` implements the `Indexer` interface with a `getPhotoFunc` function field
- Functions embedded in test assertions verify correct path/indexName params

### Results
- 2 files: photo_detail.go (62 lines), photo_detail_test.go (165 lines)
- 3 new tests (success, not found, internal error), all PASS
- All 22 tests across the package PASS

### Side Fixes During T18
- Fixed `search.go`: page/page_size defaults moved before validation so `{"query":"sunset"}` without explicit page defaults to page=1, size=20 instead of 400 error
- Fixed `stats.go`: replaced `writeError` (undefined) with `writeJSON` call
- Added `GetFilters`/`GetStats` to `PhotoSearcher` interface for `filters.go`/`stats.go` compilation
- Added `DeletePhoto` to `Indexer` interface for `photos.go` compilation

## T15: API Router Setup (2026-05-29)

### Dependencies Added
- `github.com/go-chi/chi/v5` v5.3.0
- `github.com/go-chi/cors` v1.2.2

### Implementation Notes
- chi v5.3.0 `middleware.Timeout` returns HTTP 504 (Gateway Timeout), NOT 503 as older versions did
- go-chi/cors v1.2.2: `Access-Control-Allow-Methods` header only appears on preflight (OPTIONS) responses, NOT on simple GET/POST
- CORS middleware intercepts preflight OPTIONS requests on any path (including non-existent routes) and returns 200 with CORS headers — but OPTIONS /health returns 405 because the route handler explicitly rejects non-GET methods

## T21: Timeline Page (2026-05-29)

### Implementation Notes
- Created `web/src/pages/Timeline.tsx` — full timeline page with date grouping, infinite scroll, skeleton loading
- Modified `web/src/App.tsx` — added `QueryClientProvider` wrapper and imported Timeline from `./pages/Timeline`
- `FetchPhotosResponse` has NO `total_pages` field → computed via `Math.ceil(total / page_size)` in `getNextPageParam`
- `verbatimModuleSyntax: true` in tsconfig → ALL type-only imports MUST use `import type` syntax (e.g., `import type { PhotoDocument }`)
- `noUnusedLocals: true` + `noUnusedParameters: true` → strict TS checks, every variable must be used
- Image src constructed as `/photos/${path.replace(/^\/+/, '')}` to avoid double slashes

### Date Extraction Logic
- Primary: parse `exif.date_time_original` — normalize EXIF "2024:01:15 14:30:00" → ISO by replacing `:` with `-` and space with `T`
- Fallback: `created_at` timestamp — auto-detect seconds vs milliseconds via `> 1e12` threshold

### Component Structure
- `useInfiniteQuery` from `@tanstack/react-query` v5 — `queryKey: ['photos']`, `pageParam`, `getNextPageParam`
- `IntersectionObserver` on sentinel div triggers `fetchNextPage()` when visible
- `useMemo` flattens all pages + groups by date (Map), sorted newest-first
- States: error (warning icon), loading (6 skeleton cards), empty ("还没有照片" with image icon), data (grouped sections)
- Skeleton: `animate-pulse` + gray `bg-gray-200` placeholders with aspect-square
- Photo cards: `<button>` (for clickable semantics), `aspect-square` thumbnail with `object-cover`, `group-hover:scale-105` transition, tags as purple pills

### Results
- 2 files changed: `pages/Timeline.tsx` (new, 249 lines), `App.tsx` (modified)
- `tsc -b` → PASS, `vite build` → PASS
- Bundle: 314 KB JS (102 KB gzipped), 29 KB CSS (6.5 KB gzipped)

## T24: Responsive Tuning + 404 Page + Error Boundary (2026-05-29)

### Files Created
- `web/src/pages/NotFound.tsx` — 404 page with purple-200 "404" big text, "页面未找到" heading, "返回首页" link button
- `web/src/components/ErrorBoundary.tsx` — React class component error boundary; catches render errors; shows "出错了" with error message in red box + "刷新页面" and "返回首页" buttons

### Files Modified
- `web/src/components/Layout.tsx` — Added `useState` for `mobileMenuOpen` toggle; hamburger button (3-line SVG) visible on `md:hidden`, transforms to X when open; nav links extracted to `navLinks` variable reused in both desktop (`hidden md:flex`) and mobile dropdown; mobile dropdown has `border-t` separator
- `web/src/App.tsx` — Wrapped entire app in `<ErrorBoundary>`; added `path="*"` catch-all route pointing to `<NotFound />`

### Patterns
- ErrorBoundary wraps the entire app (outside QueryClientProvider + BrowserRouter) since it doesn't use router hooks
- Mobile nav reuses the same `navLinks` JSX fragment in both desktop and mobile positions — single source of truth
- Hamburger button toggles between hamburger (3 lines) and close (X) SVG icons based on `mobileMenuOpen` state
- NavLink's `onClick` closes mobile menu on navigation, preventing stale open state

### Results
- 4 files touched (2 new, 2 modified)
- `tsc -b && vite build` → PASS
- Bundle: 338 KB JS (108 KB gzipped), 30 KB CSS (6.7 KB gzipped)

## T27: Dockerfile + Makefile (2026-05-29)

### Implementation Notes
- Multi-stage Dockerfile: `golang:1.26-alpine` (go-builder) → `node:20-alpine` (frontend-builder) → `alpine:latest` (runtime)
- `CGO_ENABLED=0` for static binary — no libheif dependency in Docker
- Makefile includes: `build`, `build-frontend`, `docker-build`, `test`, `test-race`, `clean`, `run`
- Docker-compose at `build: .` auto-discovers the Dockerfile — no changes needed to existing compose file
- Runtime copies `config.example.yaml` as `config.yaml` (default config, overridable via volume mount in compose)
- Runtime includes `ca-certificates` (for HTTPS/TLS) and `tzdata` (for timezone handling)

## T28a+T28b: E2E Integration Tests — Backend + Frontend (2026-05-29)

### Backend E2E (`internal/integration/e2e_test.go`)

**Implementation Notes:**
- Reused the same testcontainers ES pattern from `client_test.go` and `search_test.go` — `docker.elastic.co/elasticsearch/elasticsearch:8.17.0`, `wait.ForHTTP("/")`, `ES_JAVA_OPTS=-Xms512m -Xmx512m`
- Extracted `startESContainer` helper that returns `(container, address, cleanup)` for clean setup/teardown
- Mock scanner (`mockScanner`), mock watcher (`mockWatcher`), mock analyzer (`mockAnalyzer`) — same interface satisfaction pattern as `pipeline_test.go`
- Pipeline detects completion by polling `SearchService.Search` with `Status: "analyzed"` filter — `require.Eventually` with 30s timeout, 500ms interval
- After processing confirmed: `GetPhoto` for document field verification, `Search` for full-text query verification
- Graceful pipeline shutdown via context cancellation + channel read with 30s timeout

**Skip Logic:**
- `testing.Short()` → skip ("skipping E2E test in short mode")
- `exec.LookPath("docker")` → skip ("Docker not available")
- Both guards at top of `TestEndToEnd` before any expensive setup

**Verification:**
- `go vet ./internal/integration/` → PASS
- `go test -c -o /dev/null ./internal/integration/` → PASS
- `go test -timeout 120s ./internal/integration/` → PASS (skips without Docker)
- All existing tests in `./internal/...` → PASS

### Frontend E2E (`web/e2e/app.spec.ts`)

**Implementation Notes:**
- Installed `@playwright/test` as dev dependency (Playwright CLI v1.60.0 was already available)
- Created `web/playwright.config.ts` with `webServer.command: 'npm run dev'` and `reuseExistingServer: !CI`
- 3 tests: app loads + navigates, search page has input, 404 page works
- Test selectors based on actual DOM: `nav` element, `text=搜索` link, `input[type="text"]`, `text=404`

**Playwright Config:**
- `testDir: './e2e'`
- `fullyParallel: true`
- `retries: CI ? 2 : 0`
- `baseURL: 'http://localhost:5173'`
- Only `chromium` project configured

**Verification:**
- `npx playwright test --list` → 3 tests listed, all valid
- `npx tsc --noEmit --project tsconfig.app.json` → PASS (framework code compiles)

### Files Created
- `internal/integration/e2e_test.go` (227 lines)
- `web/playwright.config.ts` (23 lines)
- `web/e2e/app.spec.ts` (17 lines)

### Files Modified
- `web/package.json` — added `@playwright/test` dev dependency
- `web/package-lock.json` — updated lock file
