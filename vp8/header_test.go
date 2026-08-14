package vp8

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// keyFrame builds the uncompressed part of a key frame header.
func keyFrame(profile, partSize, width, height, xscale, yscale int, show bool) []byte {
	bits := uint32(partSize)<<5 | uint32(profile)<<1

	if show {
		bits |= 1 << 4
	}

	return []byte{
		byte(bits), byte(bits >> 8), byte(bits >> 16),
		startCode[0], startCode[1], startCode[2],
		byte(width), byte(width>>8) | byte(xscale)<<6,
		byte(height), byte(height>>8) | byte(yscale)<<6,
	}
}

func TestParseFrameHeader(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		xscale int
		yscale int
	}{
		{"tiny", 1, 1, 0, 0},
		{"odd", 129, 131, 0, 0},
		{"typical", 512, 512, 0, 0},
		{"max", maxDimension - 1, maxDimension - 1, 0, 0},
		{"scaled", 320, 240, 3, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := ParseFrameHeader(keyFrame(0, 42, tt.width, tt.height, tt.xscale, tt.yscale, true))
			if err != nil {
				t.Fatal(err)
			}

			if !h.KeyFrame || !h.Show {
				t.Errorf("key %v show %v, want both true", h.KeyFrame, h.Show)
			}

			if h.PartSize != 42 {
				t.Errorf("partition size = %d, want 42", h.PartSize)
			}

			if h.Width != tt.width || h.Height != tt.height {
				t.Errorf("size = %dx%d, want %dx%d", h.Width, h.Height, tt.width, tt.height)
			}

			if h.XScale != tt.xscale || h.YScale != tt.yscale {
				t.Errorf("scale = %d,%d, want %d,%d", h.XScale, h.YScale, tt.xscale, tt.yscale)
			}

			if h.size() != frameTagSize+keyFrameHdrSize {
				t.Errorf("size = %d, want %d", h.size(), frameTagSize+keyFrameHdrSize)
			}
		})
	}
}

func TestParseFrameHeaderProfile(t *testing.T) {
	for profile := range 4 {
		h, err := ParseFrameHeader(keyFrame(profile, 1, 16, 16, 0, 0, true))
		if err != nil {
			t.Fatal(err)
		}

		if h.Profile != profile {
			t.Errorf("profile = %d, want %d", h.Profile, profile)
		}
	}
}

func TestParseFrameHeaderInvalid(t *testing.T) {
	good := keyFrame(0, 42, 16, 16, 0, 0, true)

	badStart := append([]byte{}, good...)
	badStart[3] ^= 0xff

	zeroWidth := keyFrame(0, 42, 0, 16, 0, 0, true)
	zeroHeight := keyFrame(0, 42, 16, 0, 0, 0, true)

	// The profile field is three bits but only four values are defined, so the
	// top bit set is a bitstream error rather than a feature.
	badProfile := keyFrame(4, 42, 16, 16, 0, 0, true)

	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", nil, ErrInvalid},
		{"tag only", good[:frameTagSize], ErrInvalid},
		{"truncated picture header", good[:frameTagSize+3], ErrInvalid},
		{"bad start code", badStart, ErrInvalid},
		{"zero width", zeroWidth, ErrInvalid},
		{"zero height", zeroHeight, ErrInvalid},
		{"bad profile", badProfile, ErrInvalid},
		{"inter frame", interFrame(), ErrUnsupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseFrameHeader(tt.data); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func FuzzDecodeFrame(f *testing.F) {
	f.Add(keyFrame(0, 42, 16, 16, 0, 0, true))
	f.Add(keyFrame(3, 1, 129, 131, 1, 1, true))
	f.Add(interFrame())

	for _, name := range fuzzSeeds(f) {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}

		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		ParseFrameHeader(b)

		d := Decoder{SizeLimit: 1 << 20}
		d.DecodeFrame(b)
	})
}

func fuzzSeeds(f *testing.F) []string {
	names, err := filepath.Glob(filepath.Join("testdata", "*.vp8"))
	if err != nil {
		f.Fatal(err)
	}

	return names
}

func interFrame() []byte {
	b := keyFrame(0, 42, 16, 16, 0, 0, true)
	b[0] |= 1

	return b
}
