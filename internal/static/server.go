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

// PhotoHandler returns an http.Handler that serves photo files from the given base paths.
// The handler tries each base path in order and serves from the first where the file exists.
// Only files with image extensions (.jpg/.jpeg/.png/.webp/.heic/.heif) are served.
// Path traversal (../) is prevented.
// Cache-Control header is set to 1 day.
func PhotoHandler(photoBasePaths []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/photos/") {
			http.NotFound(w, r)
			return
		}

		requestedPath := r.URL.Path[len("/photos/"):]

		for _, basePath := range photoBasePaths {
			safePath := filepath.Clean(filepath.Join(basePath, requestedPath))

			cleanBase := filepath.Clean(basePath)
			if !strings.HasPrefix(safePath, cleanBase+string(filepath.Separator)) && safePath != cleanBase {
				continue
			}

			fi, err := os.Stat(safePath)
			if err != nil {
				continue
			}

			if fi.IsDir() {
				continue
			}

			ext := strings.ToLower(filepath.Ext(safePath))
			if !allowedExtensions[ext] {
				continue
			}

			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, safePath)
			return
		}

		http.NotFound(w, r)
	})
}
