package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
)

// DirectoryScanner 实现 Scanner 接口，递归扫描目录中的图片文件。
type DirectoryScanner struct{}

// Scan 递归扫描目录，通过返回的 channel 流式输出发现的文件路径。
// 扫描在后台 goroutine 中执行，channel 在扫描完成后自动关闭。
func (s *DirectoryScanner) Scan(ctx context.Context, dirs []string, existing map[string]int64) (<-chan string, error) {
	ch := make(chan string, 100)

	go func() {
		defer close(ch)

		for _, dir := range dirs {
			select {
			case <-ctx.Done():
				return
			default:
			}

			fi, err := os.Lstat(dir)
			if err != nil {
				if os.IsNotExist(err) {
					slog.Warn("directory does not exist, skipping", "path", dir)
					continue
				}
				slog.Warn("scan: lstat error", "path", dir, "error", err)
				return
			}
			if !fi.IsDir() {
				slog.Warn("path is not a directory, skipping", "path", dir)
				continue
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				slog.Warn("symlink directory skipped", "path", dir)
				continue
			}

			_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					slog.Warn("scan: walk error", "path", path, "error", err)
					return nil
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				if mode := info.Mode(); mode&os.ModeSymlink != 0 {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				if info.IsDir() {
					return nil
				}

				if !isImageFile(path) {
					return nil
				}

				mtime := info.ModTime().Unix()
				if existingMtime, ok := existing[path]; ok && existingMtime == mtime {
					return nil
				}

				select {
				case ch <- path:
				case <-ctx.Done():
					return ctx.Err()
				}
				return nil
			})
		}
	}()

	return ch, nil
}
