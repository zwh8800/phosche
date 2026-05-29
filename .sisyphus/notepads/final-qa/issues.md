# Issues Encountered During QA

## Issue 1: Docker Not Available
- **Impact**: Cannot start Elasticsearch container, preventing full server startup
- **Workaround**: Used mocked services in httptest for API testing; code-reviewed SIGTERM handling
- **Resolution**: QA tests cover router-level behavior; integration E2E tests exist separately in `internal/integration/e2e_test.go`

## Issue 2: Stale Binary
- **Impact**: Prebuilt `phosche` binary at project root is stale — exits with code 0 silently
- **Workaround**: Used `go run ./cmd/phosche/` which shows correct error (ES connection refused, exit 1)
- **Resolution**: Binary should be rebuilt before use

## Issue 3: HTTP Proxy Interference
- **Impact**: System `http_proxy` env var (http://127.0.0.1:7890) intercepts localhost requests, causing 502 errors
- **Workaround**: Set `NO_PROXY=localhost,127.0.0.1` for Playwright and curl tests
- **Resolution**: Works when proxy bypassed; Playwright webServer config should include NO_PROXY

## Issue 4: Static File Extension Check Order
- **Impact**: Non-image file returns 404 (file not found) before checking extension, instead of 403
- **Fix**: Test adjusted to create the file first, then extension 403 check works correctly
- **Note**: This is correct behavior — the file must exist before extension validation
