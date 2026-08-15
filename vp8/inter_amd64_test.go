//go:build amd64 && !noasm

package vp8

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

func TestSixtapSSEAndAVX2(t *testing.T) {
	const stride = 64

	r := rand.New(rand.NewPCG(13, 14))

	src := make([]byte, stride*48)
	want := make([]byte, stride*48)
	sse := make([]byte, stride*48)
	avx := make([]byte, stride*48)

	for iter := range 20000 {
		for i := range src {
			src[i] = uint8(r.IntN(256))
		}

		for i := range want {
			want[i] = uint8(r.IntN(256))
		}

		copy(sse, want)
		copy(avx, want)

		w := [3]int{4, 8, 16}[r.IntN(3)]
		h := [4]int{4, 8, 16, 21}[r.IntN(4)]
		f := &subPelFilters[1+r.IntN(7)]

		sOff := (8+r.IntN(8))*stride + 8 + r.IntN(16)
		dOff := (8+r.IntN(8))*stride + 8 + r.IntN(16)

		if iter&1 == 0 {
			sixtapHGo(want, dOff, stride, src, sOff, stride, w, h, f)
			sixtapHSSE(&sse[dOff], stride, &src[sOff], stride, w, h, &f[0])

			if hasAVX2 {
				sixtapHAVX2(&avx[dOff], stride, &src[sOff], stride, w, h, &f[0])
			}
		} else {
			sixtapVGo(want, dOff, stride, src, sOff, stride, w, h, f)
			sixtapVSSE(&sse[dOff], stride, &src[sOff], stride, w, h, &f[0])

			if hasAVX2 {
				sixtapVAVX2(&avx[dOff], stride, &src[sOff], stride, w, h, &f[0])
			}
		}

		if !bytes.Equal(want, sse) {
			t.Fatalf("iter %d: SSE2 mismatch w=%d h=%d", iter, w, h)
		}

		if hasAVX2 && !bytes.Equal(want, avx) {
			t.Fatalf("iter %d: AVX2 mismatch w=%d h=%d", iter, w, h)
		}
	}
}

func BenchmarkSixtap(b *testing.B) {
	const stride = 64

	src := make([]byte, stride*48)
	dst := make([]byte, stride*48)

	for i := range src {
		src[i] = uint8(i * 7)
	}

	f := &subPelFilters[3]

	for _, w := range []int{4, 8, 16} {
		h := w

		b.Run("H/SSE2/"+string(rune('0'+w/4))+"x", func(b *testing.B) {
			for b.Loop() {
				sixtapHSSE(&dst[8*stride+8], stride, &src[8*stride+8], stride, w, h, &f[0])
			}
		})

		if hasAVX2 {
			b.Run("H/AVX2/"+string(rune('0'+w/4))+"x", func(b *testing.B) {
				for b.Loop() {
					sixtapHAVX2(&dst[8*stride+8], stride, &src[8*stride+8], stride, w, h, &f[0])
				}
			})
		}

		b.Run("V/SSE2/"+string(rune('0'+w/4))+"x", func(b *testing.B) {
			for b.Loop() {
				sixtapVSSE(&dst[8*stride+8], stride, &src[8*stride+8], stride, w, h, &f[0])
			}
		})

		if hasAVX2 {
			b.Run("V/AVX2/"+string(rune('0'+w/4))+"x", func(b *testing.B) {
				for b.Loop() {
					sixtapVAVX2(&dst[8*stride+8], stride, &src[8*stride+8], stride, w, h, &f[0])
				}
			})
		}
	}
}
