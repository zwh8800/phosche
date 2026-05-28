package watcher

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var supportedExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".heic": true,
	".heif": true,
}

func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return supportedExts[ext]
}

// LoadExisting scans directories and returns a map of path -> mtime for all image files.
// Only files with supported image extensions are included (.jpg/.jpeg/.png/.webp/.heic/.heif).
// If a directory doesn't exist, it is skipped with a warning log.
func LoadExisting(dirs []string, recursive bool) (map[string]int64, error) {
	result := make(map[string]int64)

	for _, dir := range dirs {
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
			err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
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
			entries, err := os.ReadDir(dir)
			if err != nil {
				return result, err
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				fullPath := filepath.Join(dir, entry.Name())
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
