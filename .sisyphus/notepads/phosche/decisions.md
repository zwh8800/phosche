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
