package webp

import "math"

const (
	maxHuffImageSize = 1024
	minHistoBits     = 2
	maxHistoBits     = 9
	metaBins         = 64
)

func histoBits(method, xsize, ysize int, palette bool) uint {
	b := 7 - method
	if palette {
		b = 9 - method
	}

	b = min(max(b, minHistoBits), maxHistoBits)

	for b < maxHistoBits && subSampleSize(xsize, b)*subSampleSize(ysize, b) > maxHuffImageSize {
		b++
	}

	return uint(b)
}

func (h *histogram) addRef(r ref, xsize int) {
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

func (h *histogram) add(o *histogram) {
	for i, v := range o.literal {
		h.literal[i] += v
	}

	for i, v := range o.red {
		h.red[i] += v
	}

	for i, v := range o.blue {
		h.blue[i] += v
	}

	for i, v := range o.alpha {
		h.alpha[i] += v
	}

	for i, v := range o.dist {
		h.dist[i] += v
	}
}

func (h *histogram) planes() [treesPerGroup][]uint32 {
	return [treesPerGroup][]uint32{
		h.literal, h.red[:], h.blue[:], h.alpha[:], h.dist[:],
	}
}

func planeCost(p []uint32) float64 {
	sum, used, total := uint32(0), 0, 0.0

	for _, v := range p {
		if v == 0 {
			continue
		}

		used++
		sum += v
		total += shannon(v)
	}

	return shannon(sum) - total + float64(5*used) + 40
}

func planeMergeCost(x, y []uint32) float64 {
	sum, used, total := uint32(0), 0, 0.0

	for i, xi := range x {
		v := xi + y[i]

		if v == 0 {
			continue
		}

		used++
		sum += v
		total += shannon(v)
	}

	return shannon(sum) - total + float64(5*used) + 40
}

func (h *histogram) cost() float64 {
	cost := 0.0

	for _, p := range h.planes() {
		cost += planeCost(p)
	}

	return cost
}

func (e *losslessEncoder) blockHistograms(refs []ref, xsize, ysize int, bits, cacheBits uint) int {
	mw, mh := subSampleSize(xsize, int(bits)), subSampleSize(ysize, int(bits))
	n := mw * mh

	if cap(e.blocks) < n {
		e.blocks = make([]histogram, n)
	}

	e.blocks = e.blocks[:n]

	for i := range e.blocks {
		e.blocks[i].reset(cacheBits)
	}

	x, y := 0, 0

	for _, r := range refs {
		e.blocks[(y>>bits)*mw+(x>>bits)].addRef(r, xsize)

		x += int(r.len)
		for x >= xsize {
			x -= xsize
			y++
		}
	}

	return n
}

func (e *losslessEncoder) clusterBlocks(n int, cacheBits uint) int {
	costs := e.blockCosts[:0]

	lo, hi := math.Inf(1), math.Inf(-1)

	for i := range n {
		c := e.blocks[i].cost()
		costs = append(costs, c)

		lo, hi = min(lo, c), max(hi, c)
	}

	e.blockCosts = costs

	bin := e.blockBins[:0]
	for range metaBins {
		bin = append(bin, -1)
	}

	e.blockBins = bin

	groups := 0

	if cap(e.groupHist) < metaBins {
		e.groupHist = make([]histogram, metaBins)
	}

	e.groupHist = e.groupHist[:0]

	if cap(e.blockGroup) < n {
		e.blockGroup = make([]int, n)
	}

	e.blockGroup = e.blockGroup[:n]

	span := hi - lo

	for i := range n {
		k := 0
		if span > 0 {
			k = min(int(float64(metaBins)*(costs[i]-lo)/span), metaBins-1)
		}

		if bin[k] < 0 {
			bin[k] = groups
			groups++

			e.groupHist = e.groupHist[:groups]
			e.groupHist[groups-1].reset(cacheBits)
		}

		e.blockGroup[i] = bin[k]
		e.groupHist[bin[k]].add(&e.blocks[i])
	}

	return e.mergeGroups(n, groups, cacheBits)
}

func (e *losslessEncoder) mergedCost(a, b int) float64 {
	x, y := e.groupHist[a].planes(), e.groupHist[b].planes()

	cost := 0.0

	for i := range x {
		cost += planeMergeCost(x[i], y[i])
	}

	return cost
}

func (e *losslessEncoder) mergeGroups(n, groups int, cacheBits uint) int {
	cost := e.groupCost[:0]

	for g := range groups {
		cost = append(cost, e.groupHist[g].cost())
	}

	e.groupCost = cost

	if cap(e.pairCost) < metaBins*metaBins {
		e.pairCost = make([]float64, metaBins*metaBins)
	}

	pair := e.pairCost[:metaBins*metaBins]

	for a := range groups {
		for b := a + 1; b < groups; b++ {
			pair[a*metaBins+b] = e.mergedCost(a, b)
		}
	}

	for groups > 1 {
		bestGain, bestA, bestB := 0.0, -1, -1

		for a := range groups {
			for b := a + 1; b < groups; b++ {
				if gain := cost[a] + cost[b] - pair[a*metaBins+b]; gain > bestGain {
					bestGain, bestA, bestB = gain, a, b
				}
			}
		}

		if bestA < 0 {
			break
		}

		e.groupHist[bestA].add(&e.groupHist[bestB])
		cost[bestA] = pair[bestA*metaBins+bestB]

		e.groupHist[bestB], e.groupHist[groups-1] = e.groupHist[groups-1], e.groupHist[bestB]
		cost[bestB] = cost[groups-1]

		groups--

		for i := range n {
			switch e.blockGroup[i] {
			case bestB:
				e.blockGroup[i] = bestA
			case groups:
				e.blockGroup[i] = bestB
			}
		}

		e.groupHist = e.groupHist[:groups]

		for _, g := range [2]int{bestA, bestB} {
			if g >= groups {
				continue
			}

			for o := range groups {
				if o == g {
					continue
				}

				lo, hi := min(g, o), max(g, o)
				pair[lo*metaBins+hi] = e.mergedCost(lo, hi)
			}
		}
	}

	return groups
}

func (e *losslessEncoder) metaImage(n int) []uint32 {
	if cap(e.meta) < n {
		e.meta = make([]uint32, n)
	}

	e.meta = e.meta[:n]

	for i := range n {
		e.meta[i] = argbBlack | uint32(e.blockGroup[i])<<8
	}

	return e.meta
}

func (e *losslessEncoder) metaGroups(refs []ref, xsize, ysize int, cacheBits uint) (uint, int, int) {
	bits := histoBits(e.method, xsize, ysize, e.palettized)
	n := e.blockHistograms(refs, xsize, ysize, bits, cacheBits)

	if n < 2 {
		return bits, 1, 1
	}

	return bits, subSampleSize(xsize, int(bits)), e.clusterBlocks(n, cacheBits)
}

func (e *losslessEncoder) storeGroupCodes(w *lbitWriter, groups int, cacheBits uint) {
	if cap(e.groupCodes) < groups {
		e.groupCodes = make([][treesPerGroup]huffCode, groups)
	}

	e.groupCodes = e.groupCodes[:groups]

	for g := range groups {
		e.buildCodesFrom(&e.groupHist[g], &e.groupCodes[g], cacheBits)
		e.storeCodesFrom(w, &e.groupCodes[g])
	}
}
