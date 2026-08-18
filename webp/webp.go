/*
Package webp decodes and encodes WebP images.

	img, err := webp.Decode(r)

[Decode] returns the planes the bitstream carries: *[image.NYCbCrA] in 4:2:0
for a lossy image and *[image.NRGBA] for a lossless one. An animation frame is
composited and comes back as *[image.RGBA]. The package registers itself with
[image.RegisterFormat], so [image.Decode] works once it is imported for side
effects. [DecodeAll] returns every frame of an animation with its delay, and
[DecodeConfig] reports the dimensions without decoding pixels.

Returning *[image.NRGBA] for a lossless still is the one deliberate divergence
from the package this replaces, which converts everything to *[image.NYCbCrA].
That conversion loses data; [Options.ToYCbCr] asks for it when parity matters.

A malformed file gives [ErrInvalid] and a well formed one this package cannot
decode gives [ErrUnsupported], including a bitstream error the vp8 package
raised. A caller with another decoder to fall back on wants to tell those apart.

A reader that is also an [io.ReaderAt] and an [io.Seeker], such as an *[os.File],
is addressed by range rather than read into memory: [DecodeConfig] on a 47 KB
file touches 46 bytes of it, and [Decode] never reads metadata it does not need.

# Options

[Options] are passed to any of the decode and encode functions and default to
zero:

	img, err := webp.Decode(r, webp.Options{
		ToRGBA:     true, // premultiplied RGBA instead of the native planes
		AutoRotate: true, // apply the EXIF orientation
	})

[Options.ToRGBA] forces *[image.RGBA] with premultiplied alpha and
[Options.ToYCbCr] forces *[image.NYCbCrA], whichever the image is natively.
[Options.AlphaDither] smooths an alpha plane whose level count the encoder
reduced, which libwebp's decoder only does when asked, and so does this one.
[Options.Threads] bounds the goroutines a lossy frame is coded over; the output
is identical either way, and a lossless image is one bitstream and ignores it.
[Options.FrameSizeLimit] bounds the pixel area a header may ask to allocate, so
that a hostile file cannot exhaust memory, and gives [ErrUnsupported] above it.

# Metadata

[DecodeExif] reads the EXIF of a file without decoding its pixels and returns
an [Exif] with the orientation, the camera and lens fields, and the GPS
position. [Options.AutoRotate] applies the orientation to the decoded image.

# Encoding

	err := webp.Encode(w, img, webp.Options{Quality: 90})

[Encode] writes a lossy image by default and a lossless one with
[Options.Lossless], which is exact. [EncodeAll] writes an animation from a
[WEBP]. Alpha is carried in both modes, losslessly in both.

[Options.Method] trades speed for size in the range [0,6]. On the lossy side
subblock intra prediction comes in at 3, and rate-distortion refinement of the
whole-macroblock modes, the chroma mode and trellis quantization at 5. On the
lossless side 0 is a single fast pass and the cross-colour transform comes in
at 6. [Options.Exact] preserves the RGB of fully transparent pixels, which an
encoder is otherwise free to rewrite.

[Options.Quality] and Method map onto different quantizers in every encoder, so
comparing file sizes at a matched setting compares different operating points.
*/
package webp

import (
	"errors"
	"image"
	"image/color"
	"io"

	"github.com/gen2brain/vpx/vp8"
)

// ErrEncode is returned when an image cannot be encoded.
var ErrEncode = errors.New("webp: encode failed")

// DefaultQuality is the default quality encoding parameter.
const DefaultQuality = 75

// DefaultMethod is the default method encoding parameter.
const DefaultMethod = 4

// DefaultFrameSizeLimit is the image area, in pixels, accepted when
// [Options.FrameSizeLimit] is zero. It is the largest picture VP8 and VP8L can
// describe, so it refuses nothing a valid file asks for.
const DefaultFrameSizeLimit = vp8.DefaultFrameSizeLimit

// WEBP represents the possibly multiple images stored in a WebP file.
type WEBP struct {
	// Decoded images.
	Image []image.Image
	// Delay times, one per frame, in milliseconds.
	Delay []int
	// LoopCount is the number of times the animation repeats (0 = infinite).
	LoopCount int
}

// Options are the decoding and encoding parameters. The zero value is the
// default for both; a field a call does not apply to is ignored.
type Options struct {
	// Quality in the range [0,100]. Default is 75.
	Quality int
	// Lossless enables lossless compression. Lossless ignores quality.
	Lossless bool
	// Method is the quality/speed trade-off in the range [0,6]. Default is 4.
	Method int
	// Exact preserves the RGB of fully transparent pixels, which an encoder
	// is otherwise free to rewrite.
	Exact bool
	// AutoRotate applies the EXIF orientation to the decoded image (Decode/DecodeAll only).
	AutoRotate bool
	// AlphaDither smooths an alpha plane the encoder reduced the level count
	// of, in the range [0,100]. Default is 0, which is off.
	AlphaDither int
	// ToRGBA forces *image.RGBA output, with premultiplied alpha, instead of
	// the image's native color space. It takes precedence over ToYCbCr.
	ToRGBA bool
	// ToYCbCr forces *image.NYCbCrA output, which a lossy image is already, by
	// converting a lossless one to YUV 4:2:0. That conversion is lossy, and is
	// what libwebp's MODE_YUVA does.
	ToYCbCr bool
	// Threads bounds the goroutines a lossy frame is coded over. Zero means
	// GOMAXPROCS, one runs serially. Output is identical either way. A
	// lossless image is one bitstream and ignores it.
	Threads int
	// FrameSizeLimit bounds a frame's area in pixels, so that a corrupt or
	// hostile header cannot ask for an unbounded allocation. Zero means
	// [DefaultFrameSizeLimit], and a quarter of it for an animation, which
	// composites several canvases at once; a negative value removes the limit.
	// Decoding only.
	FrameSizeLimit int
}

type features struct {
	width, height int
	hasAlpha      bool
	hasAnimation  bool
}

func (c *container) features() (features, error) {
	f := features{
		width:        c.width,
		height:       c.height,
		hasAlpha:     c.hasAlpha(),
		hasAnimation: c.animated(),
	}

	if c.extended {
		return f, nil
	}

	b, err := c.src.header(c.image.off, min(c.image.size, bitstreamHeader))
	if err != nil {
		return f, err
	}

	switch string(c.image.id[:]) {
	case fccVP8:
		h, err := vp8.ParseFrameHeader(b)
		if err != nil {
			return f, fromVP8(err)
		}

		if !h.KeyFrame {
			return f, ErrUnsupported
		}

		f.width, f.height = h.Width, h.Height
	case fccVP8L:
		h, err := parseVP8LHeader(b)
		if err != nil {
			return f, err
		}

		f.width, f.height, f.hasAlpha = h.width, h.height, h.hasAlpha
	default:
		return f, ErrInvalid
	}

	return f, nil
}

// Decode reads a WebP image; pass Options{AutoRotate: true} to apply the EXIF orientation.
func Decode(r io.Reader, opts ...Options) (image.Image, error) {
	s, err := srcFor(r)
	if err != nil {
		return nil, err
	}

	ret, err := decode(s, options(opts), false)
	if err != nil {
		return nil, err
	}

	return ret.Image[0], nil
}

// DecodeConfig returns the color model and dimensions of a WebP image without decoding the entire image.
func DecodeConfig(r io.Reader) (image.Config, error) {
	s, err := srcFor(r)
	if err != nil {
		return image.Config{}, err
	}

	c, err := parse(s)
	if err != nil {
		return image.Config{}, err
	}

	f, err := c.features()
	if err != nil {
		return image.Config{}, err
	}

	cfg := image.Config{
		Width:      f.width,
		Height:     f.height,
		ColorModel: color.NYCbCrAModel,
	}

	if f.hasAnimation {
		cfg.ColorModel = color.RGBAModel
	}

	return cfg, nil
}

// DecodeAll returns the sequential frames and timing; pass Options{AutoRotate: true} to orient each frame.
func DecodeAll(r io.Reader, opts ...Options) (*WEBP, error) {
	s, err := srcFor(r)
	if err != nil {
		return nil, err
	}

	return decode(s, options(opts), true)
}

// Encode writes the image m to w with the given options.
func Encode(w io.Writer, m image.Image, o ...Options) error {
	return encode(w, m, options(o))
}

// EncodeAll writes the animation anim to w; all frames must share the same bounds.
func EncodeAll(w io.Writer, anim *WEBP, o ...Options) error {
	if anim == nil || len(anim.Image) == 0 {
		return ErrEncode
	}

	b := anim.Image[0].Bounds()
	for _, img := range anim.Image {
		if img.Bounds().Dx() != b.Dx() || img.Bounds().Dy() != b.Dy() {
			return ErrEncode
		}
	}

	return encodeAll(w, anim, options(o))
}

func options(opts []Options) Options {
	o := Options{Quality: DefaultQuality, Method: DefaultMethod}

	if len(opts) == 0 {
		return o
	}

	o = opts[0]

	if o.Quality <= 0 {
		o.Quality = DefaultQuality
	} else if o.Quality > 100 {
		o.Quality = 100
	}

	if o.Method < 0 {
		o.Method = DefaultMethod
	} else if o.Method > 6 {
		o.Method = 6
	}

	return o
}

func init() {
	decodeWrapper := func(r io.Reader) (image.Image, error) {
		return Decode(r)
	}

	image.RegisterFormat("webp", "RIFF????WEBPVP8", decodeWrapper, DecodeConfig)
}
