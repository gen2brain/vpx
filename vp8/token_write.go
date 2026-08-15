package vp8

const (
	numSlots   = numBlockTypes * numBands * numCtx * numProbas
	tokLiteral = 1 << 15
	tokSlotBit = 1 << 11
	tokLitBit  = 1 << 8
)

type tokenBuf struct {
	buf   []uint16
	count [numSlots][2]uint32
}

func (t *tokenBuf) reset() {
	t.buf = t.buf[:0]
	clear(t.count[:])
}

func (t *tokenBuf) slot(idx, bit int) {
	v := uint16(idx)
	if bit != 0 {
		v |= tokSlotBit
	}

	t.count[idx][bit]++
	t.buf = append(t.buf, v)
}

func (t *tokenBuf) lit(prob uint8, bit int) {
	v := uint16(prob) | tokLiteral
	if bit != 0 {
		v |= tokLitBit
	}

	t.buf = append(t.buf, v)
}

var bandFirst = func() [numBands]int {
	var t [numBands]int

	for n := 16; n >= 0; n-- {
		t[coeffBands[n]] = n
	}

	return t
}()

func slotIndex(ty, n, ctx int) int {
	return ((ty*numBands+int(coeffBands[n]))*numCtx + ctx) * numProbas
}

func (e *Encoder) recordLargeValue(base int, v int32) {
	t := &e.tokens

	if v < 5 {
		t.slot(base+3, 0)

		if v == 2 {
			t.slot(base+4, 0)

			return
		}

		t.slot(base+4, 1)
		t.slot(base+5, int(v-3))

		return
	}

	t.slot(base+3, 1)

	if v < 11 {
		t.slot(base+6, 0)

		if v < 7 {
			t.slot(base+7, 0)
			t.lit(159, int(v-5))

			return
		}

		t.slot(base+7, 1)
		t.lit(165, int(v-7)>>1)
		t.lit(145, int(v-7)&1)

		return
	}

	t.slot(base+6, 1)

	cat := 0
	for cat < 3 && v >= 3+8<<(cat+1) {
		cat++
	}

	bit1 := cat >> 1

	t.slot(base+8, bit1)
	t.slot(base+9+bit1, cat&1)

	probs := catProbs[cat]
	v -= 3 + 8<<cat

	for i, prob := range probs {
		t.lit(prob, int(v>>(len(probs)-1-i))&1)
	}
}

func (e *Encoder) recordCoeffs(ty, ctx, first int, levels []int16, nz int) {
	t := &e.tokens

	for n := first; n < 16; {
		base := slotIndex(ty, n, ctx)

		if n >= nz {
			t.slot(base, 0)

			return
		}

		t.slot(base, 1)

		for levels[n] == 0 {
			t.slot(base+1, 0)
			n++
			ctx = 0
			base = slotIndex(ty, n, 0)
		}

		t.slot(base+1, 1)

		v := int32(levels[n])

		neg := v < 0
		if neg {
			v = -v
		}

		if v == 1 {
			t.slot(base+2, 0)
			ctx = 1
		} else {
			t.slot(base+2, 1)
			e.recordLargeValue(base, v)
			ctx = 2
		}

		t.lit(0x80, b2i(neg))
		n++
	}
}

func (e *Encoder) updateProbas() {
	for i := range numSlots {
		n0, n1 := e.tokens.count[i][0], e.tokens.count[i][1]

		ty := i / (numBands * numCtx * numProbas)
		rest := i % (numBands * numCtx * numProbas)
		b := rest / (numCtx * numProbas)
		c := rest % (numCtx * numProbas) / numProbas
		p := i % numProbas

		old := e.proba.bands[ty][b][c][p]
		up := coeffUpdateProbs[ty][b][c][p]

		e.probaNew[i] = 0

		if n0+n1 == 0 {
			continue
		}

		fresh := uint8(max(1, min(254, int(uint64(n0)*256/uint64(n0+n1)))))

		keep := int(n0)*probCost(old, 0) + int(n1)*probCost(old, 1) + probCost(up, 0)
		swap := int(n0)*probCost(fresh, 0) + int(n1)*probCost(fresh, 1) + probCost(up, 1) + 8*256

		if swap < keep {
			e.proba.bands[ty][b][c][p] = fresh
			e.probaNew[i] = fresh
		}
	}

	for i := range numSlots {
		ty := i / (numBands * numCtx * numProbas)
		rest := i % (numBands * numCtx * numProbas)
		b := rest / (numCtx * numProbas)
		c := rest % (numCtx * numProbas) / numProbas
		p := i % numProbas

		e.probaFlat[i] = e.proba.bands[ty][b][c][p]
	}
}

func (e *Encoder) replayTokens() {
	w := &e.tok

	for _, tk := range e.tokens.buf {
		if tk&tokLiteral != 0 {
			w.put(int(tk>>8)&1, uint8(tk))
		} else {
			w.put(int(tk>>11)&1, e.probaFlat[tk&(tokSlotBit-1)])
		}

		w.flushIf()
	}
}
