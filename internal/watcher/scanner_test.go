package watcher_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zwh8800/phosche/internal/watcher"
)

func drainChan(ch <-chan string) []string {
	var results []string
	for path := range ch {
		results = append(results, path)
	}
	return results
}

func TestScanner_NewFiles(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "img1.jpg", 100)
	createFile(t, dir, "img2.jpeg", 100)
	createFile(t, dir, "img3.jpg", 100)
	createFile(t, dir, "img4.png", 100)
	createFile(t, dir, "readme.txt", 100)
	createFile(t, dir, "notes.md", 100)

	s := &watcher.DirectoryScanner{}
	ch, err := s.Scan(context.Background(), []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := drainChan(ch)
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
	ch, err := s.Scan(context.Background(), []string{dir}, existing)
	if err != nil {
		t.Fatal(err)
	}
	results := drainChan(ch)
	if len(results) != 3 {
		t.Errorf("expected 3 files (1 skipped), got %d: %v", len(results), results)
	}
	for _, r := range results {
		if r == img1Path {
			t.Errorf("img1.jpg should have been skipped")
		}
	}
}

func TestScanner_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	s := &watcher.DirectoryScanner{}
	ch, err := s.Scan(context.Background(), []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := drainChan(ch)
	if len(results) != 0 {
		t.Errorf("expected 0 files, got %d", len(results))
	}
}

func TestScanner_NonExistentDir(t *testing.T) {
	s := &watcher.DirectoryScanner{}
	ch, err := s.Scan(context.Background(), []string{"/nonexistent/path"}, nil)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	results := drainChan(ch)
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
	ch, err := s.Scan(context.Background(), []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := drainChan(ch)
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
	ch, err := s.Scan(context.Background(), []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := drainChan(ch)
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
	ch, err := s.Scan(ctx, []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := drainChan(ch)
	// Context already cancelled, should get 0 or very few files
	if len(results) > 5 {
		t.Errorf("expected <= 5 files with cancelled context, got %d", len(results))
	}
}
