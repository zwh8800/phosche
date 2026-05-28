package static

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Allowed image extensions (case-insensitive).
var allowedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".heic": true,
	".heif": true,
}

// PhotoHandler returns an http.Handler that serves photo files from the given base path.
// Only files with image extensions (.jpg/.jpeg/.png/.webp/.heic/.heif) are served.
// Path traversal (../) is prevented.
// Cache-Control header is set to 1 day.
func PhotoHandler(photoBasePath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/photos/") {
			http.NotFound(w, r)
			return
		}

		requestedPath := r.URL.Path[len("/photos/"):]

		safePath := filepath.Clean(filepath.Join(photoBasePath, requestedPath))

		cleanBase := filepath.Clean(photoBasePath)
		if !strings.HasPrefix(safePath, cleanBase+string(filepath.Separator)) && safePath != cleanBase {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		fi, err := os.Stat(safePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if fi.IsDir() {
			http.NotFound(w, r)
			return
		}

		ext := strings.ToLower(filepath.Ext(safePath))
		if !allowedExtensions[ext] {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, safePath)
	})
}
