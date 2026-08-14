package vp8

type quantHeader struct {
	baseQ int
	y1DC  int
	y2DC  int
	y2AC  int
	uvDC  int
	uvAC  int
}

func (h *quantHeader) parse(d *boolDec) {
	h.baseQ = int(d.getBits(7))
	h.y1DC = d.flagged(4)
	h.y2DC = d.flagged(4)
	h.y2AC = d.flagged(4)
	h.uvDC = d.flagged(4)
	h.uvAC = d.flagged(4)
}

type quantMatrix struct {
	y1      [2]uint16
	y2      [2]uint16
	uv      [2]uint16
	uvQuant int
}

func clampQ(v, max int) int {
	switch {
	case v < 0:
		return 0
	case v > max:
		return max
	}

	return v
}

func (d *Decoder) deriveQuant() {
	q := &d.quant

	for i := range numSegments {
		var base int

		switch {
		case d.seg.enabled:
			base = int(d.seg.quantizer[i])
			if !d.seg.absoluteDelta {
				base += q.baseQ
			}
		case i > 0:
			d.dqm[i] = d.dqm[0]

			continue
		default:
			base = q.baseQ
		}

		m := &d.dqm[i]

		m.y1[0] = uint16(dcTable[clampQ(base+q.y1DC, 127)])
		m.y1[1] = uint16(acTable[clampQ(base, 127)])

		m.y2[0] = uint16(dcTable[clampQ(base+q.y2DC, 127)]) * 2
		m.y2[1] = uint16(int(acTable[clampQ(base+q.y2AC, 127)]) * 101581 >> 16)

		if m.y2[1] < 8 {
			m.y2[1] = 8
		}

		m.uv[0] = uint16(dcTable[clampQ(base+q.uvDC, 117)])
		m.uv[1] = uint16(acTable[clampQ(base+q.uvAC, 127)])

		m.uvQuant = base + q.uvAC
	}
}
