package watcher_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zwh8800/phosche/internal/types"
	"github.com/zwh8800/phosche/internal/watcher"
)

func TestDedupFilter_Duplicate(t *testing.T) {
	f := watcher.NewDedupFilter()
	evt := types.FileEvent{Path: "/a.jpg", MTime: 100, Size: 200}
	if !f.ShouldProcess(evt) {
		t.Error("first event should be processed")
	}
	if f.ShouldProcess(evt) {
		t.Error("duplicate event should be filtered")
	}
}

func TestDedupFilter_Modified(t *testing.T) {
	f := watcher.NewDedupFilter()
	evt1 := types.FileEvent{Path: "/a.jpg", MTime: 100, Size: 200}
	evt2 := types.FileEvent{Path: "/a.jpg", MTime: 101, Size: 200}
	if !f.ShouldProcess(evt1) {
		t.Error("first event should be processed")
	}
	if !f.ShouldProcess(evt2) {
		t.Error("modified event should be processed")
	}
}

func TestDedupFilter_DifferentPaths(t *testing.T) {
	f := watcher.NewDedupFilter()
	evt1 := types.FileEvent{Path: "/a.jpg", MTime: 100, Size: 200}
	evt2 := types.FileEvent{Path: "/b.jpg", MTime: 100, Size: 200}
	if !f.ShouldProcess(evt1) {
		t.Error("first event should be processed")
	}
	if !f.ShouldProcess(evt2) {
		t.Error("different path event should be processed")
	}
}

func TestDedupFilter_Reset(t *testing.T) {
	f := watcher.NewDedupFilter()
	evt := types.FileEvent{Path: "/a.jpg", MTime: 100, Size: 200}
	if !f.ShouldProcess(evt) {
		t.Error("first event should be processed")
	}
	if f.ShouldProcess(evt) {
		t.Error("duplicate event should be filtered")
	}
	f.Reset(evt.Path)
	if !f.ShouldProcess(evt) {
		t.Error("after reset, event should be processed")
	}
}

func TestDedupFilter_Purge(t *testing.T) {
	f := watcher.NewDedupFilter()
	evt := types.FileEvent{Path: "/a.jpg", MTime: 100, Size: 200}
	if !f.ShouldProcess(evt) {
		t.Error("first event should be processed")
	}
	if f.ShouldProcess(evt) {
		t.Error("duplicate event should be filtered")
	}
	f.Purge()
	if !f.ShouldProcess(evt) {
		t.Error("after purge, event should be processed")
	}
}

func TestDedupFilter_Concurrent(t *testing.T) {
	f := watcher.NewDedupFilter()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			evt := types.FileEvent{
				Path:  "/a.jpg",
				MTime: int64(100 + n),
				Size:  200,
			}
			f.ShouldProcess(evt)
		}(i)
	}
	wg.Wait()
}

func TestLoadExisting_ValidDir(t *testing.T) {
	dir := t.TempDir()

	createFile(t, dir, "img1.jpg", 100)
	createFile(t, dir, "img2.jpeg", 200)
	createFile(t, dir, "img3.png", 300)
	createFile(t, dir, "img4.webp", 400)
	createFile(t, dir, "readme.txt", 500)
	createFile(t, dir, "notes.md", 600)

	result, err := watcher.LoadExisting([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 4 {
		t.Errorf("expected 4 image files, got %d", len(result))
	}
}

func TestLoadExisting_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	result, err := watcher.LoadExisting([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestLoadExisting_NonExistentDir(t *testing.T) {
	result, err := watcher.LoadExisting([]string{"/nonexistent/path"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestLoadExisting_Recursive(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	createFile(t, dir, "root.jpg", 100)
	createFile(t, subdir, "nested.png", 200)
	createFile(t, subdir, "nested.heic", 300)

	result, err := watcher.LoadExisting([]string{dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 image files, got %d", len(result))
	}
}

func createFile(t *testing.T, dir, name string, size int64) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
		t.Fatal(err)
	}
}
