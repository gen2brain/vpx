package webp

import (
	"math"
	"slices"
)

const (
	numPredModes    = 14
	lowEffortPred   = 11
	maxPaletteSize  = 256
	predictorBias   = 15
	maxTransformBit = 6
)

func subPixels(a, b uint32) uint32 {
	alphaGreen := 0x00ff00ff + (a & 0xff00ff00) - (b & 0xff00ff00)
	redBlue := 0xff00ff00 + (a & 0x00ff00ff) - (b & 0x00ff00ff)

	return alphaGreen&0xff00ff00 | redBlue&0x00ff00ff
}

func subtractGreenForward(px []uint32) {
	for i, argb := range px {
		green := argb >> 8 & 0xff
		red := (argb>>16 - green) & 0xff
		blue := (argb - green) & 0xff

		px[i] = argb&0xff00ff00 | red<<16 | blue
	}
}

func shannon(v uint32) float64 {
	if v == 0 {
		return 0
	}

	return float64(v) * math.Log2(float64(v))
}

func entropy(h []uint32) float64 {
	sum := uint32(0)
	total := 0.0

	for _, v := range h {
		sum += v
		total += shannon(v)
	}

	return shannon(sum) - total
}

func combinedEntropy(x, y []uint32) float64 {
	var sumX, sumXY uint32

	total := 0.0

	for i, xi := range x {
		yi := y[i]

		switch {
		case xi != 0:
			sumX += xi
			sumXY += xi + yi
			total -= shannon(xi) + shannon(xi+yi)
		case yi != 0:
			sumXY += yi
			total -= shannon(yi)
		}
	}

	return total + shannon(sumX) + shannon(sumXY)
}

func predictionBias(counts []uint32) float64 {
	bits := float64(counts[0])
	decay := 94.0

	for i := 1; i < 16; i++ {
		bits += decay * float64(counts[i]+counts[256-i]) / 100
		decay = decay * 6 / 10
	}

	return -bits / 10
}

type histoARGB [4 * 256]uint32

func (h *histoARGB) add(argb uint32) {
	h[argb>>24]++
	h[256+(argb>>16&0xff)]++
	h[512+(argb>>8&0xff)]++
	h[768+(argb&0xff)]++
}

func (h *histoARGB) cost(acc *histoARGB) float64 {
	total := 0.0

	for i := range 4 {
		total += predictionBias(h[i*256 : i*256+256])
		total += combinedEntropy(h[i*256:i*256+256], acc[i*256:i*256+256])
	}

	return total
}

func (h *histoARGB) addTo(acc *histoARGB) {
	for i, v := range h {
		acc[i] += v
	}
}

func predictorAt(px []uint32, width, x, y, mode int) uint32 {
	if y == 0 {
		if x == 0 {
			return argbBlack
		}

		return px[x-1]
	}

	i := y * width

	if x == 0 {
		return px[i-width]
	}

	top := i + x - width
	right := top + 1

	if x == width-1 {
		right = i
	}

	return predict(mode, px[i+x-1], px[top-1], px[top], px[right])
}

func bestPredictor(px []uint32, width, height, bits, tx, ty int, modes []uint32, tilesPerRow int, acc *histoARGB) int {
	x0, y0 := tx<<bits, ty<<bits
	x1, y1 := min(x0+1<<bits, width), min(y0+1<<bits, height)

	left, above := 0xff, 0xff

	if tx > 0 {
		left = int(modes[ty*tilesPerRow+tx-1] >> 8 & 0xff)
	}

	if ty > 0 {
		above = int(modes[(ty-1)*tilesPerRow+tx] >> 8 & 0xff)
	}

	var best histoARGB

	bestCost := math.Inf(1)
	bestMode := 0

	for mode := range numPredModes {
		var h histoARGB

		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				h.add(subPixels(px[y*width+x], predictorAt(px, width, x, y, mode)))
			}
		}

		cost := h.cost(acc)

		if mode == left {
			cost -= predictorBias
		}

		if mode == above {
			cost -= predictorBias
		}

		if cost < bestCost {
			bestCost = cost
			bestMode = mode
			best = h
		}
	}

	best.addTo(acc)

	return bestMode
}

func residualImage(px []uint32, width, height, bits int, lowEffort, exact bool, modes []uint32, rows []uint32) {
	tilesPerRow := subSampleSize(width, bits)
	tilesPerCol := subSampleSize(height, bits)

	if lowEffort {
		for i := range modes {
			modes[i] = argbBlack | lowEffortPred<<8
		}
	} else {
		var acc histoARGB

		for ty := range tilesPerCol {
			for tx := range tilesPerRow {
				mode := bestPredictor(px, width, height, bits, tx, ty, modes, tilesPerRow, &acc)
				modes[ty*tilesPerRow+tx] = argbBlack | uint32(mode)<<8
			}
		}
	}

	upper, current := rows[:width+1], rows[width+1:]

	for y := range height {
		upper, current = current, upper

		n := width
		if y+1 < height {
			n++
		}

		copy(current, px[y*width:y*width+n])

		for x := range width {
			var pred uint32

			switch {
			case y == 0:
				pred = argbBlack

				if x > 0 {
					pred = current[x-1]
				}
			case x == 0:
				pred = upper[0]
			default:
				mode := int(modes[(y>>bits)*tilesPerRow+(x>>bits)] >> 8 & 0xff)
				pred = predict(mode, current[x-1], upper[x-1], upper[x], upper[x+1])
			}

			residual := subPixels(current[x], pred)

			if !exact && current[x]&argbBlack == 0 {
				residual &= argbBlack
				current[x] = pred &^ argbBlack

				if x == 0 && y != 0 {
					upper[width] = current[0]
				}
			}

			px[y*width+x] = residual
		}
	}
}

const paletteSlots = 1 << 9

type paletteMap struct {
	keys  [paletteSlots]uint32
	index [paletteSlots]uint16
	used  [paletteSlots]bool
}

func (p *paletteMap) slot(v uint32) uint32 {
	h := v * 0x1e35a7bd >> (32 - 9)

	for p.used[h] && p.keys[h] != v {
		h = (h + 1) & (paletteSlots - 1)
	}

	return h
}

func (p *paletteMap) build(px []uint32, out []uint32) ([]uint32, bool) {
	clear(p.used[:])

	out = out[:0]

	for _, v := range px {
		h := p.slot(v)

		if p.used[h] {
			continue
		}

		if len(out) == maxPaletteSize {
			return out, false
		}

		p.used[h] = true
		p.keys[h] = v
		out = append(out, v)
	}

	slices.Sort(out)

	for i, v := range out {
		p.index[p.slot(v)] = uint16(i)
	}

	return out, len(out) > 0
}

func paletteBits(size int) int {
	switch {
	case size <= 2:
		return 3
	case size <= 4:
		return 2
	case size <= 16:
		return 1
	}

	return 0
}

func (p *paletteMap) mapToPalette(px, out []uint32, width, height, bits int) {
	packedWidth := subSampleSize(width, bits)
	perPixel := uint(8 >> bits)
	mask := 1<<bits - 1

	for y := range height {
		src := y * width
		dst := y * packedWidth

		var packed uint32

		for x := range width {
			packed |= uint32(p.index[p.slot(px[src+x])]) << (uint(x&mask) * perPixel)

			if x&mask == mask || x == width-1 {
				out[dst] = argbBlack | packed<<8
				dst++
				packed = 0
			}
		}
	}
}

func paletteDeltas(palette, out []uint32) []uint32 {
	out = out[:len(palette)]

	for i := len(palette) - 1; i > 0; i-- {
		out[i] = subPixels(palette[i], palette[i-1])
	}

	out[0] = palette[0]

	return out
}
