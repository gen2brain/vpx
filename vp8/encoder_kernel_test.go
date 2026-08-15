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

func TestQuantizeKernelMatchesScalar(t *testing.T) {
	if quantizeAsm == nil {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(19, 20))

	var m qmatrix

	for iter := range 20000 {
		saturate := iter%4 == 0

		for i := range 2 {
			m.q[i] = uint32(4 + r.IntN(300))

			if saturate {
				m.q[i] = 4
			}
		}

		m.expand(r.IntN(3))

		if !m.narrow {
			t.Fatalf("iter %d: matrix not narrow for q %v", iter, m.q[:2])
		}

		var inA, inB, outA, outB [16]int16

		for i := range inA {
			inA[i] = int16(r.IntN(1<<14) - 1<<13)

			if saturate {
				inA[i] = int16(8000 + r.IntN(1000))

				if r.IntN(2) == 0 {
					inA[i] = -inA[i]
				}
			}
		}

		inB = inA
		first := r.IntN(2)

		wantN := quantizeBlockGo(inA[:], outA[:], &m, first)
		gotN := quantizeBlock(inB[:], outB[:], &m, first)

		if wantN != gotN || inA != inB {
			t.Fatalf("iter %d: n = %d, want %d; in = %v, want %v", iter, gotN, wantN, inB, inA)
		}

		for n := first; n < 16; n++ {
			if outA[n] != outB[n] {
				t.Fatalf("iter %d: out[%d] = %d, want %d", iter, n, outB[n], outA[n])
			}
		}
	}
}
