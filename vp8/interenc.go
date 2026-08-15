package vp8

func (e *Encoder) interProbability() {
	if e.keyFrame {
		return
	}

	inter := 0

	for _, m := range e.info {
		if m.inter {
			inter++
		}
	}

	e.probIntra = uint8(max(1, min(255, 255*(len(e.info)-inter)/len(e.info))))
}

func (e *Encoder) putInterHeader() {
	w := &e.hdr

	w.putFlag(false)
	w.putFlag(false)
	w.putBits(uint32(e.filterLevel), 6)
	w.putBits(0, 3)
	w.putFlag(false)

	w.putBits(0, 2)

	w.putBits(uint32(e.baseQ), 7)

	for range 5 {
		w.putFlag(false)
	}

	w.putFlag(true)
	w.putFlag(true)
	w.putFlag(false)
	w.putFlag(false)

	w.putFlag(true)
	w.putFlag(true)

	e.putTokenProbs()

	w.putFlag(e.useSkip)

	if e.useSkip {
		w.putBits(uint32(e.skipProb), 8)
	}

	w.putBits(uint32(e.probIntra), 8)
	w.putBits(255, 8)
	w.putBits(128, 8)

	w.putFlag(false)
	w.putFlag(false)

	for i := range mvUpdateProbs {
		for j := range mvUpdateProbs[i] {
			w.putBit(0, mvUpdateProbs[i][j])
		}
	}
}

func (e *Encoder) putYMode(mode uint8) {
	w := &e.hdr

	if mode == dcPred {
		w.putBit(0, e.rec.yProbs[0])

		return
	}

	w.putBit(1, e.rec.yProbs[0])

	switch mode {
	case vPred:
		w.putBit(0, e.rec.yProbs[1])
		w.putBit(0, e.rec.yProbs[2])
	case hPred:
		w.putBit(0, e.rec.yProbs[1])
		w.putBit(1, e.rec.yProbs[2])
	case tmPred:
		w.putBit(1, e.rec.yProbs[1])
		w.putBit(0, e.rec.yProbs[3])
	default:
		w.putBit(1, e.rec.yProbs[1])
		w.putBit(1, e.rec.yProbs[3])
	}
}

func (e *Encoder) putUVMode(mode uint8) {
	w := &e.hdr

	if mode == dcPred {
		w.putBit(0, e.rec.uvProbs[0])

		return
	}

	w.putBit(1, e.rec.uvProbs[0])

	if mode == vPred {
		w.putBit(0, e.rec.uvProbs[1])

		return
	}

	w.putBit(1, e.rec.uvProbs[1])
	w.putBit(b2i(mode != hPred), e.rec.uvProbs[2])
}

func (e *Encoder) putInterModes() {
	w := &e.hdr

	for i, m := range e.info {
		mbX, mbY := i%e.mbW, i/e.mbW

		if e.useSkip {
			w.putBool(m.skip, e.skipProb)
		}

		if !m.inter {
			w.putBit(0, e.probIntra)

			ymode := m.ymode
			if m.i4x4 {
				ymode = bPred
			}

			e.putYMode(ymode)

			if m.i4x4 {
				for n := range 16 {
					putBMode(w, &bModeProbsInter, m.imodes[n])
				}
			}

			e.putUVMode(m.uvMode)

			continue
		}

		w.putBit(1, e.probIntra)
		w.putBit(0, 255)

		_, _, best, cnt := e.rec.nearMVs(mbX, mbY, refLast)

		w.putBit(b2i(m.mode != mvZero), modeContexts[cnt[0]][0])

		if m.mode == mvZero {
			continue
		}

		w.putBit(b2i(m.mode != mvNearest), modeContexts[cnt[1]][1])

		if m.mode == mvNearest {
			continue
		}

		w.putBit(b2i(m.mode != mvNear), modeContexts[cnt[2]][2])

		if m.mode == mvNear {
			continue
		}

		w.putBit(0, modeContexts[e.rec.splitContext(mbX, mbY)][3])

		putMV(w, &e.rec.mvProbs, mv{row: m.mv.row - best.row, col: m.mv.col - best.col})
	}
}
