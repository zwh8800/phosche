# Learnings - T25: Main Entry Point

## Architecture Decision: Split entry point pattern

Go's `//go:embed` directive has a fundamental constraint: patterns cannot contain `..` 
and must be relative to the source file's directory. Since `web/dist` is at the project root
and `cmd/phosche/` is two levels deep, embedding from `cmd/phosche/embed.go` is impossible.

**Solution**: Extracted all wiring logic into `internal/app/run.go` with signature:
```go
func Run(distFS fs.FS, configPath string)
```

This allows two build targets:
- `go build .` → root `main.go` + `embed.go` → embedded SPA (production)
- `go build ./cmd/phosche/` → thin wrapper calling `app.Run(nil, ...)` (no embedded SPA)

The `distFS` parameter uses `io/fs.FS` interface, accepting both `embed.FS` and `nil`.

## Module Wiring Pattern

All modules follow constructor-based dependency injection:
1. Load config via `config.LoadConfig(path)`
2. Create ES client → ping → `EnsureIndex`
3. Create `IndexerService` with circuit breaker
4. Create LLM client based on `cfg.LLM.Provider`
5. Create `ImageAnalyzer` with prompt, retries, timeout
6. Create `FSNotifyWatcher` + `DirectoryScanner`
7. Create `Pipeline` with all modules, start in goroutine
8. Create `SearchService` for API layer
9. Create API `Server` with `NewServer(searchSvc, indexerSvc, indexName)`
10. Setup HTTP routes: `/health` + `/api/` → chi router, `/photos/` → PhotoHandler, `/` → SPA

## Graceful Shutdown Pattern
- `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` for signal handling
- Cancel pipeline context first
- `http.Server.Shutdown(ctx)` with 10s timeout
- `pipelineCancel()` is safe to call multiple times (context.CancelFunc is idempotent)

## SPA Handler
- Routes `/` → serve `index.html`
- Checks if requested file exists in distFS, falls back to `index.html` for client-side routing
- Uses `http.FileServer` with `http.FS(distFS)` adapter
- DevMode: skips SPA serving entirely

## Files Changed
- `internal/app/run.go` — new: all wiring + graceful shutdown logic
- `cmd/phosche/main.go` — rewritten: thin wrapper calling `app.Run(nil, configPath)`
- `embed.go` (root) — new: `//go:embed web/dist` for production builds
- `main.go` (root) — new: embedded production entry point
