package webp

import (
	"bytes"
	"math/rand/v2"
	"testing"

	"github.com/gen2brain/vpx/vp8"
)

func TestUpsampleKernelMatchesScalar(t *testing.T) {
	if upsample16Asm == nil {
		t.Skip("no kernel compiled in")
	}

	up, conv := upsample16Asm, yuvToRGBA32Asm
	defer func() { upsample16Asm, yuvToRGBA32Asm = up, conv }()

	r := rand.New(rand.NewPCG(41, 42))

	const maxW = 300

	topY := make([]byte, maxW)
	botY := make([]byte, maxW)
	topU := make([]byte, maxW)
	topV := make([]byte, maxW)
	curU := make([]byte, maxW)
	curV := make([]byte, maxW)

	want := make([]byte, 8*maxW)
	got := make([]byte, 8*maxW)

	var scratch [128]byte

	for n := 1; n <= maxW; n++ {
		for _, p := range [][]byte{topY, botY, topU, topV, curU, curV} {
			for i := range p {
				p[i] = byte(r.UintN(256))
			}
		}

		for _, bottom := range []bool{false, true} {
			for i := range want {
				want[i] = byte(i)
				got[i] = byte(i)
			}

			wantTop, wantBot := want[:4*maxW], want[4*maxW:]
			gotTop, gotBot := got[:4*maxW], got[4*maxW:]

			by := botY
			if !bottom {
				by = nil
			}

			upsample16Asm, yuvToRGBA32Asm = nil, nil
			upsamplePair(topY, by, topU, topV, curU, curV, wantTop, wantBot, n, &scratch)

			upsample16Asm, yuvToRGBA32Asm = up, conv
			upsamplePair(topY, by, topU, topV, curU, curV, gotTop, gotBot, n, &scratch)

			if !bytes.Equal(want, got) {
				for i := range want {
					if want[i] != got[i] {
						t.Fatalf("n=%d bottom=%v: byte %d is %d, want %d", n, bottom, i, got[i], want[i])
					}
				}
			}
		}
	}
}

func BenchmarkUpsampleFrame(b *testing.B) {
	const w, h = 640, 480

	r := rand.New(rand.NewPCG(43, 44))

	cw, ch := (w+1)/2, (h+1)/2

	pic := &vp8.Picture{
		Y:        make([]byte, w*h),
		U:        make([]byte, cw*ch),
		V:        make([]byte, cw*ch),
		YStride:  w,
		UVStride: cw,
		Width:    w,
		Height:   h,
	}

	for _, p := range [][]byte{pic.Y, pic.U, pic.V} {
		for i := range p {
			p[i] = byte(r.UintN(256))
		}
	}

	dst := make([]byte, 4*w*h)

	var scratch [128]byte

	b.SetBytes(int64(len(dst)))

	b.Run("go", func(b *testing.B) {
		up, conv := upsample16Asm, yuvToRGBA32Asm
		upsample16Asm, yuvToRGBA32Asm = nil, nil

		defer func() { upsample16Asm, yuvToRGBA32Asm = up, conv }()

		b.SetBytes(int64(len(dst)))

		for b.Loop() {
			upsampleFrame(dst, 4*w, pic, &scratch)
		}
	})

	if upsample16Asm != nil {
		b.Run("asm", func(b *testing.B) {
			b.SetBytes(int64(len(dst)))

			for b.Loop() {
				upsampleFrame(dst, 4*w, pic, &scratch)
			}
		})
	}
}
