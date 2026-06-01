# internal/watcher

文件系统监控 + 目录扫描。提供两个核心接口和一个去重过滤器。

## 接口

| 接口 | 方法 | 用途 |
|------|------|------|
| `Watcher` | `Watch(ctx, dirs, recursive) (<-chan FileEvent, error)` | 实时监控，返回事件通道 |
| `Watcher` | `Close() error` | 停止监控，关闭通道 |
| `Scanner` | `Scan(ctx, dirs, existing) (<-chan string, error)` | 一次性扫描目录，流式输出路径 |

## 实现

| 文件 | 实现 |
|------|------|
| `fsnotify.go` | `FSNotifyWatcher` — 基于 fsnotify，带 debounce 去抖 |
| `scanner.go` | `DirectoryScanner` — 递归遍历，按扩展名过滤图片 |
| `existing.go` | 已存在文件管理 |

## DedupFilter

基于 `path + mtime + size` 三重校验的事件去重过滤器（线程安全）。

- `ShouldProcess(event)` — 首次出现或 mtime/size 变更返回 true
- `Reset(path)` — 移除单条记录
- `Purge()` — 清空全部

## 约定

- Watcher 返回的 channel 在 `Close()` 或 ctx 取消时关闭
- Scanner 返回的 channel 在扫描完成后自动关闭
- Debounce 间隔通过 `WatcherConfig.DebounceMs` 配置（默认 500ms）
- 仅监控图片扩展名：`.jpg`、`.jpeg`、`.png`、`.webp`、`.heic`、`.heif`
- 支持排除目录（前缀匹配 + 目录名匹配）
