//go:build amd64 && !noasm

package webp

import (
	"math/rand/v2"
	"testing"
)

func TestMatchLengthSSEAndAVX2(t *testing.T) {
	r := rand.New(rand.NewPCG(27, 28))

	a := make([]uint32, 64)
	b := make([]uint32, 64)

	for range 50000 {
		for i := range a {
			a[i] = r.Uint32()
			b[i] = a[i]
		}

		limit := 1 + r.IntN(len(a))

		if d := r.IntN(len(a) + 4); d < len(a) {
			b[d] = a[d] ^ (1 << r.IntN(32))
		}

		want := matchLength(a, b, limit)

		if got := matchLengthSSE(&a[0], &b[0], limit); got != want {
			t.Fatalf("limit %d: SSE2 got %d, want %d", limit, got, want)
		}

		if hasAVX2 {
			if got := matchLengthAVX2(&a[0], &b[0], limit); got != want {
				t.Fatalf("limit %d: AVX2 got %d, want %d", limit, got, want)
			}
		}
	}
}
