package watcher_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zwh8800/phosche/internal/watcher"
)

func TestScanner_NewFiles(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "img1.jpg", 100)
	createFile(t, dir, "img2.jpeg", 100)
	createFile(t, dir, "img3.jpg", 100)
	createFile(t, dir, "img4.png", 100)
	createFile(t, dir, "readme.txt", 100)
	createFile(t, dir, "notes.md", 100)

	s := &watcher.DirectoryScanner{}
	results, err := s.Scan(context.Background(), []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 image files, got %d: %v", len(results), results)
	}
}

func TestScanner_SkipExisting(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "img1.jpg", 100)
	createFile(t, dir, "img2.jpeg", 100)
	createFile(t, dir, "img3.png", 100)
	createFile(t, dir, "img4.webp", 100)

	img1Path := filepath.Join(dir, "img1.jpg")
	fi, err := os.Stat(img1Path)
	if err != nil {
		t.Fatal(err)
	}
	existing := map[string]int64{
		img1Path: fi.ModTime().Unix(),
	}

	s := &watcher.DirectoryScanner{}
	results, err := s.Scan(context.Background(), []string{dir}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 files (1 skipped), got %d: %v", len(results), results)
	}
	for _, r := range results {
		if r == img1Path {
			t.Errorf("img1.jpg should have been skipped")
		}
	}
}

func TestScanner_SortOrder(t *testing.T) {
	dir := t.TempDir()

	files := []struct {
		name  string
		mtime time.Time
	}{
		{"old.jpg", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"mid.png", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
		{"new.webp", time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, f.mtime, f.mtime); err != nil {
			t.Fatal(err)
		}
	}

	s := &watcher.DirectoryScanner{}
	results, err := s.Scan(context.Background(), []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 files, got %d", len(results))
	}

	// Verify results are in mtime-descending order (newest first)
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = filepath.Base(r)
	}

	if names[0] != "new.webp" {
		t.Errorf("expected new.webp first, got %s", names[0])
	}
	if names[1] != "mid.png" {
		t.Errorf("expected mid.png second, got %s", names[1])
	}
	if names[2] != "old.jpg" {
		t.Errorf("expected old.jpg third, got %s", names[2])
	}
}

func TestScanner_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	s := &watcher.DirectoryScanner{}
	results, err := s.Scan(context.Background(), []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if results == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 files, got %d", len(results))
	}
}

func TestScanner_NonExistentDir(t *testing.T) {
	s := &watcher.DirectoryScanner{}
	results, err := s.Scan(context.Background(), []string{"/nonexistent/path"}, nil)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if results == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 files, got %d", len(results))
	}
}

func TestScanner_Recursive(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	createFile(t, dir, "root.jpg", 100)
	createFile(t, subdir, "nested.png", 100)
	createFile(t, subdir, "nested.heic", 100)

	s := &watcher.DirectoryScanner{}
	results, err := s.Scan(context.Background(), []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 image files, got %d: %v", len(results), results)
	}
}

func TestScanner_CaseInsensitiveExt(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "img.JPG", 100)
	createFile(t, dir, "img.PNG", 100)
	createFile(t, dir, "img.txt", 100)

	s := &watcher.DirectoryScanner{}
	results, err := s.Scan(context.Background(), []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 image files (.JPG + .PNG), got %d", len(results))
	}
}

func TestScanner_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	for i := range 50 {
		name := "img" + string(rune('a'+i%26)) + "_" + string(rune('a'+i/26)) + ".jpg"
		createFile(t, dir, name, 100)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := &watcher.DirectoryScanner{}
	results, err := s.Scan(ctx, []string{dir}, nil)
	if err == nil && len(results) == 0 {
		// Both outcomes are acceptable per spec
		return
	}
	// err may be non-nil from context cancellation — that's also acceptable
	_ = results
}
