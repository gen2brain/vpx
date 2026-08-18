package webp

import (
	"image"

	"github.com/gen2brain/vpx/vp8"
)

func premultiply(rgba []byte) {
	for i := 0; i+3 < len(rgba); i += 4 {
		a := uint32(rgba[i+3])
		if a == 0xff {
			continue
		}

		m := a * 32897
		rgba[i] = byte(uint32(rgba[i]) * m >> 23)
		rgba[i+1] = byte(uint32(rgba[i+1]) * m >> 23)
		rgba[i+2] = byte(uint32(rgba[i+2]) * m >> 23)
	}
}

func channelwiseMultiply(pix, scale uint32) uint32 {
	const mask = 0x00ff00ff

	rb := (pix & mask) * scale >> 8
	ag := (pix >> 8 & mask) * scale

	return rb&mask | ag&^mask
}

func blendPixelPremult(src, dst uint32) uint32 {
	return src + channelwiseMultiply(dst, 256-(src>>24&0xff))
}

func loadPixel(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func storePixel(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}

func blendRow(src, dst []byte, n int) {
	for i := 0; i < 4*n; i += 4 {
		if src[i+3] == 0xff {
			continue
		}

		storePixel(src[i:], blendPixelPremult(loadPixel(src[i:]), loadPixel(dst[i:])))
	}
}

func zeroRect(buf []byte, stride, x, y, w, h int) {
	for j := range h {
		clear(buf[(y+j)*stride+4*x:][:4*w])
	}
}

func (c *container) frameHasAlpha(f frame) bool {
	if f.alpha.valid() {
		return true
	}

	if !f.image.id.is(fccVP8L) {
		return false
	}

	b, err := c.src.at(f.image.off, min(f.image.size, vp8lHeaderSize))
	if err != nil {
		return false
	}

	h, err := parseVP8LHeader(b)

	return err == nil && h.hasAlpha
}

func (f frame) full(w, h int) bool { return f.w == w && f.h == h }

func (c *container) isKeyFrame(i int, f, prev frame, prevKey bool, w, h int) bool {
	if i == 0 {
		return true
	}

	if (!c.frameHasAlpha(f) || !f.blend) && f.full(w, h) {
		return true
	}

	return prev.dispose && (prev.full(w, h) || prevKey)
}

func blendRangeAtRow(src, dst frame, y int) (int, int, int, int) {
	srcMaxX := src.x + src.w
	dstMaxX := dst.x + dst.w
	dstMaxY := dst.y + dst.h

	if y < dst.y || y >= dstMaxY || src.x >= dstMaxX || srcMaxX <= dst.x {
		return src.x, src.w, -1, 0
	}

	left1, width1, left2, width2 := -1, 0, -1, 0

	if src.x < dst.x {
		left1, width1 = src.x, dst.x-src.x
	}

	if srcMaxX > dstMaxX {
		left2, width2 = dstMaxX, srcMaxX-dstMaxX
	}

	return left1, width1, left2, width2
}

type animScratch struct {
	vp8      vp8.Decoder
	lossless losslessDecoder
	rgba     []byte
	alpha    []byte
	uv       [128]byte
}

func (a *animScratch) resize(n int) {
	if cap(a.rgba) < 4*n {
		a.rgba = make([]byte, 4*n)
		a.alpha = make([]byte, n)
	}

	a.rgba = a.rgba[:4*n]
	a.alpha = a.alpha[:n]
}

func (c *container) decodeFrameRGBA(f frame, o Options, a *animScratch) ([]byte, error) {
	data, err := c.payload(f.image)
	if err != nil {
		return nil, err
	}

	a.resize(f.w * f.h)

	a.lossless.sizeLimit = o.FrameSizeLimit

	switch string(f.image.id[:]) {
	case fccVP8:
		a.vp8.Threads = o.Threads
		a.vp8.FrameSizeLimit = o.FrameSizeLimit

		pic, err := a.vp8.DecodeFrame(data)
		if err != nil {
			return nil, fromVP8(err)
		}

		if pic == nil || pic.Width != f.w || pic.Height != f.h {
			return nil, ErrInvalid
		}

		upsampleFrame(a.rgba, 4*f.w, pic, &a.uv)

		chunk, err := c.payload(f.alpha)
		if err != nil {
			return nil, err
		}

		if err := decodeAlpha(&a.lossless, chunk, a.alpha, f.w, f.w, f.h, o.AlphaDither); err != nil {
			return nil, err
		}

		for i, v := range a.alpha {
			a.rgba[4*i+3] = v
		}

		premultiply(a.rgba)

		return a.rgba, nil
	case fccVP8L:
		px, w, h, err := decodeVP8L(&a.lossless, data)
		if err != nil {
			return nil, err
		}

		if w != f.w || h != f.h {
			return nil, ErrInvalid
		}

		argbToRGBA(a.rgba, px)
		premultiply(a.rgba)

		return a.rgba, nil
	}

	return nil, ErrInvalid
}

func decodeAnimation(c *container, o Options, all bool) (*WEBP, error) {
	if len(c.frames) == 0 {
		return nil, ErrInvalid
	}

	w, h := c.width, c.height
	stride := 4 * w

	curr := make([]byte, stride*h)
	prev := make([]byte, stride*h)

	ret := &WEBP{LoopCount: c.loopCount}
	rect := image.Rect(0, 0, w, h)

	var (
		prevFrame frame
		prevKey   bool
		scratch   animScratch
	)

	for i, f := range c.frames {
		if f.x+f.w > w || f.y+f.h > h {
			return nil, ErrInvalid
		}

		key := c.isKeyFrame(i, f, prevFrame, prevKey, w, h)

		if key {
			clear(curr)
		} else {
			copy(curr, prev)
		}

		px, err := c.decodeFrameRGBA(f, o, &scratch)
		if err != nil {
			return nil, err
		}

		for y := range f.h {
			copy(curr[(f.y+y)*stride+4*f.x:][:4*f.w], px[4*y*f.w:][:4*f.w])
		}

		if i > 0 && f.blend && !key {
			blendFrame(curr, prev, stride, f, prevFrame)
		}

		img := image.NewRGBA(rect)
		copy(img.Pix, curr)

		ret.Image = append(ret.Image, img)
		ret.Delay = append(ret.Delay, f.duration)

		copy(prev, curr)

		if f.dispose {
			zeroRect(prev, stride, f.x, f.y, f.w, f.h)
		}

		prevFrame, prevKey = f, key

		if !all {
			break
		}
	}

	return ret, nil
}

func blendFrame(curr, prev []byte, stride int, f, prevFrame frame) {
	if !prevFrame.dispose {
		for y := range f.h {
			off := (f.y+y)*stride + 4*f.x
			blendRow(curr[off:], prev[off:], f.w)
		}

		return
	}

	for y := range f.h {
		canvasY := f.y + y
		left1, width1, left2, width2 := blendRangeAtRow(f, prevFrame, canvasY)

		if width1 > 0 {
			off := canvasY*stride + 4*left1
			blendRow(curr[off:], prev[off:], width1)
		}

		if width2 > 0 {
			off := canvasY*stride + 4*left2
			blendRow(curr[off:], prev[off:], width2)
		}
	}
}
