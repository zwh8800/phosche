
## T12: ES Search Query Builder

### Implementation notes
- `ESClient` exposes `Client()` method returning `*elasticsearch.Client` from go-elasticsearch/v8
- Used `esapi.SearchRequest` for query execution, building query DSL as `map[string]any` then marshal to JSON
- ES index mapping has flat fields (not nested EXIF): `date_time_original`, `camera_model` at top level
- `SearchResponse.TotalPages = ceil(total / pageSize)` computed from ES `hits.total.value`
- `GetFilters` uses `terms` aggregations with `size: 0` to return only aggregations

### Test patterns
- Replicated `dockerAvailable()` helper and `setupTestES` pattern from indexer tests
- Used `esClient.Client().Bulk()` with raw JSON for test data indexing
- Tests cover: fulltext, date range, combined filters, pagination, empty results, get filters, defaults, invalid index, tags filter, camera filter
- All tests skip gracefully when Docker unavailable

### Field naming conventions
- Text fields: `description`, `tags`, `objects` (multi_match targets)
- Keyword subfields: `tags.keyword`, `objects.keyword` (for terms/aggregations)
- Keyword fields: `scene_type`, `camera_model`, `colors`, `status`
- Date field: `date_time_original` (range queries)

