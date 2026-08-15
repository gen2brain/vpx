package vp8

var catProbs = [4][]uint8{
	{173, 148, 140},
	{176, 155, 140, 135},
	{180, 157, 141, 134, 130},
	{254, 254, 243, 230, 196, 177, 153, 140, 133, 130, 129},
}

var zigzag = [16]uint8{0, 1, 4, 8, 5, 2, 3, 6, 9, 12, 13, 10, 7, 11, 14, 15}

type bandProbs = [numCtx][numProbas]uint8

func (d *boolDec) largeValue(p *[numProbas]uint8) int32 {
	d.fill()

	if d.getBitFast(uint32(p[3])) == 0 {
		d.fill()

		if d.getBitFast(uint32(p[4])) == 0 {
			return 2
		}

		d.fill()

		return 3 + int32(d.getBitFast(uint32(p[5])))
	}

	d.fill()

	if d.getBitFast(uint32(p[6])) == 0 {
		d.fill()

		if d.getBitFast(uint32(p[7])) == 0 {
			d.fill()

			return 5 + int32(d.getBitFast(159))
		}

		d.fill()

		v := 7 + 2*int32(d.getBitFast(165))

		d.fill()

		return v + int32(d.getBitFast(145))
	}

	d.fill()

	bit1 := d.getBitFast(uint32(p[8]))

	d.fill()

	cat := 2*bit1 + d.getBitFast(uint32(p[9+bit1]))

	var v int32

	for _, prob := range catProbs[cat] {
		d.fill()

		v += v + int32(d.getBitFast(uint32(prob)))
	}

	return v + 3 + 8<<cat
}

func getCoeffs(d *boolDec, bands *[17]*bandProbs, ctx int, dq *[2]uint16, n int, out []int16) int {
	p := &bands[n][ctx]

	for ; n < 16; n++ {
		d.fill()

		if d.getBitFast(uint32(p[0])) == 0 {
			return n
		}

		d.fill()

		for d.getBitFast(uint32(p[1])) == 0 {
			n++
			if n == 16 {
				return 16
			}

			p = &bands[n][0]

			d.fill()
		}

		next := bands[n+1]

		var v int32

		d.fill()

		if d.getBitFast(uint32(p[2])) == 0 {
			v = 1
			p = &next[1]
		} else {
			v = d.largeValue(p)
			p = &next[2]
		}

		q := int32(dq[1])
		if n == 0 {
			q = int32(dq[0])
		}

		d.fill()

		out[zigzag[n]] = int16(d.getSignedFast(v) * q)
	}

	return 16
}

func nzCodeBits(nz uint32, n int, dcNZ bool) uint32 {
	nz <<= 2

	switch {
	case n > 3:
		nz |= 3
	case n > 1:
		nz |= 2
	case dcNZ:
		nz |= 1
	}

	return nz
}

func (d *Decoder) parseResiduals(m *mbData, br *boolDec, mbX int) bool {
	q := &d.dqm[m.segment]

	top := &d.mbCtx[1+mbX]
	left := &d.mbCtx[0]

	clear(m.coeffs[:])

	dst := m.coeffs[:]
	first := 0
	acBands := &d.proba.bandsPtr[3]

	if !m.isI4x4 {
		var dc [16]int16

		ctx := int(top.nzDC) + int(left.nzDC)
		nz := getCoeffs(br, &d.proba.bandsPtr[1], ctx, &q.y2, 0, dc[:])

		nzDC := uint8(0)
		if nz > 0 {
			nzDC = 1
		}

		top.nzDC, left.nzDC = nzDC, nzDC

		if nz > 1 {
			transformWHT(dc[:], dst)
		} else {
			dc0 := (dc[0] + 3) >> 3
			for i := 0; i < 16*16; i += 16 {
				dst[i] = dc0
			}
		}

		first = 1
		acBands = &d.proba.bandsPtr[0]
	}

	var nonZeroY, nonZeroUV uint32

	tnz := top.nz & 0x0f
	lnz := left.nz & 0x0f

	for range 4 {
		l := lnz & 1
		nzCoeffs := uint32(0)

		for range 4 {
			ctx := int(l) + int(tnz&1)
			nz := getCoeffs(br, acBands, ctx, &q.y1, first, dst)

			l = 0
			if nz > first {
				l = 1
			}

			tnz = tnz>>1 | l<<7
			nzCoeffs = nzCodeBits(nzCoeffs, nz, dst[0] != 0)
			dst = dst[16:]
		}

		tnz >>= 4
		lnz = lnz>>1 | l<<7
		nonZeroY = nonZeroY<<8 | nzCoeffs
	}

	outTNZ := uint32(tnz)
	outLNZ := uint32(lnz >> 4)

	for ch := 0; ch < 4; ch += 2 {
		nzCoeffs := uint32(0)
		tnz := top.nz >> (4 + ch)
		lnz := left.nz >> (4 + ch)

		for range 2 {
			l := lnz & 1

			for range 2 {
				ctx := int(l) + int(tnz&1)
				nz := getCoeffs(br, &d.proba.bandsPtr[2], ctx, &q.uv, 0, dst)

				l = 0
				if nz > 0 {
					l = 1
				}

				tnz = tnz>>1 | l<<3
				nzCoeffs = nzCodeBits(nzCoeffs, nz, dst[0] != 0)
				dst = dst[16:]
			}

			tnz >>= 2
			lnz = lnz>>1 | l<<5
		}

		nonZeroUV |= nzCoeffs << (4 * ch)
		outTNZ |= uint32(tnz) << 4 << ch
		outLNZ |= uint32(lnz&0xf0) << ch
	}

	top.nz = uint8(outTNZ)
	left.nz = uint8(outLNZ)

	m.nonZeroY = nonZeroY
	m.nonZeroUV = nonZeroUV

	return nonZeroY|nonZeroUV == 0
}

func (d *Decoder) decodeMB(m *mbData, br *boolDec, mbX int) bool {

	skip := d.useSkipProb && m.skip

	if !skip {
		skip = d.parseResiduals(m, br, mbX)
	} else {
		top := &d.mbCtx[1+mbX]
		left := &d.mbCtx[0]

		top.nz, left.nz = 0, 0

		if !m.isI4x4 {
			top.nzDC, left.nzDC = 0, 0
		}

		m.nonZeroY = 0
		m.nonZeroUV = 0
	}

	m.skipped = skip

	return !br.eof
}
