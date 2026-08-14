package webp

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
		rle := matchLength(argb[i:], argb[i-1:], maxLen)

		prevRow := 0
		if i >= xsize {
			prevRow = matchLength(argb[i:], argb[i-xsize:], maxLen)
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
