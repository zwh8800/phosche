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
