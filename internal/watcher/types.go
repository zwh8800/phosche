package watcher

import (
	"context"
	"sync"

	"github.com/zwh8800/phosche/internal/types"
)

// Watcher watches directories for new/modified/deleted image files.
type Watcher interface {
	// Watch starts watching the given directories and returns a channel of file events.
	// The channel is closed when ctx is cancelled or Close() is called.
	Watch(ctx context.Context, dirs []string, recursive bool) (<-chan types.FileEvent, error)
	// Close stops watching and closes the event channel.
	Close() error
}

// Scanner scans directories for existing image files.
type Scanner interface {
	// Scan recursively scans the given directories for image files.
	// existing is a map of path -> mtime of already-known files to skip.
	// Returns paths of new or modified files, sorted by mtime descending (newest first).
	Scan(ctx context.Context, dirs []string, existing map[string]int64) ([]string, error)
}

type dedupEntry struct {
	mtime int64
	size  int64
}

// DedupFilter filters out duplicate FileEvents based on path+mtime+size.
// Thread-safe.
type DedupFilter struct {
	mu   sync.RWMutex
	seen map[string]dedupEntry
}

// NewDedupFilter creates a new DedupFilter.
func NewDedupFilter() *DedupFilter {
	return &DedupFilter{
		seen: make(map[string]dedupEntry),
	}
}

// ShouldProcess returns true if this event should be processed (not a duplicate).
// Uses path as key, checks mtime+size.
// If mtime or size changed for same path, treat as new (returns true).
func (f *DedupFilter) ShouldProcess(event types.FileEvent) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	entry, exists := f.seen[event.Path]
	if !exists {
		f.seen[event.Path] = dedupEntry{mtime: event.MTime, size: event.Size}
		return true
	}

	if entry.mtime != event.MTime || entry.size != event.Size {
		f.seen[event.Path] = dedupEntry{mtime: event.MTime, size: event.Size}
		return true
	}

	return false
}

// Reset removes a path from the filter so the next event for it will be processed.
func (f *DedupFilter) Reset(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.seen, path)
}

// Purge clears all entries from the filter.
func (f *DedupFilter) Purge() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = make(map[string]dedupEntry)
}
