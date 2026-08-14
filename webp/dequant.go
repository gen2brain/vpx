package webp

const (
	smoothFix  = 16
	smoothLFix = 2
	lutSize    = 1<<(8+smoothLFix) - 1
)

func countLevels(data []byte, stride, w, h int) (lo, hi, n, minDist int) {
	var used [256]bool

	lo, hi = 255, 0

	for y := range h {
		for _, v := range data[y*stride : y*stride+w] {
			lo = min(lo, int(v))
			hi = max(hi, int(v))
			used[v] = true
		}
	}

	minDist = hi - lo
	last := -1

	for i, u := range used {
		if !u {
			continue
		}

		n++

		if last >= 0 {
			minDist = min(minDist, i-last)
		}

		last = i
	}

	return lo, hi, n, minDist
}

func correctionLUT(minDist int) []int16 {
	lut := make([]int16, 2*lutSize+1)

	t1 := minDist << smoothLFix
	t2 := 3 * t1 >> 2
	delta := t1 - t2

	for i := 1; i <= lutSize; i++ {
		var c int

		switch {
		case i <= t2:
			c = i
		case i < t1:
			c = t2 * (t1 - i) / delta
		}

		c >>= smoothLFix

		lut[lutSize+i] = int16(c)
		lut[lutSize-i] = int16(-c)
	}

	return lut
}

func dequantizeLevels(data []byte, stride, w, h, strength int) {
	if strength <= 0 || strength > 100 || w <= 0 || h <= 0 {
		return
	}

	radius := 4 * strength / 100

	if 2*radius+1 > w {
		radius = (w - 1) >> 1
	}

	if 2*radius+1 > h {
		radius = (h - 1) >> 1
	}

	if radius <= 0 {
		return
	}

	lo, hi, levels, minDist := countLevels(data, stride, w, h)
	if levels <= 2 {
		return
	}

	r := 2*radius + 1
	buf := make([]uint16, (r+1)*w)
	average := make([]uint16, w)
	correction := correctionLUT(minDist)
	scale := uint32(1<<(smoothFix+smoothLFix)) / uint32(r*r)

	start, end := 0, r*w
	cur, top := start, end-w
	src, dst := 0, 0

	for row := -radius; row < h; row++ {
		var sum uint16

		for x := range w {
			sum += uint16(data[src+x])
			nv := buf[top+x] + sum
			buf[end+x] = nv - buf[cur+x]
			buf[cur+x] = nv
		}

		top = cur
		cur += w

		if cur == end {
			cur = start
		}

		if row >= 0 && row < h-1 {
			src += stride
		}

		if row < radius {
			continue
		}

		in := buf[end : end+w]

		x := 0
		for ; x <= radius; x++ {
			d := in[x+radius-1] + in[radius-x]
			average[x] = uint16(uint32(d) * scale >> smoothFix)
		}

		for ; x < w-radius; x++ {
			d := in[x+radius] - in[x-radius-1]
			average[x] = uint16(uint32(d) * scale >> smoothFix)
		}

		for ; x < w; x++ {
			d := 2*in[w-1] - in[2*w-2-radius-x] - in[x-radius-1]
			average[x] = uint16(uint32(d) * scale >> smoothFix)
		}

		for x := range w {
			v := int(data[dst+x])
			if v <= lo || v >= hi {
				continue
			}

			data[dst+x] = clipByte(v + int(correction[lutSize+int(average[x])-v<<smoothLFix]))
		}

		dst += stride
	}
}

func clipByte(v int) byte {
	if v&^0xff == 0 {
		return byte(v)
	}

	if v < 0 {
		return 0
	}

	return 255
}
