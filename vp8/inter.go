package vp8

const filterShift = 7

type predictor struct {
	src    []byte
	stride int
	sixtap bool
}

func (p *predictor) sub(src []byte) predictor {
	return predictor{src: src, stride: p.stride, sixtap: p.sixtap}
}

func filterRow(dst []int32, src []byte, off, count int, f *[6]int16) {
	for i := range count {
		p := off + i

		v := int32(src[p-2])*int32(f[0]) +
			int32(src[p-1])*int32(f[1]) +
			int32(src[p])*int32(f[2]) +
			int32(src[p+1])*int32(f[3]) +
			int32(src[p+2])*int32(f[4]) +
			int32(src[p+3])*int32(f[5]) +
			1<<(filterShift-1)

		dst[i] = int32(clip8(int(v >> filterShift)))
	}
}

func filterCol(dst []byte, off, stride int, src []int32, w, h int, f *[6]int16) {
	for j := range h {
		row := src[j*w:]

		for i := range w {
			p := i + 2*w

			v := int32(row[p-2*w])*int32(f[0]) +
				int32(row[p-w])*int32(f[1]) +
				int32(row[p])*int32(f[2]) +
				int32(row[p+w])*int32(f[3]) +
				int32(row[p+2*w])*int32(f[4]) +
				int32(row[p+3*w])*int32(f[5]) +
				1<<(filterShift-1)

			dst[off+j*stride+i] = clip8(int(v >> filterShift))
		}
	}
}

func (p *predictor) sixtapPredict(off, x, y, w, h int, dst []byte, dOff, dStride int) {
	var tmp [(16 + 5) * 16]int32

	hf := &subPelFilters[x]
	vf := &subPelFilters[y]

	for j := range h + 5 {
		filterRow(tmp[j*w:], p.src, off+(j-2)*p.stride, w, hf)
	}

	filterCol(dst, dOff, dStride, tmp[:], w, h, vf)
}

func bilinearRow(dst []int32, src []byte, off, count int, f *[2]int16) {
	for i := range count {
		p := off + i

		v := int32(src[p])*int32(f[0]) + int32(src[p+1])*int32(f[1]) + 1<<(filterShift-1)

		dst[i] = v >> filterShift
	}
}

func (p *predictor) bilinearPredict(off, x, y, w, h int, dst []byte, dOff, dStride int) {
	var tmp [(16 + 1) * 16]int32

	hf := &bilinearFilters[x]
	vf := &bilinearFilters[y]

	for j := range h + 1 {
		bilinearRow(tmp[j*w:], p.src, off+j*p.stride, w, hf)
	}

	for j := range h {
		row := tmp[j*w:]

		for i := range w {
			v := row[i]*int32(vf[0]) + row[i+w]*int32(vf[1]) + 1<<(filterShift-1)

			dst[dOff+j*dStride+i] = clip8(int(v >> filterShift))
		}
	}
}

func (p *predictor) predict(off, x, y, w, h int, dst []byte, dOff, dStride int) {
	if x == 0 && y == 0 {
		for j := range h {
			copy(dst[dOff+j*dStride:dOff+j*dStride+w], p.src[off+j*p.stride:off+j*p.stride+w])
		}

		return
	}

	if p.sixtap {
		p.sixtapPredict(off, x, y, w, h, dst, dOff, dStride)

		return
	}

	p.bilinearPredict(off, x, y, w, h, dst, dOff, dStride)
}

func (d *Decoder) reference(ref uint8) *frameBuffer {
	switch ref {
	case refGolden:
		return &d.frames[d.goldenIdx]
	case refAltRef:
		return &d.frames[d.altIdx]
	}

	return &d.frames[d.lastIdx]
}

func halve(v int) int {
	if v < 0 {
		return (v - 1) / 2
	}

	return (v + 1) / 2
}

func (d *Decoder) chromaMV(m mv) mv {
	return mv{
		row: int16(halve(int(m.row)) & d.fullPixel),
		col: int16(halve(int(m.col)) & d.fullPixel),
	}
}

func (d *Decoder) predictInter(mbX, mbY int) {
	ref := d.reference(d.mb.refFrame)

	luma := predictor{src: ref.y, stride: ref.pic.YStride, sixtap: d.sixtap}
	chroma := predictor{src: ref.u, stride: ref.pic.UVStride, sixtap: d.sixtap}

	yBase := ref.yOrigin + mbY*16*luma.stride + mbX*16
	uvBase := ref.uvOrigin + mbY*8*chroma.stride + mbX*8

	if d.mb.mode == mvSplit {
		d.predictSplit(mbX, mbY, ref, luma, chroma, yBase, uvBase)

		return
	}

	b := d.mbBounds(mbX, mbY)

	v := d.modeAt(mbX, mbY).mv
	if d.mb.needClamp {
		v = b.clampToBorder(v, 0)
	}

	off := yBase + int(v.row>>3)*luma.stride + int(v.col>>3)

	luma.predict(off, int(v.col&7), int(v.row&7), 16, 16, d.yuv[:], yOff, bps)

	c := d.chromaMV(v)

	if b.outsideBorder(c) {
		return
	}

	off = uvBase + int(c.row>>3)*chroma.stride + int(c.col>>3)

	chroma.predict(off, int(c.col&7), int(c.row&7), 8, 8, d.yuv[:], uOff, bps)

	vplane := chroma.sub(ref.v)
	vplane.predict(off, int(c.col&7), int(c.row&7), 8, 8, d.yuv[:], vOff, bps)
}

func (d *Decoder) predictSplit(mbX, mbY int, ref *frameBuffer, luma, chroma predictor, yBase, uvBase int) {
	m := d.modeAt(mbX, mbY)
	b := d.mbBounds(mbX, mbY)

	for i := range 16 {
		v := m.subMV[i]
		if d.mb.needClamp {
			v = b.clampToBorder(v, 0)
		}

		x, y := (i&3)*4, (i>>2)*4
		off := yBase + (y+int(v.row>>3))*luma.stride + x + int(v.col>>3)

		luma.predict(off, int(v.col&7), int(v.row&7), 4, 4, d.yuv[:], yOff+y*bps+x, bps)
	}

	for i := range 4 {
		col, row := i&1, i>>1
		n := row*8 + col*2

		var sumRow, sumCol int

		for _, k := range [4]int{n, n + 1, n + 4, n + 5} {
			sumRow += int(m.subMV[k].row)
			sumCol += int(m.subMV[k].col)
		}

		c := mv{row: average4(sumRow, d.fullPixel), col: average4(sumCol, d.fullPixel)}

		if d.mb.needClamp {
			c = b.clampToBorder(c, 1)
		}

		cx, cy := col*4, row*4
		off := uvBase + (cy+int(c.row>>3))*chroma.stride + cx + int(c.col>>3)

		uplane := chroma.sub(ref.u)
		uplane.predict(off, int(c.col&7), int(c.row&7), 4, 4, d.yuv[:], uOff+cy*bps+cx, bps)

		vplane := chroma.sub(ref.v)
		vplane.predict(off, int(c.col&7), int(c.row&7), 4, 4, d.yuv[:], vOff+cy*bps+cx, bps)
	}
}

func average4(sum, mask int) int16 {
	if sum < 0 {
		sum -= 4
	} else {
		sum += 4
	}

	return int16(sum / 8 & mask)
}
