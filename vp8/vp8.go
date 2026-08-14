/*
Package vp8 decodes and encodes the VP8 bitstream of RFC 6386.

Only key frames are handled. They are what the WebP still-image format carries,
and a VP8 key frame is self contained: it references no other frame and needs no
reference buffers.

The bitstream is 4:2:0 YUV, so [Picture] hands back the planes as they were
decoded. Conversion to RGB belongs to the caller; the webp package does it.
*/
package vp8

import "errors"

var (
	ErrInvalid     = errors.New("vp8: invalid bitstream")
	ErrUnsupported = errors.New("vp8: unsupported feature")
)

// DefaultFrameSizeLimit bounds the pixel area a frame header may ask to
// allocate.
const DefaultFrameSizeLimit = 1 << 28

// Picture is a decoded frame in 4:2:0. The planes are allocated at their own
// strides, which are macroblock aligned and wider than the visible image.
type Picture struct {
	Y, U, V  []byte
	YStride  int
	UVStride int
	Width    int
	Height   int
}
