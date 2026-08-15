package vp8

import (
	"math/rand/v2"
	"testing"
)

func TestSSEKernelMatchesScalar(t *testing.T) {
	if sseAsm == nil {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(15, 16))

	a := make([]byte, bps*24)
	b := make([]byte, bps*24)

	for iter := range 20000 {
		for i := range a {
			a[i] = uint8(r.IntN(256))
			b[i] = uint8(r.IntN(256))
		}

		size := [3]int{4, 8, 16}[r.IntN(3)]
		off := r.IntN(4)*bps + r.IntN(8)

		want := sseGo(a, b, off, size)
		got := sseAsm(a, b, off, size)

		if want != got {
			t.Fatalf("iter %d: sse(%d) = %d, want %d", iter, size, got, want)
		}
	}
}

func TestFTransformKernelMatchesScalar(t *testing.T) {
	if fTransformAsm == nil {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(17, 18))

	src := make([]byte, bps*8)
	ref := make([]byte, bps*8)

	var want, got [16]int16

	for iter := range 20000 {
		for i := range src {
			src[i] = uint8(r.IntN(256))
			ref[i] = uint8(r.IntN(256))
		}

		fTransformGo(src, ref, 0, 0, want[:])
		fTransformAsm(src, ref, 0, 0, got[:])

		if want != got {
			t.Fatalf("iter %d: fTransform = %v, want %v", iter, got, want)
		}
	}
}
