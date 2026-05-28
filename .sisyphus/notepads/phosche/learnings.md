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
