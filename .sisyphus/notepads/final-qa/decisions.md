# Decisions Made During QA

## Decision 1: Mocked Service Testing vs Real Server
**Choice**: Test API handlers through the router with mocked search/indexer services using `httptest.NewServer`
**Rationale**: ES is not available in the environment. Router-level testing with mocked services verifies:
- Correct HTTP status codes
- Request parsing and validation
- Response format
- CORS headers
- Error handling
This approach is consistent with the existing test patterns in `internal/api/`.

## Decision 2: Unified QA Test Suite
**Choice**: Created `internal/api/qa_test.go` with 12 focused test functions + 1 aggregation test
**Rationale**: Single file makes it easy to run `go test -run "TestQA_AllScenarios"` for complete QA report. Individual tests support targeted debugging.

## Decision 3: Playwright via CLI not MCP
**Choice**: Ran `npx playwright test` directly for functional tests; used Playwright MCP only for screenshots
**Rationale**: The existing e2e tests in `web/e2e/app.spec.ts` are already configured. CLI execution is faster and deterministic. MCP was used for visual evidence capture.

## Decision 4: SIGTERM via Code Review
**Choice**: Verified SIGTERM handling through code review instead of live test
**Rationale**: Cannot start the full server without ES. The code pattern (signal.NotifyContext → pipeline cancel → graceful HTTP shutdown) is idiomatic Go and verifiable by inspection.
