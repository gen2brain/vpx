package vp8

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

func TestTrueMotionKernelMatchesScalar(t *testing.T) {
	if trueMotionAsm == nil {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(29, 30))

	want := make([]byte, yuvSize)
	got := make([]byte, yuvSize)

	for iter := range 20000 {
		for i := range want {
			want[i] = uint8(r.IntN(256))
		}

		copy(got, want)

		size := [3]int{4, 8, 16}[r.IntN(3)]
		off := bps + 1 + r.IntN(len(want)-17*bps-16)

		trueMotionGo(want, off, size)
		trueMotionAsm(got, off, size)

		if !bytes.Equal(want, got) {
			t.Fatalf("iter %d: trueMotion mismatch size=%d off=%d", iter, size, off)
		}
	}
}
