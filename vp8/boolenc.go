package vp8

import "math/bits"

type boolEnc struct {
	buf    []byte
	value  uint32
	rng    uint32
	nbBits int
	run    int
}

func (e *boolEnc) init(buf []byte) {
	e.buf = buf[:0]
	e.value = 0
	e.rng = 254
	e.nbBits = -8
	e.run = 0
}

func (e *boolEnc) flush() {
	s := uint(8 + e.nbBits)
	v := e.value >> s

	e.value -= v << s
	e.nbBits -= 8

	if v&0xff == 0xff {
		e.run++

		return
	}

	if v&0x100 != 0 && len(e.buf) > 0 {
		e.buf[len(e.buf)-1]++
	}

	if e.run > 0 {
		pending := byte(0xff)
		if v&0x100 != 0 {
			pending = 0
		}

		for ; e.run > 0; e.run-- {
			e.buf = append(e.buf, pending)
		}
	}

	e.buf = append(e.buf, byte(v))
}

func (e *boolEnc) put(bit int, prob uint8) {
	split := e.rng * uint32(prob) >> 8

	if bit != 0 {
		e.value += split + 1
		e.rng -= split + 1
	} else {
		e.rng = split
	}

	if e.rng < 127 {
		shift := 8 - bits.Len32(e.rng+1)

		e.rng = (e.rng+1)<<uint(shift) - 1
		e.value <<= uint(shift)
		e.nbBits += shift
	}
}

func (e *boolEnc) flushIf() {
	if e.nbBits > 0 {
		e.flush()
	}
}

func (e *boolEnc) putBit(bit int, prob uint8) {
	e.put(bit, prob)
	e.flushIf()
}

func (e *boolEnc) putBool(v bool, prob uint8) {
	bit := 0
	if v {
		bit = 1
	}

	e.putBit(bit, prob)
}

func (e *boolEnc) putFlag(v bool) {
	e.putBool(v, 0x80)
}

func (e *boolEnc) putBits(v uint32, n int) {
	for n > 0 {
		n--
		e.putBit(int(v>>uint(n)&1), 0x80)
	}
}

func (e *boolEnc) finish() []byte {
	e.putBits(0, 9-e.nbBits)
	e.nbBits = 0
	e.flush()

	return e.buf
}
