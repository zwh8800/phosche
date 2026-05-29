// Package decoder 提供多格式图片解码和 EXIF 元数据提取功能。支持 JPEG、PNG、WebP、HEIC/HEIF 格式。
package decoder

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gen2brain/heic"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/zwh8800/phosche/internal/types"
	"golang.org/x/image/webp"
)

// DecodeResult 包含解码后的图片对象、MIME 类型和可选的 EXIF 元数据。
type DecodeResult struct {
	Image    image.Image
	MIMEType string
	EXIF     *types.EXIFInfo
}

// DecodeImage 根据文件扩展名自动选择解码器，解码图片并提取 EXIF 信息（仅 JPEG）。
func DecodeImage(path string) (*DecodeResult, error) {
	ext := strings.ToLower(filepath.Ext(path))

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("decoder: open file: %w", err)
	}
	defer f.Close()

	var img image.Image
	var mimeType string

	// 根据文件扩展名选择对应的图片解码器
	switch ext {
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(f)
		mimeType = "image/jpeg"
	case ".png":
		img, err = png.Decode(f)
		mimeType = "image/png"
	case ".webp":
		img, err = webp.Decode(f)
		mimeType = "image/webp"
	case ".heic", ".heif":
		img, err = decodeHEIC(f)
		mimeType = "image/heic"
	default:
		return nil, fmt.Errorf("decoder: unsupported format: %s", ext)
	}
	if err != nil {
		return nil, fmt.Errorf("decoder: decode %s: %w", ext, err)
	}

	// 仅 JPEG 格式支持 EXIF 元数据提取（其他格式缺乏标准化的 EXIF 嵌入方式）
	var exifInfo *types.EXIFInfo
	if ext == ".jpg" || ext == ".jpeg" {
		if info, exifErr := extractEXIF(path); exifErr != nil {
			slog.Warn("decoder: failed to extract EXIF", "path", path, "error", exifErr)
		} else {
			exifInfo = info
		}
	}

	return &DecodeResult{
		Image:    img,
		MIMEType: mimeType,
		EXIF:     exifInfo,
	}, nil
}

// decodeHEIC 使用 gen2brain/heic 库解码 HEIC/HEIF 格式图片。
func decodeHEIC(r io.Reader) (image.Image, error) {
	img, err := heic.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("heic decode: %w", err)
	}
	return img, nil
}

// extractEXIF 从 JPEG 文件中提取 EXIF 元数据：拍摄时间、相机型号、镜头型号、焦距、光圈、ISO、GPS 坐标。
// 如果所有字段均为空值（无有效 EXIF 数据），则返回 nil, nil。
func extractEXIF(path string) (*types.EXIFInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file for EXIF: %w", err)
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode EXIF: %w", err)
	}

	info := &types.EXIFInfo{}

	// 提取拍摄时间（DateTimeOriginal），格式化为 RFC3339
	if dt, err := x.DateTime(); err == nil {
		info.DateTimeOriginal = dt.Format(time.RFC3339)
	}

	// 提取相机型号（Make 标签的实际模型信息，而非制造商）
	if tag, err := x.Get(exif.Model); err == nil {
		if s, sErr := tag.StringVal(); sErr == nil {
			info.CameraModel = strings.TrimSpace(s)
		}
	}

	// 提取镜头型号（LensModel）
	if tag, err := x.Get(exif.LensModel); err == nil {
		if s, sErr := tag.StringVal(); sErr == nil {
			info.LensModel = strings.TrimSpace(s)
		}
	}

	// 提取焦距（FocalLength），以分数形式表示，转换为 "X.Xmm" 格式
	if tag, err := x.Get(exif.FocalLength); err == nil {
		if num, den, rErr := tag.Rat2(0); rErr == nil && den != 0 {
			info.FocalLength = fmt.Sprintf("%.1fmm", float64(num)/float64(den))
		}
	}

	// 提取光圈值（FNumber/F-Stop），转换为 "f/X.X" 格式
	if tag, err := x.Get(exif.FNumber); err == nil {
		if num, den, rErr := tag.Rat2(0); rErr == nil && den != 0 {
			fVal := float64(num) / float64(den)
			info.Aperture = fmt.Sprintf("f/%.1f", fVal)
		}
	}

	// 提取 ISO 感光度（ISOSpeedRatings）
	if tag, err := x.Get(exif.ISOSpeedRatings); err == nil {
		if iso, iErr := tag.Int(0); iErr == nil {
			info.ISO = iso
		}
	}

	// 提取 GPS 坐标（纬度/经度），精确到 6 位小数
	if lat, lon, err := x.LatLong(); err == nil {
		info.GPSLat = roundGPS(lat)
		info.GPSLon = roundGPS(lon)
	}

	if info.DateTimeOriginal == "" &&
		info.CameraModel == "" &&
		info.LensModel == "" &&
		info.FocalLength == "" &&
		info.Aperture == "" &&
		info.ISO == 0 &&
		info.GPSLat == 0 && info.GPSLon == 0 {
		return nil, nil
	}

	return info, nil
}

// roundGPS 将 GPS 坐标四舍五入到 6 位小数。
func roundGPS(v float64) float64 {
	s := strconv.FormatFloat(v, 'f', 6, 64)
	result, _ := strconv.ParseFloat(s, 64)
	return result
}
