package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// DirectoryScanner implements the Scanner interface for recursively scanning
// directories for image files.
type DirectoryScanner struct{}

type scanEntry struct {
	path  string
	mtime int64
}

// Scan recursively scans the given directories for image files (.jpg/.jpeg/.png/.webp/.heic/.heif).
// Files already present in the existing map with a matching mtime are skipped.
// Returns paths of new or modified files, sorted by mtime descending (newest first).
// Non-existent directories are logged as a warning and skipped.
// Symlinks are not followed.
func (s *DirectoryScanner) Scan(ctx context.Context, dirs []string, existing map[string]int64) ([]string, error) {
	var entries []scanEntry

	for _, dir := range dirs {
		select {
		case <-ctx.Done():
			return sortEntries(entries), ctx.Err()
		default:
		}

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

			entries = append(entries, scanEntry{path: path, mtime: mtime})
			return nil
		})

		if err != nil {
			return sortEntries(entries), err
		}
	}

	return sortEntries(entries), nil
}

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
