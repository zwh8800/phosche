package watcher

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// supportedExts 支持的图片文件扩展名集合。
var supportedExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".heic": true,
	".heif": true,
}

// isImageFile 判断文件是否为支持的图片格式。
func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return supportedExts[ext]
}

// LoadExisting 扫描目录并返回所有图片文件的 path→mtime 映射。
func LoadExisting(dirs []string, recursive bool) (map[string]int64, error) {
	result := make(map[string]int64)

	for _, dir := range dirs {
		// 处理不存在的目录：跳过并记录警告
		info, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				slog.Warn("directory does not exist, skipping", "path", dir)
				continue
			}
			return result, err
		}
		if !info.IsDir() {
			slog.Warn("path is not a directory, skipping", "path", dir)
			continue
		}

		if recursive {
			// 递归扫描：遍历所有子目录
			err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				// 仅处理支持的图片文件
				if !isImageFile(path) {
					return nil
				}
				result[path] = info.ModTime().Unix()
				return nil
			})
			if err != nil {
				return result, err
			}
		} else {
			// 非递归扫描：仅读取目录顶层文件
			entries, err := os.ReadDir(dir)
			if err != nil {
				return result, err
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				fullPath := filepath.Join(dir, entry.Name())
				// 仅处理支持的图片文件
				if !isImageFile(fullPath) {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					continue
				}
				result[fullPath] = info.ModTime().Unix()
			}
		}
	}

	return result, nil
}
