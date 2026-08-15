package webp

import (
	"errors"
	"fmt"
	"image"
	"io"
)

var ErrNoExif = errors.New("webp: no exif data")

type Exif struct {
	Orientation int // EXIF orientation (1-8). 1 = normal, values 2-8 indicate rotation/flip.
	Width       int // Image width in pixels.
	Height      int // Image height in pixels.

	Make     string // Camera manufacturer (e.g., "Canon").
	Model    string // Camera model (e.g., "Canon EOS 5D Mark III").
	Software string // Software used to process/create the image.

	DateTime         string // File modification date/time.
	DateTimeOriginal string // Original capture date/time (when photo was taken).

	ExposureTime float64 // Shutter speed in seconds (e.g., 0.004 = 1/250s).
	FNumber      float64 // Aperture f-number (e.g., 5.6 = f/5.6).
	ISOSpeed     int     // ISO speed rating (e.g., 800).
	FocalLength  float64 // Lens focal length in millimeters.
	Flash        int     // Flash mode/status (0 = no flash, non-zero = flash fired).

	GPSLatitude  float64 // Latitude in decimal degrees (positive = North, negative = South).
	GPSLongitude float64 // Longitude in decimal degrees (positive = East, negative = West).
	GPSAltitude  float64 // Altitude in meters above sea level.

	Copyright string // Copyright notice.
	Artist    string // Creator/photographer name.
}

func DecodeExif(r io.Reader) (*Exif, error) {
	s, err := srcFor(r)
	if err != nil {
		return nil, err
	}

	c, err := parse(s)
	if err != nil {
		return nil, ErrNoExif
	}

	tiff, err := c.payload(c.exif)
	if err != nil || tiff == nil {
		return nil, ErrNoExif
	}

	exif := &Exif{Orientation: 1}
	if err := parseExifData(stripExifPrefix(tiff), exif); err != nil {
		return nil, fmt.Errorf("webp: %w", err)
	}

	return exif, nil
}

func stripExifPrefix(b []byte) []byte {
	if len(b) >= 6 && string(b[0:4]) == "Exif" && b[4] == 0 && b[5] == 0 {
		return b[6:]
	}

	return b
}

func tiffOrientation(payload []byte) int {
	if len(payload) == 0 {
		return 1
	}

	exif := &Exif{Orientation: 1}
	if parseExifData(stripExifPrefix(payload), exif) != nil || exif.Orientation < 1 || exif.Orientation > 8 {
		return 1
	}

	return exif.Orientation
}

func orientTarget(orientation, sx, sy, sw, sh int) (int, int) {
	switch orientation {
	case 2: // flip horizontal
		return sw - 1 - sx, sy
	case 3: // rotate 180
		return sw - 1 - sx, sh - 1 - sy
	case 4: // flip vertical
		return sx, sh - 1 - sy
	case 5: // transpose
		return sy, sx
	case 6: // rotate 90 CW
		return sh - 1 - sy, sx
	case 7: // transverse
		return sh - 1 - sy, sw - 1 - sx
	case 8: // rotate 270 CW
		return sy, sw - 1 - sx
	}

	return sx, sy
}

func applyOrientation(img image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return img
	}

	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dw, dh := sw, sh
	if orientation >= 5 {
		dw, dh = sh, sw // 90/270 rotations swap width and height
	}

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))

	if src, ok := img.(*image.RGBA); ok {
		for sy := 0; sy < sh; sy++ {
			srow := src.Pix[(b.Min.Y+sy)*src.Stride+b.Min.X*4:]
			for sx := 0; sx < sw; sx++ {
				dx, dy := orientTarget(orientation, sx, sy, sw, sh)
				si := sx * 4
				di := dy*dst.Stride + dx*4
				copy(dst.Pix[di:di+4], srow[si:si+4])
			}
		}

		return dst
	}

	for sy := 0; sy < sh; sy++ {
		for sx := 0; sx < sw; sx++ {
			dx, dy := orientTarget(orientation, sx, sy, sw, sh)
			dst.Set(dx, dy, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}

	return dst
}

const (
	tagOrientation    = 0x0112
	tagImageWidth     = 0x0100
	tagImageLength    = 0x0101
	tagMake           = 0x010F
	tagModel          = 0x0110
	tagSoftware       = 0x0131
	tagDateTime       = 0x0132
	tagArtist         = 0x013B
	tagCopyright      = 0x8298
	tagExifIFDPointer = 0x8769
	tagGPSIFDPointer  = 0x8825

	tagExposureTime     = 0x829A
	tagFNumber          = 0x829D
	tagISOSpeedRatings  = 0x8827
	tagDateTimeOriginal = 0x9003
	tagFlash            = 0x9209
	tagFocalLength      = 0x920A

	tagGPSLatitudeRef  = 0x0001
	tagGPSLatitude     = 0x0002
	tagGPSLongitudeRef = 0x0003
	tagGPSLongitude    = 0x0004
	tagGPSAltitudeRef  = 0x0005
	tagGPSAltitude     = 0x0006
)

const (
	typeUnsignedByte     = 1
	typeASCIIString      = 2
	typeUnsignedShort    = 3
	typeUnsignedLong     = 4
	typeUnsignedRational = 5
	typeSignedByte       = 6
	typeUndefined        = 7
	typeSignedShort      = 8
	typeSignedLong       = 9
	typeSignedRational   = 10
	typeSingleFloat      = 11
	typeDoubleFloat      = 12
)

type exifReader struct {
	data         []byte
	littleEndian bool
}

func (r *exifReader) uint16(offset int) uint16 {
	if offset+1 >= len(r.data) {
		return 0
	}
	if r.littleEndian {
		return uint16(r.data[offset]) | (uint16(r.data[offset+1]) << 8)
	}
	return (uint16(r.data[offset]) << 8) | uint16(r.data[offset+1])
}

func (r *exifReader) uint32(offset int) uint32 {
	if offset+3 >= len(r.data) {
		return 0
	}
	if r.littleEndian {
		return uint32(r.data[offset]) | (uint32(r.data[offset+1]) << 8) |
			(uint32(r.data[offset+2]) << 16) | (uint32(r.data[offset+3]) << 24)
	}
	return (uint32(r.data[offset]) << 24) | (uint32(r.data[offset+1]) << 16) |
		(uint32(r.data[offset+2]) << 8) | uint32(r.data[offset+3])
}

func (r *exifReader) readString(offset, maxLen int) string {
	if offset >= len(r.data) {
		return ""
	}
	end := offset
	for end < len(r.data) && end < offset+maxLen && r.data[end] != 0 {
		end++
	}
	return string(r.data[offset:end])
}

func (r *exifReader) readRational(offset int) float64 {
	if offset+7 >= len(r.data) {
		return 0
	}
	numerator := r.uint32(offset)
	denominator := r.uint32(offset + 4)
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func parseExifData(data []byte, exif *Exif) error {
	if len(data) < 8 {
		return fmt.Errorf("EXIF data too short")
	}

	reader := &exifReader{data: data}

	if data[0] == 0x49 && data[1] == 0x49 {
		reader.littleEndian = true // Intel (little-endian)
	} else if data[0] == 0x4D && data[1] == 0x4D {
		reader.littleEndian = false // Motorola (big-endian)
	} else {
		return fmt.Errorf("invalid EXIF byte order marker")
	}

	if reader.uint16(2) != 42 {
		return fmt.Errorf("invalid EXIF magic number")
	}

	ifdOffset := reader.uint32(4)
	if ifdOffset < 8 || int(ifdOffset) >= len(data) {
		return fmt.Errorf("invalid IFD offset")
	}

	exifIFDOffset, gpsIFDOffset := parseIFD(reader, int(ifdOffset), exif)

	if exifIFDOffset > 0 {
		parseExifSubIFD(reader, exifIFDOffset, exif)
	}

	if gpsIFDOffset > 0 {
		parseGPSSubIFD(reader, gpsIFDOffset, exif)
	}

	return nil
}

func parseIFD(reader *exifReader, offset int, exif *Exif) (exifIFDOffset, gpsIFDOffset int) {
	if offset+1 >= len(reader.data) {
		return 0, 0
	}

	numEntries := reader.uint16(offset)
	offset += 2

	for i := 0; i < int(numEntries); i++ {
		entryOffset := offset + i*12
		if entryOffset+11 >= len(reader.data) {
			break
		}

		tag := reader.uint16(entryOffset)
		dataType := reader.uint16(entryOffset + 2)
		count := reader.uint32(entryOffset + 4)
		valueOffset := entryOffset + 8

		dataSize := getDataSize(dataType, count)
		if dataSize > 4 {
			valueOffset = int(reader.uint32(valueOffset))
			if valueOffset >= len(reader.data) {
				continue
			}
		}

		switch tag {
		case tagOrientation:
			if dataType == typeUnsignedShort {
				exif.Orientation = int(reader.uint16(valueOffset))
			}
		case tagImageWidth:
			if dataType == typeUnsignedShort {
				exif.Width = int(reader.uint16(valueOffset))
			} else if dataType == typeUnsignedLong {
				exif.Width = int(reader.uint32(valueOffset))
			}
		case tagImageLength:
			if dataType == typeUnsignedShort {
				exif.Height = int(reader.uint16(valueOffset))
			} else if dataType == typeUnsignedLong {
				exif.Height = int(reader.uint32(valueOffset))
			}
		case tagMake:
			if dataType == typeASCIIString {
				exif.Make = reader.readString(valueOffset, int(count))
			}
		case tagModel:
			if dataType == typeASCIIString {
				exif.Model = reader.readString(valueOffset, int(count))
			}
		case tagSoftware:
			if dataType == typeASCIIString {
				exif.Software = reader.readString(valueOffset, int(count))
			}
		case tagDateTime:
			if dataType == typeASCIIString {
				exif.DateTime = reader.readString(valueOffset, int(count))
			}
		case tagArtist:
			if dataType == typeASCIIString {
				exif.Artist = reader.readString(valueOffset, int(count))
			}
		case tagCopyright:
			if dataType == typeASCIIString {
				exif.Copyright = reader.readString(valueOffset, int(count))
			}
		case tagExifIFDPointer:
			if dataType == typeUnsignedLong {
				exifIFDOffset = int(reader.uint32(valueOffset))
			}
		case tagGPSIFDPointer:
			if dataType == typeUnsignedLong {
				gpsIFDOffset = int(reader.uint32(valueOffset))
			}
		}
	}

	return exifIFDOffset, gpsIFDOffset
}

func parseExifSubIFD(reader *exifReader, offset int, exif *Exif) {
	if offset+1 >= len(reader.data) {
		return
	}

	numEntries := reader.uint16(offset)
	offset += 2

	for i := 0; i < int(numEntries); i++ {
		entryOffset := offset + i*12
		if entryOffset+11 >= len(reader.data) {
			break
		}

		tag := reader.uint16(entryOffset)
		dataType := reader.uint16(entryOffset + 2)
		count := reader.uint32(entryOffset + 4)
		valueOffset := entryOffset + 8

		dataSize := getDataSize(dataType, count)
		if dataSize > 4 {
			valueOffset = int(reader.uint32(valueOffset))
			if valueOffset >= len(reader.data) {
				continue
			}
		}

		switch tag {
		case tagExposureTime:
			if dataType == typeUnsignedRational {
				exif.ExposureTime = reader.readRational(valueOffset)
			}
		case tagFNumber:
			if dataType == typeUnsignedRational {
				exif.FNumber = reader.readRational(valueOffset)
			}
		case tagISOSpeedRatings:
			if dataType == typeUnsignedShort {
				exif.ISOSpeed = int(reader.uint16(valueOffset))
			}
		case tagDateTimeOriginal:
			if dataType == typeASCIIString {
				exif.DateTimeOriginal = reader.readString(valueOffset, int(count))
			}
		case tagFlash:
			if dataType == typeUnsignedShort {
				exif.Flash = int(reader.uint16(valueOffset))
			}
		case tagFocalLength:
			if dataType == typeUnsignedRational {
				exif.FocalLength = reader.readRational(valueOffset)
			}
		}
	}
}

func parseGPSSubIFD(reader *exifReader, offset int, exif *Exif) {
	if offset+1 >= len(reader.data) {
		return
	}

	numEntries := reader.uint16(offset)
	offset += 2

	var latRef, lonRef string
	var latValues, lonValues []float64

	for i := 0; i < int(numEntries); i++ {
		entryOffset := offset + i*12
		if entryOffset+11 >= len(reader.data) {
			break
		}

		tag := reader.uint16(entryOffset)
		dataType := reader.uint16(entryOffset + 2)
		count := reader.uint32(entryOffset + 4)
		valueOffset := entryOffset + 8

		dataSize := getDataSize(dataType, count)
		if dataSize > 4 {
			valueOffset = int(reader.uint32(valueOffset))
			if valueOffset >= len(reader.data) {
				continue
			}
		}

		switch tag {
		case tagGPSLatitudeRef:
			if dataType == typeASCIIString {
				latRef = reader.readString(valueOffset, 2)
			}
		case tagGPSLatitude:
			if dataType == typeUnsignedRational && count == 3 {
				latValues = []float64{
					reader.readRational(valueOffset),
					reader.readRational(valueOffset + 8),
					reader.readRational(valueOffset + 16),
				}
			}
		case tagGPSLongitudeRef:
			if dataType == typeASCIIString {
				lonRef = reader.readString(valueOffset, 2)
			}
		case tagGPSLongitude:
			if dataType == typeUnsignedRational && count == 3 {
				lonValues = []float64{
					reader.readRational(valueOffset),
					reader.readRational(valueOffset + 8),
					reader.readRational(valueOffset + 16),
				}
			}
		case tagGPSAltitude:
			if dataType == typeUnsignedRational {
				alt := reader.readRational(valueOffset)
				if entryOffset+12 < len(reader.data) {
					altRef := reader.data[valueOffset-4] // Previous tag should be altitude ref
					if altRef == 1 {
						alt = -alt
					}
				}
				exif.GPSAltitude = alt
			}
		}
	}

	if len(latValues) == 3 {
		exif.GPSLatitude = latValues[0] + latValues[1]/60.0 + latValues[2]/3600.0
		if latRef == "S" {
			exif.GPSLatitude = -exif.GPSLatitude
		}
	}
	if len(lonValues) == 3 {
		exif.GPSLongitude = lonValues[0] + lonValues[1]/60.0 + lonValues[2]/3600.0
		if lonRef == "W" {
			exif.GPSLongitude = -exif.GPSLongitude
		}
	}
}

func getDataSize(dataType uint16, count uint32) int {
	var componentSize int
	switch dataType {
	case typeUnsignedByte, typeSignedByte, typeASCIIString, typeUndefined:
		componentSize = 1
	case typeUnsignedShort, typeSignedShort:
		componentSize = 2
	case typeUnsignedLong, typeSignedLong, typeSingleFloat:
		componentSize = 4
	case typeUnsignedRational, typeSignedRational, typeDoubleFloat:
		componentSize = 8
	default:
		componentSize = 1
	}
	return componentSize * int(count)
}
