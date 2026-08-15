package vp8

import "encoding/binary"

const (
	bDCPred = iota
	bTMPred
	bVEPred
	bHEPred
	bRDPred
	bVRPred
	bLDPred
	bVLPred
	bHDPred
	bHUPred
	numBModes
)

const (
	bPred  = numBModes
	dcPred = bDCPred
	tmPred = bTMPred
	vPred  = bVEPred
	hPred  = bHEPred
)

const (
	dcPredNoTop = 4 + iota
	dcPredNoLeft
	dcPredNoTopLeft
	numDCModes
)

func avg2(a, b int) uint8 { return uint8((a + b + 1) >> 1) }

func avg3(a, b, c int) uint8 { return uint8((a + 2*b + c + 2) >> 2) }

func fill(b []byte, off, n int, v uint8) {
	row := b[off : off+n]
	x := uint64(v) * 0x0101010101010101

	for len(row) >= 8 {
		binary.LittleEndian.PutUint64(row, x)
		row = row[8:]
	}

	if len(row) >= 4 {
		binary.LittleEndian.PutUint32(row, uint32(x))
		row = row[4:]
	}

	for i := range row {
		row[i] = v
	}
}

func fillBlock(b []byte, off, n int, v uint8) {
	for y := range n {
		fill(b, off+y*bps, n, v)
	}
}

func trueMotion(b []byte, off, size int) {
	top := off - bps
	tl := int(b[top-1])

	for range size {
		left := int(b[off-1])

		for x := range size {
			b[off+x] = clip8(left + int(b[top+x]) - tl)
		}

		off += bps
	}
}

func tm4(b []byte, off int) { trueMotion(b, off, 4) }

func tm8uv(b []byte, off int) { trueMotion(b, off, 8) }

func tm16(b []byte, off int) { trueMotion(b, off, 16) }

func ve16(b []byte, off int) {
	top := b[off-bps : off-bps+16]

	for y := range 16 {
		copy(b[off+y*bps:off+y*bps+16], top)
	}
}

func he16(b []byte, off int) {
	for y := range 16 {
		fill(b, off+y*bps, 16, b[off+y*bps-1])
	}
}

func dc16(b []byte, off int) {
	dc := 16
	for j := range 16 {
		dc += int(b[off-1+j*bps]) + int(b[off+j-bps])
	}

	fillBlock(b, off, 16, uint8(dc>>5))
}

func dc16NoTop(b []byte, off int) {
	dc := 8
	for j := range 16 {
		dc += int(b[off-1+j*bps])
	}

	fillBlock(b, off, 16, uint8(dc>>4))
}

func dc16NoLeft(b []byte, off int) {
	dc := 8
	for i := range 16 {
		dc += int(b[off+i-bps])
	}

	fillBlock(b, off, 16, uint8(dc>>4))
}

func dc16NoTopLeft(b []byte, off int) { fillBlock(b, off, 16, 0x80) }

func ve4(b []byte, off int) {
	top := off - bps

	vals := [4]uint8{
		avg3(int(b[top-1]), int(b[top]), int(b[top+1])),
		avg3(int(b[top]), int(b[top+1]), int(b[top+2])),
		avg3(int(b[top+1]), int(b[top+2]), int(b[top+3])),
		avg3(int(b[top+2]), int(b[top+3]), int(b[top+4])),
	}

	for y := range 4 {
		copy(b[off+y*bps:off+y*bps+4], vals[:])
	}
}

func he4(b []byte, off int) {
	a := int(b[off-1-bps])
	c := int(b[off-1])
	d := int(b[off-1+bps])
	e := int(b[off-1+2*bps])
	f := int(b[off-1+3*bps])

	fill(b, off, 4, avg3(a, c, d))
	fill(b, off+bps, 4, avg3(c, d, e))
	fill(b, off+2*bps, 4, avg3(d, e, f))
	fill(b, off+3*bps, 4, avg3(e, f, f))
}

func dc4(b []byte, off int) {
	dc := 4
	for i := range 4 {
		dc += int(b[off+i-bps]) + int(b[off-1+i*bps])
	}

	fillBlock(b, off, 4, uint8(dc>>3))
}

func dst(b []byte, off, x, y int, v uint8) { b[off+x+y*bps] = v }

func rd4(b []byte, off int) {
	i := int(b[off-1])
	j := int(b[off-1+bps])
	k := int(b[off-1+2*bps])
	l := int(b[off-1+3*bps])
	x := int(b[off-1-bps])
	a := int(b[off-bps])
	c := int(b[off+1-bps])
	d := int(b[off+2-bps])
	e := int(b[off+3-bps])

	v := avg3(j, k, l)
	dst(b, off, 0, 3, v)

	v = avg3(i, j, k)
	dst(b, off, 1, 3, v)
	dst(b, off, 0, 2, v)

	v = avg3(x, i, j)
	dst(b, off, 2, 3, v)
	dst(b, off, 1, 2, v)
	dst(b, off, 0, 1, v)

	v = avg3(a, x, i)
	dst(b, off, 3, 3, v)
	dst(b, off, 2, 2, v)
	dst(b, off, 1, 1, v)
	dst(b, off, 0, 0, v)

	v = avg3(c, a, x)
	dst(b, off, 3, 2, v)
	dst(b, off, 2, 1, v)
	dst(b, off, 1, 0, v)

	v = avg3(d, c, a)
	dst(b, off, 3, 1, v)
	dst(b, off, 2, 0, v)

	dst(b, off, 3, 0, avg3(e, d, c))
}

func ld4(b []byte, off int) {
	top := off - bps
	a := int(b[top])
	c := int(b[top+1])
	d := int(b[top+2])
	e := int(b[top+3])
	f := int(b[top+4])
	g := int(b[top+5])
	h := int(b[top+6])
	i := int(b[top+7])

	v := avg3(a, c, d)
	dst(b, off, 0, 0, v)

	v = avg3(c, d, e)
	dst(b, off, 1, 0, v)
	dst(b, off, 0, 1, v)

	v = avg3(d, e, f)
	dst(b, off, 2, 0, v)
	dst(b, off, 1, 1, v)
	dst(b, off, 0, 2, v)

	v = avg3(e, f, g)
	dst(b, off, 3, 0, v)
	dst(b, off, 2, 1, v)
	dst(b, off, 1, 2, v)
	dst(b, off, 0, 3, v)

	v = avg3(f, g, h)
	dst(b, off, 3, 1, v)
	dst(b, off, 2, 2, v)
	dst(b, off, 1, 3, v)

	v = avg3(g, h, i)
	dst(b, off, 3, 2, v)
	dst(b, off, 2, 3, v)

	dst(b, off, 3, 3, avg3(h, i, i))
}

func vr4(b []byte, off int) {
	i := int(b[off-1])
	j := int(b[off-1+bps])
	k := int(b[off-1+2*bps])
	x := int(b[off-1-bps])
	a := int(b[off-bps])
	c := int(b[off+1-bps])
	d := int(b[off+2-bps])
	e := int(b[off+3-bps])

	v := avg2(x, a)
	dst(b, off, 0, 0, v)
	dst(b, off, 1, 2, v)

	v = avg2(a, c)
	dst(b, off, 1, 0, v)
	dst(b, off, 2, 2, v)

	v = avg2(c, d)
	dst(b, off, 2, 0, v)
	dst(b, off, 3, 2, v)

	dst(b, off, 3, 0, avg2(d, e))

	dst(b, off, 0, 3, avg3(k, j, i))
	dst(b, off, 0, 2, avg3(j, i, x))

	v = avg3(i, x, a)
	dst(b, off, 0, 1, v)
	dst(b, off, 1, 3, v)

	v = avg3(x, a, c)
	dst(b, off, 1, 1, v)
	dst(b, off, 2, 3, v)

	v = avg3(a, c, d)
	dst(b, off, 2, 1, v)
	dst(b, off, 3, 3, v)

	dst(b, off, 3, 1, avg3(c, d, e))
}

func vl4(b []byte, off int) {
	top := off - bps
	a := int(b[top])
	c := int(b[top+1])
	d := int(b[top+2])
	e := int(b[top+3])
	f := int(b[top+4])
	g := int(b[top+5])
	h := int(b[top+6])
	i := int(b[top+7])

	dst(b, off, 0, 0, avg2(a, c))

	v := avg2(c, d)
	dst(b, off, 1, 0, v)
	dst(b, off, 0, 2, v)

	v = avg2(d, e)
	dst(b, off, 2, 0, v)
	dst(b, off, 1, 2, v)

	v = avg2(e, f)
	dst(b, off, 3, 0, v)
	dst(b, off, 2, 2, v)

	dst(b, off, 0, 1, avg3(a, c, d))

	v = avg3(c, d, e)
	dst(b, off, 1, 1, v)
	dst(b, off, 0, 3, v)

	v = avg3(d, e, f)
	dst(b, off, 2, 1, v)
	dst(b, off, 1, 3, v)

	v = avg3(e, f, g)
	dst(b, off, 3, 1, v)
	dst(b, off, 2, 3, v)

	dst(b, off, 3, 2, avg3(f, g, h))
	dst(b, off, 3, 3, avg3(g, h, i))
}

func hu4(b []byte, off int) {
	i := int(b[off-1])
	j := int(b[off-1+bps])
	k := int(b[off-1+2*bps])
	l := int(b[off-1+3*bps])

	dst(b, off, 0, 0, avg2(i, j))

	v := avg2(j, k)
	dst(b, off, 2, 0, v)
	dst(b, off, 0, 1, v)

	v = avg2(k, l)
	dst(b, off, 2, 1, v)
	dst(b, off, 0, 2, v)

	dst(b, off, 1, 0, avg3(i, j, k))

	v = avg3(j, k, l)
	dst(b, off, 3, 0, v)
	dst(b, off, 1, 1, v)

	v = avg3(k, l, l)
	dst(b, off, 3, 1, v)
	dst(b, off, 1, 2, v)

	e := uint8(l)
	dst(b, off, 3, 2, e)
	dst(b, off, 2, 2, e)
	dst(b, off, 0, 3, e)
	dst(b, off, 1, 3, e)
	dst(b, off, 2, 3, e)
	dst(b, off, 3, 3, e)
}

func hd4(b []byte, off int) {
	i := int(b[off-1])
	j := int(b[off-1+bps])
	k := int(b[off-1+2*bps])
	l := int(b[off-1+3*bps])
	x := int(b[off-1-bps])
	a := int(b[off-bps])
	c := int(b[off+1-bps])
	d := int(b[off+2-bps])

	v := avg2(i, x)
	dst(b, off, 0, 0, v)
	dst(b, off, 2, 1, v)

	v = avg2(j, i)
	dst(b, off, 0, 1, v)
	dst(b, off, 2, 2, v)

	v = avg2(k, j)
	dst(b, off, 0, 2, v)
	dst(b, off, 2, 3, v)

	dst(b, off, 0, 3, avg2(l, k))

	dst(b, off, 3, 0, avg3(a, c, d))
	dst(b, off, 2, 0, avg3(x, a, c))

	v = avg3(i, x, a)
	dst(b, off, 1, 0, v)
	dst(b, off, 3, 1, v)

	v = avg3(j, i, x)
	dst(b, off, 1, 1, v)
	dst(b, off, 3, 2, v)

	v = avg3(k, j, i)
	dst(b, off, 1, 2, v)
	dst(b, off, 3, 3, v)

	dst(b, off, 1, 3, avg3(l, k, j))
}

func ve8uv(b []byte, off int) {
	top := b[off-bps : off-bps+8]

	for y := range 8 {
		copy(b[off+y*bps:off+y*bps+8], top)
	}
}

func he8uv(b []byte, off int) {
	for y := range 8 {
		fill(b, off+y*bps, 8, b[off+y*bps-1])
	}
}

func dc8uv(b []byte, off int) {
	dc := 8
	for i := range 8 {
		dc += int(b[off+i-bps]) + int(b[off-1+i*bps])
	}

	fillBlock(b, off, 8, uint8(dc>>4))
}

func dc8uvNoLeft(b []byte, off int) {
	dc := 4
	for i := range 8 {
		dc += int(b[off+i-bps])
	}

	fillBlock(b, off, 8, uint8(dc>>3))
}

func dc8uvNoTop(b []byte, off int) {
	dc := 4
	for i := range 8 {
		dc += int(b[off-1+i*bps])
	}

	fillBlock(b, off, 8, uint8(dc>>3))
}

func dc8uvNoTopLeft(b []byte, off int) { fillBlock(b, off, 8, 0x80) }

var (
	predLuma16 = [numDCModes]func([]byte, int){
		dc16, tm16, ve16, he16, dc16NoTop, dc16NoLeft, dc16NoTopLeft,
	}
	predChroma8 = [numDCModes]func([]byte, int){
		dc8uv, tm8uv, ve8uv, he8uv, dc8uvNoTop, dc8uvNoLeft, dc8uvNoTopLeft,
	}
	predLuma4 = [numBModes]func([]byte, int){
		dc4, tm4, ve4, he4, rd4, vr4, ld4, vl4, hd4, hu4,
	}
)

func checkMode(mbX, mbY, mode int) int {
	if mode != dcPred {
		return mode
	}

	if mbX == 0 {
		if mbY == 0 {
			return dcPredNoTopLeft
		}

		return dcPredNoLeft
	}

	if mbY == 0 {
		return dcPredNoTop
	}

	return dcPred
}
