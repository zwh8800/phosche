package decoder

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

func generateTestJPEGWithEXIF(t *testing.T) (string, *expectedEXIF) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 25), G: uint8(y * 25), B: 128, A: 255})
		}
	}

	var jpegBuf bytes.Buffer
	err := jpeg.Encode(&jpegBuf, img, nil)
	if err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	jpegBytes := jpegBuf.Bytes()

	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		t.Fatalf("create ifd mapping: %v", err)
	}

	ti := exif.NewTagIndex()

	rootIb := exif.NewIfdBuilder(im, ti, exifcommon.IfdStandardIfdIdentity, exifcommon.TestDefaultByteOrder)

	err = rootIb.SetStandardWithName("Model", "TestCamera X1")
	if err != nil {
		t.Fatalf("set Model: %v", err)
	}

	exifIb := exif.NewIfdBuilder(im, ti, exifcommon.IfdExifStandardIfdIdentity, exifcommon.TestDefaultByteOrder)

	err = exifIb.SetStandardWithName("DateTimeOriginal", "2024:01:15 10:30:00")
	if err != nil {
		t.Fatalf("set DateTimeOriginal: %v", err)
	}

	err = exifIb.SetStandardWithName("LensModel", "TestLens 50mm f/1.8")
	if err != nil {
		t.Fatalf("set LensModel: %v", err)
	}

	err = exifIb.SetStandardWithName("FocalLength", []exifcommon.Rational{{Numerator: 50, Denominator: 1}})
	if err != nil {
		t.Fatalf("set FocalLength: %v", err)
	}

	err = exifIb.SetStandardWithName("FNumber", []exifcommon.Rational{{Numerator: 18, Denominator: 10}})
	if err != nil {
		t.Fatalf("set FNumber: %v", err)
	}

	err = exifIb.SetStandardWithName("ExposureTime", []exifcommon.Rational{{Numerator: 1, Denominator: 125}})
	if err != nil {
		t.Fatalf("set ExposureTime: %v", err)
	}

	err = exifIb.SetStandardWithName("ISOSpeedRatings", []uint16{400})
	if err != nil {
		t.Fatalf("set ISOSpeedRatings: %v", err)
	}

	gpsIb := exif.NewIfdBuilder(im, ti, exifcommon.IfdGpsInfoStandardIfdIdentity, exifcommon.TestDefaultByteOrder)

	err = gpsIb.SetStandardWithName("GPSLatitudeRef", "N")
	if err != nil {
		t.Fatalf("set GPSLatitudeRef: %v", err)
	}
	err = gpsIb.SetStandardWithName("GPSLatitude", []exifcommon.Rational{
		{Numerator: 39, Denominator: 1},
		{Numerator: 54, Denominator: 1},
		{Numerator: 1512, Denominator: 100},
	})
	if err != nil {
		t.Fatalf("set GPSLatitude: %v", err)
	}

	err = gpsIb.SetStandardWithName("GPSLongitudeRef", "E")
	if err != nil {
		t.Fatalf("set GPSLongitudeRef: %v", err)
	}
	err = gpsIb.SetStandardWithName("GPSLongitude", []exifcommon.Rational{
		{Numerator: 116, Denominator: 1},
		{Numerator: 24, Denominator: 1},
		{Numerator: 2664, Denominator: 100},
	})
	if err != nil {
		t.Fatalf("set GPSLongitude: %v", err)
	}

	err = rootIb.AddChildIb(exifIb)
	if err != nil {
		t.Fatalf("add exif child: %v", err)
	}
	err = rootIb.AddChildIb(gpsIb)
	if err != nil {
		t.Fatalf("add gps child: %v", err)
	}

	ibe := exif.NewIfdByteEncoder()
	exifBytes, err := ibe.EncodeToExif(rootIb)
	if err != nil {
		t.Fatalf("encode exif: %v", err)
	}

	var result bytes.Buffer

	result.Write([]byte{0xFF, 0xD8})

	app1Payload := append([]byte("Exif\x00\x00"), exifBytes...)
	app1Length := uint16(len(app1Payload) + 2)
	result.WriteByte(0xFF)
	result.WriteByte(0xE1)
	result.WriteByte(byte(app1Length >> 8))
	result.WriteByte(byte(app1Length & 0xFF))
	result.Write(app1Payload)

	result.Write(jpegBytes[2:])

	dir := t.TempDir()
	path := filepath.Join(dir, "test_exif.jpg")

	err = os.WriteFile(path, result.Bytes(), 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	expected := &expectedEXIF{
		DateTimeOriginal: "2024-01-15T10:30:00Z",
		CameraModel:      "TestCamera X1",
		LensModel:        "TestLens 50mm f/1.8",
		FocalLength:      "50.0mm",
		Aperture:         "f/1.8",
		ShutterSpeed:     "1/125s",
		ISO:              400,
		GPSLat:           39.9042,
		GPSLon:           116.4074,
	}

	return path, expected
}

type expectedEXIF struct {
	DateTimeOriginal string
	CameraModel      string
	LensModel        string
	FocalLength      string
	Aperture         string
	ShutterSpeed     string
	ISO              int
	GPSLat           float64
	GPSLon           float64
}
