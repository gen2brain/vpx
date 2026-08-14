package vp8

type mbData struct {
	coeffs    [384]int16
	imodes    [16]uint8
	uvMode    uint8
	segment   uint8
	isI4x4    bool
	skip      bool
	skipped   bool
	refFrame  uint8
	mode      uint8
	needClamp bool
	nonZeroY  uint32
	nonZeroUV uint32
}

func (m *mbData) inter() bool { return m.refFrame != refIntra }

func (m *mbData) lfMode() uint8 {
	if !m.inter() {
		if m.isI4x4 {
			return 0
		}

		return 1
	}

	switch m.mode {
	case mvZero:
		return 1
	case mvSplit:
		return 3
	}

	return 2
}

func (m *mbData) filterFlags() uint8 {
	f := m.segment&3 | m.lfMode()<<2 | m.refFrame<<4

	if !m.skipped || m.isI4x4 {
		f |= 1 << 6
	}

	return f
}

type mbCtx struct {
	nz   uint8
	nzDC uint8
}

type topSample struct {
	y [16]uint8
	u [8]uint8
	v [8]uint8
}

func parseBMode(d *boolDec, p *[numBModes - 1]uint8) uint8 {
	if d.getBit(p[0]) == 0 {
		return bDCPred
	}

	if d.getBit(p[1]) == 0 {
		return bTMPred
	}

	if d.getBit(p[2]) == 0 {
		return bVEPred
	}

	if d.getBit(p[3]) == 0 {
		if d.getBit(p[4]) == 0 {
			return bHEPred
		}

		if d.getBit(p[5]) == 0 {
			return bRDPred
		}

		return bVRPred
	}

	if d.getBit(p[6]) == 0 {
		return bLDPred
	}

	if d.getBit(p[7]) == 0 {
		return bVLPred
	}

	if d.getBit(p[8]) == 0 {
		return bHDPred
	}

	return bHUPred
}

func (d *Decoder) parseIntraMode(mbX, mbY int) {
	m := &d.mb
	top := d.intraT[4*mbX : 4*mbX+4 : 4*mbX+4]
	left := &d.intraL

	m.refFrame = refIntra
	m.mode = 0
	m.needClamp = false

	m.segment = 0
	if d.seg.updateMap {
		m.segment = d.readSegmentID()
	}

	d.segmap[mbY*d.mbW+mbX] = m.segment

	m.skip = false
	if d.useSkipProb {
		m.skip = d.br.getBit(d.skipProb) != 0
	}

	m.isI4x4 = d.br.getBit(145) == 0

	if !m.isI4x4 {
		ymode := uint8(dcPred)

		if d.br.getBit(156) != 0 {
			ymode = hPred
			if d.br.getBit(128) != 0 {
				ymode = tmPred
			}
		} else if d.br.getBit(163) != 0 {
			ymode = vPred
		}

		m.imodes[0] = ymode

		for i := range 4 {
			top[i] = ymode
			left[i] = ymode
		}
	} else {
		for y := range 4 {
			ymode := left[y]

			for x := range 4 {
				ymode = parseBMode(&d.br, &bModeProbs[top[x]][ymode])
				top[x] = ymode
			}

			copy(m.imodes[4*y:4*y+4], top)
			left[y] = ymode
		}
	}

	switch {
	case d.br.getBit(142) == 0:
		m.uvMode = dcPred
	case d.br.getBit(114) == 0:
		m.uvMode = vPred
	case d.br.getBit(183) != 0:
		m.uvMode = tmPred
	default:
		m.uvMode = hPred
	}
}
