package decoder

import (
	"bytes"
	"fmt"
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

func createFile(path string) (*os.File, error) {
	return os.Create(path)
}

func generateTestJPEG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")
	if err := os.WriteFile(path, generateJPEGBytes(), 0o644); err != nil {
		t.Fatalf("failed to create test JPEG: %v", err)
	}
	return path
}

func generateJPEGBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := range 10 {
		for x := range 10 {
			img.Set(x, y, color.RGBA{R: uint8(x * 25), G: uint8(y * 25), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		panic(fmt.Sprintf("failed to encode JPEG: %v", err))
	}
	return buf.Bytes()
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

func TestDecodeFormatMismatch_JPEGasHEIC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actually_jpeg.heic")
	if err := os.WriteFile(path, generateJPEGBytes(), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	result, err := DecodeImage(path)
	if err != nil {
		t.Fatalf("DecodeImage(%q) error: %v", path, err)
	}
	if result.MIMEType != "image/jpeg" {
		t.Errorf("expected MIME type image/jpeg for JPEG content in .heic file, got %q", result.MIMEType)
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

func TestEXIFExtraction_WithEXIF(t *testing.T) {
	path, expected := generateTestJPEGWithEXIF(t)

	result, err := DecodeImage(path)
	if err != nil {
		t.Fatalf("DecodeImage(%q) error: %v", path, err)
	}

	if result.EXIF == nil {
		t.Fatal("expected non-nil EXIF, got nil")
	}

	exif := result.EXIF

	if exif.DateTimeOriginal != expected.DateTimeOriginal {
		t.Errorf("DateTimeOriginal: got %q, want %q", exif.DateTimeOriginal, expected.DateTimeOriginal)
	}
	if exif.CameraModel != expected.CameraModel {
		t.Errorf("CameraModel: got %q, want %q", exif.CameraModel, expected.CameraModel)
	}
	if exif.LensModel != expected.LensModel {
		t.Errorf("LensModel: got %q, want %q", exif.LensModel, expected.LensModel)
	}
	if exif.FocalLength != expected.FocalLength {
		t.Errorf("FocalLength: got %q, want %q", exif.FocalLength, expected.FocalLength)
	}
	if exif.Aperture != expected.Aperture {
		t.Errorf("Aperture: got %q, want %q", exif.Aperture, expected.Aperture)
	}
	if exif.ShutterSpeed != expected.ShutterSpeed {
		t.Errorf("ShutterSpeed: got %q, want %q", exif.ShutterSpeed, expected.ShutterSpeed)
	}
	if exif.ISO != expected.ISO {
		t.Errorf("ISO: got %d, want %d", exif.ISO, expected.ISO)
	}
	if exif.GPSLat != expected.GPSLat {
		t.Errorf("GPSLat: got %f, want %f", exif.GPSLat, expected.GPSLat)
	}
	if exif.GPSLon != expected.GPSLon {
		t.Errorf("GPSLon: got %f, want %f", exif.GPSLon, expected.GPSLon)
	}
}
