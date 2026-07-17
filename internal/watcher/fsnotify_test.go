package watcher_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zwh8800/phosche/internal/watcher"
)

func TestFSNotify_CreateEvent(t *testing.T) {
	dir := t.TempDir()

	w := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{DebounceMs: 100})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := w.Watch(ctx, []string{dir}, false)
	if err != nil {
		t.Fatal("Watch:", err)
	}

	// Give fsnotify time to set up
	time.Sleep(50 * time.Millisecond)

	testPath := filepath.Join(dir, "test.jpg")
	if err := os.WriteFile(testPath, []byte("fake image data"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case evt := <-events:
		if evt.Path != testPath {
			t.Errorf("expected path %q, got %q", testPath, evt.Path)
		}
		if evt.Op != "create" && evt.Op != "modify" {
			t.Errorf("expected create or modify op, got %q", evt.Op)
		}
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for file event")
	}
}

func TestFSNotify_IgnoreNonImage(t *testing.T) {
	dir := t.TempDir()

	w := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{DebounceMs: 100})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := w.Watch(ctx, []string{dir}, false)
	if err != nil {
		t.Fatal("Watch:", err)
	}

	time.Sleep(50 * time.Millisecond)

	testPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testPath, []byte("not an image"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should not receive any event for non-image files
	select {
	case evt := <-events:
		t.Errorf("unexpected event for non-image file: %+v", evt)
	case <-time.After(500 * time.Millisecond):
		// expected: no event received
	}
}

func TestFSNotify_Debounce(t *testing.T) {
	dir := t.TempDir()

	w := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{DebounceMs: 300})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := w.Watch(ctx, []string{dir}, false)
	if err != nil {
		t.Fatal("Watch:", err)
	}

	time.Sleep(50 * time.Millisecond)

	testPath := filepath.Join(dir, "test.jpg")

	// Rapid writes: 0ms, 80ms, 160ms
	os.WriteFile(testPath, []byte("data1"), 0644)
	time.Sleep(80 * time.Millisecond)
	os.WriteFile(testPath, []byte("data2"), 0644)
	time.Sleep(80 * time.Millisecond)
	os.WriteFile(testPath, []byte("data3"), 0644)

	// Collect events over a generous window
	var eventCount int
	deadline := time.After(1500 * time.Millisecond)

loop:
	for {
		select {
		case _, ok := <-events:
			if !ok {
				break loop
			}
			eventCount++
		case <-deadline:
			break loop
		}
	}

	if eventCount != 1 {
		t.Errorf("expected 1 debounced event, got %d", eventCount)
	}
}

func TestFSNotify_Recursive(t *testing.T) {
	dir := t.TempDir()

	sub1 := filepath.Join(dir, "sub1")
	deep := filepath.Join(sub1, "deep")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	w := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{DebounceMs: 100})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := w.Watch(ctx, []string{dir}, true)
	if err != nil {
		t.Fatal("Watch:", err)
	}

	time.Sleep(100 * time.Millisecond)

	createFile(t, dir, "root.jpg", 100)
	createFile(t, sub1, "mid.png", 200)
	createFile(t, deep, "deep.webp", 300)

	eventPaths := make(map[string]bool)
	deadline := time.After(5 * time.Second)

	for len(eventPaths) < 3 {
		select {
		case evt, ok := <-events:
			if !ok {
				t.Fatal("events channel closed unexpectedly")
			}
			eventPaths[evt.Path] = true
		case <-deadline:
			t.Fatalf("timed out waiting for events, got %d/3: %v", len(eventPaths), eventPaths)
		}
	}

	rootPath := filepath.Join(dir, "root.jpg")
	midPath := filepath.Join(sub1, "mid.png")
	deepPath := filepath.Join(deep, "deep.webp")

	if !eventPaths[rootPath] {
		t.Error("missing event for root.jpg")
	}
	if !eventPaths[midPath] {
		t.Error("missing event for mid.png")
	}
	if !eventPaths[deepPath] {
		t.Error("missing event for deep.webp")
	}
}

func TestFSNotify_NewSubdirectory(t *testing.T) {
	dir := t.TempDir()

	w := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{DebounceMs: 100})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := w.Watch(ctx, []string{dir}, true)
	if err != nil {
		t.Fatal("Watch:", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Create a new subdirectory while watching
	newSub := filepath.Join(dir, "runtime-sub")
	if err := os.Mkdir(newSub, 0755); err != nil {
		t.Fatal(err)
	}

	// Wait briefly for watcher to add the new subdirectory
	time.Sleep(200 * time.Millisecond)

	// Now create an image inside the new subdirectory
	imgPath := filepath.Join(newSub, "new-image.jpeg")
	if err := os.WriteFile(imgPath, []byte("new image"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case evt := <-events:
		if evt.Path != imgPath {
			t.Errorf("expected path %q, got %q", imgPath, evt.Path)
		}
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for event in new subdirectory")
	}
}

func TestFSNotify_Close(t *testing.T) {
	dir := t.TempDir()

	w := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{DebounceMs: 100})
	ctx := context.Background()

	events, err := w.Watch(ctx, []string{dir}, false)
	if err != nil {
		t.Fatal("Watch:", err)
	}

	// Give the watcher a moment to start
	time.Sleep(50 * time.Millisecond)

	if err := w.Close(); err != nil {
		t.Fatal("Close:", err)
	}

	// Channel should eventually be closed
	select {
	case _, ok := <-events:
		if ok {
			t.Error("expected channel to be closed after Close()")
		}
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for channel to close")
	}
}

func TestFSNotify_ExcludeDirs(t *testing.T) {
	dir := t.TempDir()

	// 预置排除目录（模拟群晖 @eaDir 缩略图目录）和正常目录
	eaDir := filepath.Join(dir, "2025", "@eaDir")
	if err := os.MkdirAll(eaDir, 0755); err != nil {
		t.Fatal(err)
	}
	normal := filepath.Join(dir, "2025", "normal")
	if err := os.MkdirAll(normal, 0755); err != nil {
		t.Fatal(err)
	}

	w := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{
		DebounceMs:  100,
		ExcludeDirs: []string{"@eaDir"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := w.Watch(ctx, []string{dir}, true)
	if err != nil {
		t.Fatal("Watch:", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 排除目录中的图片不应产生事件（walk 时未注册监控）
	excludedPath := filepath.Join(eaDir, "thumb.jpg")
	if err := os.WriteFile(excludedPath, []byte("thumbnail"), 0644); err != nil {
		t.Fatal(err)
	}

	// 运行时新建的排除目录也不应注册监控
	runtimeEaDir := filepath.Join(normal, "@eaDir")
	if err := os.Mkdir(runtimeEaDir, 0755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	runtimeExcludedPath := filepath.Join(runtimeEaDir, "thumb2.jpg")
	if err := os.WriteFile(runtimeExcludedPath, []byte("thumbnail"), 0644); err != nil {
		t.Fatal(err)
	}

	// 正常目录中的图片应产生事件
	normalPath := filepath.Join(normal, "photo.jpg")
	if err := os.WriteFile(normalPath, []byte("real photo"), 0644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	gotNormal := false
	for !gotNormal {
		select {
		case evt, ok := <-events:
			if !ok {
				t.Fatal("events channel closed unexpectedly")
			}
			if evt.Path == excludedPath || evt.Path == runtimeExcludedPath {
				t.Errorf("unexpected event from excluded dir: %q", evt.Path)
			}
			if evt.Path == normalPath {
				gotNormal = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for event in normal dir")
		}
	}

	// 再确认没有来自排除目录的残留事件
	select {
	case evt := <-events:
		t.Errorf("unexpected trailing event: %q", evt.Path)
	case <-time.After(500 * time.Millisecond):
	}
}
