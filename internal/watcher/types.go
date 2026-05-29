// Package watcher 提供文件系统监控能力，包括目录扫描、实时文件监控、事件去重和已有文件管理。
package watcher

import (
	"context"
	"sync"

	"github.com/zwh8800/phosche/internal/types"
)

// Watcher 接口定义了目录监控的生命周期：启动监控并返回事件通道，以及关闭监控。
type Watcher interface {
	// Watch 启动目录监控。返回一个 FileEvent 通道，ctx 取消或 Close() 调用时关闭。
	Watch(ctx context.Context, dirs []string, recursive bool) (<-chan types.FileEvent, error)
	// Close 停止监控，关闭事件通道并释放资源。
	Close() error
}

// Scanner 定义目录扫描器接口，用于一次性扫描目录中的已有图片文件。
type Scanner interface {
	// Scan 递归扫描目录中的图片文件。existing 参数用于增量扫描（跳过已存在的文件）。返回按修改时间降序排列的文件路径。
	Scan(ctx context.Context, dirs []string, existing map[string]int64) ([]string, error)
}

// dedupEntry 用于去重的文件条目（mtime + size 组合）。
type dedupEntry struct {
	mtime int64 // 文件修改时间戳（Unix 时间戳）
	size  int64 // 文件大小（字节）
}

// DedupFilter 基于 path + mtime + size 三重校验的事件去重过滤器，线程安全。
type DedupFilter struct {
	mu   sync.RWMutex              // 读写锁，保护 seen 映射的并发访问
	seen map[string]dedupEntry     // path -> dedupEntry 的映射，记录已见过的文件及其状态
}

// NewDedupFilter 创建 DedupFilter。
func NewDedupFilter() *DedupFilter {
	return &DedupFilter{
		seen: make(map[string]dedupEntry),
	}
}

// ShouldProcess 判断事件是否应被处理。首次出现的路径或 mtime/size 变更的路径返回 true。
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

// Reset 移除路径的去重记录，下次同路径事件将被处理。
func (f *DedupFilter) Reset(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.seen, path)
}

// Purge 清空所有去重记录。
func (f *DedupFilter) Purge() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = make(map[string]dedupEntry)
}
