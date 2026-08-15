package vp8

import "math"

const (
	mvSearchRange = 16 << 3
	mvSearchSteps = 8
)

var smallMVPath = [8][3]uint8{
	{0, 0, 0}, {0, 0, 1}, {0, 1, 0}, {0, 1, 1},
	{1, 0, 0}, {1, 0, 1}, {1, 1, 0}, {1, 1, 1},
}

func putSmallMV(w *boolEnc, p *[mvProbCount]uint8, v int) {
	path := &smallMVPath[v]

	w.putBit(int(path[0]), p[mvShort])

	if path[0] == 0 {
		w.putBit(int(path[1]), p[mvShort+1])
		w.putBit(int(path[2]), p[mvShort+2+path[1]])

		return
	}

	w.putBit(int(path[1]), p[mvShort+4])
	w.putBit(int(path[2]), p[mvShort+5+path[1]])
}

func smallMVCost(p *[mvProbCount]uint8, v int) int {
	path := &smallMVPath[v]

	if path[0] == 0 {
		return probCost(p[mvShort], 0) + probCost(p[mvShort+1], int(path[1])) +
			probCost(p[mvShort+2+path[1]], int(path[2]))
	}

	return probCost(p[mvShort], 1) + probCost(p[mvShort+4], int(path[1])) +
		probCost(p[mvShort+5+path[1]], int(path[2]))
}

func putMVComponent(w *boolEnc, p *[mvProbCount]uint8, v int) {
	x := v
	if x < 0 {
		x = -x
	}

	if x < 8 {
		w.putBit(0, p[mvIsShort])
		putSmallMV(w, p, x)
	} else {
		w.putBit(1, p[mvIsShort])

		for i := range 3 {
			w.putBit(x>>i&1, p[mvBits+i])
		}

		for i := mvLongWidth - 1; i > 3; i-- {
			w.putBit(x>>i&1, p[mvBits+i])
		}

		if x&0xfff0 != 0 {
			w.putBit(x>>3&1, p[mvBits+3])
		}
	}

	if x != 0 {
		w.putBit(b2i(v < 0), p[mvSign])
	}
}

func mvComponentCost(p *[mvProbCount]uint8, v int) int {
	x := v
	if x < 0 {
		x = -x
	}

	cost := 0

	if x < 8 {
		cost = probCost(p[mvIsShort], 0) + smallMVCost(p, x)
	} else {
		cost = probCost(p[mvIsShort], 1)

		for i := range 3 {
			cost += probCost(p[mvBits+i], x>>i&1)
		}

		for i := mvLongWidth - 1; i > 3; i-- {
			cost += probCost(p[mvBits+i], x>>i&1)
		}

		if x&0xfff0 != 0 {
			cost += probCost(p[mvBits+3], x>>3&1)
		}
	}

	if x != 0 {
		cost += probCost(p[mvSign], b2i(v < 0))
	}

	return cost
}

func putMV(w *boolEnc, p *mvProbs, v mv) {
	putMVComponent(w, &p[0], int(v.row)/2)
	putMVComponent(w, &p[1], int(v.col)/2)
}

func mvCost(p *mvProbs, v mv) int {
	return mvComponentCost(&p[0], int(v.row)/2) + mvComponentCost(&p[1], int(v.col)/2)
}

func (e *Encoder) predictLuma(mbX, mbY int, v mv) {
	ref := e.rec.reference(refLast)

	luma := predictor{src: ref.y, tmp: &e.rec.mcTmp, stride: ref.pic.YStride, sixtap: e.rec.sixtap}
	base := ref.yOrigin + mbY*16*luma.stride + mbX*16

	off := base + int(v.row>>3)*luma.stride + int(v.col>>3)

	luma.predict(off, int(v.col&7), int(v.row&7), 16, 16, e.rec.yuv[:], yOff, bps)
}

func (e *Encoder) lumaError(mbX, mbY int, v mv, b bounds) int {
	c := v
	if b.outside(c) {
		c = b.clampToBorder(c, 0)
	}

	e.predictLuma(mbX, mbY, c)

	return sse(e.sc[:], e.rec.yuv[:], yOff, 16)
}

func (e *Encoder) searchMV(mbX, mbY int, best mv, b bounds) (mv, int) {
	center := best

	bestMV := center
	bestScore := 256*e.lumaError(mbX, mbY, center, b) + e.lambdaMode*mvCost(&e.rec.mvProbs, mv{})

	try := func(v mv) {
		if v.row < center.row-mvSearchRange || v.row > center.row+mvSearchRange ||
			v.col < center.col-mvSearchRange || v.col > center.col+mvSearchRange {
			return
		}

		d := mv{row: v.row - best.row, col: v.col - best.col}

		s := 256*e.lumaError(mbX, mbY, v, b) + e.lambdaMode*mvCost(&e.rec.mvProbs, d)
		if s < bestScore {
			bestScore, bestMV = s, v
		}
	}

	for step := int16(mvSearchSteps << 3); step >= 8; step >>= 1 {
		for {
			start := bestMV

			try(mv{row: start.row - step, col: start.col})
			try(mv{row: start.row + step, col: start.col})
			try(mv{row: start.row, col: start.col - step})
			try(mv{row: start.row, col: start.col + step})

			if bestMV == start {
				break
			}
		}
	}

	for _, step := range [2]int16{4, 2} {
		start := bestMV

		for _, d := range [8]mv{
			{-step, 0}, {step, 0}, {0, -step}, {0, step},
			{-step, -step}, {-step, step}, {step, -step}, {step, step},
		} {
			try(mv{row: start.row + d.row, col: start.col + d.col})
		}
	}

	return bestMV, bestScore
}

type interChoice struct {
	mode uint8
	mv   mv
	rate int
}

func (d *Decoder) splitContext(mbX, mbY int) int {
	n := 0

	if d.modeAt(mbX, mbY-1).split {
		n += 2
	}

	if d.modeAt(mbX-1, mbY).split {
		n += 2
	}

	if d.modeAt(mbX-1, mbY-1).split {
		n++
	}

	return n
}

func (e *Encoder) setInter(mbX, mbY int, m *mbData, c interChoice, b bounds) {
	mi := e.rec.modeAt(mbX, mbY)

	*mi = modeInfo{refFrame: refLast, mv: c.mv}

	for i := range mi.subMV {
		mi.subMV[i] = c.mv
	}

	m.refFrame = refLast
	m.mode = c.mode
	m.isI4x4 = false
	m.needClamp = c.mode == mvNew && b.outside(c.mv)

	e.rec.predictInter(m, mbX, mbY)
}

func (e *Encoder) pickInter(mbX, mbY int, m *mbData, lv *mbLevels) int {
	nearest, near, best, cnt := e.rec.nearMVs(mbX, mbY, refLast)
	b := e.rec.mbBounds(mbX, mbY)

	zero := modeContexts[cnt[0]][0]
	notNearest := modeContexts[cnt[1]][1]
	notNear := modeContexts[cnt[2]][2]
	notSplit := modeContexts[e.rec.splitContext(mbX, mbY)][3]

	toNearest := probCost(zero, 1)
	toNear := toNearest + probCost(notNearest, 1)
	toNew := toNear + probCost(notNear, 1)

	found, _ := e.searchMV(mbX, mbY, best, b)
	delta := mv{row: found.row - best.row, col: found.col - best.col}

	cands := [4]interChoice{
		{mode: mvZero, rate: probCost(zero, 0)},
		{mode: mvNearest, mv: nearest, rate: toNearest + probCost(notNearest, 0)},
		{mode: mvNear, mv: near, rate: toNear + probCost(notNear, 0)},
		{mode: mvNew, mv: found, rate: toNew + probCost(notSplit, 0) + mvCost(&e.rec.mvProbs, delta)},
	}

	out, score := cands[0], math.MaxInt

	for _, c := range cands {
		e.setInter(mbX, mbY, m, c, b)

		m.nonZeroY = e.transformLuma(m, lv, false)

		if s := e.lumaScore(m, lv, c.rate); s < score {
			out, score = c, s
		}
	}

	e.setInter(mbX, mbY, m, out, b)

	return score
}
