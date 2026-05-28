
## T17: Search API Endpoint

### What was done
- Created `internal/api/search.go` — POST /api/search handler with validation and defaults
- Created `internal/api/search_test.go` — 6 test cases (success, defaults, validation, errors)

### Key decisions
- Used T16's existing `PhotoSearcher` interface for dependency injection (not a separate interface)
- Added `GetFilters` and `GetStats` to `PhotoSearcher` interface to support filters.go/stats.go
- Merged T16's Server struct fields with T17-T19 needs: `searchService PhotoSearcher`, `Indexer Indexer`, `IndexName string`
- Validation: apply defaults BEFORE validation (page=1, page_size=20 when zero-valued)
- Route registered: `r.Post("/search", srv.searchHandler)`

### Issues encountered & fixed
1. **Pre-existing codebase inconsistencies**: T16-T19 files were partially created with conflicting field names (`indexName` vs `IndexName`, `indexer` vs `Indexer`). Reconciled to use exported `IndexName` and `Indexer`.
2. **Chi regex params don't support slashes**: Changed photo detail route from `{id:.+}` to catch-all `/*`. Updated handler to strip leading `/` and unescape.
3. **PhotoSearcher interface was incomplete**: Added `GetFilters`/`GetStats` methods needed by filters.go/stats.go.
4. **mockSearchService was incomplete**: Added `Search` method to support search tests.
5. **Pre-existing test bug**: `photo_detail_test.go` accessed `body["iso"]` at root level but `iso` is nested inside `exif`. Fixed assertion.

### Test results
- All 6 search tests PASS
- All pre-existing tests (health, CORS, photos, filters, stats, photo_detail) PASS
- `go test ./internal/api/` → PASS
