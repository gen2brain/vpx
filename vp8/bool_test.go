package vp8

import (
	"math/rand/v2"
	"testing"
)

// boolRef is the boolean decoder written the way RFC 6386 §7.3 spells it out,
// a bit at a time with a sixteen bit window. It is the reference the packed
// decoder is compared against.
type boolRef struct {
	buf   []byte
	pos   int
	value uint32
	rng   uint32
	count int
}

func (d *boolRef) byteAt() uint32 {
	if d.pos >= len(d.buf) {
		d.pos++

		return 0
	}

	b := d.buf[d.pos]
	d.pos++

	return uint32(b)
}

func (d *boolRef) init(b []byte) {
	d.buf = b
	d.pos = 0
	d.value = d.byteAt()<<8 | d.byteAt()
	d.rng = 255
	d.count = 0
}

func (d *boolRef) getBit(prob uint8) int {
	split := 1 + (d.rng-1)*uint32(prob)>>8
	bigsplit := split << 8

	retval := 0
	rng := split

	if d.value >= bigsplit {
		retval = 1
		rng = d.rng - split
		d.value -= bigsplit
	}

	for rng < 128 {
		d.value <<= 1
		rng <<= 1

		d.count++
		if d.count == 8 {
			d.count = 0
			d.value |= d.byteAt()
		}
	}

	d.rng = rng

	return retval
}

func (d *boolRef) getBits(n int) uint32 {
	var v uint32

	for n > 0 {
		n--
		v |= uint32(d.getBit(0x80)) << uint(n)
	}

	return v
}

func randomBuf(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.UintN(256))
	}

	return b
}

// TestBoolDecMatchesReference decodes the same buffer with the packed decoder
// and the reference one, with the same probabilities, and requires every bit
// to agree. The buffer is far larger than the decodes can consume, so neither
// runs off the end.
func TestBoolDecMatchesReference(t *testing.T) {
	const (
		size    = 4096
		decodes = 1000
	)

	for seed := range uint64(64) {
		r := rand.New(rand.NewPCG(seed, 0x9d012a))
		buf := randomBuf(r, size)

		var got boolDec

		var want boolRef

		got.init(buf)
		want.init(buf)

		for i := range decodes {
			prob := uint8(r.UintN(256))

			g, w := got.getBit(prob), want.getBit(prob)
			if g != w {
				t.Fatalf("seed %d decode %d prob %d: bit %d, want %d", seed, i, prob, g, w)
			}
		}
	}
}

// TestBoolDecGetBits checks the multi-bit reads, which the frame header uses
// for every literal field.
func TestBoolDecGetBits(t *testing.T) {
	for seed := range uint64(16) {
		r := rand.New(rand.NewPCG(seed, 1))
		buf := randomBuf(r, 4096)

		var got boolDec

		var want boolRef

		got.init(buf)
		want.init(buf)

		for i := range 300 {
			n := 1 + int(r.UintN(8))

			g, w := got.getBits(n), want.getBits(n)
			if g != w {
				t.Fatalf("seed %d read %d of %d bits: %d, want %d", seed, i, n, g, w)
			}
		}
	}
}

// TestBoolDecGetSigned checks the folded sign read against the plain one. The
// two decoders must stay in step, so the state is compared as well as the
// values. One bit is read first because the folded form needs rng settled.
func TestBoolDecGetSigned(t *testing.T) {
	for seed := range uint64(16) {
		r := rand.New(rand.NewPCG(seed, 2))
		buf := randomBuf(r, 4096)

		var folded, plain boolDec

		folded.init(buf)
		plain.init(buf)

		folded.getBit(0x80)
		plain.getBit(0x80)

		for i := range 500 {
			v := int32(r.UintN(2048))

			got := folded.getSigned(v)

			want := v
			if plain.getBit(0x80) != 0 {
				want = -v
			}

			if got != want {
				t.Fatalf("seed %d decode %d of %d: %d, want %d", seed, i, v, got, want)
			}

			if folded.value != plain.value || folded.rng != plain.rng ||
				folded.bits != plain.bits || folded.pos != plain.pos {
				t.Fatalf("seed %d decode %d: state diverged", seed, i)
			}
		}
	}
}

// TestBoolDecTruncated runs a decoder off the end of every buffer length up to
// a word, which must terminate rather than spin or read out of bounds.
func TestBoolDecTruncated(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))

	for n := range 16 {
		buf := randomBuf(r, n)

		var d boolDec

		d.init(buf)

		for range 200 {
			d.getBit(0x80)
		}

		if !d.eof {
			t.Errorf("length %d: not at eof after 200 decodes", n)
		}
	}
}

// TestBoolDecNoAlloc pins the decoder to zero allocations, which the frame
// loop depends on.
func TestBoolDecNoAlloc(t *testing.T) {
	buf := randomBuf(rand.New(rand.NewPCG(5, 6)), 4096)

	var d boolDec

	n := testing.AllocsPerRun(100, func() {
		d.init(buf)

		for range 1000 {
			d.getBit(0x80)
		}
	})

	if n != 0 {
		t.Errorf("%v allocations, want 0", n)
	}
}

// TestBoolDecGetSignedUnsettled pins the one state the folded sign read is
// wrong in, so that the precondition on it stays a fact rather than a hope.
func TestBoolDecGetSignedUnsettled(t *testing.T) {
	// A first byte below 128 takes the zero branch out of the initial range,
	// which is the case the folded form does not renormalize.
	buf := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}

	var folded, plain boolDec

	folded.init(buf)
	plain.init(buf)

	folded.getSigned(1)
	plain.getBit(0x80)

	if folded.rng == plain.rng {
		t.Error("folded sign read agrees on an unsettled range; the precondition can be dropped")
	}
}

// skewedProbs draws from the coefficient probability table, so the branch in
// getBit predicts about as well as it does on a real stream. Uniform random
// probabilities make it mispredict on every call, which overstates the cost of
// the branch by a factor that has already produced one wrong conclusion.
func skewedProbs(r *rand.Rand, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		t := r.IntN(numBlockTypes)
		b := r.IntN(numBands)
		c := r.IntN(numCtx)
		out[i] = coeffProbs[t][b][c][r.IntN(numProbas)]
	}

	return out
}

// BenchmarkBoolDecLoop is the control: the same loop with the decode taken
// out, so the harness cost can be subtracted from BenchmarkBoolDecGetBit.
func BenchmarkBoolDecLoop(b *testing.B) {
	const run = 1 << 13

	buf := randomBuf(rand.New(rand.NewPCG(7, 8)), 1<<20)
	probs := skewedProbs(rand.New(rand.NewPCG(9, 10)), 256)

	var d boolDec

	d.init(buf)

	b.ReportAllocs()

	sink := 0

	for i := 0; b.Loop(); i++ {
		if i%run == 0 {
			d.init(buf)
		}

		sink += int(probs[i&255])
	}

	boolSink = sink
}

var boolSink int

// BenchmarkBoolDecGetBit measures the steady state, re-initialising before the
// decoder can run off the end and start measuring the eof path instead.
func BenchmarkBoolDecGetBit(b *testing.B) {
	const run = 1 << 13

	buf := randomBuf(rand.New(rand.NewPCG(7, 8)), 1<<20)
	probs := skewedProbs(rand.New(rand.NewPCG(9, 10)), 256)

	var d boolDec

	d.init(buf)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		if i%run == 0 {
			d.init(buf)
		}

		d.getBit(probs[i&255])
	}
}

// TestBoolEncRoundTrip requires the encoder and the decoder to agree bit for
// bit on a sequence with realistic probabilities.
func TestBoolEncRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 32))

	for _, n := range []int{1, 2, 7, 64, 100000} {
		bits := make([]int, n)
		probs := make([]uint8, n)

		for i := range bits {
			bits[i] = rng.IntN(2)
			probs[i] = uint8(rng.IntN(255) + 1)
		}

		var e boolEnc

		e.init(nil)

		for i := range bits {
			e.putBit(bits[i], probs[i])
		}

		var d boolDec

		d.init(e.finish())

		for i := range bits {
			if got := d.getBit(probs[i]); got != bits[i] {
				t.Fatalf("n=%d: bit %d decoded as %d, want %d", n, i, got, bits[i])
			}
		}

		if d.eof {
			t.Errorf("n=%d: decoder ran past the end of what the encoder wrote", n)
		}
	}
}

func TestBoolEncCarry(t *testing.T) {
	var e boolEnc

	e.init(nil)

	// A long run of ones at a high probability is what makes the encoder hold
	// bytes back for a carry.
	for range 4096 {
		e.putBit(1, 254)
	}

	e.putBit(0, 2)

	var d boolDec

	d.init(e.finish())

	for i := range 4096 {
		if got := d.getBit(254); got != 1 {
			t.Fatalf("bit %d decoded as %d, want 1", i, got)
		}
	}

	if got := d.getBit(2); got != 0 {
		t.Fatalf("last bit decoded as %d, want 0", got)
	}
}

func TestBoolEncUniform(t *testing.T) {
	rng := rand.New(rand.NewPCG(33, 34))

	values := make([]uint32, 500)

	var e boolEnc

	e.init(nil)

	for i := range values {
		values[i] = rng.Uint32() & 0xfff
		e.putBits(values[i], 12)
	}

	var d boolDec

	d.init(e.finish())

	for i, want := range values {
		if got := d.getBits(12); got != want {
			t.Fatalf("value %d: %#x, want %#x", i, got, want)
		}
	}
}
