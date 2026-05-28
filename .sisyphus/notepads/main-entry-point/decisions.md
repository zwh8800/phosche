# Decisions

## Dual entry point pattern

Decided to split the entry point into two locations:
1. `cmd/phosche/main.go` — development build (no embedded SPA, pass nil distFS)
2. `main.go` (root) + `embed.go` (root) — production build (embedded SPA)

Shared logic lives in `internal/app/run.go` with parameterized `distFS`.

## Reason
Go's `//go:embed` cannot use `..` in paths, making it impossible to embed `../../web/dist` 
from `cmd/phosche/embed.go`. The root-based embed approach is the only viable solution 
without restructuring the entire project.

## Configuration path handling
Instead of using `os.Getenv` or `flag` in the shared code, the config path is passed as a 
parameter to `app.Run(distFS, configPath)`. Each entry point handles flag parsing independently.
