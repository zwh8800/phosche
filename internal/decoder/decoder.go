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

// extractEXIF 从照片文件中提取 EXIF 元数据：拍摄时间、相机型号、镜头型号、焦距、光圈、ISO、GPS 坐标。
// 支持 JPEG 和 HEIC/HEIF 格式。JPEG 直接解析，HEIC 从 ISOBMFF 容器中提取 Exif box。
// 如果所有字段均为空值（无有效 EXIF 数据），则返回 nil, nil。
func extractEXIF(path string) (*types.EXIFInfo, error) {
	ext := strings.ToLower(filepath.Ext(path))

	var x *exif.Exif
	var err error

	if ext == ".heic" || ext == ".heif" {
		x, err = extractExifFromHEIC(path)
		if err != nil {
			return nil, fmt.Errorf("decode HEIC EXIF: %w", err)
		}
	} else {
		f, openErr := os.Open(path)
		if openErr != nil {
			return nil, fmt.Errorf("open file for EXIF: %w", openErr)
		}
		defer f.Close()

		x, err = exif.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decode EXIF: %w", err)
		}
	}

	info := &types.EXIFInfo{}

	if dt, err := x.DateTime(); err == nil {
		info.DateTimeOriginal = dt.Format(time.RFC3339)
	}

	if tag, err := x.Get(exif.Model); err == nil {
		if s, sErr := tag.StringVal(); sErr == nil {
			info.CameraModel = strings.TrimSpace(s)
		}
	}

	if tag, err := x.Get(exif.LensModel); err == nil {
		if s, sErr := tag.StringVal(); sErr == nil {
			info.LensModel = strings.TrimSpace(s)
		}
	}

	if tag, err := x.Get(exif.FocalLength); err == nil {
		if num, den, rErr := tag.Rat2(0); rErr == nil && den != 0 {
			info.FocalLength = fmt.Sprintf("%.1fmm", float64(num)/float64(den))
		}
	}

	if tag, err := x.Get(exif.FNumber); err == nil {
		if num, den, rErr := tag.Rat2(0); rErr == nil && den != 0 {
			fVal := float64(num) / float64(den)
			info.Aperture = fmt.Sprintf("f/%.1f", fVal)
		}
	}

	if tag, err := x.Get(exif.ExposureTime); err == nil {
		if num, den, rErr := tag.Rat2(0); rErr == nil && den != 0 {
			info.ShutterSpeed = fmt.Sprintf("%d/%ds", num, den)
		}
	}

	if tag, err := x.Get(exif.ISOSpeedRatings); err == nil {
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

// extractExifFromHEIC 从 HEIC/HEIF 文件的 ISOBMFF 容器中提取 EXIF 数据。
// ISOBMFF 盒子结构：4字节大小 + 4字节类型 + 内容。
// EXIF 数据存储在类型为 "Exif" 的盒子中。
func extractExifFromHEIC(path string) (*exif.Exif, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	exifBytes, err := findExifBox(data)
	if err != nil {
		return nil, fmt.Errorf("find exif box: %w", err)
	}

	// 预置 JPEG APP1 marker 格式，goexif.Decode 需要此格式
	marker := []byte{0xFF, 0xE1}
	exifHeader := []byte{0x45, 0x78, 0x69, 0x66, 0x00, 0x00} // "Exif\0\0"
	length := len(exifBytes) + len(exifHeader)
	lengthBytes := []byte{byte(length >> 8), byte(length & 0xFF)}

	app1Data := make([]byte, 0, 2+2+len(exifHeader)+len(exifBytes))
	app1Data = append(app1Data, marker...)
	app1Data = append(app1Data, lengthBytes...)
	app1Data = append(app1Data, exifHeader...)
	app1Data = append(app1Data, exifBytes...)

	x, err := exif.Decode(bytes.NewReader(app1Data))
	if err != nil {
		return nil, fmt.Errorf("parse exif: %w", err)
	}

	return x, nil
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

		// 递归进入父盒子（如 moov, meta, iprp, ipco, ispe）
		if boxType == "moov" || boxType == "meta" || boxType == "iprp" || boxType == "ipco" {
			result, err := findExifBox(data[i+8 : i+boxSize])
			if err == nil {
				return result, nil
			}
		}

		i += boxSize
	}

	return nil, fmt.Errorf("exif box not found")
}
