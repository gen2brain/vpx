package vp8

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

func TestSixtapKernelsMatchScalar(t *testing.T) {
	if sixtapHAsm == nil && sixtapVAsm == nil {
		t.Skip("no kernels compiled in")
	}

	const stride = 64

	r := rand.New(rand.NewPCG(11, 12))

	src := make([]byte, stride*48)
	want := make([]byte, stride*48)
	got := make([]byte, stride*48)

	for iter := range 20000 {
		for i := range src {
			src[i] = uint8(r.IntN(256))
		}

		for i := range want {
			want[i] = uint8(r.IntN(256))
		}

		copy(got, want)

		w := [3]int{4, 8, 16}[r.IntN(3)]
		h := [4]int{4, 8, 16, 21}[r.IntN(4)]
		f := &subPelFilters[1+r.IntN(7)]

		sOff := (8+r.IntN(8))*stride + 8 + r.IntN(16)
		dOff := (8+r.IntN(8))*stride + 8 + r.IntN(16)

		if iter&1 == 0 {
			sixtapHGo(want, dOff, stride, src, sOff, stride, w, h, f)
			sixtapHAsm(got, dOff, stride, src, sOff, stride, w, h, f)
		} else {
			sixtapVGo(want, dOff, stride, src, sOff, stride, w, h, f)
			sixtapVAsm(got, dOff, stride, src, sOff, stride, w, h, f)
		}

		if !bytes.Equal(want, got) {
			t.Fatalf("iter %d: sixtap mismatch w=%d h=%d sOff=%d dOff=%d f=%v", iter, w, h, sOff, dOff, f)
		}
	}
}
