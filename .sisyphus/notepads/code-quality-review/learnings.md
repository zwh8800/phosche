# Learnings

- Project uses Go 1.26.2 — `any` preferred over `interface{}`
- Project structure is clean: separated packages for analyzer, api, config, decoder, errors, indexer, pipeline, search, static, types, watcher
- Uses `testcontainers-go` for integration tests with Elasticsearch
- TypeScript frontend is clean — no `as any`, no `@ts-ignore`, no `console.log`
- `gofmt -d .` exit code 1 indicates formatting issues exist (not just diff output)
- `go vet ./...` catches unused imports and other issues — PASS means imports are clean
- `go test -short` includes a `node_modules/flatted/golang/pkg/flatted` package from testcontainers side-effect — non-issue, has no test files
