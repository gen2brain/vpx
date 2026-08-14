package webp

import (
	"image"
	"image/color"
	"io"
	"sync"

	"github.com/gen2brain/vpx/vp8"
)

const vp8MaxDimension = 1<<14 - 1

func divAlpha(v, a uint8) uint32 {
	return min(uint32(v)*255/uint32(a), 255)
}

func unpremultiply(r, g, b, a uint8) uint32 {
	switch a {
	case 0xff:
		return argbBlack | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	case 0:
		return 0
	}

	return uint32(a)<<24 | divAlpha(r, a)<<16 | divAlpha(g, a)<<8 | divAlpha(b, a)
}

func ycbcrToARGB(src *image.YCbCr, b image.Rectangle, px []uint32) {
	i := 0

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			ci := src.COffset(x, y)
			r, g, bl := color.YCbCrToRGB(src.Y[src.YOffset(x, y)], src.Cb[ci], src.Cr[ci])

			px[i] = argbBlack | uint32(r)<<16 | uint32(g)<<8 | uint32(bl)
			i++
		}
	}
}

func (e *encoder) toARGB(m image.Image) []uint32 {
	b := m.Bounds()

	e.argb = grow(e.argb, b.Dx()*b.Dy())

	px := e.argb
	i := 0

	switch src := m.(type) {
	case *image.NRGBA:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			row := src.Pix[src.PixOffset(b.Min.X, y):]

			for x := range b.Dx() {
				p := row[4*x : 4*x+4 : 4*x+4]
				px[i] = uint32(p[3])<<24 | uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
				i++
			}
		}
	case *image.RGBA:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			row := src.Pix[src.PixOffset(b.Min.X, y):]

			for x := range b.Dx() {
				p := row[4*x : 4*x+4 : 4*x+4]
				px[i] = unpremultiply(p[0], p[1], p[2], p[3])
				i++
			}
		}
	case *image.Gray:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			row := src.Pix[src.PixOffset(b.Min.X, y):]

			for x := range b.Dx() {
				g := uint32(row[x])
				px[i] = argbBlack | g<<16 | g<<8 | g
				i++
			}
		}
	case *image.NYCbCrA:
		ycbcrToARGB(&src.YCbCr, b, px)

		for y := b.Min.Y; y < b.Max.Y; y++ {
			row := src.A[src.AOffset(b.Min.X, y):]
			dst := px[(y-b.Min.Y)*b.Dx():]

			for x := range b.Dx() {
				dst[x] = dst[x]&^argbBlack | uint32(row[x])<<24
			}
		}
	case *image.YCbCr:
		ycbcrToARGB(src, b, px)
	default:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := color.NRGBAModel.Convert(m.At(x, y)).(color.NRGBA)
				px[i] = uint32(c.A)<<24 | uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
				i++
			}
		}
	}

	return px
}

type span struct {
	off, end int
}

type encoder struct {
	lossless losslessEncoder
	lossy    vp8.Encoder
	argb     []uint32
	alpha    []uint32
	yuv      *image.NYCbCrA

	pic    vp8.Picture
	chunks [2]chunkOut
	list   []chunkOut
	frames []span
	body   []byte
	hdr    [riffHeaderSize + chunkHeaderSize]byte
	meta   [vp8xPayloadSize]byte
	anim   [animPayloadSize]byte
}

var encoderPool sync.Pool

func getEncoder() *encoder {
	if e, ok := encoderPool.Get().(*encoder); ok {
		return e
	}

	return &encoder{}
}

func putEncoder(e *encoder) {
	e.lossy.Release()
	e.lossless.release()

	e.pic = vp8.Picture{}
	e.chunks = [2]chunkOut{}
	e.list = e.list[:0]
	e.frames = e.frames[:0]
	e.body = e.body[:0]

	encoderPool.Put(e)
}

func (e *encoder) picture(px []uint32, width, height int) *vp8.Picture {
	if e.yuv == nil || e.yuv.Rect.Dx() != width || e.yuv.Rect.Dy() != height {
		e.yuv = image.NewNYCbCrA(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	}

	argbToYUVA(px, width, height, e.yuv)

	e.pic = vp8.Picture{
		Y: e.yuv.Y, U: e.yuv.Cb, V: e.yuv.Cr,
		YStride:  e.yuv.YStride,
		UVStride: e.yuv.CStride,
		Width:    width,
		Height:   height,
	}

	return &e.pic
}

func (e *encoder) yuvPlanes(m image.Image) (*vp8.Picture, []byte, int) {
	b := m.Bounds()

	if b.Min.X&1 != 0 || b.Min.Y&1 != 0 {
		return nil, nil, 0
	}

	var (
		src     *image.YCbCr
		alpha   []byte
		aStride int
	)

	switch v := m.(type) {
	case *image.NYCbCrA:
		src, alpha, aStride = &v.YCbCr, v.A[v.AOffset(b.Min.X, b.Min.Y):], v.AStride
	case *image.YCbCr:
		src = v
	default:
		return nil, nil, 0
	}

	if src.SubsampleRatio != image.YCbCrSubsampleRatio420 {
		return nil, nil, 0
	}

	ci := src.COffset(b.Min.X, b.Min.Y)

	e.pic = vp8.Picture{
		Y:        src.Y[src.YOffset(b.Min.X, b.Min.Y):],
		U:        src.Cb[ci:],
		V:        src.Cr[ci:],
		YStride:  src.YStride,
		UVStride: src.CStride,
		Width:    b.Dx(),
		Height:   b.Dy(),
	}

	return &e.pic, alpha, aStride
}

func (e *encoder) alphaFromARGB(px []uint32) bool {
	e.alpha = grow(e.alpha, len(px))

	opaque := true

	for i, v := range px {
		a := v >> 24
		opaque = opaque && a == 0xff

		e.alpha[i] = argbBlack | a<<8
	}

	return !opaque
}

func (e *encoder) alphaFromPlane(a []byte, stride, width, height int) bool {
	e.alpha = grow(e.alpha, width*height)

	opaque := true

	for y := range height {
		row := a[y*stride : y*stride+width]
		dst := e.alpha[y*width : y*width+width]

		for x, v := range row {
			opaque = opaque && v == 0xff
			dst[x] = argbBlack | uint32(v)<<8
		}
	}

	return !opaque
}

func (e *encoder) frameChunks(m image.Image, o Options) ([]chunkOut, error) {
	b := m.Bounds()
	width, height := b.Dx(), b.Dy()

	if o.Lossless {
		if width > vp8lMaxDimension || height > vp8lMaxDimension {
			return nil, ErrEncode
		}

		px := e.toARGB(m)

		e.chunks[0] = chunkOut{id: fccVP8L, payload: e.lossless.encode(px, width, height, o)}

		return e.chunks[:1], nil
	}

	if width > vp8MaxDimension || height > vp8MaxDimension {
		return nil, ErrEncode
	}

	pic, alpha, stride := e.yuvPlanes(m)
	transparent := false

	switch {
	case pic == nil:
		px := e.toARGB(m)
		pic = e.picture(px, width, height)
		transparent = e.alphaFromARGB(px)
	case alpha != nil:
		transparent = e.alphaFromPlane(alpha, stride, width, height)
	}

	data, err := e.lossy.Encode(pic, vp8.EncodeOptions{
		Quality: o.Quality,
		Method:  o.Method,
	})
	if err != nil {
		return nil, err
	}

	if !transparent {
		e.chunks[0] = chunkOut{id: fccVP8, payload: data}

		return e.chunks[:1], nil
	}

	e.chunks[0] = chunkOut{id: fccALPH, payload: e.lossless.encodeAlpha(e.alpha, width, height, o)}
	e.chunks[1] = chunkOut{id: fccVP8, payload: data}

	return e.chunks[:2], nil
}

func encode(w io.Writer, m image.Image, o Options) error {
	b := m.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return ErrEncode
	}

	e := getEncoder()

	defer putEncoder(e)

	chunks, err := e.frameChunks(m, o)
	if err != nil {
		return err
	}

	list := e.list[:0]

	if len(chunks) > 1 {
		list = append(list, chunkOut{id: fccVP8X, payload: vp8xPayload(e.meta[:], b.Dx(), b.Dy(), flagAlpha)})
	}

	list = append(list, chunks...)
	e.list = list

	return writeFile(w, e.hdr[:], list)
}

func encodeAll(w io.Writer, anim *WEBP, o Options) error {
	b := anim.Image[0].Bounds()
	width, height := b.Dx(), b.Dy()

	if width <= 0 || height <= 0 {
		return ErrEncode
	}

	e := getEncoder()

	defer putEncoder(e)

	flags := byte(flagAnimation)

	e.body = e.body[:0]
	e.frames = e.frames[:0]

	for i, img := range anim.Image {
		chunks, err := e.frameChunks(img, o)
		if err != nil {
			return err
		}

		if len(chunks) > 1 {
			flags |= flagAlpha
		}

		duration := 0
		if i < len(anim.Delay) {
			duration = anim.Delay[i]
		}

		off := len(e.body)
		e.body = appendANMF(e.body, frame{w: width, h: height, duration: duration}, chunks)

		e.frames = append(e.frames, span{off: off, end: len(e.body)})
	}

	list := append(e.list[:0],
		chunkOut{id: fccVP8X, payload: vp8xPayload(e.meta[:], width, height, flags)},
		chunkOut{id: fccANIM, payload: animPayload(e.anim[:], anim.LoopCount)},
	)

	for _, s := range e.frames {
		list = append(list, chunkOut{id: fccANMF, payload: e.body[s.off:s.end]})
	}

	e.list = list

	return writeFile(w, e.hdr[:], list)
}
