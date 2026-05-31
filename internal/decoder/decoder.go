// Package decoder 提供多格式图片解码和 EXIF 元数据提取功能。支持 JPEG、PNG、WebP、HEIC/HEIF 格式。
package decoder

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	_ "image/gif"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/meta/exif"
	"github.com/gen2brain/heic"
	goexif "github.com/rwcarlsen/goexif/exif"
	"github.com/zwh8800/phosche/internal/types"
	"golang.org/x/image/webp"
)

// DecodeResult 包含解码后的图片对象、MIME 类型和可选的 EXIF 元数据。
type DecodeResult struct {
	Image    image.Image
	MIMEType string
	EXIF     *types.EXIFInfo
}

// DecodeImage 根据文件扩展名自动选择解码器，解码图片并提取 EXIF 元数据。
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

	var exifInfo *types.EXIFInfo
	if info, exifErr := extractEXIF(path); exifErr != nil {
		slog.Warn("decoder: failed to extract EXIF", "path", path, "error", exifErr)
	} else {
		exifInfo = info
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

// extractEXIF 从照片文件中提取 EXIF 元数据：拍摄时间、相机型号、镜头型号、焦距、光圈、快门速度、ISO、GPS 坐标。
// 首先使用 imagemeta 库处理所有格式（JPEG、HEIC/HEIF、PNG 等）。
// 如果 imagemeta 返回空数据且文件为 HEIC/HEIF 格式，则回退到 goexif + ISOBMFF 解析。
func extractEXIF(path string) (*types.EXIFInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file for EXIF: %w", err)
	}
	defer f.Close()

	ex, err := imagemeta.Decode(f)
	if err == nil {
		if info := exifToEXIFInfo(ex); info != nil {
			return info, nil
		}
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".heic" || ext == ".heif" {
		slog.Debug("decoder: imagemeta failed or returned empty, falling back to goexif for HEIC", "path", path)
		return extractExifFromHEIC(path)
	}

	if err != nil {
		return nil, fmt.Errorf("decode EXIF: %w", err)
	}
	return nil, nil
}

// exifToEXIFInfo 将 imagemeta 解析的 EXIF 数据转换为 types.EXIFInfo。
// 如果所有字段均为空值，则返回 nil。
func exifToEXIFInfo(ex exif.Exif) *types.EXIFInfo {
	info := &types.EXIFInfo{}

	if !ex.ExifIFD.DateTimeOriginal.IsZero() {
		info.DateTimeOriginal = ex.ExifIFD.DateTimeOriginal.Format(time.RFC3339)
	}

	info.CameraModel = strings.TrimSpace(ex.IFD0.Model)
	info.LensModel = strings.TrimSpace(ex.ExifIFD.LensModel)

	if fl := float64(ex.ExifIFD.FocalLength); fl > 0 {
		info.FocalLength = fmt.Sprintf("%.1fmm", fl)
	}

	if fn := float64(ex.ExifIFD.FNumber); fn > 0 {
		info.Aperture = fmt.Sprintf("f/%.1f", fn)
	}

	if et := float64(ex.ExifIFD.ExposureTime); et > 0 {
		if et < 1 {
			denom := math.Round(1.0 / et)
			info.ShutterSpeed = fmt.Sprintf("1/%ds", int(denom))
		} else {
			info.ShutterSpeed = fmt.Sprintf("%.0fs", et)
		}
	}

	if iso := ex.ExifIFD.ISOSpeedRatings; iso > 0 {
		info.ISO = int(iso)
	}

	if lat := ex.GPS.Latitude(); lat != 0 {
		info.GPSLat = roundGPS(lat)
	}
	if lon := ex.GPS.Longitude(); lon != 0 {
		info.GPSLon = roundGPS(lon)
	}

	if info.DateTimeOriginal == "" &&
		info.CameraModel == "" &&
		info.LensModel == "" &&
		info.FocalLength == "" &&
		info.Aperture == "" &&
		info.ShutterSpeed == "" &&
		info.ISO == 0 &&
		info.GPSLat == 0 && info.GPSLon == 0 {
		return nil
	}

	return info
}

// roundGPS 将 GPS 坐标四舍五入到 6 位小数。
func roundGPS(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

func extractExifFromHEIC(path string) (*types.EXIFInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	exifBytes, err := findExifBox(data)
	if err != nil {
		return nil, err
	}

	x, err := goexif.Decode(bytes.NewReader(exifBytes))
	if err != nil {
		return nil, err
	}
	return goexifToEXIFInfo(x), nil
}

func findExifBox(data []byte) ([]byte, error) {
	i := 0
	for i < len(data)-8 {
		boxSize := int(data[i])<<24 | int(data[i+1])<<16 | int(data[i+2])<<8 | int(data[i+3])
		boxType := string(data[i+4 : i+8])
		if boxSize < 8 {
			i += 8
			continue
		}
		if boxType == "Exif" {
			return data[i+8 : i+boxSize], nil
		}
		if boxType == "moov" || boxType == "meta" || boxType == "iprp" || boxType == "ipco" {
			result, err := findExifBox(data[i+8 : i+boxSize])
			if err == nil {
				return result, nil
			}
		}
		i += boxSize
	}
	// 兜底：直接扫描 Exif\x00\x00 + TIFF header 模式
	for j := 0; j < len(data)-10; j++ {
		if data[j] == 'E' && data[j+1] == 'x' && data[j+2] == 'i' && data[j+3] == 'f' &&
			data[j+4] == 0 && data[j+5] == 0 {
			if j+8 < len(data) && ((data[j+6] == 'M' && data[j+7] == 'M') || (data[j+6] == 'I' && data[j+7] == 'I')) {
				return data[j+6:], nil
			}
		}
	}

	return nil, fmt.Errorf("exif box not found")
}

func goexifToEXIFInfo(x *goexif.Exif) *types.EXIFInfo {
	info := &types.EXIFInfo{}
	if dt, err := x.DateTime(); err == nil {
		info.DateTimeOriginal = dt.Format(time.RFC3339)
	}
	if tag, err := x.Get(goexif.Model); err == nil {
		if s, sErr := tag.StringVal(); sErr == nil {
			info.CameraModel = strings.TrimSpace(s)
		}
	}
	if tag, err := x.Get(goexif.LensModel); err == nil {
		if s, sErr := tag.StringVal(); sErr == nil {
			info.LensModel = strings.TrimSpace(s)
		}
	}
	if tag, err := x.Get(goexif.FocalLength); err == nil {
		if num, den, rErr := tag.Rat2(0); rErr == nil && den != 0 {
			info.FocalLength = fmt.Sprintf("%.1fmm", float64(num)/float64(den))
		}
	}
	if tag, err := x.Get(goexif.FNumber); err == nil {
		if num, den, rErr := tag.Rat2(0); rErr == nil && den != 0 {
			fVal := float64(num) / float64(den)
			info.Aperture = fmt.Sprintf("f/%.1f", fVal)
		}
	}
	if tag, err := x.Get(goexif.ExposureTime); err == nil {
		if num, den, rErr := tag.Rat2(0); rErr == nil && den != 0 {
			info.ShutterSpeed = fmt.Sprintf("%d/%ds", num, den)
		}
	}
	if tag, err := x.Get(goexif.ISOSpeedRatings); err == nil {
		if iso, iErr := tag.Int(0); iErr == nil {
			info.ISO = iso
		}
	}
	if lat, lon, err := x.LatLong(); err == nil {
		info.GPSLat = roundGPS(lat)
		info.GPSLon = roundGPS(lon)
	}

	if info.DateTimeOriginal == "" &&
		info.CameraModel == "" &&
		info.LensModel == "" &&
		info.FocalLength == "" &&
		info.Aperture == "" &&
		info.ShutterSpeed == "" &&
		info.ISO == 0 &&
		info.GPSLat == 0 && info.GPSLon == 0 {
		return nil
	}

	return info
}

