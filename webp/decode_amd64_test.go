//go:build amd64 && !noasm

package webp

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

func TestARGBToRGBASSEAndAVX2(t *testing.T) {
	r := rand.New(rand.NewPCG(33, 34))

	px := make([]uint32, 137)
	for i := range px {
		px[i] = r.Uint32()
	}

	want := make([]byte, 4*len(px))
	got := make([]byte, 4*len(px))

	for n := 1; n <= len(px); n++ {
		clear(want)
		clear(got)
		argbToRGBAScalar(want[:4*n], px[:n])

		argbToRGBASSE(&got[0], &px[0], n)

		if !bytes.Equal(want, got) {
			t.Fatalf("n=%d: SSE2 got %v, want %v", n, got[:4*n], want[:4*n])
		}

		if !hasAVX2 {
			continue
		}

		clear(got)
		argbToRGBAAVX2(&got[0], &px[0], n)

		if !bytes.Equal(want, got) {
			t.Fatalf("n=%d: AVX2 got %v, want %v", n, got[:4*n], want[:4*n])
		}
	}
}
