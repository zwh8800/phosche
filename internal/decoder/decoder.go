// Package decoder 提供多格式图片解码和 EXIF 元数据提取功能。支持 JPEG、PNG、WebP、HEIC/HEIF 格式。
package decoder

import (
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

	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	heicexif "github.com/dsoprea/go-heic-exif-extractor/v2"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure"
	pngstructure "github.com/dsoprea/go-png-image-structure"
	"github.com/gen2brain/heic"
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
// 使用 dsoprea/go-exif/v3 库解析 EXIF 数据。
func extractEXIF(path string) (info *types.EXIFInfo, err error) {
	// 捕获 dsoprea/go-logging 可能产生的 panic
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("exif panic: %v", e)
		}
	}()

	rawExif, err := extractRawExif(path)
	if err != nil {
		if err == exif.ErrNoExif {
			return nil, nil
		}
		return nil, fmt.Errorf("extract raw exif: %w", err)
	}
	return parseExif(rawExif)
}

// extractRawExif 从照片文件中提取原始 EXIF 字节。
// JPEG/PNG/HEIC 使用 dsoprea 格式专用库精确解析，WebP 使用暴力搜索。
func extractRawExif(path string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".jpg", ".jpeg":
		return extractRawExifFromJPEG(path)
	case ".png":
		return extractRawExifFromPNG(path)
	case ".heic", ".heif":
		return extractRawExifFromHEIC(path)
	default:
		return exif.SearchFileAndExtractExif(path)
	}
}

// extractRawExifFromJPEG 使用 go-jpeg-image-structure 解析 JPEG APP1 segment 中的 EXIF。
// 解析失败时回退到暴力搜索。
func extractRawExifFromJPEG(path string) ([]byte, error) {
	jpegmp := jpegstructure.NewJpegMediaParser()
	sl, err := jpegmp.ParseFile(path)
	if err != nil {
		slog.Debug("decoder: jpeg parser failed, falling back to brute-force", "path", path, "error", err)
		return exif.SearchFileAndExtractExif(path)
	}

	_, rawExif, err := sl.Exif()
	if err != nil {
		slog.Debug("decoder: jpeg exif extraction failed, falling back to brute-force", "path", path, "error", err)
		return exif.SearchFileAndExtractExif(path)
	}
	if len(rawExif) == 0 {
		return nil, exif.ErrNoExif
	}
	return rawExif, nil
}

// extractRawExifFromPNG 使用 go-png-image-structure 解析 PNG iTXt chunk 中的 EXIF。
// 解析失败时回退到暴力搜索。
func extractRawExifFromPNG(path string) ([]byte, error) {
	pngmp := pngstructure.NewPngMediaParser()
	ec, err := pngmp.ParseFile(path)
	if err != nil {
		slog.Debug("decoder: png parser failed, falling back to brute-force", "path", path, "error", err)
		return exif.SearchFileAndExtractExif(path)
	}

	cs, ok := ec.(*pngstructure.ChunkSlice)
	if !ok {
		slog.Debug("decoder: unexpected png type, falling back to brute-force", "path", path, "type", fmt.Sprintf("%T", ec))
		return exif.SearchFileAndExtractExif(path)
	}

	_, rawExif, err := cs.Exif()
	if err != nil {
		slog.Debug("decoder: png exif extraction failed, falling back to brute-force", "path", path, "error", err)
		return exif.SearchFileAndExtractExif(path)
	}
	if len(rawExif) == 0 {
		return nil, exif.ErrNoExif
	}
	return rawExif, nil
}

// extractRawExifFromHEIC 使用 go-heic-exif-extractor 解析 HEIC BMFF 容器中的 EXIF。
// 解析失败时回退到暴力搜索（兼容损坏或非标准 HEIC 文件）。
func extractRawExifFromHEIC(path string) ([]byte, error) {
	hemp := heicexif.NewHeicExifMediaParser()
	mc, err := hemp.ParseFile(path)
	if err != nil {
		slog.Debug("decoder: heic parser failed, falling back to brute-force", "path", path, "error", err)
		return exif.SearchFileAndExtractExif(path)
	}

	hec, ok := mc.(heicexif.HeicExifContext)
	if !ok {
		slog.Debug("decoder: unexpected heic type, falling back to brute-force", "path", path, "type", fmt.Sprintf("%T", mc))
		return exif.SearchFileAndExtractExif(path)
	}

	_, rawExif, err := hec.Exif()
	if err != nil {
		slog.Debug("decoder: heic exif extraction failed, falling back to brute-force", "path", path, "error", err)
		return exif.SearchFileAndExtractExif(path)
	}
	if len(rawExif) == 0 {
		return nil, exif.ErrNoExif
	}
	return rawExif, nil
}

// parseExif 解析原始 EXIF 字节，提取所有需要的元数据字段。
// 使用 exif.Collect 解析 IFD 结构，然后通过 IFD 路径导航提取各字段。
func parseExif(rawExif []byte) (*types.EXIFInfo, error) {
	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return nil, fmt.Errorf("create ifd mapping: %w", err)
	}

	ti := exif.NewTagIndex()

	_, index, err := exif.Collect(im, ti, rawExif)
	if err != nil {
		return nil, fmt.Errorf("collect exif: %w", err)
	}

	info := &types.EXIFInfo{}

	// IFD0 (root): Model
	if v, err := getStringTag(index.RootIfd, "Model"); err == nil {
		info.CameraModel = strings.TrimSpace(v)
	}

	// IFD/Exif: DateTimeOriginal, LensModel, FocalLength, FNumber, ExposureTime, ISOSpeedRatings
	if exifIfd, ok := index.Lookup["IFD/Exif"]; ok {
		if v, err := getStringTag(exifIfd, "DateTimeOriginal"); err == nil {
			// EXIF 时间格式: "2006:01:02 15:04:05" -> RFC3339
			if t, tErr := time.Parse("2006:01:02 15:04:05", v); tErr == nil {
				info.DateTimeOriginal = t.Format(time.RFC3339)
			}
		}

		if v, err := getStringTag(exifIfd, "LensModel"); err == nil {
			info.LensModel = strings.TrimSpace(v)
		}

		if v, err := getRationalTag(exifIfd, "FocalLength"); err == nil && v > 0 {
			info.FocalLength = fmt.Sprintf("%.1fmm", v)
		}

		if v, err := getRationalTag(exifIfd, "FNumber"); err == nil && v > 0 {
			info.Aperture = fmt.Sprintf("f/%.1f", v)
		}

		if v, err := getRationalTag(exifIfd, "ExposureTime"); err == nil && v > 0 {
			if v < 1 {
				denom := math.Round(1.0 / v)
				info.ShutterSpeed = fmt.Sprintf("1/%ds", int(denom))
			} else {
				info.ShutterSpeed = fmt.Sprintf("%.0fs", v)
			}
		}

		if v, err := getShortTag(exifIfd, "ISOSpeedRatings"); err == nil && v > 0 {
			info.ISO = int(v)
		}
	}

	// IFD/GPSInfo: GPS 坐标
	if gpsIfd, ok := index.Lookup["IFD/GPSInfo"]; ok {
		if gi, err := gpsIfd.GpsInfo(); err == nil {
			if lat := gi.Latitude.Decimal(); lat != 0 {
				info.GPSLat = roundGPS(lat)
			}
			if lon := gi.Longitude.Decimal(); lon != 0 {
				info.GPSLon = roundGPS(lon)
			}
		}
	}

	// 如果所有字段都为空，返回 nil（保持与原实现一致的行为）
	if info.DateTimeOriginal == "" &&
		info.CameraModel == "" &&
		info.LensModel == "" &&
		info.FocalLength == "" &&
		info.Aperture == "" &&
		info.ShutterSpeed == "" &&
		info.ISO == 0 &&
		info.GPSLat == 0 && info.GPSLon == 0 {
		return nil, nil
	}

	return info, nil
}

// getStringTag 从 IFD 中提取字符串类型的 tag。
func getStringTag(ifd *exif.Ifd, tagName string) (string, error) {
	results, err := ifd.FindTagWithName(tagName)
	if err != nil {
		return "", err
	}
	v, err := results[0].Value()
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("tag %s is not a string", tagName)
	}
	return s, nil
}

// getRationalTag 从 IFD 中提取有理数类型的 tag，返回 float64。
// 用于 FocalLength、FNumber、ExposureTime 等字段。
func getRationalTag(ifd *exif.Ifd, tagName string) (float64, error) {
	results, err := ifd.FindTagWithName(tagName)
	if err != nil {
		return 0, err
	}
	v, err := results[0].Value()
	if err != nil {
		return 0, err
	}
	rationals, ok := v.([]exifcommon.Rational)
	if !ok || len(rationals) == 0 {
		return 0, fmt.Errorf("tag %s is not rational", tagName)
	}
	r := rationals[0]
	if r.Denominator == 0 {
		return 0, fmt.Errorf("tag %s has zero denominator", tagName)
	}
	return float64(r.Numerator) / float64(r.Denominator), nil
}

// getShortTag 从 IFD 中提取短整数类型的 tag。
// 用于 ISOSpeedRatings 等字段。
func getShortTag(ifd *exif.Ifd, tagName string) (uint16, error) {
	results, err := ifd.FindTagWithName(tagName)
	if err != nil {
		return 0, err
	}
	v, err := results[0].Value()
	if err != nil {
		return 0, err
	}
	shorts, ok := v.([]uint16)
	if !ok || len(shorts) == 0 {
		return 0, fmt.Errorf("tag %s is not short", tagName)
	}
	return shorts[0], nil
}

// roundGPS 将 GPS 坐标四舍五入到 6 位小数。
func roundGPS(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
