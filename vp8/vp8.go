/*
Package vp8 decodes and encodes the VP8 bitstream of RFC 6386.

	d := vp8.Decoder{}

	pic, err := d.DecodeFrame(data)

[Decoder.DecodeFrame] takes one frame with its uncompressed header and returns
the reconstruction. It handles key and inter frames, so a whole video decodes a
frame at a time; the webp package only ever hands it a key frame. A frame the
stream marks as not shown updates the reference buffers and returns a nil
picture with a nil error. Reusing a [Decoder] across frames reuses its buffers
and is what carries the references from one frame to the next.

The bitstream is 4:2:0 YUV, so [Picture] hands back the planes as they were
decoded, at macroblock-aligned strides that are wider than the visible image.
Conversion to RGB belongs to the caller; the webp package does it.

A malformed frame gives [ErrInvalid] and a well formed one this package cannot
decode gives [ErrUnsupported]. Nothing panics on untrusted input, and
[Decoder.FrameSizeLimit] bounds the pixel area a frame header may ask to allocate so
that a hostile file cannot exhaust memory.

# Encoding

	e := vp8.Encoder{}

	key, err := e.Encode(pic, vp8.EncodeOptions{Quality: 90})
	next, err := e.EncodeInter(pic2, vp8.EncodeOptions{Quality: 90})

[Encoder.Encode] writes a key frame and refreshes every reference buffer, so it
begins a group of pictures. [Encoder.EncodeInter] writes a frame predicted from
the one encoded before it, and fails with [ErrInvalid] before any key frame or
at a changed size. The caller drives the group structure; the encoder never
inserts a key frame on its own.

[EncodeOptions.Method] trades speed for size. Subblock intra prediction comes in
at 3, and rate-distortion refinement of the whole-macroblock modes, the chroma
mode and trellis quantization at 5. [EncodeOptions.Quality] and Method map onto
different quantizers in every encoder, so comparing sizes at a matched setting
compares different operating points.

The encoder reconstructs by driving a [Decoder], so the two cannot disagree
about what a frame decodes to.
*/
package vp8

import (
	"errors"
	"math"
)

var (
	// ErrInvalid is returned for a malformed bitstream.
	ErrInvalid = errors.New("vp8: invalid bitstream")
	// ErrUnsupported is returned for a well formed bitstream this package
	// cannot decode, or a frame it cannot encode.
	ErrUnsupported = errors.New("vp8: unsupported feature")
)

// DefaultFrameSizeLimit bounds the pixel area a frame header may ask to
// allocate.
const DefaultFrameSizeLimit = min(1<<28, math.MaxInt>>6)

// Picture is a decoded frame in 4:2:0. The planes are allocated at their own
// strides, which are macroblock aligned and wider than the visible image.
type Picture struct {
	Y, U, V  []byte // Luma and chroma planes, the chroma at half resolution.
	YStride  int    // Bytes between luma rows.
	UVStride int    // Bytes between chroma rows.
	Width    int    // Visible width in pixels.
	Height   int    // Visible height in pixels.
}
