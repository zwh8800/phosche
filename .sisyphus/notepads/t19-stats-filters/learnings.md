# T19: Stats & Filters API - Learnings

## Summary
Implemented `GET /api/stats` and `GET /api/filters` endpoints with corresponding SearchService methods.

## Key Files Created/Modified
- `internal/api/stats.go` - statsHandler (returns PhotoService.GetStats)
- `internal/api/filters.go` - filtersHandler (returns PhotoService.GetFilters)
- `internal/search/search.go` - Added `GetStats()` method with ES terms aggregation on `status` field + `created_at` date range for recent count
- `internal/api/router.go` - Added routes and SearchProvider/Indexer interfaces
- Test files: stats_test.go, filters_test.go, mock_test.go

## API Contracts
- `GET /api/stats` → 200 `{"total", "by_status", "recent_count"}` or 503 on service error
- `GET /api/filters` → 200 `{"tags", "scene_types", "cameras"}` or 503 on service error

## Gotchas
- `{id:*}` in chi v5 does NOT match multi-segment paths - use `*` (unnamed wildcard) + `chi.URLParam(r, "*")` + `url.PathUnescape` instead
- URL-encode paths with slashes for single-segment route params (`%2F`)
- chi's `{name:regex}` only applies within a single path segment
- Order of defaults vs validation matters: set defaults BEFORE validation checks
- Go requires exported struct fields (`Indexer`, `IndexName`) for tests in same package to set them in struct literals
