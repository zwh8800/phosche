# Learnings

## T1: Project Scaffold

### Go Project Layout
- Module: `github.com/zwh8800/phosche`, Go 1.26.2
- Standard layout: `cmd/` for binaries, `internal/` for private packages
- Internal packages created: config, watcher, decoder, analyzer, indexer, search, api, static, pipeline, types, errors
- `go build` with no external dependencies works fine for a minimal main.go

### slog Logger Setup
- Use `slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})` for production logging
- Set as default via `slog.SetDefault()` so all packages can use `slog.Info()` etc.
- No external logging dependency needed

### Web Project
- `npm create vite@latest web -- --template react-ts` creates the scaffold
- Additional deps installed: react-router-dom, axios, @tanstack/react-query
- Built successfully with `npm run build` → `dist/` folder generated

### Testing
- Test files in `package main` need explicit import of `log/slog` to reference `slog.Default`
- Build test (`go build -o /dev/null`) passes independently of test file imports

## T3 - Types and Errors

- `internal/types/types.go`: 11 type definitions with `json` and `es` struct tags. Used Go doc comments on all exported symbols (required by convention).
- `internal/errors/errors.go`: `AppError` struct with `Code`, `Message`, `HTTPStatus` (excluded from JSON via `json:"-"`), `Details` (included in JSON), and `Err` (excluded from JSON). Constructor functions for 4 error types.
- `AppError` implements both `Error()` and `Unwrap()` (for `errors.Is`/`errors.As` support).
- `internal/errors/errors_test.go` includes: `TestAppError_Error`, `TestAppError_ErrorWithWrapped`, `TestAppError_JSONMarshal`, `TestAppError_Unwrap`, `TestAppError_UnwrapNil`, `TestConstructors` (table-driven with subtests), `TestValidationError_Details`.
- `PhotoDocument` uses Go struct embedding — fields from `Photo` and `AnalysisResult` are flattened in JSON.
- All 11 tests pass, 0 LSP diagnostics.

## T2 - YAML Config System

- `internal/config/config.go`: 6 struct types (`Config`, `WatchConfig`, `LLMConfig`, `OllamaConfig`, `OpenAIConfig`, `ESConfig`, `ServerConfig`), `LoadConfig()` reading+unmarshal+defaults+validation.
- `gopkg.in/yaml.v3` as YAML library — minimal, `go get` adds it.
- `applyDefaults()` sets missing optional fields: debounce_ms=500, max_retries=3, concurrency=2, timeout_seconds=60, host="0.0.0.0", port=8080, recursive=true, output_language="zh", min_dir_depth=1.
- `validate()` checks: watch.directories non-empty, llm.provider in [ollama, openai], elasticsearch.addresses non-empty, elasticsearch.index_name non-empty.
- Bool default limitation: `Recursive bool` has zero-value=false, so we always default to true. If user sets `recursive: false`, it's overridden. Future fix: use `*bool`.
- Tests use `t.TempDir()` to create temp YAML files — 6 test functions, all passing.
- `config.example.yaml` at repo root is the reference example with all fields.

## T6 - Image Decoder

### HEIC Library Research
- **`github.com/gen2brain/heic`** (v0.4.9): Pure Go HEIC decoder, CGo-free. Uses libheif/libde265 compiled to WASM and executed via `github.com/tetratelabs/wazero` runtime. Falls back to dynamic/shared library via `github.com/ebitengine/purego` if available. This is the ideal choice — no CGo dependency, no system library required.
- **`github.com/jdeng/goheif`**: Requires CGo + libde265. Not suitable.
- **`github.com/vegidio/heif-go`**: Requires CGo despite claiming "without system dependencies". Not suitable.
- **`github.com/klippa-app/go-libheif`**: Requires CGo + libheif. Not suitable.
- Decision: Use `gen2brain/heic` — works out of the box on all platforms.

### Dependencies Added
- `golang.org/x/image v0.41.0` — WebP decoding
- `github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd` — EXIF extraction
- `github.com/gen2brain/heic v0.4.9` — HEIC decoding (CGo-free)
- `github.com/tetratelabs/wazero v1.9.0` — WASM runtime (transitive dep of gen2brain/heic)
- `github.com/ebitengine/purego v0.9.1` — dynamic lib loader (transitive dep)

### Implementation Notes
- `DecodeImage()` detects format by file extension (string mapping), not magic bytes. Simple and fast for our use case.
- `types.EXIFInfo` from `internal/types` is reused (not redefined) — it already has JSON struct tags for search indexing.
- EXIF extraction only attempted for JPEG (.jpg/.jpeg) since EXIF is TIFF-based and embedded in JPEG APP1 marker. PNG/WebP/HEIC don't contain EXIF in this format.
- EXIF field mapping uses goexif constants: `exif.DateTimeOriginal`, `exif.Model`, `exif.LensModel`, `exif.FocalLength`, `exif.FNumber`, `exif.ISOSpeedRatings`, and `LatLong()` for GPS.
- Rational values (FocalLength, FNumber) are converted from num/den to formatted strings ("85.0mm", "f/2.8").
- GPS coordinates rounded to 6 decimal places using string-conversion trick (avoids floating-point formatting issues).
- `slog.Warn` used for non-critical EXIF extraction failures — decode still succeeds with EXIF=nil.
- If all EXIF fields are zero/empty after extraction, returns nil (no EXIF data) rather than an empty struct.
- Test data: JPEG/PNG generated programmatically; WebP+HEIC embedded via `//go:embed` from `testdata/` directory.
- All 8 tests pass (JPEG, PNG, WebP, HEIC, Corrupt, UnknownFormat, NonExistent, NoEXIF).
