package webp

import (
	"image"
	"sync"

	"github.com/gen2brain/vpx/vp8"
)

// still is the working set one still decode needs, pooled because none of it
// survives the call: the planes and the lossless buffers are copied into the
// image that is returned.
type still struct {
	vp8      vp8.Decoder
	lossless losslessDecoder
}

var stillPool sync.Pool

func getStill() *still {
	if s, ok := stillPool.Get().(*still); ok {
		return s
	}

	return new(still)
}

func putStill(s *still) {
	s.vp8.Release()
	s.lossless.release()

	stillPool.Put(s)
}

type output int

const (
	outNative output = iota
	outRGBA
	outYCbCr
)

func (o Options) output() output {
	switch {
	case o.ToRGBA:
		return outRGBA
	case o.ToYCbCr:
		return outYCbCr
	}

	return outNative
}

func decode(s *source, o Options, all bool) (*WEBP, error) {
	c, err := parse(s)
	if err != nil {
		return nil, err
	}

	limit := uint64(maxStillArea)
	if c.animated() {
		limit = maxCanvasArea
	}

	if uint64(c.width)*uint64(c.height) > limit {
		return nil, ErrUnsupported
	}

	var ret *WEBP

	if c.animated() {
		ret, err = decodeAnimation(c, o, all)
	} else {
		var img image.Image

		if img, err = c.decodeStill(o); err == nil {
			ret = &WEBP{Image: []image.Image{img}, Delay: []int{}}
		}
	}

	if err != nil {
		return nil, err
	}

	if o.AutoRotate {
		exif, err := c.payload(c.exif)
		if err != nil {
			return nil, err
		}

		if orientation := tiffOrientation(exif); orientation > 1 {
			for i := range ret.Image {
				ret.Image[i] = applyOrientation(ret.Image[i], orientation)
			}
		}
	}

	return ret, nil
}

func (c *container) decodeStill(o Options) (image.Image, error) {
	data, err := c.payload(c.image)
	if err != nil {
		return nil, err
	}

	alpha, err := c.payload(c.alpha)
	if err != nil {
		return nil, err
	}

	switch string(c.image.id[:]) {
	case fccVP8:
		return decodeLossy(data, alpha, o)
	case fccVP8L:
		return decodeLossless(data, o)
	}

	return nil, ErrInvalid
}

func copyPlane(dst []byte, dstStride int, src []byte, srcStride, w, h int) {
	for y := range h {
		copy(dst[y*dstStride:y*dstStride+w], src[y*srcStride:y*srcStride+w])
	}
}

func decodeLossy(data, alpha []byte, o Options) (image.Image, error) {
	s := getStill()
	defer putStill(s)

	pic, err := s.vp8.DecodeFrame(data)
	if err != nil {
		return nil, err
	}

	if o.output() == outRGBA {
		img := image.NewRGBA(image.Rect(0, 0, pic.Width, pic.Height))

		upsampleFrame(img.Pix, img.Stride, pic)

		if err := applyAlphaRGBA(&s.lossless, img, alpha, o.AlphaDither); err != nil {
			return nil, err
		}

		return img, nil
	}

	img := image.NewNYCbCrA(image.Rect(0, 0, pic.Width, pic.Height), image.YCbCrSubsampleRatio420)

	copyPlane(img.Y, img.YStride, pic.Y, pic.YStride, pic.Width, pic.Height)

	cw, ch := (pic.Width+1)/2, (pic.Height+1)/2
	copyPlane(img.Cb, img.CStride, pic.U, pic.UVStride, cw, ch)
	copyPlane(img.Cr, img.CStride, pic.V, pic.UVStride, cw, ch)

	if err := decodeAlpha(&s.lossless, alpha, img.A, img.AStride, pic.Width, pic.Height, o.AlphaDither); err != nil {
		return nil, err
	}

	return img, nil
}

func applyAlphaRGBA(ll *losslessDecoder, img *image.RGBA, alpha []byte, dither int) error {
	w, h := img.Rect.Dx(), img.Rect.Dy()

	plane := make([]byte, w*h)
	if err := decodeAlpha(ll, alpha, plane, w, w, h, dither); err != nil {
		return err
	}

	for y := range h {
		row := img.Pix[y*img.Stride:]
		src := plane[y*w : (y+1)*w]

		for x, a := range src {
			row[4*x+3] = a
		}

		premultiply(row[:4*w])
	}

	return nil
}

func decodeLossless(data []byte, o Options) (image.Image, error) {
	s := getStill()
	defer putStill(s)

	px, w, h, err := decodeVP8L(&s.lossless, data)
	if err != nil {
		return nil, err
	}

	switch o.output() {
	case outRGBA:
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		argbToRGBA(img.Pix, px)
		premultiply(img.Pix)

		return img, nil
	case outYCbCr:
		img := image.NewNYCbCrA(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
		argbToYUVA(px, w, h, img)

		return img, nil
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	argbToRGBA(img.Pix, px)

	return img, nil
}

func argbToRGBA(dst []byte, px []uint32) {
	for i, argb := range px {
		p := dst[4*i : 4*i+4 : 4*i+4]
		p[0] = uint8(argb >> 16)
		p[1] = uint8(argb >> 8)
		p[2] = uint8(argb)
		p[3] = uint8(argb >> 24)
	}
}
