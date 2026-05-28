package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/zwh8800/phosche/internal/types"
)

const defaultDebounceMs = 500

type WatcherConfig struct {
	DebounceMs int
}

type pendingItem struct {
	event types.FileEvent
	timer *time.Timer
}

type FSNotifyWatcher struct {
	cfg      WatcherConfig
	fw       *fsnotify.Watcher
	events   chan types.FileEvent
	cancel   context.CancelFunc
	mu       sync.Mutex
	started  bool
	closed   atomic.Bool
	timersMu sync.Mutex
	pending  map[string]*pendingItem
}

func NewFSNotifyWatcher(cfg WatcherConfig) *FSNotifyWatcher {
	if cfg.DebounceMs <= 0 {
		cfg.DebounceMs = defaultDebounceMs
	}
	return &FSNotifyWatcher{
		cfg:     cfg,
		pending: make(map[string]*pendingItem),
	}
}

func (w *FSNotifyWatcher) Watch(ctx context.Context, dirs []string, recursive bool) (<-chan types.FileEvent, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return nil, fmt.Errorf("watcher already started")
	}
	w.started = true

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	w.fw = fw

	for _, dir := range dirs {
		if err := fw.Add(dir); err != nil {
			fw.Close()
			return nil, fmt.Errorf("add directory %s: %w", dir, err)
		}

		if recursive {
			if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					slog.Warn("walk error", "path", path, "err", err)
					return nil
				}
				if info.IsDir() && path != dir {
					if addErr := fw.Add(path); addErr != nil {
						slog.Warn("add subdirectory", "path", path, "err", addErr)
					}
				}
				return nil
			}); err != nil {
				fw.Close()
				return nil, fmt.Errorf("walk %s: %w", dir, err)
			}
		}
	}

	w.events = make(chan types.FileEvent, 256)
	ctx, w.cancel = context.WithCancel(ctx)

	go w.loop(ctx)

	return w.events, nil
}

func (w *FSNotifyWatcher) Close() error {
	if w.closed.Swap(true) {
		return nil
	}

	w.timersMu.Lock()
	for path := range w.pending {
		item := w.pending[path]
		item.timer.Stop()
		delete(w.pending, path)
	}
	w.timersMu.Unlock()

	if w.cancel != nil {
		w.cancel()
	}

	return nil
}

func (w *FSNotifyWatcher) loop(ctx context.Context) {
	defer close(w.events)
	defer w.fw.Close()

	for {
		select {
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			w.handleFsnotifyEvent(event)
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			slog.Error("fsnotify error", "err", err)
		case <-ctx.Done():
			return
		}
	}
}

func (w *FSNotifyWatcher) handleFsnotifyEvent(e fsnotify.Event) {
	if e.Has(fsnotify.Create) {
		info, err := os.Stat(e.Name)
		if err == nil && info.IsDir() {
			if w.fw != nil {
				if addErr := w.fw.Add(e.Name); addErr != nil {
					slog.Warn("add created directory", "path", e.Name, "err", addErr)
				}
			}
			return
		}
	}

	if e.Has(fsnotify.Create) || e.Has(fsnotify.Write) {
		if !isImageFile(e.Name) {
			return
		}

		op := types.OpCreate
		if e.Has(fsnotify.Write) {
			op = types.OpModify
		}

		info, err := os.Stat(e.Name)
		if err != nil {
			return
		}

		evt := types.FileEvent{
			Path:      e.Name,
			Op:        op,
			Timestamp: time.Now().Unix(),
			MTime:     info.ModTime().Unix(),
			Size:      info.Size(),
		}

		w.debounce(e.Name, evt)
	}
}

func (w *FSNotifyWatcher) debounce(path string, evt types.FileEvent) {
	w.timersMu.Lock()
	defer w.timersMu.Unlock()

	item, exists := w.pending[path]
	if exists {
		item.event = evt
		item.timer.Stop()
	} else {
		item = &pendingItem{event: evt}
		w.pending[path] = item
	}

	item.timer = time.AfterFunc(time.Duration(w.cfg.DebounceMs)*time.Millisecond, func() {
		if w.closed.Load() {
			return
		}

		w.timersMu.Lock()
		cur, ok := w.pending[path]
		if ok {
			delete(w.pending, path)
		}
		w.timersMu.Unlock()

		if !ok {
			return
		}

		select {
		case w.events <- cur.event:
		default:
			slog.Warn("event channel full, dropping event", "path", cur.event.Path)
		}
	})
}
