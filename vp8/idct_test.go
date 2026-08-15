package vp8

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

func randCoeffs(r *rand.Rand, in []int16, dense bool) {
	clear(in)

	amp := 1 << (1 + r.IntN(14))

	in[0] = int16(r.IntN(2*amp) - amp)

	n := 1 + r.IntN(3)
	if dense {
		n = len(in)
	}

	for range n {
		in[r.IntN(len(in))] = int16(r.IntN(2*amp) - amp)
	}
}

func TestTransformKernelsMatchScalar(t *testing.T) {
	if transformAsm == nil && transformDCAsm == nil {
		t.Skip("no kernels compiled in")
	}

	r := rand.New(rand.NewPCG(7, 8))

	var (
		in   [32]int16
		want [yuvSize]uint8
		got  [yuvSize]uint8
	)

	for iter := range 20000 {
		randCoeffs(r, in[:], iter%4 == 0)

		for i := range want {
			want[i] = uint8(r.IntN(256))
		}

		got = want

		off := 32 + r.IntN(len(want)-4*bps-16)

		switch kind := iter % 3; kind {
		case 0:
			if transformAsm == nil {
				continue
			}

			transformOneScalar(in[:], want[:], off)
			transformAsm(in[:], got[:], off, 0)
		case 1:
			if transformAsm == nil {
				continue
			}

			transformOneScalar(in[:], want[:], off)
			transformOneScalar(in[16:], want[:], off+4)
			transformAsm(in[:], got[:], off, 1)
		case 2:
			if transformDCAsm == nil {
				continue
			}

			transformDCScalar(in[:], want[:], off)
			transformDCAsm(in[:], got[:], off)
		}

		if !bytes.Equal(want[:], got[:]) {
			t.Fatalf("iter %d: transform mismatch at off=%d, coeffs %v", iter, off, in)
		}
	}
}
