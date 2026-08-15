package webp

const (
	predictorTransform = iota
	crossColorTransform
	subtractGreenTransform
	colorIndexingTransform
	numTransforms
)

const argbBlack = 0xff000000

type transform struct {
	kind int
	bits int
	data []uint32
	size int
}

func subSampleSize(size, bits int) int {
	return (size + 1<<bits - 1) >> bits
}

func average2(a0, a1 uint32) uint32 {
	return (a0^a1)&0xfefefefe>>1 + a0&a1
}

func average3(a0, a1, a2 uint32) uint32 {
	return average2(average2(a0, a2), a1)
}

func average4(a0, a1, a2, a3 uint32) uint32 {
	return average2(average2(a0, a1), average2(a2, a3))
}

func clip255(a uint32) uint32 {
	if a < 256 {
		return a
	}

	return ^a >> 24
}

func addSubFull(a, b, c int) uint32 {
	return clip255(uint32(a + b - c))
}

func clampedAddSubtractFull(c0, c1, c2 uint32) uint32 {
	a := addSubFull(int(c0>>24), int(c1>>24), int(c2>>24))
	r := addSubFull(int(c0>>16&0xff), int(c1>>16&0xff), int(c2>>16&0xff))
	g := addSubFull(int(c0>>8&0xff), int(c1>>8&0xff), int(c2>>8&0xff))
	b := addSubFull(int(c0&0xff), int(c1&0xff), int(c2&0xff))

	return a<<24 | r<<16 | g<<8 | b
}

func addSubHalf(a, b int) uint32 {
	return clip255(uint32(a + (a-b)/2))
}

func clampedAddSubtractHalf(c0, c1, c2 uint32) uint32 {
	ave := average2(c0, c1)

	a := addSubHalf(int(ave>>24), int(c2>>24))
	r := addSubHalf(int(ave>>16&0xff), int(c2>>16&0xff))
	g := addSubHalf(int(ave>>8&0xff), int(c2>>8&0xff))
	b := addSubHalf(int(ave&0xff), int(c2&0xff))

	return a<<24 | r<<16 | g<<8 | b
}

func absDiff(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

func sub3(a, b, c int) int {
	return absDiff(b-c) - absDiff(a-c)
}

func selectPredictor(a, b, c uint32) uint32 {
	d := sub3(int(a>>24), int(b>>24), int(c>>24)) +
		sub3(int(a>>16&0xff), int(b>>16&0xff), int(c>>16&0xff)) +
		sub3(int(a>>8&0xff), int(b>>8&0xff), int(c>>8&0xff)) +
		sub3(int(a&0xff), int(b&0xff), int(c&0xff))

	if d <= 0 {
		return a
	}

	return b
}

func predict(mode int, left, topLeft, top, topRight uint32) uint32 {
	switch mode {
	case 1:
		return left
	case 2:
		return top
	case 3:
		return topRight
	case 4:
		return topLeft
	case 5:
		return average3(left, top, topRight)
	case 6:
		return average2(left, topLeft)
	case 7:
		return average2(left, top)
	case 8:
		return average2(topLeft, top)
	case 9:
		return average2(top, topRight)
	case 10:
		return average4(left, topLeft, top, topRight)
	case 11:
		return selectPredictor(top, left, topLeft)
	case 12:
		return clampedAddSubtractFull(left, top, topLeft)
	case 13:
		return clampedAddSubtractHalf(left, top, topLeft)
	}

	return argbBlack
}

func applyPredictor(px []uint32, width, height, bits int, data []uint32) {
	if len(px) == 0 {
		return
	}

	px[0] = addPixels(px[0], argbBlack)

	for x := 1; x < width; x++ {
		px[x] = addPixels(px[x], px[x-1])
	}

	tilesPerRow := subSampleSize(width, bits)

	for y := 1; y < height; y++ {
		base := y * width
		modes := data[(y>>bits)*tilesPerRow:]

		px[base] = addPixels(px[base], px[base-width])

		for x := 1; x < width; x++ {
			i := base + x
			t := i - width
			mode := int(modes[x>>bits] >> 8 & 0xf)
			px[i] = addPixels(px[i], predict(mode, px[i-1], px[t-1], px[t], px[t+1]))
		}
	}
}

func colorDelta(pred, c int8) int {
	return int(pred) * int(c) >> 5
}

func transformColorInverse(code, argb uint32) uint32 {
	green := int8(argb >> 8)

	red := int(argb >> 16 & 0xff)
	red += colorDelta(int8(code), green)
	red &= 0xff

	blue := int(argb & 0xff)
	blue += colorDelta(int8(code>>8), green)
	blue += colorDelta(int8(code>>16), int8(red))
	blue &= 0xff

	return argb&0xff00ff00 | uint32(red)<<16 | uint32(blue)
}

func applyCrossColor(px []uint32, width, height, bits int, data []uint32) {
	tilesPerRow := subSampleSize(width, bits)

	for y := range height {
		base := y * width
		modes := data[(y>>bits)*tilesPerRow:]

		for x := range width {
			px[base+x] = transformColorInverse(modes[x>>bits], px[base+x])
		}
	}
}

func applySubtractGreen(px []uint32) {
	for i, argb := range px {
		g := argb >> 8 & 0xff
		rb := argb & 0x00ff00ff
		rb = (rb + (g<<16 | g)) & 0x00ff00ff

		px[i] = argb&0xff00ff00 | rb
	}
}

func applyColorIndexing(px, out []uint32, width, height, bits int, table []uint32) []uint32 {
	if bits == 0 {
		t := table[:256]

		for i, v := range px {
			px[i] = t[v>>8&0xff]
		}

		return px
	}

	packedWidth := subSampleSize(width, bits)

	row := func(y int) []uint32 { return out[y*width : y*width+width] }
	packed := func(y int) []uint32 { return px[y*packedWidth : y*packedWidth+packedWidth] }

	switch bits {
	case 3:
		var exp [256][8]uint32

		t := table[:2]

		for i := range exp {
			for k := range exp[i] {
				exp[i][k] = t[i>>k&1]
			}
		}

		for y := range height {
			expandIndex8(row(y), packed(y), &exp, t)
		}
	case 2:
		var exp [256][4]uint32

		t := table[:4]

		for i := range exp {
			for k := range exp[i] {
				exp[i][k] = t[i>>(2*k)&3]
			}
		}

		for y := range height {
			expandIndex4(row(y), packed(y), &exp, t)
		}
	default:
		var exp [256][2]uint32

		t := table[:16]

		for i := range exp {
			exp[i][0] = t[i&15]
			exp[i][1] = t[i>>4]
		}

		for y := range height {
			expandIndex2(row(y), packed(y), &exp, t)
		}
	}

	return out
}

func expandIndex8(row, src []uint32, exp *[256][8]uint32, t []uint32) {
	x := 0
	for ; x+8 <= len(row); x += 8 {
		*(*[8]uint32)(row[x:]) = exp[src[x>>3]>>8&0xff]
	}

	for ; x < len(row); x++ {
		row[x] = t[src[x>>3]>>8>>(x&7)&1]
	}
}

func expandIndex4(row, src []uint32, exp *[256][4]uint32, t []uint32) {
	x := 0
	for ; x+4 <= len(row); x += 4 {
		*(*[4]uint32)(row[x:]) = exp[src[x>>2]>>8&0xff]
	}

	for ; x < len(row); x++ {
		row[x] = t[src[x>>2]>>8>>(2*(x&3))&3]
	}
}

func expandIndex2(row, src []uint32, exp *[256][2]uint32, t []uint32) {
	x := 0
	for ; x+2 <= len(row); x += 2 {
		*(*[2]uint32)(row[x:]) = exp[src[x>>1]>>8&0xff]
	}

	if x < len(row) {
		row[x] = t[src[x>>1]>>8&15]
	}
}
