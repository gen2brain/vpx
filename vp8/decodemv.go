package vp8

type modeInfo struct {
	mv       mv
	subMV    [16]mv
	refFrame uint8
	split    bool
}

func (d *Decoder) modeAt(mbX, mbY int) *modeInfo {
	return &d.modes[(mbY+1)*(d.mbW+1)+mbX+1]
}

var (
	yModeTree  = [8]int8{-dcPred, 2, 4, 6, -vPred, -hPred, -tmPred, -bPred}
	uvModeTree = [6]int8{-dcPred, 2, -vPred, 4, -hPred, -tmPred}
)

func (d *boolDec) readTree(tree []int8, p []uint8) int {
	i := 0

	for {
		i = int(tree[i+d.getBit(p[i>>1])])
		if i <= 0 {
			return -i
		}
	}
}

func (d *Decoder) readSegmentID() uint8 {
	if d.br.getBit(d.proba.segments[0]) == 0 {
		return uint8(d.br.getBit(d.proba.segments[1]))
	}

	return uint8(d.br.getBit(d.proba.segments[2]) + 2)
}

func (d *Decoder) leftMV(m *modeInfo, mbX, mbY, k int) mv {
	if k&3 != 0 {
		return m.subMV[k-1]
	}

	left := d.modeAt(mbX-1, mbY)

	if !left.split {
		return left.mv
	}

	return left.subMV[k+3]
}

func (d *Decoder) aboveMV(m *modeInfo, mbX, mbY, k int) mv {
	if k>>2 != 0 {
		return m.subMV[k-4]
	}

	above := d.modeAt(mbX, mbY-1)

	if !above.split {
		return above.mv
	}

	return above.subMV[k+12]
}

func subMVRefProb(left, above mv) *[3]uint8 {
	i := 0

	if above.zero() {
		i |= 4
	}

	if left.zero() {
		i |= 2
	}

	if left == above {
		i |= 1
	}

	return &subMVRefProbs[i]
}

func (d *Decoder) decodeSplitMV(m *modeInfo, mbX, mbY int, best mv, b bounds) bool {
	part := 3
	count := 16

	if d.br.getBit(110) != 0 {
		part = 2
		count = 4

		if d.br.getBit(111) != 0 {
			part = d.br.getBit(150)
			count = 2
		}
	}

	needClamp := false

	for j := range count {
		k := int(mbSplitOffset[part][j])

		left := d.leftMV(m, mbX, mbY, k)
		above := d.aboveMV(m, mbX, mbY, k)
		p := subMVRefProb(left, above)

		var v mv

		switch {
		case d.br.getBit(p[0]) == 0:
			v = left
		case d.br.getBit(p[1]) == 0:
			v = above
		case d.br.getBit(p[2]) != 0:
			v = d.br.readMV(&d.mvProbs)
			v.row += best.row
			v.col += best.col
		}

		needClamp = needClamp || b.outside(v)

		for i, s := range mbSplits[part] {
			if int(s) == j {
				m.subMV[i] = v
			}
		}
	}

	m.mv = m.subMV[15]

	return needClamp
}

func (d *Decoder) nearMVs(mbX, mbY int, ref uint8) (nearest, near, best mv, cnt [4]int) {
	var found [4]mv

	n := 0

	add := func(m *modeInfo, weight int) {
		if m.refFrame == refIntra {
			return
		}

		if m.mv.zero() {
			cnt[0] += weight

			return
		}

		v := m.mv
		if d.signBias[m.refFrame] != d.signBias[ref] {
			v.row, v.col = -v.row, -v.col
		}

		if v != found[n] {
			n++
			found[n] = v
		}

		cnt[n] += weight
	}

	add(d.modeAt(mbX, mbY-1), 2)
	add(d.modeAt(mbX-1, mbY), 2)
	add(d.modeAt(mbX-1, mbY-1), 1)

	if cnt[3] > 0 && found[3] == found[1] {
		cnt[1]++
	}

	if cnt[2] > cnt[1] {
		cnt[1], cnt[2] = cnt[2], cnt[1]
		found[1], found[2] = found[2], found[1]
	}

	best = found[1]
	if cnt[1] < cnt[0] {
		best = found[0]
	}

	b := d.mbBounds(mbX, mbY)

	return b.clamp(found[1]), b.clamp(found[2]), b.clamp(best), cnt
}

func (d *Decoder) parseInterModes(mbX, mbY int) {
	m := d.modeAt(mbX, mbY)
	mb := &d.mb

	*m = modeInfo{}
	mb.needClamp = false

	if d.seg.updateMap {
		d.segmap[mbY*d.mbW+mbX] = d.readSegmentID()
	}

	mb.segment = d.segmap[mbY*d.mbW+mbX]

	mb.skip = false
	if d.useSkipProb {
		mb.skip = d.br.getBit(d.skipProb) != 0
	}

	if d.br.getBit(d.probIntra) == 0 {
		d.parseIntraInInter()

		mb.refFrame = refIntra

		return
	}

	m.refFrame = refLast
	if d.br.getBit(d.probLast) != 0 {
		m.refFrame = uint8(refGolden + d.br.getBit(d.probGF))
	}

	nearest, near, best, cnt := d.nearMVs(mbX, mbY, m.refFrame)
	b := d.mbBounds(mbX, mbY)

	mb.refFrame = m.refFrame
	mb.isI4x4 = false

	switch {
	case d.br.getBit(modeContexts[cnt[0]][0]) == 0:
		mb.mode = mvZero
	case d.br.getBit(modeContexts[cnt[1]][1]) == 0:
		mb.mode = mvNearest
		m.mv = nearest
	case d.br.getBit(modeContexts[cnt[2]][2]) == 0:
		mb.mode = mvNear
		m.mv = near
	default:
		cnt[3] = 0

		if d.modeAt(mbX, mbY-1).split {
			cnt[3] += 2
		}

		if d.modeAt(mbX-1, mbY).split {
			cnt[3] += 2
		}

		if d.modeAt(mbX-1, mbY-1).split {
			cnt[3]++
		}

		if d.br.getBit(modeContexts[cnt[3]][3]) != 0 {
			mb.mode = mvSplit
			mb.isI4x4 = true
			m.split = true
			mb.needClamp = d.decodeSplitMV(m, mbX, mbY, best, b)
		} else {
			mb.mode = mvNew
			m.mv = d.br.readMV(&d.mvProbs)
			m.mv.row += best.row
			m.mv.col += best.col
			mb.needClamp = b.outside(m.mv)
		}
	}

	if !m.split {
		for i := range m.subMV {
			m.subMV[i] = m.mv
		}
	}
}

func (d *Decoder) parseIntraInInter() {
	mb := &d.mb

	mode := uint8(d.br.readTree(yModeTree[:], d.yProbs[:]))

	mb.isI4x4 = mode == bPred
	mb.imodes[0] = mode

	if mb.isI4x4 {
		for i := range 16 {
			mb.imodes[i] = parseBMode(&d.br, &bModeProbsInter)
		}
	}

	mb.uvMode = uint8(d.br.readTree(uvModeTree[:], d.uvProbs[:]))
}
