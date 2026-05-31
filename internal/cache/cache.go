// Package cache 提供照片缓存生成与查询功能。
// 在流水线处理完照片后预先生成缩略图和全尺寸 JPEG 缓存，
// 避免每次 HTTP 请求时实时进行 HEIC 解码和图片缩放。
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/gen2brain/heic"
	"golang.org/x/image/draw"
)

const ThumbWidth = 400
const JPEGQuality = 85

// PhotoID 计算照片的 SHA-256 16 进制字符串 ID。
func PhotoID(path string) string {
	h := sha256.Sum256([]byte(path))
	return hex.EncodeToString(h[:])
}

// Generator 负责生成和管理照片缓存文件。
type Generator struct {
	CacheDir string
}

// NewGenerator 创建缓存生成器。cacheDir 为空时不会执行任何缓存操作。
func NewGenerator(cacheDir string) *Generator {
	if cacheDir != "" {
		os.MkdirAll(cacheDir, 0755)
	}
	return &Generator{CacheDir: cacheDir}
}

// GenerateThumb 为任意图片格式生成 400px 宽的 JPEG 缩略图。
// 输出文件：{cacheDir}/{id}_thumb.jpg
// 缓存已存在或 cacheDir 为空时直接返回 nil。
func (g *Generator) GenerateThumb(srcPath string) error {
	if g.CacheDir == "" {
		return nil
	}
	id := PhotoID(srcPath)
	dstPath := filepath.Join(g.CacheDir, id+"_thumb.jpg")
	if _, err := os.Stat(dstPath); err == nil {
		return nil
	}

	img, err := decodeImage(srcPath)
	if err != nil {
		return err
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w > ThumbWidth {
		newH := h * ThumbWidth / w
		dst := image.NewRGBA(image.Rect(0, 0, ThumbWidth, newH))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}

	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: JPEGQuality})
}

// GenerateFull 为 HEIC/HEIF 图片生成全尺寸 JPEG 缓存。
// 非 HEIC 格式直接跳过（浏览器可原生显示）。
// 输出文件：{cacheDir}/{id}_full.jpg
// 缓存已存在或 cacheDir 为空时直接返回 nil。
func (g *Generator) GenerateFull(srcPath string) error {
	if g.CacheDir == "" {
		return nil
	}
	id := PhotoID(srcPath)
	dstPath := filepath.Join(g.CacheDir, id+"_full.jpg")
	if _, err := os.Stat(dstPath); err == nil {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(srcPath))
	if ext != ".heic" && ext != ".heif" {
		return nil
	}

	img, err := decodeImage(srcPath)
	if err != nil {
		return err
	}

	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: JPEGQuality})
}

// ThumbPath 返回缩略图缓存文件的完整路径。
func (g *Generator) ThumbPath(id string) string {
	if g.CacheDir == "" {
		return ""
	}
	return filepath.Join(g.CacheDir, id+"_thumb.jpg")
}

// FullPath 返回全尺寸 JPEG 缓存文件的完整路径。
func (g *Generator) FullPath(id string) string {
	if g.CacheDir == "" {
		return ""
	}
	return filepath.Join(g.CacheDir, id+"_full.jpg")
}

// decodeImage 根据文件扩展名自动选择解码器解码图片。
func decodeImage(path string) (image.Image, error) {
	ext := strings.ToLower(filepath.Ext(path))
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if ext == ".heic" || ext == ".heif" {
		return heic.Decode(f)
	}
	img, _, err := image.Decode(f)
	return img, err
}
