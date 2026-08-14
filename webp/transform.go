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
		for i, v := range px {
			px[i] = table[v>>8&0xff]
		}

		return px
	}

	packedWidth := subSampleSize(width, bits)
	bitsPerPixel := uint(8 >> bits)
	countMask := 1<<bits - 1
	bitMask := uint32(1)<<bitsPerPixel - 1

	for y := range height {
		var packed uint32

		src := y * packedWidth
		dst := y * width

		for x := range width {
			if x&countMask == 0 {
				packed = px[src] >> 8 & 0xff
				src++
			}

			out[dst+x] = table[packed&bitMask]
			packed >>= bitsPerPixel
		}
	}

	return out
}
