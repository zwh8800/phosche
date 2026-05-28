package static

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPhotoHandler_ServeJPEG(t *testing.T) {
	tmpDir := t.TempDir()
	jpgPath := filepath.Join(tmpDir, "test.jpg")
	if err := os.WriteFile(jpgPath, []byte("fake jpeg data"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := PhotoHandler(tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/photos/test.jpg", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestPhotoHandler_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	handler := PhotoHandler(tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/photos/../etc/passwd", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.StatusCode)
	}
}

func TestPhotoHandler_NonImageExtension(t *testing.T) {
	tmpDir := t.TempDir()
	txtPath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(txtPath, []byte("text content"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := PhotoHandler(tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/photos/test.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.StatusCode)
	}
}

func TestPhotoHandler_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	handler := PhotoHandler(tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/photos/nonexistent.jpg", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestPhotoHandler_CacheControl(t *testing.T) {
	tmpDir := t.TempDir()
	jpgPath := filepath.Join(tmpDir, "test.jpg")
	if err := os.WriteFile(jpgPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := PhotoHandler(tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/photos/test.jpg", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("expected Cache-Control header, got %s", resp.Header.Get("Cache-Control"))
	}
}

func TestPhotoHandler_DirectoryRequest(t *testing.T) {
	tmpDir := t.TempDir()

	handler := PhotoHandler(tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/photos/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestPhotoHandler_CaseInsensitiveExt(t *testing.T) {
	tmpDir := t.TempDir()
	jpgPath := filepath.Join(tmpDir, "test.JPG")
	if err := os.WriteFile(jpgPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := PhotoHandler(tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/photos/test.JPG", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestPhotoHandler_PathTraversalEncoded(t *testing.T) {
	tmpDir := t.TempDir()

	handler := PhotoHandler(tmpDir)

	// URL-encoded path traversal: %2e%2e%2f = ../
	req := httptest.NewRequest(http.MethodGet, "/photos/%2e%2e%2fetc%2fpasswd", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.StatusCode)
	}
}

func TestPhotoHandler_SubdirectoryFile(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	subPath := filepath.Join(subDir, "photo.webp")
	if err := os.WriteFile(subPath, []byte("fake webp data"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := PhotoHandler(tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/photos/sub/photo.webp", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "image/webp" {
		t.Errorf("expected Content-Type image/webp, got %s", resp.Header.Get("Content-Type"))
	}
}
