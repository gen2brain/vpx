package vp8

import (
	"bytes"
	"testing"
)

type byteSource struct {
	b []byte
	i int
}

func (s *byteSource) u8() int {
	if s.i >= len(s.b) {
		s.i++

		return 0
	}

	v := s.b[s.i]
	s.i++

	return int(v)
}

func (s *byteSource) intN(n int) int {
	return s.u8() * n / 256
}

func (s *byteSource) fill(p []byte) {
	for i := range p {
		p[i] = byte(s.u8())
	}
}

func (s *byteSource) fInfo() fInfo {
	level := 1 + s.intN(63)

	f := fInfo{
		ilevel:    1 + s.intN(63),
		hevThresh: s.intN(4),
	}

	f.limit = 2*level + f.ilevel

	if s.intN(2) == 0 {
		f = f.edge()
	}

	return f
}

// FuzzKernels drives every dispatched kernel against its scalar form on
// fuzzer-chosen planes, sizes and filter parameters.
func FuzzKernels(f *testing.F) {
	f.Add(bytes.Repeat([]byte{0x80}, 512))
	f.Add(bytes.Repeat([]byte{0x00}, 512))
	f.Add(bytes.Repeat([]byte{0xff}, 512))

	seed := make([]byte, 512)
	for i := range seed {
		seed[i] = byte(i*37 + i/16)
	}

	f.Add(seed)

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 64 {
			return
		}

		s := &byteSource{b: data}

		switch s.intN(6) {
		case 0:
			fuzzFilters(t, s)
		case 1:
			fuzzTransforms(t, s)
		case 2:
			fuzzSixtap(t, s)
		case 3:
			fuzzTrueMotion(t, s)
		case 4:
			fuzzSSE(t, s)
		case 5:
			fuzzEncode(t, s)
		}
	})
}

func fuzzFilters(t *testing.T, s *byteSource) {
	if vFilter16Asm == nil {
		return
	}

	const (
		stride = 64
		rows   = 40
	)

	src := make([]byte, stride*rows)
	s.fill(src)

	want := make([]byte, len(src))
	got := make([]byte, len(src))
	wantV := make([]byte, len(src))
	gotV := make([]byte, len(src))

	inf := s.fInfo()
	off := (8+s.intN(8))*stride + 8 + s.intN(16)

	for _, p := range [][]byte{want, got, wantV, gotV} {
		copy(p, src)
	}

	switch s.intN(8) {
	case 0:
		vFilterLoop26(want, off, stride, 16, inf)
		vFilter16Asm(got, off, stride, inf.limit, inf.ilevel, inf.hevThresh)
	case 1:
		for k := range 3 {
			vFilterLoop24(want, off+(k+1)*4*stride, stride, 16, inf)
		}

		vFilter16iAsm(got, off, stride, inf.limit, inf.ilevel, inf.hevThresh)
	case 2:
		vFilterLoop26(want, off, stride, 8, inf)
		vFilterLoop26(wantV, off, stride, 8, inf)
		vFilter8Asm(got, gotV, off, stride, inf.limit, inf.ilevel, inf.hevThresh)
	case 3:
		vFilterLoop24(want, off+4*stride, stride, 8, inf)
		vFilterLoop24(wantV, off+4*stride, stride, 8, inf)
		vFilter8iAsm(got, gotV, off, stride, inf.limit, inf.ilevel, inf.hevThresh)
	case 4:
		hFilterLoop26(want, off, stride, 16, inf)
		hFilter16Asm(got, off, stride, inf.limit, inf.ilevel, inf.hevThresh)
	case 5:
		for k := range 3 {
			hFilterLoop24(want, off+(k+1)*4, stride, 16, inf)
		}

		hFilter16iAsm(got, off, stride, inf.limit, inf.ilevel, inf.hevThresh)
	case 6:
		hFilterLoop26(want, off, stride, 8, inf)
		hFilterLoop26(wantV, off, stride, 8, inf)
		hFilter8Asm(got, gotV, off, stride, inf.limit, inf.ilevel, inf.hevThresh)
	case 7:
		hFilterLoop24(want, off+4, stride, 8, inf)
		hFilterLoop24(wantV, off+4, stride, 8, inf)
		hFilter8iAsm(got, gotV, off, stride, inf.limit, inf.ilevel, inf.hevThresh)
	}

	if !bytes.Equal(want, got) || !bytes.Equal(wantV, gotV) {
		t.Fatalf("filter mismatch: off=%d %+v", off, inf)
	}
}

func fuzzTransforms(t *testing.T, s *byteSource) {
	if transformAsm == nil || transformDCAsm == nil {
		return
	}

	var (
		in   [32]int16
		want [yuvSize]uint8
		got  [yuvSize]uint8
	)

	for i := range in {
		in[i] = int16(s.u8()<<8 | s.u8())
	}

	for i := range want {
		want[i] = uint8(s.u8())
	}

	got = want
	off := 32 + s.intN(len(want)-4*bps-16)

	switch s.intN(3) {
	case 0:
		transformOneScalar(in[:], want[:], off)
		transformAsm(in[:], got[:], off, 0)
	case 1:
		transformOneScalar(in[:], want[:], off)
		transformOneScalar(in[16:], want[:], off+4)
		transformAsm(in[:], got[:], off, 1)
	case 2:
		transformDCScalar(in[:], want[:], off)
		transformDCAsm(in[:], got[:], off)
	}

	if !bytes.Equal(want[:], got[:]) {
		t.Fatalf("transform mismatch: off=%d coeffs %v", off, in)
	}
}

func fuzzSixtap(t *testing.T, s *byteSource) {
	if sixtapHAsm == nil {
		return
	}

	const stride = 64

	src := make([]byte, stride*48)
	want := make([]byte, stride*48)
	got := make([]byte, stride*48)

	s.fill(src)
	s.fill(want)
	copy(got, want)

	w := [3]int{4, 8, 16}[s.intN(3)]
	h := [4]int{4, 8, 16, 21}[s.intN(4)]
	filt := &subPelFilters[1+s.intN(7)]

	sOff := (8+s.intN(8))*stride + 8 + s.intN(16)
	dOff := (8+s.intN(8))*stride + 8 + s.intN(16)

	if s.intN(2) == 0 {
		sixtapHGo(want, dOff, stride, src, sOff, stride, w, h, filt)
		sixtapHAsm(got, dOff, stride, src, sOff, stride, w, h, filt)
	} else {
		sixtapVGo(want, dOff, stride, src, sOff, stride, w, h, filt)
		sixtapVAsm(got, dOff, stride, src, sOff, stride, w, h, filt)
	}

	if !bytes.Equal(want, got) {
		t.Fatalf("sixtap mismatch: w=%d h=%d", w, h)
	}
}

func fuzzTrueMotion(t *testing.T, s *byteSource) {
	if trueMotionAsm == nil {
		return
	}

	want := make([]byte, yuvSize)
	got := make([]byte, yuvSize)

	s.fill(want)
	copy(got, want)

	size := [3]int{4, 8, 16}[s.intN(3)]
	off := bps + 1 + s.intN(len(want)-17*bps-16)

	trueMotionGo(want, off, size)
	trueMotionAsm(got, off, size)

	if !bytes.Equal(want, got) {
		t.Fatalf("trueMotion mismatch: size=%d off=%d", size, off)
	}
}

func fuzzSSE(t *testing.T, s *byteSource) {
	if sseAsm == nil {
		return
	}

	a := make([]byte, bps*24)
	c := make([]byte, bps*24)

	s.fill(a)
	s.fill(c)

	size := [3]int{4, 8, 16}[s.intN(3)]
	off := s.intN(4)*bps + s.intN(8)

	if want, got := sseGo(a, c, off, size), sseAsm(a, c, off, size); want != got {
		t.Fatalf("sse(%d) = %d, want %d", size, got, want)
	}
}

func fuzzEncode(t *testing.T, s *byteSource) {
	if quantizeAsm == nil || fTransformAsm == nil {
		return
	}

	var m qmatrix

	m.q[0] = uint32(4 + s.intN(300))
	m.q[1] = uint32(4 + s.intN(300))
	m.expand(s.intN(3))

	var in, want, got [16]int16

	for i := range in {
		in[i] = int16(s.u8()<<8 | s.u8())
	}

	scratch := in

	quantizeBlockGo(in[:], want[:], &m, 0)
	quantizeAsm(scratch[:], got[:], &m)

	for n := range 16 {
		if got[zigzag[n]] != want[n] {
			t.Fatalf("quantize lane %d: got %d, want %d", zigzag[n], got[zigzag[n]], want[n])
		}
	}

	src := make([]byte, bps*8)
	ref := make([]byte, bps*8)

	s.fill(src)
	s.fill(ref)

	var fwant, fgot [16]int16

	fTransformGo(src, ref, 0, 0, fwant[:])
	fTransformAsm(src, ref, 0, 0, fgot[:])

	if fwant != fgot {
		t.Fatalf("fTransform: got %v, want %v", fgot, fwant)
	}

	if fTransform2Asm == nil {
		return
	}

	var twant, tgot [32]int16

	fTransformGo(src, ref, 0, 0, twant[:16])
	fTransformGo(src, ref, 4, 4, twant[16:])
	fTransform2Asm(src, ref, 0, 0, tgot[:])

	if twant != tgot {
		t.Fatalf("fTransform2: got %v, want %v", tgot, twant)
	}
}
