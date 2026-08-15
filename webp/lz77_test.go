package webp

import (
	"math/rand/v2"
	"testing"
)

func TestMatchLengthKernelMatchesScalar(t *testing.T) {
	if matchLengthAsm == nil {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(25, 26))

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

		if got := matchLengthAsm(a, b, limit); got != want {
			t.Fatalf("limit %d: got %d, want %d", limit, got, want)
		}
	}
}

func BenchmarkMatchLength(b *testing.B) {
	x := make([]uint32, 4096)
	y := make([]uint32, 4096)

	for i := range x {
		x[i] = uint32(i) * 2654435761
		y[i] = x[i]
	}

	for _, n := range []int{1, 4, 16, 64, 256} {
		y[n] = ^x[n]

		b.Run("go/"+string(rune('a'+len(b.Name())%26)), func(b *testing.B) {
			for b.Loop() {
				matchLength(x, y, 4096)
			}
		})

		if matchLengthAsm != nil {
			b.Run("asm/"+string(rune('a'+len(b.Name())%26)), func(b *testing.B) {
				for b.Loop() {
					matchLengthAsm(x, y, 4096)
				}
			})
		}

		y[n] = x[n]
	}
}
