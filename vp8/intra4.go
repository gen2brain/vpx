package vp8

import "math"

var bitCost = func() [257]uint16 {
	var t [257]uint16

	for p := 1; p <= 256; p++ {
		t[p] = uint16(-math.Log2(float64(p)/256) * 256)
	}

	t[0] = t[1]

	return t
}()

func probCost(p uint8, bit int) int {
	if bit == 0 {
		return int(bitCost[p])
	}

	return int(bitCost[256-int(p)])
}

type bModeStep struct{ idx, bit uint8 }

var bModePath = [numBModes][]bModeStep{
	bDCPred: {{0, 0}},
	bTMPred: {{0, 1}, {1, 0}},
	bVEPred: {{0, 1}, {1, 1}, {2, 0}},
	bHEPred: {{0, 1}, {1, 1}, {2, 1}, {3, 0}, {4, 0}},
	bRDPred: {{0, 1}, {1, 1}, {2, 1}, {3, 0}, {4, 1}, {5, 0}},
	bVRPred: {{0, 1}, {1, 1}, {2, 1}, {3, 0}, {4, 1}, {5, 1}},
	bLDPred: {{0, 1}, {1, 1}, {2, 1}, {3, 1}, {6, 0}},
	bVLPred: {{0, 1}, {1, 1}, {2, 1}, {3, 1}, {6, 1}, {7, 0}},
	bHDPred: {{0, 1}, {1, 1}, {2, 1}, {3, 1}, {6, 1}, {7, 1}, {8, 0}},
	bHUPred: {{0, 1}, {1, 1}, {2, 1}, {3, 1}, {6, 1}, {7, 1}, {8, 1}},
}

func putBMode(w *boolEnc, p *[numBModes - 1]uint8, mode uint8) {
	for _, s := range bModePath[mode] {
		w.putBit(int(s.bit), p[s.idx])
	}
}

func bModeCost(p *[numBModes - 1]uint8, mode uint8) int {
	cost := 0

	for _, s := range bModePath[mode] {
		cost += probCost(p[s.idx], int(s.bit))
	}

	return cost
}

func largeValueCost(p *[numProbas]uint8, v int32) int {
	if v < 5 {
		cost := probCost(p[3], 0)

		if v == 2 {
			return cost + probCost(p[4], 0)
		}

		return cost + probCost(p[4], 1) + probCost(p[5], int(v-3))
	}

	cost := probCost(p[3], 1)

	if v < 11 {
		cost += probCost(p[6], 0)

		if v < 7 {
			return cost + probCost(p[7], 0) + probCost(159, int(v-5))
		}

		return cost + probCost(p[7], 1) + probCost(165, int(v-7)>>1) + probCost(145, int(v-7)&1)
	}

	cost += probCost(p[6], 1)

	cat := 0
	for cat < 3 && v >= 3+8<<(cat+1) {
		cat++
	}

	bit1 := cat >> 1

	cost += probCost(p[8], bit1) + probCost(p[9+bit1], cat&1)

	probs := catProbs[cat]
	v -= 3 + 8<<cat

	for i, prob := range probs {
		cost += probCost(prob, int(v>>(len(probs)-1-i))&1)
	}

	return cost
}

func coeffCost(bands *[17]*bandProbs, ctx, first int, levels []int16, nz int) int {
	p := &bands[first][ctx]
	cost := 0

	for n := first; n < 16; {
		if n >= nz {
			return cost + probCost(p[0], 0)
		}

		cost += probCost(p[0], 1)

		for levels[n] == 0 {
			cost += probCost(p[1], 0)
			n++
			p = &bands[n][0]
		}

		cost += probCost(p[1], 1)

		v := int32(levels[n])
		if v < 0 {
			v = -v
		}

		next := bands[n+1]

		if v == 1 {
			cost += probCost(p[2], 0)
			p = &next[1]
		} else {
			cost += probCost(p[2], 1)
			cost += largeValueCost(p, v)
			p = &next[2]
		}

		cost += 256
		n++
	}

	return cost
}

func (e *Encoder) pickIntra4(mbX, mbY int, m *mbData, lv *mbLevels) (int, uint32) {
	b := e.rec.yuv[:]

	e.rec.fillTopRight(mbX, mbY)

	top := e.topB[4*mbX : 4*mbX+4 : 4*mbX+4]
	left := &e.leftB

	levels := &e.i4Levels
	coeffs := &e.i4Coeffs

	score := 0
	bits := uint32(0)

	for n := range 16 {
		off := yOff + scan[n]
		x, y := n&3, n>>2

		probs := &bModeProbs[top[x]][left[y]]

		bestScore, bestMode, bestNz := math.MaxInt, 0, 0

		for mode := range numBModes {
			predLuma4[mode](b, off)
			fTransform(e.sc[:], b, off, off, coeffs[:])

			nz := quantizeBlock(coeffs[:], levels[:], &e.y1, 0)

			transformOne(coeffs[:], b, off)

			dist := sse(e.sc[:], b, off, 4)
			rate := bModeCost(probs, uint8(mode)) + coeffCost(&e.proba.bandsPtr[3], 0, 0, levels[:], nz)

			if s := 256*dist + e.lambdaMode*rate; s < bestScore {
				bestScore, bestMode, bestNz = s, mode, nz

				copy(m.coeffs[16*n:16*n+16], coeffs[:])
				copy(lv.levels[n][:], levels[:])
			}
		}

		predLuma4[bestMode](b, off)
		transformOne(m.coeffs[16*n:16*n+16], b, off)

		m.imodes[n] = uint8(bestMode)
		lv.nz[n] = bestNz
		bits = nzCodeBits(bits, bestNz, m.coeffs[16*n] != 0)

		top[x] = uint8(bestMode)
		left[y] = uint8(bestMode)

		score += bestScore
	}

	return score, bits
}
