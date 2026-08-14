/*
Package vp8 decodes and encodes the VP8 bitstream of RFC 6386.

The decoder handles key and inter frames. The encoder writes key frames.

The bitstream is 4:2:0 YUV, so [Picture] hands back the planes as they were
decoded. Conversion to RGB belongs to the caller; the webp package does it.
*/
package vp8

import (
	"errors"
	"math"
)

var (
	ErrInvalid     = errors.New("vp8: invalid bitstream")
	ErrUnsupported = errors.New("vp8: unsupported feature")
)

// DefaultFrameSizeLimit bounds the pixel area a frame header may ask to
// allocate.
const DefaultFrameSizeLimit = min(1<<28, math.MaxInt>>6)

// Picture is a decoded frame in 4:2:0. The planes are allocated at their own
// strides, which are macroblock aligned and wider than the visible image.
type Picture struct {
	Y, U, V  []byte
	YStride  int
	UVStride int
	Width    int
	Height   int
}
