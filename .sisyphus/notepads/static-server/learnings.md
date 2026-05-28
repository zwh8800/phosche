## T13: Static file server

### Implementation notes
- `PhotoHandler(photoBasePath string) http.Handler` — no error return, so all validation is per-request
- Check ordering matters: path traversal → os.Stat → IsDir → extension → serve
- Extension check after IsDir check so directory requests get 404, not 403
- Used `filepath.Clean` + `strings.HasPrefix` with separator for path traversal prevention
- Public API docstring kept (Go convention), all inline comments removed

### Allowed extensions
`.jpg`, `.jpeg`, `.png`, `.webp`, `.heic`, `.heif` (case-insensitive via `strings.ToLower`)

### Tests
- 9 tests, all pass
- Uses `t.TempDir()`, `httptest.NewRecorder`, no external test deps
