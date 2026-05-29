package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// DirectoryScanner 实现 Scanner 接口，递归扫描目录中的图片文件。
type DirectoryScanner struct{}

// scanEntry 是扫描过程中的中间条目，用于排序。
type scanEntry struct {
	path  string // 文件路径
	mtime int64  // 文件修改时间（Unix 时间戳）
}

// Scan 递归扫描目录。处理不存在的目录（警告跳过）、符号链接（跳过）、非图片文件（忽略），按 mtime 降序返回结果。
func (s *DirectoryScanner) Scan(ctx context.Context, dirs []string, existing map[string]int64) ([]string, error) {
	var entries []scanEntry

	for _, dir := range dirs {
		select {
		case <-ctx.Done():
			return sortEntries(entries), ctx.Err()
		default:
		}

		// 验证目录是否存在、是否为普通目录、是否符号链接
		fi, err := os.Lstat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				slog.Warn("directory does not exist, skipping", "path", dir)
				continue
			}
			return sortEntries(entries), err
		}
		if !fi.IsDir() {
			slog.Warn("path is not a directory, skipping", "path", dir)
			continue
		}
		// 跳过符号链接目录
		if fi.Mode()&os.ModeSymlink != 0 {
			slog.Warn("symlink directory skipped", "path", dir)
			continue
		}

		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// 跳过符号链接（目录符号链接则跳过整个子目录树）
			if mode := info.Mode(); mode&os.ModeSymlink != 0 {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// 跳过目录
			if info.IsDir() {
				return nil
			}

			// 跳过非图片文件
			if !isImageFile(path) {
				return nil
			}

			// 增量扫描：跳过 mtime 未变化的已有文件
			mtime := info.ModTime().Unix()
			if existingMtime, ok := existing[path]; ok && existingMtime == mtime {
				return nil
			}

			entries = append(entries, scanEntry{path: path, mtime: mtime})
			return nil
		})

		if err != nil {
			return sortEntries(entries), err
		}
	}

	return sortEntries(entries), nil
}

// sortEntries 按修改时间降序排序并提取路径列表。
func sortEntries(entries []scanEntry) []string {
	if len(entries) == 0 {
		return []string{}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mtime > entries[j].mtime
	})
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.path
	}
	return result
}
