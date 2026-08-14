package webp

import "math/bits"

type histogram struct {
	literal []uint32
	red     [numLiteralCodes]uint32
	blue    [numLiteralCodes]uint32
	alpha   [numLiteralCodes]uint32
	dist    [numDistanceCodes]uint32
}

func (h *histogram) reset(cacheBits uint) {
	n := numLiteralCodes + numLengthCodes

	if cacheBits > 0 {
		n += 1 << cacheBits
	}

	if cap(h.literal) < n {
		h.literal = make([]uint32, n)
	}

	h.literal = h.literal[:n]

	clear(h.literal)
	clear(h.red[:])
	clear(h.blue[:])
	clear(h.alpha[:])
	clear(h.dist[:])
}

func prefixEncode(v int) (code int, nbits uint, extra uint32) {
	d := v - 1

	if d < 4 {
		return d, 0, 0
	}

	high := bits.Len32(uint32(d)) - 1
	second := d >> (high - 1) & 1

	nbits = uint(high - 1)

	return 2*high + second, nbits, uint32(d) & (1<<nbits - 1)
}

func distanceToPlaneCode(xsize, dist int) int {
	y := dist / xsize
	x := dist - y*xsize

	if x <= 8 && y < 8 {
		return int(planeToCode[y*16+8-x]) + 1
	}

	if x > xsize-8 && y < 7 {
		return int(planeToCode[(y+1)*16+8+xsize-x]) + 1
	}

	return dist + codeToPlaneCodes
}

func (h *histogram) storeRefs(refs []ref, xsize int) {
	for _, r := range refs {
		switch r.mode {
		case refLiteral:
			h.literal[r.value>>8&0xff]++
			h.red[r.value>>16&0xff]++
			h.blue[r.value&0xff]++
			h.alpha[r.value>>24]++
		case refCache:
			h.literal[numLiteralCodes+numLengthCodes+int(r.value)]++
		default:
			code, _, _ := prefixEncode(int(r.len))
			h.literal[numLiteralCodes+code]++

			code, _, _ = prefixEncode(distanceToPlaneCode(xsize, int(r.value)))
			h.dist[code]++
		}
	}
}

type losslessEncoder struct {
	chain   hashChain
	builder huffBuilder
	hist    histogram
	refs    []ref
	alt     []ref
	codes   [treesPerGroup]huffCode
	cache   colorCache
	pmap    paletteMap
	palette []uint32
	deltas  []uint32
	packed  []uint32
	modes   []uint32
	rows    []uint32
	out     []byte
}

func (e *losslessEncoder) release() {
	e.refs = e.refs[:0]
	e.alt = e.alt[:0]
	e.out = e.out[:0]
}

func (e *losslessEncoder) alphabets(cacheBits uint) [treesPerGroup]int {
	sizes := alphabetSize

	if cacheBits > 0 {
		sizes[greenTree] += 1 << cacheBits
	}

	return sizes
}

func (e *losslessEncoder) buildCodes(cacheBits uint) {
	sizes := e.alphabets(cacheBits)
	hists := [treesPerGroup][]uint32{
		e.hist.literal, e.hist.red[:], e.hist.blue[:], e.hist.alpha[:], e.hist.dist[:],
	}

	for i, n := range sizes {
		c := &e.codes[i]

		if cap(c.lengths) < n {
			c.lengths = make([]uint8, n)
			c.codes = make([]uint16, n)
		}

		c.lengths, c.codes = c.lengths[:n], c.codes[:n]

		e.builder.build(hists[i][:n], maxCodeLength, c)
	}
}

func (e *losslessEncoder) storeCodes(w *lbitWriter) {
	for i := range e.codes {
		e.builder.storeCode(w, &e.codes[i])
		clearIfSingle(&e.codes[i])
	}
}

func writeSymbol(w *lbitWriter, c *huffCode, s int) {
	w.write(uint32(c.codes[s]), uint(c.lengths[s]))
}

func (e *losslessEncoder) storeImage(w *lbitWriter, refs []ref, xsize int) {
	for _, r := range refs {
		switch r.mode {
		case refLiteral:
			writeSymbol(w, &e.codes[greenTree], int(r.value>>8&0xff))
			writeSymbol(w, &e.codes[redTree], int(r.value>>16&0xff))
			writeSymbol(w, &e.codes[blueTree], int(r.value&0xff))
			writeSymbol(w, &e.codes[alphaTree], int(r.value>>24))
		case refCache:
			writeSymbol(w, &e.codes[greenTree], numLiteralCodes+numLengthCodes+int(r.value))
		default:
			code, n, extra := prefixEncode(int(r.len))

			writeSymbol(w, &e.codes[greenTree], numLiteralCodes+code)
			w.write(extra, n)

			code, n, extra = prefixEncode(distanceToPlaneCode(xsize, int(r.value)))

			writeSymbol(w, &e.codes[distTree], code)
			w.write(extra, n)
		}
	}
}

func refsExtraBits(refs []ref, xsize int) int {
	n := 0

	for _, r := range refs {
		if r.mode != refCopy {
			continue
		}

		_, ln, _ := prefixEncode(int(r.len))
		_, dn, _ := prefixEncode(distanceToPlaneCode(xsize, int(r.value)))

		n += int(ln + dn)
	}

	return n
}

func (e *losslessEncoder) codeCost() int {
	hists := [treesPerGroup][]uint32{
		e.hist.literal, e.hist.red[:], e.hist.blue[:], e.hist.alpha[:], e.hist.dist[:],
	}

	cost := 0

	for i, c := range e.codes {
		for s, l := range c.lengths {
			cost += int(l) * int(hists[i][s])
		}
	}

	return cost
}

func (e *losslessEncoder) refsCost(refs []ref, xsize int) int {
	e.hist.reset(0)
	e.hist.storeRefs(refs, xsize)
	e.buildCodes(0)

	return e.codeCost() + refsExtraBits(refs, xsize)
}

func (e *losslessEncoder) bestRefs(argb []uint32, xsize, quality int, lowEffort bool) []ref {
	e.chain.fill(argb, xsize, quality, lowEffort)

	e.refs = backwardRefsLz77(argb, &e.chain, e.refs)

	if lowEffort {
		return e.refs
	}

	e.alt = backwardRefsRle(argb, xsize, e.alt)

	if e.refsCost(e.alt, xsize) < e.refsCost(e.refs, xsize) {
		e.refs, e.alt = e.alt, e.refs
	}

	return e.refs
}

func (e *losslessEncoder) cacheCost(argb []uint32, refs []ref, cacheBits uint) float64 {
	e.hist.reset(cacheBits)

	if cacheBits > 0 {
		e.cache.init(cacheBits)
	}

	i := 0

	for _, r := range refs {
		if r.mode == refCopy {
			code, _, _ := prefixEncode(int(r.len))
			e.hist.literal[numLiteralCodes+code]++

			if cacheBits > 0 {
				for _, v := range argb[i : i+int(r.len)] {
					e.cache.insert(v)
				}
			}

			i += int(r.len)

			continue
		}

		v := r.value
		i++

		if cacheBits > 0 {
			key := e.cache.index(v)

			if e.cache.data[key] == v {
				e.hist.literal[numLiteralCodes+numLengthCodes+int(key)]++

				continue
			}

			e.cache.data[key] = v
		}

		e.hist.literal[v>>8&0xff]++
		e.hist.red[v>>16&0xff]++
		e.hist.blue[v&0xff]++
		e.hist.alpha[v>>24]++
	}

	return entropy(e.hist.literal) + entropy(e.hist.red[:]) +
		entropy(e.hist.blue[:]) + entropy(e.hist.alpha[:])
}

func (e *losslessEncoder) bestCacheBits(argb []uint32, refs []ref) uint {
	best := uint(0)
	bestCost := e.cacheCost(argb, refs, 0)

	for bits := uint(1); bits <= maxCacheBits; bits++ {
		if cost := e.cacheCost(argb, refs, bits); cost < bestCost {
			bestCost = cost
			best = bits
		}
	}

	return best
}

func (e *losslessEncoder) encodeImage(w *lbitWriter, argb []uint32, xsize, quality int, lowEffort, top bool) {
	refs := e.bestRefs(argb, xsize, quality, lowEffort)

	cacheBits := uint(0)

	if top && !lowEffort {
		if cacheBits = e.bestCacheBits(argb, refs); cacheBits > 0 {
			e.cache.init(cacheBits)
			applyCache(refs, argb, &e.cache)
		}
	}

	if cacheBits > 0 {
		w.write(1, 1)
		w.write(uint32(cacheBits), 4)
	} else {
		w.write(0, 1)
	}

	if top {
		w.write(0, 1)
	}

	e.hist.reset(cacheBits)
	e.hist.storeRefs(refs, xsize)
	e.buildCodes(cacheBits)
	e.storeCodes(w)
	e.storeImage(w, refs, xsize)
}

func putTransform(w *lbitWriter, kind int) {
	w.write(1, 1)
	w.write(uint32(kind), 2)
}

func transformBits(method int) int {
	switch {
	case method < 4:
		return maxTransformBit
	case method > 4:
		return 4
	}

	return 5
}

func (e *losslessEncoder) encode(argb []uint32, width, height int, o Options) []byte {
	var w lbitWriter

	w.init(e.out)

	w.write(vp8lSignature, 8)
	w.write(uint32(width-1), 14)
	w.write(uint32(height-1), 14)
	w.write(b2u(hasAlpha(argb)), 1)
	w.write(vp8lVersion, 3)

	e.encodeStream(&w, argb, width, height, o)

	e.out = w.flush()

	return e.out
}

func (e *losslessEncoder) encodeAlpha(argb []uint32, width, height int, o Options) []byte {
	var w lbitWriter

	w.init(e.out)

	w.write(alphaLossless, 8)

	e.encodeStream(&w, argb, width, height, o)

	e.out = w.flush()

	return e.out
}

func (e *losslessEncoder) encodeStream(w *lbitWriter, argb []uint32, width, height int, o Options) {
	lowEffort := o.Method == 0
	xsize := width

	palette, usePalette := e.pmap.build(argb, e.palette)
	e.palette = palette

	if usePalette {
		bits := paletteBits(len(palette))

		putTransform(w, colorIndexingTransform)
		w.write(uint32(len(palette)-1), 8)

		e.deltas = grow(e.deltas, len(palette))
		e.encodeImage(w, paletteDeltas(palette, e.deltas), len(palette), o.Quality, lowEffort, false)

		xsize = subSampleSize(width, bits)
		e.packed = grow(e.packed, xsize*height)

		e.pmap.mapToPalette(argb, e.packed, width, height, bits)

		argb = e.packed
	} else {
		putTransform(w, subtractGreenTransform)

		subtractGreenForward(argb)

		bits := transformBits(o.Method)
		tw, th := subSampleSize(width, bits), subSampleSize(height, bits)

		putTransform(w, predictorTransform)
		w.write(uint32(bits-2), 3)

		e.modes = grow(e.modes, tw*th)
		e.rows = grow(e.rows, 2*(width+1))

		residualImage(argb, width, height, bits, lowEffort, o.Exact, e.modes, e.rows)
		e.encodeImage(w, e.modes, tw, o.Quality, lowEffort, false)
	}

	w.write(0, 1)

	e.encodeImage(w, argb, xsize, o.Quality, lowEffort, true)
}

func hasAlpha(argb []uint32) bool {
	for _, v := range argb {
		if v>>24 != 0xff {
			return true
		}
	}

	return false
}
