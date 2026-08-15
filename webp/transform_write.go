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

var slog2 = func() [1 << 12]float64 {
	var t [1 << 12]float64

	for i := 1; i < len(t); i++ {
		t[i] = float64(i) * math.Log2(float64(i))
	}

	return t
}()

func shannon(v uint32) float64 {
	if v < uint32(len(slog2)) {
		return slog2[v]
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

func crossColorBias(counts []uint32) float64 {
	bits := 3 * float64(counts[0])
	decay := 1.44

	for i := 1; i < 16; i++ {
		bits += decay * float64(counts[i]+counts[256-i])
		decay = decay * 6 / 10
	}

	return -bits / 10
}

func colorDeltaForward(m, c int8) int32 {
	return int32(m) * int32(c) >> 5
}

func transformColorForward(code, argb uint32) uint32 {
	green := int8(argb >> 8)
	red := argb >> 16 & 0xff

	nr := int32(red) - colorDeltaForward(int8(code), green)
	nb := int32(argb&0xff) - colorDeltaForward(int8(code>>8), green) - colorDeltaForward(int8(code>>16), int8(red))

	return argb&0xff00ff00 | uint32(uint8(nr))<<16 | uint32(uint8(nb))
}

type crossSearch struct {
	px           []uint32
	stride       int
	tw, th       int
	histo        [256]uint32
	accRed       [256]uint32
	accBlue      [256]uint32
	prevX, prevY uint32
}

func (s *crossSearch) redCost(g2r int) float64 {
	clear(s.histo[:])

	for y := range s.th {
		row := s.px[y*s.stride:]

		for _, argb := range row[:s.tw] {
			s.histo[uint8(int32(argb>>16&0xff)-colorDeltaForward(int8(g2r), int8(argb>>8)))]++
		}
	}

	cost := combinedEntropy(s.histo[:], s.accRed[:]) + crossColorBias(s.histo[:])

	if uint8(g2r) == uint8(s.prevX) {
		cost -= 3
	}

	if uint8(g2r) == uint8(s.prevY) {
		cost -= 3
	}

	if uint8(g2r) == 0 {
		cost -= 3
	}

	return cost
}

func (s *crossSearch) blueCost(g2b, r2b int) float64 {
	clear(s.histo[:])

	for y := range s.th {
		row := s.px[y*s.stride:]

		for _, argb := range row[:s.tw] {
			v := int32(argb&0xff) - colorDeltaForward(int8(g2b), int8(argb>>8)) - colorDeltaForward(int8(r2b), int8(argb>>16))
			s.histo[uint8(v)]++
		}
	}

	cost := combinedEntropy(s.histo[:], s.accBlue[:]) + crossColorBias(s.histo[:])

	if uint8(g2b) == uint8(s.prevX>>8) {
		cost -= 3
	}

	if uint8(g2b) == uint8(s.prevY>>8) {
		cost -= 3
	}

	if uint8(r2b) == uint8(s.prevX>>16) {
		cost -= 3
	}

	if uint8(r2b) == uint8(s.prevY>>16) {
		cost -= 3
	}

	if uint8(g2b) == 0 {
		cost -= 3
	}

	if uint8(r2b) == 0 {
		cost -= 3
	}

	return cost
}

var crossAxes = [8][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}, {-1, -1}, {-1, 1}, {1, -1}, {1, 1}}

var crossDeltas = [7]int{16, 16, 8, 4, 2, 2, 2}

func (s *crossSearch) best() uint32 {
	g2r := 0
	cost := s.redCost(g2r)

	for iter := range 6 {
		delta := 32 >> iter

		for _, off := range [2]int{-delta, delta} {
			if c := s.redCost(g2r + off); c < cost {
				cost, g2r = c, g2r+off
			}
		}
	}

	g2b, r2b := 0, 0
	cost = s.blueCost(g2b, r2b)

	for _, delta := range crossDeltas {
		for _, axis := range crossAxes {
			cg, cr := g2b+axis[0]*delta, r2b+axis[1]*delta

			if c := s.blueCost(cg, cr); c < cost {
				cost, g2b, r2b = c, cg, cr
			}
		}

		if delta == 2 && g2b == 0 && r2b == 0 {
			break
		}
	}

	return argbBlack | uint32(uint8(r2b))<<16 | uint32(uint8(g2b))<<8 | uint32(uint8(g2r))
}

func crossColorImage(px []uint32, width, height, bits int, codes []uint32) float64 {
	var red, blue [256]uint32

	redBlueHistograms(px, width, &red, &blue)

	before := entropy(red[:]) + entropy(blue[:])

	tilesPerRow := subSampleSize(width, bits)

	var s crossSearch

	for ty := range subSampleSize(height, bits) {
		for tx := range tilesPerRow {
			x0, y0 := tx<<bits, ty<<bits
			x1, y1 := min(x0+1<<bits, width), min(y0+1<<bits, height)

			s.px = px[y0*width+x0:]
			s.stride = width
			s.tw, s.th = x1-x0, y1-y0

			s.prevX, s.prevY = argbBlack, argbBlack

			if tx > 0 {
				s.prevX = codes[ty*tilesPerRow+tx-1]
			}

			if ty > 0 {
				s.prevY = codes[(ty-1)*tilesPerRow+tx]
			}

			code := s.best()
			codes[ty*tilesPerRow+tx] = code

			for y := y0; y < y1; y++ {
				row := px[y*width : y*width+x1]

				for x := x0; x < x1; x++ {
					row[x] = transformColorForward(code, row[x])
				}
			}

			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					i := y*width + x

					if x >= 2 && px[i] == px[i-2] && px[i] == px[i-1] {
						continue
					}

					if i >= width+2 && px[i-2] == px[i-width-2] && px[i-1] == px[i-width-1] && px[i] == px[i-width] {
						continue
					}

					s.accRed[px[i]>>16&0xff]++
					s.accBlue[px[i]&0xff]++
				}
			}
		}
	}

	return before - entropy(s.accRed[:]) - entropy(s.accBlue[:])
}

func redBlueHistograms(px []uint32, width int, red, blue *[256]uint32) {
	for i, v := range px {
		if i >= 2 && v == px[i-2] && v == px[i-1] {
			continue
		}

		if i >= width+2 && px[i-2] == px[i-width-2] && px[i-1] == px[i-width-1] && v == px[i-width] {
			continue
		}

		red[v>>16&0xff]++
		blue[v&0xff]++
	}
}

func redBlueAlwaysZero(px []uint32) bool {
	for _, v := range px {
		if v&0x00ff00ff != 0 {
			return false
		}
	}

	return true
}
