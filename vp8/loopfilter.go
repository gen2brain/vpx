package vp8

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

func filterLoop26(p []byte, off, hstride, vstride, size int, f fInfo) {
	t := 2*f.limit + 1

	for range size {
		if needsFilter2(p, off, hstride, t, f.ilevel) {
			if hev(p, off, hstride, f.hevThresh) {
				doFilter2(p, off, hstride)
			} else {
				doFilter6(p, off, hstride)
			}
		}

		off += vstride
	}
}

func filterLoop24(p []byte, off, hstride, vstride, size int, f fInfo) {
	t := 2*f.limit + 1

	for range size {
		if needsFilter2(p, off, hstride, t, f.ilevel) {
			if hev(p, off, hstride, f.hevThresh) {
				doFilter2(p, off, hstride)
			} else {
				doFilter4(p, off, hstride)
			}
		}

		off += vstride
	}
}

func (f fInfo) edge() fInfo {
	f.limit += 4

	return f
}

func vFilter16(p []byte, off, stride int, f fInfo) {
	filterLoop26(p, off, stride, 1, 16, f)
}

func hFilter16(p []byte, off, stride int, f fInfo) {
	filterLoop26(p, off, 1, stride, 16, f)
}

func vFilter16i(p []byte, off, stride int, f fInfo) {
	for range 3 {
		off += 4 * stride
		filterLoop24(p, off, stride, 1, 16, f)
	}
}

func hFilter16i(p []byte, off, stride int, f fInfo) {
	for range 3 {
		off += 4
		filterLoop24(p, off, 1, stride, 16, f)
	}
}

func vFilter8(u, v []byte, off, stride int, f fInfo) {
	filterLoop26(u, off, stride, 1, 8, f)
	filterLoop26(v, off, stride, 1, 8, f)
}

func hFilter8(u, v []byte, off, stride int, f fInfo) {
	filterLoop26(u, off, 1, stride, 8, f)
	filterLoop26(v, off, 1, stride, 8, f)
}

func vFilter8i(u, v []byte, off, stride int, f fInfo) {
	filterLoop24(u, off+4*stride, stride, 1, 8, f)
	filterLoop24(v, off+4*stride, stride, 1, 8, f)
}

func hFilter8i(u, v []byte, off, stride int, f fInfo) {
	filterLoop24(u, off+4, 1, stride, 8, f)
	filterLoop24(v, off+4, 1, stride, 8, f)
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

		for i4x4 := range 2 {
			level := base

			if d.filter.useDelta {
				level += int(d.filter.refDelta[0])
				if i4x4 != 0 {
					level += int(d.filter.modeDelta[0])
				}
			}

			level = min(max(level, 0), 63)

			info := &d.fStrengths[s][i4x4]
			*info = fInfo{}

			if level == 0 {
				continue
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
			info.hevThresh = 0

			switch {
			case level >= 40:
				info.hevThresh = 2
			case level >= 15:
				info.hevThresh = 1
			}
		}
	}
}

func (d *Decoder) filterMB(mbX, mbY int) {
	flags := d.fInfoRow[mbY*d.mbW+mbX]

	f := d.fStrengths[flags&3][flags>>2&1]
	if f.limit == 0 {
		return
	}

	inner := flags>>3&1 != 0

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

func (d *Decoder) filterFrame() {
	if d.filterType == 0 {
		return
	}

	for mbY := range d.mbH {
		for mbX := range d.mbW {
			d.filterMB(mbX, mbY)
		}
	}
}
