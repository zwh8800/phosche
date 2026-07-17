package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/zwh8800/phosche/internal/types"
)

// defaultDebounceMs 是默认的文件事件去抖间隔（毫秒），默认 500ms。
// 同一文件在去抖间隔内的多次修改会被合并为一次事件。
const defaultDebounceMs = 500

// WatcherConfig 是文件监控器的配置。
type WatcherConfig struct {
	DebounceMs  int
	ExcludeDirs []string // 排除的目录名列表，支持前缀匹配和目录名匹配（如 "@eaDir"）
}

// pendingItem 记录待触发的去抖事件和目标定时器。
type pendingItem struct {
	event types.FileEvent
	timer *time.Timer
}

// FSNotifyWatcher 基于 fsnotify 的文件监控器实现，支持递归监控和事件去抖。
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

// NewFSNotifyWatcher 创建 fsnotify 监控器。DebounceMs 为 0 时默认 500ms。
func NewFSNotifyWatcher(cfg WatcherConfig) *FSNotifyWatcher {
	if cfg.DebounceMs <= 0 {
		cfg.DebounceMs = defaultDebounceMs
	}
	return &FSNotifyWatcher{
		cfg:     cfg,
		pending: make(map[string]*pendingItem),
	}
}

// Watch 启动目录监控。递归模式下自动注册所有子目录并监控新增目录。
func (w *FSNotifyWatcher) Watch(ctx context.Context, dirs []string, recursive bool) (<-chan types.FileEvent, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return nil, fmt.Errorf("watcher already started")
	}
	w.started = true

	// 创建 fsnotify 原生监控器
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	w.fw = fw

	// 注册需要监控的目录
	for _, dir := range dirs {
		if err := fw.Add(dir); err != nil {
			fw.Close()
			return nil, fmt.Errorf("add directory %s: %w", dir, err)
		}

		// 递归模式下，walk 所有子目录并注册监控
		if recursive {
			if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					slog.Warn("walk error", "path", path, "err", err)
					return nil
				}
				if info.IsDir() {
					// 排除的目录：不注册监控，也不递归进入
					if w.isExcludedDir(path) {
						return filepath.SkipDir
					}
					if path != dir {
						if addErr := fw.Add(path); addErr != nil {
							slog.Warn("add subdirectory", "path", path, "err", addErr)
						}
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

// Close 停止监控。先停止所有去抖定时器，再取消上下文。
func (w *FSNotifyWatcher) Close() error {
	if w.closed.Swap(true) {
		return nil
	}

	// 停止所有待处理的去抖定时器
	w.timersMu.Lock()
	for path := range w.pending {
		item := w.pending[path]
		item.timer.Stop()
		delete(w.pending, path)
	}
	w.timersMu.Unlock()

	// 取消上下文，通知事件循环退出
	if w.cancel != nil {
		w.cancel()
	}

	return nil
}

// loop 事件循环：处理 fsnotify 事件、错误和取消信号。
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

// isExcludedDir 判断路径是否命中排除目录规则。
// 与 pipeline 的 isExcluded 语义一致：支持前缀匹配（d 为路径前缀）和目录名匹配（任意路径分量等于 d）。
func (w *FSNotifyWatcher) isExcludedDir(path string) bool {
	for _, d := range w.cfg.ExcludeDirs {
		// 前缀匹配：/photos/#recycle 匹配 /photos/#recycle 及其子路径
		if path == d || strings.HasPrefix(path, d+string(filepath.Separator)) {
			return true
		}
		// 目录名匹配：@eaDir 匹配任意路径中名为 @eaDir 的目录
		for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
			if part == d {
				return true
			}
		}
	}
	return false
}

// handleFsnotifyEvent 处理 fsnotify 事件。Create 目录 → 自动注册监控；Create/Write 图片文件 → 经过去抖后发送。
func (w *FSNotifyWatcher) handleFsnotifyEvent(e fsnotify.Event) {
	// 目录创建事件：自动注册对该目录的 fsnotify 监控，支持递归
	if e.Has(fsnotify.Create) {
		info, err := os.Stat(e.Name)
		if err == nil && info.IsDir() {
			// 排除的目录不注册监控
			if w.isExcludedDir(e.Name) {
				return
			}
			if w.fw != nil {
				if addErr := w.fw.Add(e.Name); addErr != nil {
					slog.Warn("add created directory", "path", e.Name, "err", addErr)
				}
			}
			return
		}
	}

	// Create / Write 事件：仅处理图片文件，分类后经去抖发送
	if e.Has(fsnotify.Create) || e.Has(fsnotify.Write) {
		// 跳过非图片文件
		if !isImageFile(e.Name) {
			return
		}

		// 区分创建与修改操作
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

// debounce 去抖处理：同路径的多次事件在指定时间窗口内合并为一次。
func (w *FSNotifyWatcher) debounce(path string, evt types.FileEvent) {
	w.timersMu.Lock()
	defer w.timersMu.Unlock()

	// 路径已存在待处理记录 → 更新事件内容，重置定时器（重新计时）
	item, exists := w.pending[path]
	if exists {
		item.event = evt
		item.timer.Stop()
	} else {
		item = &pendingItem{event: evt}
		w.pending[path] = item
	}

	// 启动/重置去抖定时器：DebounceMs 后触发事件发送
	item.timer = time.AfterFunc(time.Duration(w.cfg.DebounceMs)*time.Millisecond, func() {
		if w.closed.Load() {
			return
		}

		// 从待处理映射中取出并删除该路径的记录
		w.timersMu.Lock()
		cur, ok := w.pending[path]
		if ok {
			delete(w.pending, path)
		}
		w.timersMu.Unlock()

		if !ok {
			return
		}

		// 非阻塞发送：通道满时记录告警并丢弃事件，避免阻塞事件循环
		select {
		case w.events <- cur.event:
		default:
			slog.Warn("event channel full, dropping event", "path", cur.event.Path)
		}
	})
}
