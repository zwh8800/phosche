// Package static 提供照片文件的静态 HTTP 服务，内置路径遍历防护和扩展名白名单。
package static

import (
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gen2brain/heic"
	"golang.org/x/image/draw"
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

		// 解析图片处理查询参数
		convert := r.URL.Query().Get("convert")
		wStr := r.URL.Query().Get("w")
		var maxWidth int
		if wStr != "" {
			var err error
			maxWidth, err = strconv.Atoi(wStr)
			if err != nil || maxWidth <= 0 {
				http.Error(w, "invalid width parameter", http.StatusBadRequest)
				return
			}
		}
		needsProcessing := convert == "1" || maxWidth > 0

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

			if !needsProcessing {
				w.Header().Set("Cache-Control", "public, max-age=86400")
				http.ServeFile(w, r, safePath)
				return
			}

			serveProcessedImage(w, safePath, maxWidth)
			return
		}

		http.NotFound(w, r)
	})
}

// serveProcessedImage 解码、缩放并编码图片。HEIC 自动转 JPEG，PNG 仅当缩放时转 JPEG。
func serveProcessedImage(w http.ResponseWriter, safePath string, maxWidth int) {
	f, err := os.Open(safePath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(safePath))

	var img image.Image
	var decodeErr error
	outputJPEG := ext == ".jpg" || ext == ".jpeg"

	switch ext {
	case ".jpg", ".jpeg":
		img, decodeErr = jpeg.Decode(f)
	case ".png":
		img, decodeErr = png.Decode(f)
	case ".heic", ".heif":
		img, decodeErr = heic.Decode(f)
		outputJPEG = true
	default:
		fi, err := f.Stat()
		if err != nil {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeContent(w, &http.Request{}, filepath.Base(safePath), fi.ModTime(), f)
		return
	}

	if decodeErr != nil {
		http.Error(w, "failed to decode image", http.StatusInternalServerError)
		return
	}

	if maxWidth > 0 {
		bounds := img.Bounds()
		if bounds.Dx() > maxWidth {
			newHeight := maxWidth * bounds.Dy() / bounds.Dx()
			dst := image.NewRGBA(image.Rect(0, 0, maxWidth, newHeight))
			draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
			img = dst
			outputJPEG = true
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")

	if outputJPEG {
		w.Header().Set("Content-Type", "image/jpeg")
		jpeg.Encode(w, img, &jpeg.Options{Quality: 85})
	} else {
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, img)
	}
}
