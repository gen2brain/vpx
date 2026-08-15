package vp8

import (
	"encoding/binary"
)

var normShift = [256]uint8{
	8, 7, 6, 6, 5, 5, 5, 5, 4, 4, 4, 4, 4, 4, 4, 4,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

var rngNorm = [256]uint8{
	255, 127, 127, 191, 127, 159, 191, 223, 127, 143, 159, 175, 191, 207, 223, 239,
	127, 135, 143, 151, 159, 167, 175, 183, 191, 199, 207, 215, 223, 231, 239, 247,
	127, 131, 135, 139, 143, 147, 151, 155, 159, 163, 167, 171, 175, 179, 183, 187,
	191, 195, 199, 203, 207, 211, 215, 219, 223, 227, 231, 235, 239, 243, 247, 251,
	127, 129, 131, 133, 135, 137, 139, 141, 143, 145, 147, 149, 151, 153, 155, 157,
	159, 161, 163, 165, 167, 169, 171, 173, 175, 177, 179, 181, 183, 185, 187, 189,
	191, 193, 195, 197, 199, 201, 203, 205, 207, 209, 211, 213, 215, 217, 219, 221,
	223, 225, 227, 229, 231, 233, 235, 237, 239, 241, 243, 245, 247, 249, 251, 253,
	127, 128, 129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140, 141, 142,
	143, 144, 145, 146, 147, 148, 149, 150, 151, 152, 153, 154, 155, 156, 157, 158,
	159, 160, 161, 162, 163, 164, 165, 166, 167, 168, 169, 170, 171, 172, 173, 174,
	175, 176, 177, 178, 179, 180, 181, 182, 183, 184, 185, 186, 187, 188, 189, 190,
	191, 192, 193, 194, 195, 196, 197, 198, 199, 200, 201, 202, 203, 204, 205, 206,
	207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219, 220, 221, 222,
	223, 224, 225, 226, 227, 228, 229, 230, 231, 232, 233, 234, 235, 236, 237, 238,
	239, 240, 241, 242, 243, 244, 245, 246, 247, 248, 249, 250, 251, 252, 253, 254,
}

const boolBits = 56

type boolDec struct {
	value uint64
	rng   uint32
	bits  int
	buf   []byte
	pos   int
	max   int
	eof   bool
}

func (d *boolDec) init(b []byte) {
	d.value = 0
	d.rng = 254
	d.bits = -8
	d.buf = b
	d.pos = 0
	d.eof = false

	d.max = 0
	if len(b) >= 8 {
		d.max = len(b) - 7
	}

	d.load()
}

func (d *boolDec) load() {
	if d.pos < d.max {
		d.value |= binary.BigEndian.Uint64(d.buf[d.pos:]) >> 8 << (uint(-d.bits) & 63)
		d.pos += boolBits / 8
		d.bits += boolBits

		return
	}

	d.loadFinal()
}

//go:noinline
func (d *boolDec) loadFinal() {
	switch {
	case d.pos < len(d.buf):
		d.value |= uint64(d.buf[d.pos]) << (uint(48-d.bits) & 63)
		d.bits += 8
		d.pos++
	case !d.eof:
		d.bits += 8
		d.eof = true
	default:
		d.bits = 0
	}
}

func (d *boolDec) fill() {
	if d.bits < 0 {
		d.load()
	}
}

func (d *boolDec) getBitFast(prob uint32) int {
	if boolDebug && d.bits < 0 {
		panic("vp8: fast boolean read without fill")
	}

	r := d.rng
	split := r * prob >> 8

	bit := 0

	if uint32(d.value>>56) > split {
		bit = 1
		r -= split
		d.value -= uint64(split+1) << 56
	} else {
		r = split + 1
	}

	shift := uint(normShift[r])
	d.bits -= int(shift)
	d.rng = uint32(rngNorm[r])
	d.value <<= shift

	return bit
}

func (d *boolDec) getBit(prob uint8) int {
	d.fill()

	return d.getBitFast(uint32(prob))
}

func (d *boolDec) getFlag() bool {
	return d.getBit(0x80) != 0
}

func (d *boolDec) getBits(n int) uint32 {
	var v uint32

	for n > 0 {
		n--
		v |= uint32(d.getBit(0x80)) << uint(n)
	}

	return v
}

func (d *boolDec) getSignedBits(n int) int {
	v := int(d.getBits(n))

	if d.getFlag() {
		return -v
	}

	return v
}

func (d *boolDec) getSigned(v int32) int32 {
	d.fill()

	return d.getSignedFast(v)
}

func (d *boolDec) getSignedFast(v int32) int32 {
	if boolDebug && d.bits < 0 {
		panic("vp8: fast boolean read without fill")
	}

	split := d.rng >> 1
	mask := int32(split-uint32(d.value>>56)) >> 31

	d.bits--
	d.rng += uint32(mask)
	d.rng |= 1
	d.value -= uint64((split+1)&uint32(mask)) << 56
	d.value <<= 1

	return (v ^ mask) - mask
}

func (d *boolDec) flagged(n int) int {
	if !d.getFlag() {
		return 0
	}

	return d.getSignedBits(n)
}
