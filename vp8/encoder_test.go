package vp8

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestFDCTInverts(t *testing.T) {
	rng := rand.New(rand.NewPCG(21, 22))

	var (
		src, ref [yuvSize]byte
		out      [16]int16
	)

	for range 200 {
		for j := range 4 {
			for i := range 4 {
				src[yOff+i+j*bps] = uint8(rng.IntN(256))
				ref[yOff+i+j*bps] = uint8(rng.IntN(256))
			}
		}

		fTransform(src[:], ref[:], yOff, yOff, out[:])

		var got [yuvSize]byte

		copy(got[:], ref[:])
		transformOne(out[:], got[:], yOff)

		for j := range 4 {
			for i := range 4 {
				want := int(src[yOff+i+j*bps])
				have := int(got[yOff+i+j*bps])

				if d := want - have; d > 2 || d < -2 {
					t.Fatalf("sample %d,%d: %d, want %d", i, j, have, want)
				}
			}
		}
	}
}

func TestFWHTInverts(t *testing.T) {
	rng := rand.New(rand.NewPCG(23, 24))

	var in, fwd, back [16 * 16]int16

	for range 200 {
		for i := range 16 {
			in[16*i] = int16(rng.IntN(2048) - 1024)
		}

		fTransformWHT(in[:], fwd[:])
		transformWHT(fwd[:], back[:])

		for i := range 16 {
			want := int(in[16*i])
			have := int(back[16*i])

			if d := want - have; d > 1 || d < -1 {
				t.Fatalf("dc %d: %d, want %d", i, have, want)
			}
		}
	}
}

func testPicture(w, h int) *Picture {
	p := &Picture{
		Width: w, Height: h,
		YStride: w, UVStride: (w + 1) / 2,
	}

	p.Y = make([]byte, p.YStride*h)
	p.U = make([]byte, p.UVStride*((h+1)/2))
	p.V = make([]byte, len(p.U))

	for y := range h {
		for x := range w {
			p.Y[y*p.YStride+x] = uint8(16 + (x*3+y*2)%220)
		}
	}

	for y := range (h + 1) / 2 {
		for x := range (w + 1) / 2 {
			p.U[y*p.UVStride+x] = uint8(60 + (x*5)%128)
			p.V[y*p.UVStride+x] = uint8(200 - (y*7)%128)
		}
	}

	return p
}

func psnr(a, b []byte, aStride, bStride, w, h int) float64 {
	sum := 0.0

	for y := range h {
		for x := range w {
			d := float64(a[y*aStride+x]) - float64(b[y*bStride+x])
			sum += d * d
		}
	}

	if sum == 0 {
		return 99
	}

	return 10 * math.Log10(255*255*float64(w*h)/sum)
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	for _, size := range [][2]int{{16, 16}, {17, 13}, {64, 48}, {121, 97}} {
		src := testPicture(size[0], size[1])

		var (
			e    Encoder
			d    Decoder
			last float64
		)

		for _, q := range []int{20, 50, 75, 95} {
			data, err := e.Encode(src, EncodeOptions{Quality: q})
			if err != nil {
				t.Fatalf("%dx%d q%d: encode: %v", size[0], size[1], q, err)
			}

			pic, err := d.DecodeFrame(data)
			if err != nil {
				t.Fatalf("%dx%d q%d: decode: %v", size[0], size[1], q, err)
			}

			if pic.Width != src.Width || pic.Height != src.Height {
				t.Fatalf("decoded %dx%d, want %dx%d", pic.Width, pic.Height, src.Width, src.Height)
			}

			got := psnr(pic.Y, src.Y, pic.YStride, src.YStride, src.Width, src.Height)

			if got < 25 {
				t.Errorf("%dx%d q%d: luma PSNR %.1f dB, want at least 25", size[0], size[1], q, got)
			}

			if got < last-1 {
				t.Errorf("%dx%d q%d: luma PSNR %.1f dB, below %.1f at the lower quality", size[0], size[1], q, got, last)
			}

			last = got
		}
	}
}

func TestEncodePartition0Limit(t *testing.T) {
	worst := 0

	for a := range numBModes {
		for b := range numBModes {
			for m := range numBModes {
				worst = max(worst, int(bModeCost[a][b][m]))
			}
		}
	}

	if 16*worst >= maxI4HeaderBits {
		t.Fatalf("16 blocks of the costliest subblock mode is %d, at or over the cap %d", 16*worst, maxI4HeaderBits)
	}

	src := testPicture(160, 128)

	var (
		e    Encoder
		d    Decoder
		last int
	)

	for _, limit := range []int{maxPartition0, 4096, 2048, 1024, 512} {
		e.p0Limit = limit

		data, err := e.Encode(src, EncodeOptions{Quality: 80, Method: 4})
		if err != nil {
			t.Fatalf("limit %d: encode: %v", limit, err)
		}

		size := int(uint32(data[0])|uint32(data[1])<<8|uint32(data[2])<<16) >> 5

		if size >= limit {
			t.Fatalf("limit %d: partition 0 is %d bytes", limit, size)
		}

		pic, err := d.DecodeFrame(data)
		if err != nil {
			t.Fatalf("limit %d: decode: %v", limit, err)
		}

		if got := psnr(pic.Y, src.Y, pic.YStride, src.YStride, src.Width, src.Height); got < 25 {
			t.Errorf("limit %d: luma PSNR %.1f dB, want at least 25", limit, got)
		}

		if last != 0 && size > last {
			t.Errorf("limit %d: partition 0 grew from %d to %d", limit, last, size)
		}

		last = size
	}

	e.p0Limit = 64

	if _, err := e.Encode(src, EncodeOptions{Quality: 80, Method: 4}); err != ErrUnsupported {
		t.Fatalf("partition 0 that cannot fit: got %v, want ErrUnsupported", err)
	}
}

func TestLevelCostMatchesCoeffCost(t *testing.T) {
	rng := rand.New(rand.NewPCG(51, 52))

	var e Encoder

	e.proba.reset()

	for range 20000 {
		var levels [16]int16

		first := rng.IntN(2)
		ty := rng.IntN(4)
		ctx0 := rng.IntN(3)

		nz := first + rng.IntN(17-first)

		for n := first; n < nz; n++ {
			switch rng.IntN(4) {
			case 0:
				levels[n] = 0
			case 1:
				levels[n] = int16(1 + rng.IntN(3))
			default:
				levels[n] = int16(1 + rng.IntN(200))
			}

			if rng.IntN(2) == 0 {
				levels[n] = -levels[n]
			}
		}

		if nz > first {
			for levels[nz-1] == 0 {
				levels[nz-1] = int16(1 + rng.IntN(9))
			}
		}

		bands := &e.proba.bandsPtr[ty]
		want := coeffCost(bands, ctx0, first, levels[:], nz)

		got := 0
		ctx := ctx0

		if nz > first && ctx0 == 0 {
			got += probCost(bands[first][0][0], 1)
		}

		for n := first; n < nz; n++ {
			v := int(levels[n])
			if v < 0 {
				v = -v
			}

			got += levelCost(&bands[n][ctx], ctx, v)
			ctx = min(v, 2)
		}

		if nz < 16 {
			got += probCost(bands[nz][ctx][0], 0)
		}

		if got != want {
			t.Fatalf("type %d first %d ctx %d nz %d: levelCost sums to %d, coeffCost is %d\n%v",
				ty, first, ctx0, nz, got, want, levels)
		}
	}
}
