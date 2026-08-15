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

func TestEncoderKernelsSSEAndAVX2(t *testing.T) {
	r := rand.New(rand.NewPCG(21, 22))

	a := make([]byte, bps*24)
	c := make([]byte, bps*24)

	var m qmatrix

	for iter := range 20000 {
		for i := range a {
			a[i] = uint8(r.IntN(256))
			c[i] = uint8(r.IntN(256))
		}

		size := [3]int{4, 8, 16}[r.IntN(3)]
		off := r.IntN(4)*bps + r.IntN(8)

		want := sseGo(a, c, off, size)

		if got := sseSSE(&a[off], &c[off], size); got != want {
			t.Fatalf("iter %d: sseSSE(%d) = %d, want %d", iter, size, got, want)
		}

		if hasAVX2 && size == 16 {
			if got := sseAVX2(&a[off], &c[off], size); got != want {
				t.Fatalf("iter %d: sseAVX2 = %d, want %d", iter, got, want)
			}
		}

		m.q[0] = uint32(4 + r.IntN(300))
		m.q[1] = uint32(4 + r.IntN(300))
		m.expand(r.IntN(3))

		var in0, in1, in2, out0, out1, out2 [16]int16

		for i := range in0 {
			in0[i] = int16(r.IntN(1<<14) - 1<<13)
		}

		in1, in2 = in0, in0

		quantizeBlockGo(in0[:], out0[:], &m, 0)
		quantizeSSE(&in1[0], &out1[0], &m)

		if hasAVX2 {
			quantizeAVX2(&in2[0], &out2[0], &m)
		}

		for n := range 16 {
			j := zigzag[n]

			if out1[j] != out0[n] || in1[j] != in0[j] {
				t.Fatalf("iter %d: quantizeSSE lane %d", iter, j)
			}

			if hasAVX2 && (out2[j] != out0[n] || in2[j] != in0[j]) {
				t.Fatalf("iter %d: quantizeAVX2 lane %d", iter, j)
			}
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

func TestTrueMotionSSEAndAVX2(t *testing.T) {
	r := rand.New(rand.NewPCG(31, 32))

	want := make([]byte, yuvSize)
	sse := make([]byte, yuvSize)
	avx := make([]byte, yuvSize)

	for iter := range 20000 {
		for i := range want {
			want[i] = uint8(r.IntN(256))
		}

		copy(sse, want)
		copy(avx, want)

		size := [3]int{4, 8, 16}[r.IntN(3)]
		off := bps + 1 + r.IntN(len(want)-17*bps-16)

		trueMotionGo(want, off, size)
		trueMotionSSE(&sse[off], bps, size)

		if !bytes.Equal(want, sse) {
			t.Fatalf("iter %d: trueMotionSSE size=%d", iter, size)
		}

		if hasAVX2 && size == 16 {
			trueMotionAVX2(&avx[off], bps, size)

			if !bytes.Equal(want, avx) {
				t.Fatalf("iter %d: trueMotionAVX2", iter)
			}
		}
	}
}

func BenchmarkEncoderKernels(b *testing.B) {
	a := make([]byte, bps*24)
	c := make([]byte, bps*24)

	for i := range a {
		a[i] = uint8(i * 7)
		c[i] = uint8(i * 11)
	}

	var m qmatrix

	m.q[0], m.q[1] = 20, 24
	m.expand(0)

	var in, out [16]int16

	for i := range in {
		in[i] = int16(i*211 - 1000)
	}

	b.Run("sse16/SSE2", func(b *testing.B) {
		for b.Loop() {
			sseSSE(&a[0], &c[0], 16)
		}
	})

	if hasAVX2 {
		b.Run("sse16/AVX2", func(b *testing.B) {
			for b.Loop() {
				sseAVX2(&a[0], &c[0], 16)
			}
		})
	}

	b.Run("quantize/SSE2", func(b *testing.B) {
		for b.Loop() {
			quantizeSSE(&in[0], &out[0], &m)
		}
	})

	if hasAVX2 {
		b.Run("quantize/AVX2", func(b *testing.B) {
			for b.Loop() {
				quantizeAVX2(&in[0], &out[0], &m)
			}
		})
	}

	b.Run("fTransform/SSE2", func(b *testing.B) {
		for b.Loop() {
			fTransformSSE(&a[0], &c[0], &out[0])
		}
	})

	tm := make([]byte, yuvSize)

	b.Run("trueMotion16/SSE2", func(b *testing.B) {
		for b.Loop() {
			trueMotionSSE(&tm[bps+1], bps, 16)
		}
	})

	if hasAVX2 {
		b.Run("trueMotion16/AVX2", func(b *testing.B) {
			for b.Loop() {
				trueMotionAVX2(&tm[bps+1], bps, 16)
			}
		})
	}
}

func TestFTransform2AVX2MatchesScalar(t *testing.T) {
	if !hasAVX2 {
		t.Skip("no AVX2")
	}

	r := rand.New(rand.NewPCG(23, 24))

	src := make([]byte, bps*8)
	ref := make([]byte, bps*8)

	var want, got [32]int16

	for iter := range 20000 {
		for i := range src {
			src[i] = uint8(r.IntN(256))
			ref[i] = uint8(r.IntN(256))
		}

		fTransformGo(src, ref, 0, 0, want[:16])
		fTransformGo(src, ref, 4, 4, want[16:])
		fTransform2AVX2(&src[0], &ref[0], &got[0])

		if want != got {
			t.Fatalf("iter %d:\n got %v\nwant %v", iter, got, want)
		}
	}
}
