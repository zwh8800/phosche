// Package static 提供照片文件的静态 HTTP 服务，内置路径遍历防护和扩展名白名单。
package static

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// allowedExtensions 允许的图片文件扩展名（大小写不敏感）。
var allowedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".heic": true,
	".heif": true,
}

// PhotoHandler 返回照片文件静态服务的 HTTP 处理器。支持多基础路径（依次尝试）、路径遍历防护、扩展名白名单、目录排除。响应包含 Cache-Control: public, max-age=86400（1 天缓存）。
func PhotoHandler(photoBasePaths []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/photos/") {
			http.NotFound(w, r)
			return
		}

		requestedPath := r.URL.Path[len("/photos/"):]

		// 多基础路径：依次尝试，第一个找到文件的路径提供服务
		for _, basePath := range photoBasePaths {
			cleanBase := filepath.Clean(basePath)

			// 路径遍历防护：filepath.Clean 标准化 + HasPrefix 前缀检查
			var safePath string
			absRequested := "/" + requestedPath
			if strings.HasPrefix(absRequested, cleanBase) {
				// requestedPath is an absolute path without leading slash (e.g. Volumes/photo/单反/xxx.jpg)
				safePath = filepath.Clean(absRequested)
			} else {
				safePath = filepath.Clean(filepath.Join(basePath, requestedPath))
			}

			// 确保安全路径仍以 basePath 为前缀（防止 path traversal）
			if !strings.HasPrefix(safePath, cleanBase+string(filepath.Separator)) && safePath != cleanBase {
				continue
			}

			// 检查文件存在且不是目录
			fi, err := os.Stat(safePath)
			if err != nil {
				continue
			}

			if fi.IsDir() {
				continue
			}

			// 扩展名白名单校验
			ext := strings.ToLower(filepath.Ext(safePath))
			if !allowedExtensions[ext] {
				continue
			}

			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, safePath)
			return
		}

		http.NotFound(w, r)
	})
}
