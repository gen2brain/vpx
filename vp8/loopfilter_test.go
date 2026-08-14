package vp8

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

func scalarFilterLoop(p []byte, off, hstride, vstride, size int, f fInfo, six bool) {
	t := 2*f.limit + 1

	for range size {
		if needsFilter2(p, off, hstride, t, f.ilevel) {
			switch {
			case hev(p, off, hstride, f.hevThresh):
				doFilter2(p, off, hstride)
			case six:
				doFilter6(p, off, hstride)
			default:
				doFilter4(p, off, hstride)
			}
		}

		off += vstride
	}
}

func randWalk(r *rand.Rand, v []int, amp, jump int) {
	for i := range v {
		d := r.IntN(2*amp) - amp
		if r.IntN(8) == 0 {
			d = r.IntN(2*jump) - jump
		}

		if i > 0 {
			v[i] = v[i-1] + d
		}
	}
}

func randPlane(r *rand.Rand, p []byte, stride int, rows, cols []int) {
	amp := 1 + r.IntN(1+r.IntN(40))
	jump := 1 + r.IntN(120)

	randWalk(r, rows, amp, jump)
	randWalk(r, cols, amp, jump)

	for y := range rows {
		for x := range cols {
			p[y*stride+x] = clip8(128 + rows[y] + cols[x] + r.IntN(2*amp) - amp)
		}
	}
}

func randFInfo(r *rand.Rand) fInfo {
	level := 1 + r.IntN(63)

	f := fInfo{
		ilevel:    1 + r.IntN(63),
		hevThresh: r.IntN(4),
	}

	f.limit = 2*level + f.ilevel

	if r.IntN(2) == 0 {
		f = f.edge()
	}

	return f
}

func TestFilterLoopsMatchScalar(t *testing.T) {
	const (
		stride = 64
		rows   = 40
	)

	r := rand.New(rand.NewPCG(3, 4))

	src := make([]byte, stride*rows)
	want := make([]byte, stride*rows)
	got := make([]byte, stride*rows)
	rowWalk := make([]int, rows)
	colWalk := make([]int, stride)

	for range 20000 {
		randPlane(r, src, stride, rowWalk, colWalk)

		f := randFInfo(r)
		six := r.IntN(2) == 0

		size := 8
		if r.IntN(2) == 0 {
			size = 16
		}

		vertical := r.IntN(2) == 0

		off := (8+r.IntN(16))*stride + 8 + r.IntN(16)

		copy(want, src)
		copy(got, src)

		if vertical {
			scalarFilterLoop(want, off, stride, 1, size, f, six)

			if six {
				vFilterLoop26(got, off, stride, size, f)
			} else {
				vFilterLoop24(got, off, stride, size, f)
			}
		} else {
			scalarFilterLoop(want, off, 1, stride, size, f, six)

			if six {
				hFilterLoop26(got, off, stride, size, f)
			} else {
				hFilterLoop24(got, off, stride, size, f)
			}
		}

		if !bytes.Equal(want, got) {
			t.Fatalf("filter loop mismatch: vertical=%v six=%v size=%d off=%d %+v", vertical, six, size, off, f)
		}
	}
}

func TestFilterKernelsMatchScalar(t *testing.T) {
	if vFilter16Asm == nil {
		t.Skip("no kernels compiled in")
	}

	const (
		stride = 64
		rows   = 40
	)

	r := rand.New(rand.NewPCG(5, 6))

	src := make([]byte, stride*rows)
	want := make([]byte, stride*rows)
	got := make([]byte, stride*rows)
	wantV := make([]byte, stride*rows)
	gotV := make([]byte, stride*rows)
	rowWalk := make([]int, rows)
	colWalk := make([]int, stride)

	for iter := range 20000 {
		randPlane(r, src, stride, rowWalk, colWalk)

		f := randFInfo(r)
		off := (8+r.IntN(8))*stride + 8 + r.IntN(16)

		copy(want, src)
		copy(got, src)
		copy(wantV, src)
		copy(gotV, src)

		kind := iter & 7

		switch kind {
		case 0:
			vFilterLoop26(want, off, stride, 16, f)
			vFilter16Asm(got, off, stride, f.limit, f.ilevel, f.hevThresh)
		case 1:
			for k := range 3 {
				vFilterLoop24(want, off+(k+1)*4*stride, stride, 16, f)
			}

			vFilter16iAsm(got, off, stride, f.limit, f.ilevel, f.hevThresh)
		case 2:
			vFilterLoop26(want, off, stride, 8, f)
			vFilterLoop26(wantV, off, stride, 8, f)
			vFilter8Asm(got, gotV, off, stride, f.limit, f.ilevel, f.hevThresh)
		case 3:
			vFilterLoop24(want, off+4*stride, stride, 8, f)
			vFilterLoop24(wantV, off+4*stride, stride, 8, f)
			vFilter8iAsm(got, gotV, off, stride, f.limit, f.ilevel, f.hevThresh)
		case 4:
			hFilterLoop26(want, off, stride, 16, f)
			hFilter16Asm(got, off, stride, f.limit, f.ilevel, f.hevThresh)
		case 5:
			for k := range 3 {
				hFilterLoop24(want, off+(k+1)*4, stride, 16, f)
			}

			hFilter16iAsm(got, off, stride, f.limit, f.ilevel, f.hevThresh)
		case 6:
			hFilterLoop26(want, off, stride, 8, f)
			hFilterLoop26(wantV, off, stride, 8, f)
			hFilter8Asm(got, gotV, off, stride, f.limit, f.ilevel, f.hevThresh)
		case 7:
			hFilterLoop24(want, off+4, stride, 8, f)
			hFilterLoop24(wantV, off+4, stride, 8, f)
			hFilter8iAsm(got, gotV, off, stride, f.limit, f.ilevel, f.hevThresh)
		}

		if !bytes.Equal(want, got) || !bytes.Equal(wantV, gotV) {
			t.Fatalf("kernel %d mismatch: off=%d %+v", kind, off, f)
		}
	}
}
