# AGENTS.md — Phosche

Personal photo search service: monitors directories, analyzes photos with multimodal AI (Ollama/OpenAI), indexes to Elasticsearch, serves a React SPA.

## Quick Reference

```bash
# Dev mode (backend only, needs ES running)
make run

# Dev mode (frontend with hot reload, separate terminal)
cd web && npm run dev

# Build everything
make build-frontend && make build

# Tests
make test              # all Go unit tests
make test-race         # with race detector
cd web && npm test     # frontend unit tests (vitest)
cd web && npx playwright test  # E2E tests

# Docker
docker compose up -d                        # ES + phosche
docker compose --profile ollama up -d       # + local Ollama
```

## Architecture

### Two Entrypoints (Critical)

- **`main.go` (root)** — Production. Embeds `web/dist` via `//go:embed`. Single binary serves API + SPA.
- **`cmd/phosche/main.go`** — Development. Passes `nil` for distFS; frontend served by Vite dev server.

Both call `app.Run(distFS, configPath)`. The `distFS` parameter controls whether the embedded SPA is used.

### Pipeline Pattern

```
watch (fsnotify) → decode (JPEG/PNG/WebP/HEIC) → analyze (LLM) → index (ES) → serve (React SPA)
```

Orchestrated in `internal/pipeline/`. Each stage is a separate package with clean interfaces.

### Internal Packages

| Package | Purpose |
|---------|---------|
| `analyzer` | LLM client interface + Ollama/OpenAI implementations. Factory: `NewLLMClient()` |
| `api` | chi router, all REST handlers. Interfaces: `PhotoSearcher`, `Indexer` |
| `app` | Wiring, lifecycle, graceful shutdown |
| `cache` | Thumbnail/full-size image cache generation |
| `config` | YAML loading, defaults, validation |
| `decoder` | Multi-format image decode + EXIF extraction |
| `errors` | Unified `AppError` types (NOT_FOUND, VALIDATION_ERROR, etc.) |
| `geocoder` | Reverse geocoding via Amap API |
| `indexer` | ES client wrapper, circuit breaker, bounded write queue |
| `integration` | E2E tests using testcontainers-go (auto-starts ES container) |
| `pipeline` | Stage orchestration, concurrency control |
| `search` | ES query builder (full-text, filters, aggregations) |
| `static` | Photo file serving with path traversal protection |
| `types` | Shared types: PhotoDocument, EXIF, SearchRequest/Response |
| `watcher` | fsnotify watcher + directory scanner + dedup filter |

### Photo Status State Machine

`unanalyzed → analyzing → analyzed` (success)
`unanalyzed → analyzing → pending_analysis` (LLM down, retries 10x every 5min)
`unanalyzed → analyzing → failed` (unrecoverable error)

## Configuration

- Copy `config.example.yaml` → `config.yaml` (gitignored)
- Required fields: `watch.directories`, `elasticsearch.addresses`, `llm.provider`
- Config loaded in `internal/config/config.go` with defaults applied and validated
- Env override: set `CONFIG_PATH` to override default `config.yaml` path

## Frontend (web/)

- React 19 + TypeScript 6 + Vite 8 + Tailwind CSS 4
- State: TanStack React Query 5
- Routing: react-router-dom 7
- HTTP client: axios
- Vite dev server proxies `/api`, `/photos`, `/health` → `localhost:8080`
- Tests: vitest (unit), Playwright (E2E)

## Testing Quirks

- Integration tests (`internal/integration/`) use testcontainers-go — requires Docker running
- `make test` runs all Go tests including integration (will pull ES image on first run)
- Frontend tests are separate: `cd web && npm test`
- E2E tests require backend running: start with `make run`, then `cd web && npx playwright test`

## Conventions

- Go standard formatting (`go fmt`) — no custom linter config found
- All API endpoints prefixed `/api/` except `/health` and `/photos/*`
- Photo IDs are SHA-256 hash of file path
- Structured logging via `log/slog` (JSON format)
- `cache/` directory is gitignored (generated thumbnails from `internal/cache/`)
