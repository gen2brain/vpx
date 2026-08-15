package webp

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

func TestARGBToRGBAKernelMatchesScalar(t *testing.T) {
	if argbToRGBAAsm == nil {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(31, 32))

	px := make([]uint32, 1031)
	for i := range px {
		px[i] = r.Uint32()
	}

	want := make([]byte, 4*len(px))
	got := make([]byte, 4*len(px))

	for n := 1; n <= len(px); n++ {
		clear(want)
		clear(got)

		argbToRGBAScalar(want[:4*n], px[:n])
		argbToRGBAAsm(got[:4*n], px[:n])

		if !bytes.Equal(want, got) {
			t.Fatalf("n=%d: got %v, want %v", n, got[:4*n], want[:4*n])
		}

		if n > 64 {
			n += 37
		}
	}
}

func BenchmarkARGBToRGBA(b *testing.B) {
	px := make([]uint32, 1<<18)
	for i := range px {
		px[i] = uint32(i) * 2654435761
	}

	dst := make([]byte, 4*len(px))

	b.Run("go", func(b *testing.B) {
		b.SetBytes(int64(len(dst)))

		for b.Loop() {
			argbToRGBAScalar(dst, px)
		}
	})

	if argbToRGBAAsm != nil {
		b.Run("asm", func(b *testing.B) {
			b.SetBytes(int64(len(dst)))

			for b.Loop() {
				argbToRGBAAsm(dst, px)
			}
		})
	}
}
