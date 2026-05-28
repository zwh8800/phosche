package decoder

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	_ "embed"
	"os"
	"path/filepath"
	"testing"

	_ "golang.org/x/image/webp"
)

//go:embed testdata/test.webp
var testWebP []byte

//go:embed testdata/gray.heic
var testHEIC []byte

func writeTestFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write test file %s: %v", name, err)
	}
	return path
}

func generateTestJPEG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := range 10 {
		for x := range 10 {
			img.Set(x, y, color.RGBA{R: uint8(x * 25), G: uint8(y * 25), B: 128, A: 255})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create test JPEG: %v", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("failed to encode test JPEG: %v", err)
	}
	return path
}

func generateTestPNG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := range 10 {
		for x := range 10 {
			img.Set(x, y, color.RGBA{R: uint8(x * 25), G: uint8(y * 25), B: 128, A: 255})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create test PNG: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("failed to encode test PNG: %v", err)
	}
	return path
}

func TestDecodeJPEG(t *testing.T) {
	path := generateTestJPEG(t)

	result, err := DecodeImage(path)
	if err != nil {
		t.Fatalf("DecodeImage(%q) error: %v", path, err)
	}

	if result.Image == nil {
		t.Fatal("expected non-nil Image")
	}
	if result.MIMEType != "image/jpeg" {
		t.Errorf("expected MIME type image/jpeg, got %q", result.MIMEType)
	}
	bounds := result.Image.Bounds()
	if bounds.Dx() != 10 || bounds.Dy() != 10 {
		t.Errorf("expected 10x10 image, got %dx%d", bounds.Dx(), bounds.Dy())
	}
	// No EXIF data expected (programmatically generated JPEG)
	if result.EXIF != nil {
		t.Errorf("expected nil EXIF for generated JPEG, got %+v", result.EXIF)
	}
}

func TestDecodePNG(t *testing.T) {
	path := generateTestPNG(t)

	result, err := DecodeImage(path)
	if err != nil {
		t.Fatalf("DecodeImage(%q) error: %v", path, err)
	}

	if result.Image == nil {
		t.Fatal("expected non-nil Image")
	}
	if result.MIMEType != "image/png" {
		t.Errorf("expected MIME type image/png, got %q", result.MIMEType)
	}
	bounds := result.Image.Bounds()
	if bounds.Dx() != 10 || bounds.Dy() != 10 {
		t.Errorf("expected 10x10 image, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestDecodeWebP(t *testing.T) {
	path := writeTestFile(t, t.TempDir(), "test.webp", testWebP)

	result, err := DecodeImage(path)
	if err != nil {
		t.Fatalf("DecodeImage(%q) error: %v", path, err)
	}

	if result.Image == nil {
		t.Fatal("expected non-nil Image")
	}
	if result.MIMEType != "image/webp" {
		t.Errorf("expected MIME type image/webp, got %q", result.MIMEType)
	}
}

func TestDecodeHEIC(t *testing.T) {
	path := writeTestFile(t, t.TempDir(), "test.heic", testHEIC)

	result, err := DecodeImage(path)
	if err != nil {
		t.Fatalf("DecodeImage(%q) error: %v", path, err)
	}

	if result.Image == nil {
		t.Fatal("expected non-nil Image")
	}
	if result.MIMEType != "image/heic" {
		t.Errorf("expected MIME type image/heic, got %q", result.MIMEType)
	}
}

func TestDecodeCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.jpg")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}

	_, err := DecodeImage(path)
	if err == nil {
		t.Fatal("expected error decoding 0-byte file, got nil")
	}
}

func TestDecodeUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("failed to write txt file: %v", err)
	}

	_, err := DecodeImage(path)
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
}

func TestDecodeNonExistent(t *testing.T) {
	_, err := DecodeImage("/nonexistent/path/image.jpg")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestEXIFExtraction_NoEXIF(t *testing.T) {
	// Programmatically generated JPEG has no EXIF data.
	// Verify that EXIF extraction returns nil gracefully (no crash).
	path := generateTestJPEG(t)

	result, err := DecodeImage(path)
	if err != nil {
		t.Fatalf("DecodeImage(%q) error: %v", path, err)
	}

	if result.EXIF != nil {
		t.Errorf("expected nil EXIF for generated JPEG, got %+v", result.EXIF)
	}
}
