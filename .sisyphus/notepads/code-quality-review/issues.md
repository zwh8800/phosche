# Code Quality Review Issues

## Automated Checks

| Check | Result |
|-------|--------|
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `gofmt -d .` | FAIL — 8 files with formatting issues |
| `go test -short ./...` | PASS — 14/14 packages |

## gofmt Issues (8 files)

These are struct field alignment and import ordering issues. Fixable with `gofmt -w .`:

1. `internal/analyzer/analyzer.go` — import ordering (`jpeg` vs `gif`)
2. `internal/analyzer/ollama.go` — struct field alignment (tags misaligned)
3. `internal/analyzer/openai.go` — struct field alignment (tags misaligned)
4. `internal/api/photo_detail_test.go` — struct field alignment
5. `internal/app/run.go` — struct field alignment
6. `internal/decoder/decoder_test.go` — import ordering (`embed` before `image`)
7. `internal/search/search_test.go` — struct field alignment
8. `internal/types/types.go` — struct field alignment

## Go Code Smells

### 1. `interface{}` → `any` (Go 1.18+)
- **File**: `internal/analyzer/client_test.go`
- **Lines**: 47, 63, 67, 73, 82, 85, 114, 117, 156, 167, 171, 176, 183, 186, 201, 206, 209, 235, 240, 243
- **Severity**: LOW (test file only, 20 occurrences)
- **Fix**: Replace `interface{}` with `any`

### 2. Error suppression in production code (`_ = err`)
- **File**: `internal/pipeline/pipeline.go`
- **Lines**: 168, 237, 239, 294
- **Severity**: MEDIUM
- **Details**: `UpdateStatus` errors are silently discarded. Should at least log the error.
  ```go
  _ = p.cfg.Indexer.UpdateStatus(ctx, path, types.StatusAnalyzing, p.cfg.IndexName)  // line 168
  _ = p.cfg.Indexer.UpdateStatus(ctx, path, types.StatusPendingAnalysis, p.cfg.IndexName) // line 237
  _ = p.cfg.Indexer.UpdateStatus(ctx, path, types.StatusFailed, p.cfg.IndexName) // line 239
  _ = p.cfg.Indexer.UpdateStatus(ctx, pth, types.StatusFailed, p.cfg.IndexName) // line 294
  ```

### 3. Unused context parameter
- **File**: `internal/pipeline/pipeline.go:159`
- **Severity**: LOW
- **Details**: `func (p *Pipeline) worker(_ context.Context)` — discards context and creates `context.Background()` internally. Should either use the passed context or not accept it.

### 4. `panic` in test helper
- **File**: `internal/analyzer/analyzer_test.go:20`
- **Severity**: LOW (test code)
- **Details**: `panic(fmt.Sprintf("makeTestJPEG: %v", err))` — acceptable in test helper but `t.Fatalf` would be more idiomatic

## TypeScript/React Review

- **`as any` / `@ts-ignore`**: NONE — CLEAN
- **`console.log` in production**: NONE — CLEAN
- **Empty catch blocks**: NONE — CLEAN
- **`any` type usage**: NONE — CLEAN
- **Catch in Search.tsx:194**: Proper error handling — sets error state

## Verdict

**Go files: 8 with formatting issues, 4 code smell items (1 medium, 3 low)**
**TS files: ALL CLEAN**
