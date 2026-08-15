package webp

import (
	"math"
	"slices"
)

const (
	hashBits       = 18
	hashSize       = 1 << hashBits
	maxLengthBits  = 12
	maxLength      = 1<<maxLengthBits - 1
	windowSizeBits = 20
	windowSize     = 1<<windowSizeBits - 120
	minLength      = 4
	maxCacheBits   = 10
)

const (
	hashMulHi = 0xc6a4a793
	hashMulLo = 0x5bd1e996
)

const (
	refLiteral = iota
	refCache
	refCopy
)

type ref struct {
	value uint32
	len   uint16
	mode  uint8
}

func literalRef(argb uint32) ref {
	return ref{value: argb, len: 1, mode: refLiteral}
}

func cacheRef(key uint32) ref {
	return ref{value: key, len: 1, mode: refCache}
}

func copyRef(dist uint32, length int) ref {
	return ref{value: dist, len: uint16(length), mode: refCopy}
}

func pixPairHash(a, b uint32, shift uint) uint32 {
	return (b*hashMulHi + a*hashMulLo) >> shift
}

func matchLengthLong(a, b []uint32, limit int) int {
	if matchLengthAsm != nil && limit > 0 && len(a) >= limit && len(b) >= limit {
		return matchLengthAsm(a, b, limit)
	}

	return matchLength(a, b, limit)
}

func matchLength(a, b []uint32, limit int) int {
	a, b = a[:limit], b[:limit:limit]

	for i := range a {
		if a[i] != b[i] {
			return i
		}
	}

	return limit
}

func matchLengthFrom(a, b []uint32, best, limit int) int {
	if a[best] != b[best] {
		return 0
	}

	return matchLength(a, b, limit)
}

func copyLimit(n int) int {
	return min(n, maxLength)
}

type hashChain struct {
	offsetLength []uint32
	firstIndex   []int32
	shift        uint
}

func (h *hashChain) sizeTable(size int) {
	bits := 8
	for bits < hashBits && 1<<bits < 2*size {
		bits++
	}

	h.shift = 32 - uint(bits)

	if cap(h.firstIndex) < 1<<bits {
		h.firstIndex = make([]int32, 1<<bits)
	}

	h.firstIndex = h.firstIndex[:1<<bits]

	for i := range h.firstIndex {
		h.firstIndex[i] = -1
	}
}

func (h *hashChain) findOffset(pos int) int {
	return int(h.offsetLength[pos] >> maxLengthBits)
}

func (h *hashChain) findLength(pos int) int {
	return int(h.offsetLength[pos] & maxLength)
}

func maxItersForQuality(quality int) int {
	return 8 + quality*quality/128
}

func windowForQuality(quality, xsize int) int {
	var w int

	switch {
	case quality > 75:
		w = windowSize
	case quality > 50:
		w = xsize << 8
	case quality > 25:
		w = xsize << 6
	default:
		w = xsize << 4
	}

	return min(w, windowSize)
}

func (h *hashChain) fill(argb []uint32, xsize, quality int, lowEffort bool) {
	size := len(argb)

	h.offsetLength = grow(h.offsetLength, size)

	if size <= 2 {
		h.offsetLength[0] = 0
		h.offsetLength[size-1] = 0

		return
	}

	h.sizeTable(size)

	chain := h.offsetLength
	same := argb[0] == argb[1]
	pos := 0

	for pos < size-2 {
		sameNext := argb[pos+1] == argb[pos+2]

		if same && sameNext {
			n := 1

			for pos+n+2 < size && argb[pos+n+2] == argb[pos] {
				n++
			}

			if n > maxLength {
				for k := range n - maxLength {
					chain[pos+k] = ^uint32(0)
				}

				pos += n - maxLength
				n = maxLength
			}

			for n > 0 {
				hash := pixPairHash(argb[pos], uint32(n), h.shift)
				n--

				chain[pos] = uint32(h.firstIndex[hash])
				h.firstIndex[hash] = int32(pos)
				pos++
			}

			same = false

			continue
		}

		hash := pixPairHash(argb[pos], argb[pos+1], h.shift)
		chain[pos] = uint32(h.firstIndex[hash])
		h.firstIndex[hash] = int32(pos)
		pos++
		same = sameNext
	}

	chain[pos] = uint32(h.firstIndex[pixPairHash(argb[pos], argb[pos+1], h.shift)])

	iterMax := maxItersForQuality(quality)
	window := windowForQuality(quality, xsize)

	h.offsetLength[0] = 0
	h.offsetLength[size-1] = 0

	for base := size - 2; base > 0; {
		maxLen := copyLimit(size - 1 - base)
		start := argb[base:]
		iter := iterMax
		bestLen := 0
		bestDist := 0
		minPos := max(base-window, 0)
		lengthMax := min(maxLen, 256)

		p := int32(chain[base])

		if !lowEffort {
			if base >= xsize {
				if n := matchLengthFrom(argb[base-xsize:], start, bestLen, maxLen); n > bestLen {
					bestLen = n
					bestDist = xsize
				}

				iter--
			}

			if n := matchLengthFrom(argb[base-1:], start, bestLen, maxLen); n > bestLen {
				bestLen = n
				bestDist = 1
			}

			iter--

			if bestLen == maxLength {
				p = int32(minPos) - 1
			}
		}

		bestArgb := start[bestLen]

		for ; p >= int32(minPos); p = int32(chain[p]) {
			iter--
			if iter == 0 {
				break
			}

			if argb[int(p)+bestLen] != bestArgb {
				continue
			}

			n := matchLength(argb[p:], start, maxLen)
			if n > bestLen {
				bestLen = n
				bestDist = base - int(p)
				bestArgb = start[bestLen]

				if bestLen >= lengthMax {
					break
				}
			}
		}

		maxBase := base

		for {
			h.offsetLength[base] = uint32(bestDist)<<maxLengthBits | uint32(bestLen)
			base--

			if bestDist == 0 || base == 0 {
				break
			}

			if base < bestDist || argb[base-bestDist] != argb[base] {
				break
			}

			if bestLen == maxLength && bestDist != 1 && base+maxLength < maxBase {
				break
			}

			if bestLen < maxLength {
				bestLen++
				maxBase = base
			}
		}
	}
}

func applyCache(refs []ref, argb []uint32, cache *colorCache) {
	i := 0

	for k := range refs {
		r := &refs[k]

		if r.mode == refCopy {
			for _, v := range argb[i : i+int(r.len)] {
				cache.insert(v)
			}

			i += int(r.len)

			continue
		}

		if key := cache.index(r.value); cache.data[key] == r.value {
			*r = cacheRef(key)
		} else {
			cache.data[key] = r.value
		}

		i++
	}
}

func backwardRefsRle(argb []uint32, xsize int, refs []ref) []ref {
	refs = append(refs[:0], literalRef(argb[0]))

	for i := 1; i < len(argb); {
		maxLen := copyLimit(len(argb) - i)
		rle := matchLengthLong(argb[i:], argb[i-1:], maxLen)

		prevRow := 0
		if i >= xsize {
			prevRow = matchLengthLong(argb[i:], argb[i-xsize:], maxLen)
		}

		switch {
		case rle >= prevRow && rle >= minLength:
			refs = append(refs, copyRef(1, rle))
			i += rle
		case prevRow >= minLength:
			refs = append(refs, copyRef(uint32(xsize), prevRow))

			i += prevRow
		default:
			refs = append(refs, literalRef(argb[i]))
			i++
		}
	}

	return refs
}

func backwardRefsLz77(argb []uint32, chain *hashChain, refs []ref) []ref {
	refs = refs[:0]
	n := len(argb)
	lastCheck := -1

	for i := 0; i < n; {
		offset := chain.findOffset(i)
		length := chain.findLength(i)

		if length >= minLength {
			reachMax := 0
			jMax := min(i+length, n-1)
			lastCheck = max(lastCheck, i)

			for j := lastCheck + 1; j <= jMax; j++ {
				lenJ := chain.findLength(j)
				step := 1

				if lenJ >= minLength {
					step = lenJ
				}

				if reach := j + step; reach > reachMax {
					length = j - i
					reachMax = reach

					if reachMax >= n {
						break
					}
				}
			}
		} else {
			length = 1
		}

		if length == 1 {
			refs = append(refs, literalRef(argb[i]))
		} else {
			refs = append(refs, copyRef(uint32(offset), length))
		}

		i += length
	}

	return refs
}

type costModel struct {
	literal []float64
	red     [numLiteralCodes]float64
	blue    [numLiteralCodes]float64
	alpha   [numLiteralCodes]float64
	dist    [numDistanceCodes]float64
	lengths []float64
}

func bitEstimates(counts []uint32, out []float64) {
	sum, nonzero := uint32(0), 0

	for _, c := range counts {
		sum += c

		if c > 0 {
			nonzero++
		}
	}

	if nonzero <= 1 {
		clear(out)

		return
	}

	total := math.Log2(float64(sum))

	for i, c := range counts {
		if c == 0 {
			out[i] = total

			continue
		}

		out[i] = total - math.Log2(float64(c))
	}
}

func (m *costModel) build(h *histogram, cacheBits uint) {
	n := numLiteralCodes + numLengthCodes

	if cacheBits > 0 {
		n += 1 << cacheBits
	}

	if cap(m.literal) < n {
		m.literal = make([]float64, n)
	}

	m.literal = m.literal[:n]

	bitEstimates(h.literal, m.literal)
	bitEstimates(h.red[:], m.red[:])
	bitEstimates(h.blue[:], m.blue[:])
	bitEstimates(h.alpha[:], m.alpha[:])
	bitEstimates(h.dist[:], m.dist[:])

	if cap(m.lengths) < maxLength+1 {
		m.lengths = make([]float64, maxLength+1)
	}

	m.lengths = m.lengths[:maxLength+1]

	for l := 1; l <= maxLength; l++ {
		code, n, _ := prefixEncode(l)
		m.lengths[l] = m.literal[numLiteralCodes+code] + float64(n)
	}
}

func (m *costModel) literalCost(v uint32) float64 {
	return m.alpha[v>>24] + m.red[v>>16&0xff] + m.literal[v>>8&0xff] + m.blue[v&0xff]
}

func (m *costModel) cacheCost(key uint32) float64 {
	return m.literal[numLiteralCodes+numLengthCodes+int(key)]
}

func (m *costModel) distanceCost(planeCode int) float64 {
	code, n, _ := prefixEncode(planeCode)

	return m.dist[code] + float64(n)
}

const (
	costFudge   = 68065.0 / 65536
	costMaxSpan = 256
)

func backwardRefsCost(argb []uint32, xsize int, chain *hashChain, cacheBits uint,
	m *costModel, cache *colorCache, cost []float64, dist []uint16, path []int, refs []ref,
) ([]ref, []int) {
	n := len(argb)

	for i := range cost[:n] {
		cost[i] = math.Inf(1)
	}

	useCache := cacheBits > 0
	if useCache {
		cache.init(cacheBits)
	}

	literal := func(i int, prev float64) {
		v := argb[i]
		c := prev

		if key := cache.index(v); useCache && cache.data[key] == v {
			c += m.cacheCost(key) * costFudge
		} else {
			if useCache {
				cache.insert(v)
			}

			c += m.literalCost(v) * costFudge
		}

		if c < cost[i] {
			cost[i] = c
			dist[i] = 1
		}
	}

	literal(0, 0)

	for i := 1; i < n; i++ {
		prev := cost[i-1]

		literal(i, prev)

		length := chain.findLength(i)
		if length < minLength {
			continue
		}

		base := prev + m.distanceCost(distanceToPlaneCode(xsize, chain.findOffset(i)))

		span := min(length, costMaxSpan)

		for k := 1; k < span; k++ {
			if c := base + m.lengths[k+1]; c < cost[i+k] {
				cost[i+k] = c
				dist[i+k] = uint16(k + 1)
			}
		}

		if length > span {
			if c := base + m.lengths[length]; c < cost[i+length-1] {
				cost[i+length-1] = c
				dist[i+length-1] = uint16(length)
			}
		}
	}

	path = path[:0]

	for i := n - 1; i >= 0; {
		l := int(dist[i])
		path = append(path, l)
		i -= l
	}

	slices.Reverse(path)

	refs = refs[:0]
	i := 0

	for _, l := range path {
		if l == 1 {
			refs = append(refs, literalRef(argb[i]))
		} else {
			refs = append(refs, copyRef(uint32(chain.findOffset(i)), l))
		}

		i += l
	}

	return refs, path
}
