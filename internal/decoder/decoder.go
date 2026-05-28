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

// DecodeResult holds the decoded image together with its MIME type and optional EXIF metadata.
type DecodeResult struct {
	Image    image.Image
	MIMEType string
	EXIF     *types.EXIFInfo
}

// DecodeImage detects the image format from the file extension, decodes the image,
// and extracts EXIF metadata for JPEG files. Supported formats: .jpg/.jpeg, .png, .webp, .heic/.heif.
func DecodeImage(path string) (*DecodeResult, error) {
	ext := strings.ToLower(filepath.Ext(path))

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("decoder: open file: %w", err)
	}
	defer f.Close()

	var img image.Image
	var mimeType string

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

func decodeHEIC(r io.Reader) (image.Image, error) {
	img, err := heic.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("heic decode: %w", err)
	}
	return img, nil
}

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

func roundGPS(v float64) float64 {
	s := strconv.FormatFloat(v, 'f', 6, 64)
	result, _ := strconv.ParseFloat(s, 64)
	return result
}
