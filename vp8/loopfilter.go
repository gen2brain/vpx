package vp8

import (
	"encoding/binary"
	"math/bits"
)

const (
	pairsEven   = 0x00ff00ff00ff00ff
	pairsOne    = 0x0001000100010001
	pairsBias   = 0x7f007f007f007f00
	pairsThresh = 0x0800080008000800
	pairsHigh   = 0x8000800080008000
	pairsInner  = 0x0000800000008000
	pairsHev    = 0x0000800080000000
	pairsGather = 0x0100040010004000
)

type fInfo struct {
	limit     int
	ilevel    int
	hevThresh int
}

func sclip1(v int) int {
	return min(max(v, -128), 127)
}

func sclip2(v int) int {
	return min(max(v, -16), 15)
}

func abs0(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

func doFilter2(p []byte, off, step int) {
	p1 := int(p[off-2*step])
	p0 := int(p[off-step])
	q0 := int(p[off])
	q1 := int(p[off+step])

	a := 3*(q0-p0) + sclip1(p1-q1)
	a1 := sclip2((a + 4) >> 3)
	a2 := sclip2((a + 3) >> 3)

	p[off-step] = clip8(p0 + a2)
	p[off] = clip8(q0 - a1)
}

func doFilter4(p []byte, off, step int) {
	p1 := int(p[off-2*step])
	p0 := int(p[off-step])
	q0 := int(p[off])
	q1 := int(p[off+step])

	a := 3 * (q0 - p0)
	a1 := sclip2((a + 4) >> 3)
	a2 := sclip2((a + 3) >> 3)
	a3 := (a1 + 1) >> 1

	p[off-2*step] = clip8(p1 + a3)
	p[off-step] = clip8(p0 + a2)
	p[off] = clip8(q0 - a1)
	p[off+step] = clip8(q1 - a3)
}

func doFilter6(p []byte, off, step int) {
	p2 := int(p[off-3*step])
	p1 := int(p[off-2*step])
	p0 := int(p[off-step])
	q0 := int(p[off])
	q1 := int(p[off+step])
	q2 := int(p[off+2*step])

	a := sclip1(3*(q0-p0) + sclip1(p1-q1))
	a1 := (27*a + 63) >> 7
	a2 := (18*a + 63) >> 7
	a3 := (9*a + 63) >> 7

	p[off-3*step] = clip8(p2 + a3)
	p[off-2*step] = clip8(p1 + a2)
	p[off-step] = clip8(p0 + a1)
	p[off] = clip8(q0 - a1)
	p[off+step] = clip8(q1 - a2)
	p[off+2*step] = clip8(q2 - a3)
}

func hev(p []byte, off, step, thresh int) bool {
	p1 := int(p[off-2*step])
	p0 := int(p[off-step])
	q0 := int(p[off])
	q1 := int(p[off+step])

	return abs0(p1-p0) > thresh || abs0(q1-q0) > thresh
}

func needsFilter(p []byte, off, step, t int) bool {
	p1 := int(p[off-2*step])
	p0 := int(p[off-step])
	q0 := int(p[off])
	q1 := int(p[off+step])

	return 4*abs0(p0-q0)+abs0(p1-q1) <= t
}

func needsFilter2(p []byte, off, step, t, it int) bool {
	p3 := int(p[off-4*step])
	p2 := int(p[off-3*step])
	p1 := int(p[off-2*step])
	p0 := int(p[off-step])
	q0 := int(p[off])
	q1 := int(p[off+step])
	q2 := int(p[off+2*step])
	q3 := int(p[off+3*step])

	if 4*abs0(p0-q0)+abs0(p1-q1) > t {
		return false
	}

	return abs0(p3-p2) <= it && abs0(p2-p1) <= it && abs0(p1-p0) <= it &&
		abs0(q3-q2) <= it && abs0(q2-q1) <= it && abs0(q1-q0) <= it
}

func gatherPairs(x uint64) uint32 {
	return uint32((((x >> 15) & pairsOne) * pairsGather) >> 56)
}

type filterWords struct {
	t     int
	k     uint64
	lim   uint64
	hevLo uint64
	hevHi uint64
}

func wordsFor(f fInfo) filterWords {
	return filterWords{
		t:     2*f.limit + 1,
		k:     uint64(0x0100+f.ilevel) * pairsOne,
		lim:   uint64(0x8100+2*f.ilevel) * pairsOne,
		hevLo: uint64(0x7f00-f.ilevel+f.hevThresh) * pairsOne,
		hevHi: uint64(0x8100+f.ilevel+f.hevThresh) * pairsOne,
	}
}

func (l filterWords) needs(w uint64) (bool, bool) {
	p1 := int(w >> 16 & 0xff)
	p0 := int(w >> 24 & 0xff)
	q0 := int(w >> 32 & 0xff)
	q1 := int(w >> 40 & 0xff)

	if 4*abs0(p0-q0)+abs0(p1-q1) > l.t {
		return false, false
	}

	even := w & pairsEven
	odd := (w >> 8) & pairsEven
	next := (w >> 16) & pairsEven

	a := (even + l.k) - odd
	b := (odd + l.k) - next

	lo := a + pairsBias

	if (lo & (l.lim - a) & pairsHigh) != pairsHigh {
		return false, false
	}

	if ((b + pairsBias) & (l.lim - b) & pairsInner) != pairsInner {
		return false, false
	}

	return true, ((a + l.hevLo) & (l.hevHi - a) & pairsHev) != pairsHev
}

func filterMask(p []byte, off, stride, size int, f fInfo) (uint32, uint32) {
	l := wordsFor(f)

	lo := uint64(0x7800+l.t) * pairsOne
	hi := uint64(0x8800+l.t) * pairsOne

	var need, high uint32

	for g := 0; g < size; g += 8 {
		o := off + g

		w0 := binary.LittleEndian.Uint64(p[o-4*stride:])
		w1 := binary.LittleEndian.Uint64(p[o-3*stride:])
		w2 := binary.LittleEndian.Uint64(p[o-2*stride:])
		w3 := binary.LittleEndian.Uint64(p[o-stride:])
		w4 := binary.LittleEndian.Uint64(p[o:])
		w5 := binary.LittleEndian.Uint64(p[o+stride:])
		w6 := binary.LittleEndian.Uint64(p[o+2*stride:])
		w7 := binary.LittleEndian.Uint64(p[o+3*stride:])

		for sh := range 2 {
			p3 := (w0 >> (8 * sh)) & pairsEven
			p2 := (w1 >> (8 * sh)) & pairsEven
			p1 := (w2 >> (8 * sh)) & pairsEven
			p0 := (w3 >> (8 * sh)) & pairsEven
			q0 := (w4 >> (8 * sh)) & pairsEven
			q1 := (w5 >> (8 * sh)) & pairsEven
			q2 := (w6 >> (8 * sh)) & pairsEven
			q3 := (w7 >> (8 * sh)) & pairsEven

			s0 := (p0 << 2) + pairsThresh
			s1 := q0 << 2

			u := (s0 + p1) - (s1 + q1)
			v := (s0 + q1) - (s1 + p1)

			ok := ((u + lo) & (hi - u)) & ((v + lo) & (hi - v))

			p10 := (p1 + l.k) - p0
			q10 := (q1 + l.k) - q0

			hev := ((p10 + l.hevLo) & (l.hevHi - p10)) & ((q10 + l.hevLo) & (l.hevHi - q10))

			ok &= ((p10 + pairsBias) & (l.lim - p10)) & ((q10 + pairsBias) & (l.lim - q10))

			p32 := (p3 + l.k) - p2
			p21 := (p2 + l.k) - p1
			q21 := (q2 + l.k) - q1
			q32 := (q3 + l.k) - q2

			ok &= ((p32 + pairsBias) & (l.lim - p32)) & ((p21 + pairsBias) & (l.lim - p21))
			ok &= ((q21 + pairsBias) & (l.lim - q21)) & ((q32 + pairsBias) & (l.lim - q32))

			need |= (gatherPairs(ok&pairsHigh) << sh) << g
			high |= (gatherPairs((^hev)&pairsHigh) << sh) << g
		}
	}

	return need, high
}

func vFilterLoop26(p []byte, off, stride, size int, f fInfo) {
	m, h := filterMask(p, off, stride, size, f)

	for m != 0 {
		i := bits.TrailingZeros32(m)
		m &= m - 1

		if h>>i&1 != 0 {
			doFilter2(p, off+i, stride)
		} else {
			doFilter6(p, off+i, stride)
		}
	}
}

func vFilterLoop24(p []byte, off, stride, size int, f fInfo) {
	m, h := filterMask(p, off, stride, size, f)

	for m != 0 {
		i := bits.TrailingZeros32(m)
		m &= m - 1

		if h>>i&1 != 0 {
			doFilter2(p, off+i, stride)
		} else {
			doFilter4(p, off+i, stride)
		}
	}
}

func hFilterLoop26(p []byte, off, stride, size int, f fInfo) {
	l := wordsFor(f)

	for range size {
		if need, high := l.needs(binary.LittleEndian.Uint64(p[off-4:])); need {
			if high {
				doFilter2(p, off, 1)
			} else {
				doFilter6(p, off, 1)
			}
		}

		off += stride
	}
}

func hFilterLoop24(p []byte, off, stride, size int, f fInfo) {
	l := wordsFor(f)

	for range size {
		if need, high := l.needs(binary.LittleEndian.Uint64(p[off-4:])); need {
			if high {
				doFilter2(p, off, 1)
			} else {
				doFilter4(p, off, 1)
			}
		}

		off += stride
	}
}

func simpleFilter16(p []byte, off, hstride, vstride, thresh int) {
	t := 2*thresh + 1

	for range 16 {
		if needsFilter(p, off, hstride, t) {
			doFilter2(p, off, hstride)
		}

		off += vstride
	}
}

func simpleVFilter16(p []byte, off, stride, thresh int) {
	simpleFilter16(p, off, stride, 1, thresh)
}

func simpleHFilter16(p []byte, off, stride, thresh int) {
	simpleFilter16(p, off, 1, stride, thresh)
}

func simpleVFilter16i(p []byte, off, stride, thresh int) {
	for range 3 {
		off += 4 * stride
		simpleVFilter16(p, off, stride, thresh)
	}
}

func simpleHFilter16i(p []byte, off, stride, thresh int) {
	for range 3 {
		off += 4
		simpleHFilter16(p, off, stride, thresh)
	}
}

func (f fInfo) edge() fInfo {
	f.limit += 4

	return f
}

func vFilter16(p []byte, off, stride int, f fInfo) {
	if vFilter16Asm != nil && off >= 4*stride && len(p)-off >= 3*stride+16 {
		vFilter16Asm(p, off, stride, f.limit, f.ilevel, f.hevThresh)

		return
	}

	vFilterLoop26(p, off, stride, 16, f)
}

func hFilter16(p []byte, off, stride int, f fInfo) {
	if hFilter16Asm != nil && off >= 4 && len(p)-off >= 15*stride+4 {
		hFilter16Asm(p, off, stride, f.limit, f.ilevel, f.hevThresh)

		return
	}

	hFilterLoop26(p, off, stride, 16, f)
}

func vFilter16i(p []byte, off, stride int, f fInfo) {
	if vFilter16iAsm != nil && off >= 0 && len(p)-off >= 15*stride+16 {
		vFilter16iAsm(p, off, stride, f.limit, f.ilevel, f.hevThresh)

		return
	}

	for range 3 {
		off += 4 * stride
		vFilterLoop24(p, off, stride, 16, f)
	}
}

func hFilter16i(p []byte, off, stride int, f fInfo) {
	if hFilter16iAsm != nil && off >= 0 && len(p)-off >= 15*stride+16 {
		hFilter16iAsm(p, off, stride, f.limit, f.ilevel, f.hevThresh)

		return
	}

	for range 3 {
		off += 4
		hFilterLoop24(p, off, stride, 16, f)
	}
}

func vFilter8(u, v []byte, off, stride int, f fInfo) {
	if vFilter8Asm != nil && off >= 4*stride && len(u)-off >= 3*stride+8 && len(v)-off >= 3*stride+8 {
		vFilter8Asm(u, v, off, stride, f.limit, f.ilevel, f.hevThresh)

		return
	}

	vFilterLoop26(u, off, stride, 8, f)
	vFilterLoop26(v, off, stride, 8, f)
}

func hFilter8(u, v []byte, off, stride int, f fInfo) {
	if hFilter8Asm != nil && off >= 4 && len(u)-off >= 7*stride+4 && len(v)-off >= 7*stride+4 {
		hFilter8Asm(u, v, off, stride, f.limit, f.ilevel, f.hevThresh)

		return
	}

	hFilterLoop26(u, off, stride, 8, f)
	hFilterLoop26(v, off, stride, 8, f)
}

func vFilter8i(u, v []byte, off, stride int, f fInfo) {
	if vFilter8iAsm != nil && off >= 0 && len(u)-off >= 7*stride+8 && len(v)-off >= 7*stride+8 {
		vFilter8iAsm(u, v, off, stride, f.limit, f.ilevel, f.hevThresh)

		return
	}

	vFilterLoop24(u, off+4*stride, stride, 8, f)
	vFilterLoop24(v, off+4*stride, stride, 8, f)
}

func hFilter8i(u, v []byte, off, stride int, f fInfo) {
	if hFilter8iAsm != nil && off >= 0 && len(u)-off >= 7*stride+8 && len(v)-off >= 7*stride+8 {
		hFilter8iAsm(u, v, off, stride, f.limit, f.ilevel, f.hevThresh)

		return
	}

	hFilterLoop24(u, off+4, stride, 8, f)
	hFilterLoop24(v, off+4, stride, 8, f)
}

func (d *Decoder) precomputeFilterStrengths() {
	d.filterType = 0

	if d.filter.level != 0 {
		d.filterType = 2
		if d.filter.simple {
			d.filterType = 1
		}
	}

	if d.filterType == 0 {
		return
	}

	for s := range numSegments {
		base := d.filter.level

		if d.seg.enabled {
			base = int(d.seg.filterStrength[s])
			if !d.seg.absoluteDelta {
				base += d.filter.level
			}
		}

		for ref := range numRefFrames {
			for mode := range 4 {
				if ref == refIntra && mode > 1 {
					continue
				}

				if ref != refIntra && mode == 0 {
					continue
				}

				level := base

				if d.filter.useDelta {
					level += int(d.filter.refDelta[ref])

					if ref != refIntra || mode == 0 {
						level += int(d.filter.modeDelta[mode])
					}
				}

				level = min(max(level, 0), 63)

				d.setFilterInfo(&d.fStrengths[s][ref][mode], level)
			}
		}
	}
}

func (d *Decoder) setFilterInfo(info *fInfo, level int) {
	*info = fInfo{}

	if level == 0 {
		return
	}

	ilevel := level

	if d.filter.sharpness > 0 {
		if d.filter.sharpness > 4 {
			ilevel >>= 2
		} else {
			ilevel >>= 1
		}

		ilevel = min(ilevel, 9-d.filter.sharpness)
	}

	info.ilevel = max(ilevel, 1)
	info.limit = 2*level + info.ilevel
	inter := 0
	if !d.hdr.KeyFrame {
		inter = 1
	}

	switch {
	case level >= 40:
		info.hevThresh = 2 + inter
	case level >= 20:
		info.hevThresh = 1 + inter
	case level >= 15:
		info.hevThresh = 1
	default:
		info.hevThresh = 0
	}
}

func (d *Decoder) filterMB(mbX, mbY int) {
	flags := d.fInfoRow[mbX]

	f := d.fStrengths[flags&3][flags>>4&3][flags>>2&3]
	if f.limit == 0 {
		return
	}

	inner := flags>>6&1 != 0

	yStride := d.pic.YStride
	yOff := mbY*16*yStride + mbX*16

	if d.filterType == 1 {
		if mbX > 0 {
			simpleHFilter16(d.pic.Y, yOff, yStride, f.limit+4)
		}

		if inner {
			simpleHFilter16i(d.pic.Y, yOff, yStride, f.limit)
		}

		if mbY > 0 {
			simpleVFilter16(d.pic.Y, yOff, yStride, f.limit+4)
		}

		if inner {
			simpleVFilter16i(d.pic.Y, yOff, yStride, f.limit)
		}

		return
	}

	uvStride := d.pic.UVStride
	uvOff := mbY*8*uvStride + mbX*8

	if mbX > 0 {
		hFilter16(d.pic.Y, yOff, yStride, f.edge())
		hFilter8(d.pic.U, d.pic.V, uvOff, uvStride, f.edge())
	}

	if inner {
		hFilter16i(d.pic.Y, yOff, yStride, f)
		hFilter8i(d.pic.U, d.pic.V, uvOff, uvStride, f)
	}

	if mbY > 0 {
		vFilter16(d.pic.Y, yOff, yStride, f.edge())
		vFilter8(d.pic.U, d.pic.V, uvOff, uvStride, f.edge())
	}

	if inner {
		vFilter16i(d.pic.Y, yOff, yStride, f)
		vFilter8i(d.pic.U, d.pic.V, uvOff, uvStride, f)
	}
}

func (d *Decoder) filterRow(mbY int) {
	for mbX := range d.mbW {
		d.filterMB(mbX, mbY)
	}
}
