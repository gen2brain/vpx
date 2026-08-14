package vp8

const bps = 32

const (
	idctC1 = 20091
	idctC2 = 35468
)

func mul1(a int) int { return a*idctC1>>16 + a }

func mul2(a int) int { return a * idctC2 >> 16 }

func clip8(v int) uint8 {
	if v&^0xff == 0 {
		return uint8(v)
	}

	if v < 0 {
		return 0
	}

	return 255
}

func store(b []byte, off, x, y, v int) {
	p := off + x + y*bps
	b[p] = clip8(int(b[p]) + v>>3)
}

func store2(b []byte, off, y, dc, d, c int) {
	store(b, off, 0, y, dc+d)
	store(b, off, 1, y, dc+c)
	store(b, off, 2, y, dc-c)
	store(b, off, 3, y, dc-d)
}

func transformOne(in []int16, b []byte, off int) {
	var c [16]int

	for i := range 4 {
		a := int(in[i]) + int(in[8+i])
		d := int(in[i]) - int(in[8+i])
		e := mul2(int(in[4+i])) - mul1(int(in[12+i]))
		f := mul1(int(in[4+i])) + mul2(int(in[12+i]))

		c[4*i] = a + f
		c[4*i+1] = d + e
		c[4*i+2] = d - e
		c[4*i+3] = a - f
	}

	for i := range 4 {
		dc := c[i] + 4
		a := dc + c[8+i]
		d := dc - c[8+i]
		e := mul2(c[4+i]) - mul1(c[12+i])
		f := mul1(c[4+i]) + mul2(c[12+i])

		store(b, off, 0, i, a+f)
		store(b, off, 1, i, d+e)
		store(b, off, 2, i, d-e)
		store(b, off, 3, i, a-f)
	}
}

func transformAC3(in []int16, b []byte, off int) {
	a := int(in[0]) + 4
	c4 := mul2(int(in[4]))
	d4 := mul1(int(in[4]))
	c1 := mul2(int(in[1]))
	d1 := mul1(int(in[1]))

	store2(b, off, 0, a+d4, d1, c1)
	store2(b, off, 1, a+c4, d1, c1)
	store2(b, off, 2, a-c4, d1, c1)
	store2(b, off, 3, a-d4, d1, c1)
}

func transformDC(in []int16, b []byte, off int) {
	dc := int(in[0]) + 4

	for y := range 4 {
		for x := range 4 {
			store(b, off, x, y, dc)
		}
	}
}

func transform(in []int16, b []byte, off int, two bool) {
	transformOne(in, b, off)

	if two {
		transformOne(in[16:], b, off+4)
	}
}

func transformUV(in []int16, b []byte, off int) {
	transform(in, b, off, true)
	transform(in[32:], b, off+4*bps, true)
}

func transformDCUV(in []int16, b []byte, off int) {
	if in[0] != 0 {
		transformDC(in, b, off)
	}

	if in[16] != 0 {
		transformDC(in[16:], b, off+4)
	}

	if in[32] != 0 {
		transformDC(in[32:], b, off+4*bps)
	}

	if in[48] != 0 {
		transformDC(in[48:], b, off+4*bps+4)
	}
}

func transformWHT(in []int16, out []int16) {
	var tmp [16]int

	for i := range 4 {
		a0 := int(in[i]) + int(in[12+i])
		a1 := int(in[4+i]) + int(in[8+i])
		a2 := int(in[4+i]) - int(in[8+i])
		a3 := int(in[i]) - int(in[12+i])

		tmp[i] = a0 + a1
		tmp[8+i] = a0 - a1
		tmp[4+i] = a3 + a2
		tmp[12+i] = a3 - a2
	}

	for i := range 4 {
		dc := tmp[4*i] + 3
		a0 := dc + tmp[4*i+3]
		a1 := tmp[4*i+1] + tmp[4*i+2]
		a2 := tmp[4*i+1] - tmp[4*i+2]
		a3 := dc - tmp[4*i+3]

		out[64*i] = int16((a0 + a1) >> 3)
		out[64*i+16] = int16((a3 + a2) >> 3)
		out[64*i+32] = int16((a0 - a1) >> 3)
		out[64*i+48] = int16((a3 - a2) >> 3)
	}
}
