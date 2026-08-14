package vp8

import (
	"encoding/binary"
	"math/bits"
)

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
		d.value = binary.BigEndian.Uint64(d.buf[d.pos:])>>8 | d.value<<boolBits
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
		d.value = uint64(d.buf[d.pos]) | d.value<<8
		d.bits += 8
		d.pos++
	case !d.eof:
		d.value <<= 8
		d.bits += 8
		d.eof = true
	default:
		d.bits = 0
	}
}

func (d *boolDec) getBit(prob uint8) int {
	r := d.rng

	if d.bits < 0 {
		d.load()
	}

	pos := uint(d.bits)
	split := r * uint32(prob) >> 8

	bit := 0

	if uint32(d.value>>pos) > split {
		bit = 1
		r -= split
		d.value -= uint64(split+1) << pos
	} else {
		r = split + 1
	}

	shift := 8 - bits.Len32(r)
	r <<= uint(shift)
	d.bits -= shift
	d.rng = r - 1

	return bit
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
	if d.bits < 0 {
		d.load()
	}

	pos := uint(d.bits)
	split := d.rng >> 1
	mask := int32(split-uint32(d.value>>pos)) >> 31

	d.bits--
	d.rng += uint32(mask)
	d.rng |= 1
	d.value -= uint64((split+1)&uint32(mask)) << pos

	return (v ^ mask) - mask
}

func (d *boolDec) flagged(n int) int {
	if !d.getFlag() {
		return 0
	}

	return d.getSignedBits(n)
}
