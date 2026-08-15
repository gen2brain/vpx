package vp8

const (
	qfix        = 17
	maxLevel    = 2047
	sharpenBits = 11
)

var freqSharpening = [16]uint16{
	0, 30, 60, 90,
	30, 60, 90, 90,
	60, 90, 90, 90,
	90, 90, 90, 90,
}

var biasMatrices = [3][2]uint32{{96, 110}, {96, 108}, {110, 115}}

type qmatrix struct {
	q       [16]uint32
	iq      [16]uint32
	bias    [16]uint32
	zthresh [16]uint32
	sharpen [16]uint32

	q16       [16]uint16
	iq16      [16]uint16
	sharpen16 [16]uint16
	narrow    bool
}

func (m *qmatrix) expand(kind int) int {
	for i := range 2 {
		bias := biasMatrices[kind][i]

		m.iq[i] = 1 << qfix / m.q[i]
		m.bias[i] = bias << (qfix - 8)
		m.zthresh[i] = (1<<qfix - 1 - m.bias[i]) / m.iq[i]
	}

	for i := 2; i < 16; i++ {
		m.q[i] = m.q[1]
		m.iq[i] = m.iq[1]
		m.bias[i] = m.bias[1]
		m.zthresh[i] = m.zthresh[1]
	}

	sum := uint32(0)

	for i := range 16 {
		if kind == 0 {
			m.sharpen[i] = uint32(freqSharpening[i]) * m.q[i] >> sharpenBits
		} else {
			m.sharpen[i] = 0
		}

		sum += m.q[i]

		m.q16[i] = uint16(m.q[i])
		m.iq16[i] = uint16(m.iq[i])
		m.sharpen16[i] = uint16(m.sharpen[i])
	}

	m.narrow = true

	for i := range 16 {
		if m.q[i] > 0xffff || m.iq[i] > 0xffff || m.sharpen[i] > 0xffff {
			m.narrow = false
		}
	}

	return int((sum + 8) >> 4)
}

func fTransform(src, ref []byte, sOff, rOff int, out []int16) {
	if fTransformAsm != nil && sOff >= 0 && rOff >= 0 &&
		len(src)-sOff >= 3*bps+8 && len(ref)-rOff >= 3*bps+8 && len(out) >= 16 {
		fTransformAsm(src, ref, sOff, rOff, out)

		return
	}

	fTransformGo(src, ref, sOff, rOff, out)
}

func fTransform2(src, ref []byte, sOff, rOff int, out []int16) {
	if fTransform2Asm != nil && sOff >= 0 && rOff >= 0 &&
		len(src)-sOff >= 3*bps+8 && len(ref)-rOff >= 3*bps+8 && len(out) >= 32 {
		fTransform2Asm(src, ref, sOff, rOff, out)

		return
	}

	fTransform(src, ref, sOff, rOff, out)
	fTransform(src, ref, sOff+4, rOff+4, out[16:])
}

func fTransformGo(src, ref []byte, sOff, rOff int, out []int16) {
	var tmp [16]int

	for i := range 4 {
		s := src[sOff+i*bps:]
		r := ref[rOff+i*bps:]

		d0 := int(s[0]) - int(r[0])
		d1 := int(s[1]) - int(r[1])
		d2 := int(s[2]) - int(r[2])
		d3 := int(s[3]) - int(r[3])

		a0 := d0 + d3
		a1 := d1 + d2
		a2 := d1 - d2
		a3 := d0 - d3

		tmp[4*i] = (a0 + a1) * 8
		tmp[4*i+1] = (a2*2217 + a3*5352 + 1812) >> 9
		tmp[4*i+2] = (a0 - a1) * 8
		tmp[4*i+3] = (a3*2217 - a2*5352 + 937) >> 9
	}

	for i := range 4 {
		a0 := tmp[i] + tmp[12+i]
		a1 := tmp[4+i] + tmp[8+i]
		a2 := tmp[4+i] - tmp[8+i]
		a3 := tmp[i] - tmp[12+i]

		out[i] = int16((a0 + a1 + 7) >> 4)
		out[8+i] = int16((a0 - a1 + 7) >> 4)

		v := (a2*2217 + a3*5352 + 12000) >> 16
		if a3 != 0 {
			v++
		}

		out[4+i] = int16(v)
		out[12+i] = int16((a3*2217 - a2*5352 + 51000) >> 16)
	}
}

func fTransformWHT(in []int16, out []int16) {
	var tmp [16]int

	for i := range 4 {
		b := in[64*i:]

		a0 := int(b[0]) + int(b[32])
		a1 := int(b[16]) + int(b[48])
		a2 := int(b[16]) - int(b[48])
		a3 := int(b[0]) - int(b[32])

		tmp[4*i] = a0 + a1
		tmp[4*i+1] = a3 + a2
		tmp[4*i+2] = a3 - a2
		tmp[4*i+3] = a0 - a1
	}

	for i := range 4 {
		a0 := tmp[i] + tmp[8+i]
		a1 := tmp[4+i] + tmp[12+i]
		a2 := tmp[4+i] - tmp[12+i]
		a3 := tmp[i] - tmp[8+i]

		out[i] = int16((a0 + a1) >> 1)
		out[4+i] = int16((a3 + a2) >> 1)
		out[8+i] = int16((a3 - a2) >> 1)
		out[12+i] = int16((a0 - a1) >> 1)
	}
}

func quantizeBlock(in, out []int16, m *qmatrix, first int) int {
	if quantizeAsm != nil && m.narrow && len(in) >= 16 && len(out) >= 16 {
		dc := in[0]

		quantizeAsm(in, out, m)

		if first == 1 {
			in[0] = dc
		}

		var tmp [16]int16

		copy(tmp[:], out[:16])

		last := first - 1

		for n := first; n < 16; n++ {
			l := tmp[zigzag[n]]
			out[n] = l

			if l != 0 {
				last = n
			}
		}

		return last + 1
	}

	return quantizeBlockGo(in, out, m, first)
}

func quantizeBlockGo(in, out []int16, m *qmatrix, first int) int {
	last := first - 1

	for n := first; n < 16; n++ {
		j := zigzag[n]

		v := int32(in[j])
		neg := v < 0

		if neg {
			v = -v
		}

		coeff := uint32(v) + m.sharpen[j]

		if coeff <= m.zthresh[j] {
			out[n] = 0
			in[j] = 0

			continue
		}

		level := min((coeff*m.iq[j]+m.bias[j])>>qfix, maxLevel)

		q := int16(level * m.q[j])
		l := int16(level)

		if neg {
			q, l = -q, -l
		}

		out[n] = l
		in[j] = q
		last = n
	}

	return last + 1
}
