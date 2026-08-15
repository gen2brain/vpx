package vp8

const (
	mvIsShort   = 0
	mvSign      = 1
	mvShort     = 2
	mvLongWidth = 10
	mvBits      = mvShort + 7
	mvProbCount = mvBits + mvLongWidth
)

const (
	refIntra = iota
	refLast
	refGolden
	refAltRef
	numRefFrames
)

const (
	mvNearest = iota
	mvNear
	mvZero
	mvNew
	mvSplit
)

const (
	mvMargin     = 16 << 3
	mvBorderNear = 19 << 3
	mvBorderFar  = 18 << 3
)

type mv struct {
	row, col int16
}

func (m mv) zero() bool { return m == mv{} }

type mvProbs [2][mvProbCount]uint8

var smallMVTree = [14]int8{2, 8, 4, 6, -0, -1, -2, -3, 10, 12, -4, -5, -6, -7}

func (d *boolDec) readSmallMV(p *[mvProbCount]uint8) int {
	i := 0

	for {
		d.fill()

		i = int(smallMVTree[i+d.getBitFast(uint32(p[mvShort+(i>>1)]))])
		if i <= 0 {
			return -i
		}
	}
}

func (d *boolDec) readMVComponent(p *[mvProbCount]uint8) int {
	x := 0

	if d.getBit(p[mvIsShort]) != 0 {
		for i := range 3 {
			d.fill()

			x += d.getBitFast(uint32(p[mvBits+i])) << i
		}

		for i := mvLongWidth - 1; i > 3; i-- {
			d.fill()

			x += d.getBitFast(uint32(p[mvBits+i])) << i
		}

		if x&0xfff0 == 0 || d.getBit(p[mvBits+3]) != 0 {
			x += 8
		}
	} else {
		x = d.readSmallMV(p)
	}

	if x != 0 && d.getBit(p[mvSign]) != 0 {
		x = -x
	}

	return x
}

func (d *boolDec) readMV(p *mvProbs) mv {
	return mv{
		row: int16(d.readMVComponent(&p[0]) * 2),
		col: int16(d.readMVComponent(&p[1]) * 2),
	}
}

func (d *boolDec) readMVProbs(p *mvProbs) {
	for i := range p {
		for j := range p[i] {
			if d.getBit(mvUpdateProbs[i][j]) == 0 {
				continue
			}

			v := uint8(d.getBits(7))

			p[i][j] = 1
			if v != 0 {
				p[i][j] = v << 1
			}
		}
	}
}

type bounds struct {
	left, right, top, bottom int
}

func (d *Decoder) mbBounds(mbX, mbY int) bounds {
	return bounds{
		left:   -(mbX * 16) << 3,
		right:  (d.mbW - 1 - mbX) * 16 << 3,
		top:    -(mbY * 16) << 3,
		bottom: (d.mbH - 1 - mbY) * 16 << 3,
	}
}

func (b bounds) clamp(m mv) mv {
	return mv{
		row: int16(min(max(int(m.row), b.top-mvMargin), b.bottom+mvMargin)),
		col: int16(min(max(int(m.col), b.left-mvMargin), b.right+mvMargin)),
	}
}

func (b bounds) outsideBorder(m mv) bool {
	return 2*int(m.col) < b.left-mvBorderNear || 2*int(m.col) > b.right+mvBorderFar ||
		2*int(m.row) < b.top-mvBorderNear || 2*int(m.row) > b.bottom+mvBorderFar
}

func (b bounds) outside(m mv) bool {
	return int(m.col) < b.left-mvMargin || int(m.col) > b.right+mvMargin ||
		int(m.row) < b.top-mvMargin || int(m.row) > b.bottom+mvMargin
}

func (b bounds) clampToBorder(m mv, shift uint) mv {
	switch col := int(m.col) << shift; {
	case col < b.left-mvBorderNear:
		m.col = int16((b.left - mvMargin) >> shift)
	case col > b.right+mvBorderFar:
		m.col = int16((b.right + mvMargin) >> shift)
	}

	switch row := int(m.row) << shift; {
	case row < b.top-mvBorderNear:
		m.row = int16((b.top - mvMargin) >> shift)
	case row > b.bottom+mvBorderFar:
		m.row = int16((b.bottom + mvMargin) >> shift)
	}

	return m
}
