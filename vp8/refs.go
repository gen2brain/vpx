package vp8

const (
	yBorder         = 32
	uvBorder        = yBorder / 2
	numFrameBuffers = 4
)

type frameBuffer struct {
	pic      Picture
	y, u, v  []byte
	yOrigin  int
	uvOrigin int
}

func (f *frameBuffer) alloc(mbW, mbH, width, height int) {
	yStride := mbW*16 + 2*yBorder
	uvStride := mbW*8 + 2*uvBorder

	if f.pic.YStride == yStride && f.pic.Height == height && f.pic.Width == width {
		return
	}

	ySize := yStride * (mbH*16 + 2*yBorder)
	uvSize := uvStride * (mbH*8 + 2*uvBorder)

	if cap(f.y) < ySize || cap(f.u) < uvSize {
		f.y = make([]byte, ySize)
		f.u = make([]byte, uvSize)
		f.v = make([]byte, uvSize)
	}

	f.y, f.u, f.v = f.y[:ySize], f.u[:uvSize], f.v[:uvSize]

	f.yOrigin = yBorder*yStride + yBorder
	f.uvOrigin = uvBorder*uvStride + uvBorder

	f.pic = Picture{
		Y: f.y[f.yOrigin:], U: f.u[f.uvOrigin:], V: f.v[f.uvOrigin:],
		YStride:  yStride,
		UVStride: uvStride,
		Width:    width,
		Height:   height,
	}
}

func extendPlane(p []byte, origin, stride, w, h, border int) {
	for y := range h {
		row := origin + y*stride

		fill(p, row-border, border, p[row])
		fill(p, row+w, border, p[row+w-1])
	}

	top := origin - border
	for y := 1; y <= border; y++ {
		copy(p[top-y*stride:top-y*stride+stride], p[top:top+stride])
	}

	bottom := origin + (h-1)*stride - border
	for y := 1; y <= border; y++ {
		copy(p[bottom+y*stride:bottom+y*stride+stride], p[bottom:bottom+stride])
	}
}

func (f *frameBuffer) extend(mbW, mbH int) {
	extendPlane(f.y, f.yOrigin, f.pic.YStride, mbW*16, mbH*16, yBorder)
	extendPlane(f.u, f.uvOrigin, f.pic.UVStride, mbW*8, mbH*8, uvBorder)
	extendPlane(f.v, f.uvOrigin, f.pic.UVStride, mbW*8, mbH*8, uvBorder)
}

func (d *Decoder) freeBuffer() int {
	for i := range d.refCnt {
		if d.refCnt[i] == 0 {
			return i
		}
	}

	return 0
}

func (d *Decoder) assignBuffer(slot *int, to int) {
	d.refCnt[*slot]--
	*slot = to
	d.refCnt[to]++
}

func (d *Decoder) rotateBuffers() {
	switch d.copyAlt {
	case 1:
		d.assignBuffer(&d.altIdx, d.lastIdx)
	case 2:
		d.assignBuffer(&d.altIdx, d.goldenIdx)
	}

	switch d.copyGolden {
	case 1:
		d.assignBuffer(&d.goldenIdx, d.lastIdx)
	case 2:
		d.assignBuffer(&d.goldenIdx, d.altIdx)
	}

	if d.refreshGolden {
		d.assignBuffer(&d.goldenIdx, d.newIdx)
	}

	if d.refreshAlt {
		d.assignBuffer(&d.altIdx, d.newIdx)
	}

	if d.refreshLast {
		d.assignBuffer(&d.lastIdx, d.newIdx)
	}

	d.refCnt[d.newIdx]--
}
