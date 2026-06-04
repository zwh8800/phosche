// Package static 提供照片文件的静态 HTTP 服务，内置路径遍历防护和扩展名白名单，
// 并支持基于缓存文件的缩略图和 HEIC 转换加速。
package static

import (
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/gen2brain/heic"
	"github.com/zwh8800/phosche/internal/cache"
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

// PhotoHandler 返回照片文件静态服务的 HTTP 处理器。
// 支持多基础路径（依次尝试）、路径遍历防护、扩展名白名单，以及缓存加速：
//   - 无参数：直接返回原始文件
//   - ?thumb=1：优先从缓存返回缩略图，缓存缺失时实时生成并写入缓存
//   - ?convert=1：HEIC 优先从缓存返回全尺寸 JPEG，缓存缺失时实时生成并写入缓存；非 HEIC 直接返回原始文件
func PhotoHandler(photoBasePaths []string, cacheGen *cache.Generator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/photos/") {
			http.NotFound(w, r)
			return
		}

		requestedPath := r.URL.Path[len("/photos/"):]
		useThumb := r.URL.Query().Get("thumb") == "1"
		useConvert := r.URL.Query().Get("convert") == "1"

		var safePath string
		var ext string
		var found bool

		for _, basePath := range photoBasePaths {
			cleanBase := filepath.Clean(basePath)

			absRequested := "/" + requestedPath
			if strings.HasPrefix(absRequested, cleanBase) {
				safePath = filepath.Clean(absRequested)
			} else {
				safePath = filepath.Clean(filepath.Join(basePath, requestedPath))
			}

			if !strings.HasPrefix(safePath, cleanBase+string(filepath.Separator)) && safePath != cleanBase {
				continue
			}

			fi, err := os.Stat(safePath)
			if err != nil {
				continue
			}
			if fi.IsDir() {
				continue
			}

			ext = strings.ToLower(filepath.Ext(safePath))
			if !allowedExtensions[ext] {
				continue
			}
			found = true
			break
		}

		if !found {
			http.NotFound(w, r)
			return
		}

		photoID := cache.PhotoID(safePath)

		if useThumb {
			thumbPath := cacheGen.ThumbPath(photoID)
			if thumbPath != "" {
				if _, err := os.Stat(thumbPath); err == nil {
					w.Header().Set("Cache-Control", "public, max-age=86400")
					w.Header().Set("Content-Type", "image/jpeg")
					http.ServeFile(w, r, thumbPath)
					return
				}
				// 缓存缺失，实时生成并写入
				if err := cacheGen.GenerateThumb(safePath); err == nil {
					if _, err := os.Stat(thumbPath); err == nil {
						w.Header().Set("Cache-Control", "public, max-age=86400")
						w.Header().Set("Content-Type", "image/jpeg")
						http.ServeFile(w, r, thumbPath)
						return
					}
				}
			}
			// 如果缓存生成失败（或 cacheDir 为空），回退到实时生成
			serveProcessedImage(w, safePath, cache.ThumbWidth)
			return
		}

		if useConvert {
			isHEIC := ext == ".heic" || ext == ".heif"
			if isHEIC {
				fullPath := cacheGen.FullPath(photoID)
				if fullPath != "" {
					if _, err := os.Stat(fullPath); err == nil {
						w.Header().Set("Cache-Control", "public, max-age=86400")
						w.Header().Set("Content-Type", "image/jpeg")
						http.ServeFile(w, r, fullPath)
						return
					}
					if err := cacheGen.GenerateFull(safePath); err == nil {
						if _, err := os.Stat(fullPath); err == nil {
							w.Header().Set("Cache-Control", "public, max-age=86400")
							w.Header().Set("Content-Type", "image/jpeg")
							http.ServeFile(w, r, fullPath)
							return
						}
					}
				}
				// 缓存生成失败或 cacheDir 为空 → 实时 HEIC 转 JPEG
				serveProcessedImage(w, safePath, 0)
				return
			}
			// 非 HEIC 格式：直接返回原始文件（浏览器原生支持）
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, safePath)
			return
		}

		// 无参数：直接返回原始文件
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, safePath)
	})
}

// serveProcessedImage 实时解码、缩放并编码图片。HEIC 自动转 JPEG，PNG 仅当缩放时转 JPEG。
// 使用 image.Decode 通过文件魔数识别真实格式，不依赖文件扩展名。
func serveProcessedImage(w http.ResponseWriter, safePath string, maxWidth int) {
	f, err := os.Open(safePath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	img, decodeErr := decodeImageStatic(f)
	if decodeErr != nil {
		fi, err := f.Stat()
		if err != nil {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeContent(w, &http.Request{}, filepath.Base(safePath), fi.ModTime(), f)
		return
	}

	outputJPEG := isHEICContent(safePath)

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

func decodeImageStatic(r io.ReadSeeker) (image.Image, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	r.Seek(0, io.SeekStart)
	return img, nil
}

func isHEICContent(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 12)
	n, _ := io.ReadFull(f, buf)
	return n >= 8 && string(buf[4:8]) == "ftyp"
}
