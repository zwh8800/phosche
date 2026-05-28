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
