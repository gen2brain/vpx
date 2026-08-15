package webp

import (
	"bytes"
	"image"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func testImages() map[string]*image.NRGBA {
	rng := rand.New(rand.NewPCG(9, 10))
	out := map[string]*image.NRGBA{}

	sizes := []image.Point{{X: 1, Y: 1}, {X: 1, Y: 17}, {X: 17, Y: 1}, {X: 3, Y: 5}, {X: 64, Y: 48}}

	for _, s := range sizes {
		flat := image.NewNRGBA(image.Rect(0, 0, s.X, s.Y))
		for i := range flat.Pix {
			flat.Pix[i] = 0xff
		}

		out[fmtSize("flat", s)] = flat

		grad := image.NewNRGBA(image.Rect(0, 0, s.X, s.Y))
		for y := range s.Y {
			for x := range s.X {
				o := grad.PixOffset(x, y)
				grad.Pix[o] = uint8(x * 4)
				grad.Pix[o+1] = uint8(y * 4)
				grad.Pix[o+2] = uint8(x + y)
				grad.Pix[o+3] = 0xff
			}
		}

		out[fmtSize("gradient", s)] = grad

		noise := image.NewNRGBA(image.Rect(0, 0, s.X, s.Y))
		for i := range noise.Pix {
			noise.Pix[i] = uint8(rng.UintN(256))
		}

		out[fmtSize("noise", s)] = noise

		pal := image.NewNRGBA(image.Rect(0, 0, s.X, s.Y))
		colors := [4][4]uint8{{0, 0, 0, 0xff}, {0xff, 0, 0, 0x80}, {0, 0xff, 0, 0xff}, {9, 9, 9, 0}}

		for y := range s.Y {
			for x := range s.X {
				c := colors[(x/3+y/2)%len(colors)]
				copy(pal.Pix[pal.PixOffset(x, y):], c[:])
			}
		}

		out[fmtSize("palette", s)] = pal
	}

	return out
}

func fmtSize(name string, s image.Point) string {
	return name + "-" + itoa(s.X) + "x" + itoa(s.Y)
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}

	return itoa(n/10) + string(rune('0'+n%10))
}

func TestEncodeLosslessRoundTrip(t *testing.T) {
	for name, img := range testImages() {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer

			if err := Encode(&buf, img, Options{Lossless: true}); err != nil {
				t.Fatalf("encode: %v", err)
			}

			got, err := Decode(&buf)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			nrgba, ok := got.(*image.NRGBA)
			if !ok {
				t.Fatalf("decoded %T, want *image.NRGBA", got)
			}

			if !nrgba.Bounds().Eq(img.Bounds()) {
				t.Fatalf("bounds %v, want %v", nrgba.Bounds(), img.Bounds())
			}

			for y := range img.Bounds().Dy() {
				for x := range img.Bounds().Dx() {
					o := img.PixOffset(x, y)

					want := img.Pix[o : o+4]
					if want[3] == 0 {
						continue
					}

					if have := nrgba.Pix[nrgba.PixOffset(x, y):][:4]; !bytes.Equal(have, want) {
						t.Fatalf("pixel %d,%d: %v, want %v", x, y, have, want)
					}
				}
			}
		})
	}
}

// TestEncodeLosslessAgainstDwebp encodes what the decoder read back and
// requires libwebp to decode the result to the same pixels it reads from the
// file it came from. The decoder is byte-exact against libwebp already, so the
// two agreeing means the bitstream is valid and unambiguous.
func TestEncodeLosslessAgainstDwebp(t *testing.T) {
	bin := dwebpBin(t)

	dir := t.TempDir()

	for _, name := range []string{"simple", "palette", "2-color", "lossless_alpha"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name+".webp")

			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}

			defer f.Close()

			img, err := Decode(f)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			var buf bytes.Buffer

			if err := Encode(&buf, img, Options{Lossless: true, Exact: true}); err != nil {
				t.Fatalf("encode: %v", err)
			}

			enc := filepath.Join(dir, name+".enc.webp")

			if err := os.WriteFile(enc, buf.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}

			want, w, h, err := dwebpPAM(bin, path, filepath.Join(dir, "want.pam"))
			if err != nil {
				t.Fatalf("dwebp on the source: %v", err)
			}

			got, gw, gh, err := dwebpPAM(bin, enc, filepath.Join(dir, "got.pam"))
			if err != nil {
				t.Fatalf("dwebp on our output: %v", err)
			}

			if gw != w || gh != h {
				t.Fatalf("libwebp read %dx%d, want %dx%d", gw, gh, w, h)
			}

			if diff := comparePixels(got, want, w, h); diff != "" {
				t.Fatal(diff)
			}

			src, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("%dx%d: %d bytes, cwebp wrote %d", w, h, buf.Len(), src.Size())
		})
	}
}

func randomImage(rng *rand.Rand, w, h, colors int) []uint32 {
	px := make([]uint32, w*h)

	for i := range px {
		switch {
		case colors > 0:
			px[i] = uint32(rng.IntN(colors)) * 0x00010307 % 0xffffffff
		default:
			px[i] = rng.Uint32()
		}
	}

	return px
}

func TestSubtractGreenForward(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 12))

	px := randomImage(rng, 32, 8, 0)
	want := slices.Clone(px)

	subtractGreenForward(px)
	applySubtractGreen(px)

	if !slices.Equal(px, want) {
		t.Error("subtract green does not invert")
	}
}

func TestResidualImage(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 14))

	for _, size := range []image.Point{{X: 1, Y: 1}, {X: 1, Y: 9}, {X: 9, Y: 1}, {X: 7, Y: 5}, {X: 40, Y: 33}} {
		for _, bits := range []int{2, 4, 6} {
			for _, colors := range []int{0, 3} {
				px := randomImage(rng, size.X, size.Y, colors)
				want := slices.Clone(px)

				tw, th := subSampleSize(size.X, bits), subSampleSize(size.Y, bits)
				modes := make([]uint32, tw*th)
				rows := make([]uint32, 2*(size.X+1))

				residualImage(px, size.X, size.Y, bits, false, true, modes, rows)
				applyPredictor(px, size.X, size.Y, bits, modes)

				if !slices.Equal(px, want) {
					t.Fatalf("%dx%d bits %d colors %d: predictor does not invert", size.X, size.Y, bits, colors)
				}
			}
		}
	}
}

func TestPaletteForward(t *testing.T) {
	rng := rand.New(rand.NewPCG(15, 16))

	for _, size := range []image.Point{{X: 1, Y: 1}, {X: 1, Y: 9}, {X: 9, Y: 1}, {X: 7, Y: 5}, {X: 40, Y: 33}} {
		for _, colors := range []int{2, 3, 5, 17, 200} {
			px := randomImage(rng, size.X, size.Y, colors)
			want := slices.Clone(px)

			var pm paletteMap

			palette, ok := pm.build(px, nil)
			if !ok {
				t.Fatalf("%dx%d colors %d: no palette", size.X, size.Y, colors)
			}

			bits := paletteBits(len(palette))
			packedWidth := subSampleSize(size.X, bits)
			packed := make([]uint32, packedWidth*size.Y)

			pm.mapToPalette(px, packed, size.X, size.Y, bits)

			table := expandColorMap(paletteDeltas(palette, make([]uint32, len(palette))), bits)
			out := make([]uint32, size.X*size.Y)
			got := applyColorIndexing(packed, out, size.X, size.Y, bits, table)

			if !slices.Equal(got, want) {
				t.Fatalf("%dx%d colors %d bits %d: palette does not invert", size.X, size.Y, colors, bits)
			}
		}
	}
}

func TestBitWriterRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))

	type item struct {
		v uint32
		n uint
	}

	items := make([]item, 4096)

	for i := range items {
		n := uint(rng.IntN(32) + 1)
		items[i] = item{v: rng.Uint32() & uint32(1<<n-1), n: n}
	}

	var w lbitWriter

	w.init(nil)

	for _, it := range items {
		w.write(it.v, it.n)
	}

	buf := w.flush()

	var r lbitReader

	r.init(buf)

	for i, it := range items {
		if got := r.read(it.n); got != it.v {
			t.Fatalf("item %d: read %d bits, got %#x want %#x", i, it.n, got, it.v)
		}
	}

	if r.overrun() {
		t.Error("reader overran a stream the writer produced")
	}

	total := 0
	for _, it := range items {
		total += int(it.n)
	}

	if len(buf) != (total+7)/8 {
		t.Errorf("len = %d, want %d", len(buf), (total+7)/8)
	}
}

func TestBitWriterWidths(t *testing.T) {
	for n := uint(0); n <= 32; n++ {
		var w lbitWriter

		w.init(nil)
		w.write(3, 2)
		w.write(1<<n-1, n)
		w.write(2, 2)

		buf := w.flush()

		var r lbitReader

		r.init(buf)

		if got := r.read(2); got != 3 {
			t.Fatalf("n=%d: prefix %d", n, got)
		}

		if got := r.read(n); got != uint32(1<<n-1) {
			t.Fatalf("n=%d: value %#x", n, got)
		}

		if got := r.read(2); got != 2 {
			t.Fatalf("n=%d: suffix %d", n, got)
		}
	}
}

func histograms(rng *rand.Rand, size int) [][]uint32 {
	var out [][]uint32

	single := make([]uint32, size)
	single[size/2] = 7
	out = append(out, single)

	two := make([]uint32, size)
	two[0], two[size-1] = 1, 99
	out = append(out, two)

	flat := make([]uint32, size)
	for i := range flat {
		flat[i] = 1
	}

	out = append(out, flat)

	// Fibonacci counts give the deepest tree a histogram can, so the depth
	// limit has to bite.
	deep := make([]uint32, size)
	a, b := uint32(1), uint32(1)

	for i := range deep {
		deep[i] = a
		a, b = b, a+b

		if b > 1<<28 {
			a, b = 1, 1
		}
	}

	out = append(out, deep)

	for range 16 {
		h := make([]uint32, size)

		for i := range h {
			switch rng.IntN(4) {
			case 0:
			case 1:
				h[i] = uint32(rng.IntN(4))
			default:
				h[i] = uint32(rng.IntN(1 << uint(rng.IntN(20))))
			}
		}

		out = append(out, h)
	}

	return out
}

func codeSymbols(c *huffCode) []int {
	var used []int

	for i, l := range c.lengths {
		if l != 0 {
			used = append(used, i)
		}
	}

	return used
}

func TestHuffCodeRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))

	var b huffBuilder

	for _, size := range []int{2, 19, 40, 256, numLiteralCodes + numLengthCodes} {
		for hi, hist := range histograms(rng, size) {
			c := huffCode{lengths: make([]uint8, size), codes: make([]uint16, size)}

			b.build(hist, maxCodeLength, &c)

			used := codeSymbols(&c)
			if len(used) == 0 {
				continue
			}

			kraft := 0.0

			for _, l := range c.lengths {
				if l == 0 {
					continue
				}

				if int(l) > maxCodeLength {
					t.Fatalf("size %d hist %d: length %d", size, hi, l)
				}

				kraft += 1 / float64(uint32(1)<<l)
			}

			if len(used) > 1 && kraft != 1 {
				t.Fatalf("size %d hist %d: kraft sum %v, code is not complete", size, hi, kraft)
			}

			lengths := make([]uint16, size)
			for i, l := range c.lengths {
				lengths[i] = uint16(l)
			}

			tree, err := buildTree(lengths)
			if err != nil {
				t.Fatalf("size %d hist %d: buildTree: %v", size, hi, err)
			}

			var w lbitWriter

			w.init(nil)

			for _, s := range used {
				w.write(uint32(c.codes[s]), uint(c.lengths[s]))
			}

			var r lbitReader

			r.init(w.flush())

			for _, s := range used {
				r.fill()

				if got := tree.readSymbol(&r); int(got) != s {
					t.Fatalf("size %d hist %d: symbol %d decoded as %d", size, hi, s, got)
				}
			}
		}
	}
}

func TestHuffCodeDepthLimit(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))

	var b huffBuilder

	for _, size := range []int{19, 64} {
		for hi, hist := range histograms(rng, size) {
			c := huffCode{lengths: make([]uint8, size), codes: make([]uint16, size)}

			b.build(hist, codeLengthDepthLimit, &c)

			for _, l := range c.lengths {
				if int(l) > codeLengthDepthLimit {
					t.Fatalf("size %d hist %d: length %d over the limit", size, hi, l)
				}
			}
		}
	}
}

func TestStoreHuffmanCode(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))

	var b huffBuilder

	for _, size := range []int{19, 40, 256, numLiteralCodes + numLengthCodes} {
		for hi, hist := range histograms(rng, size) {
			c := huffCode{lengths: make([]uint8, size), codes: make([]uint16, size)}

			b.build(hist, maxCodeLength, &c)

			used := codeSymbols(&c)

			var w lbitWriter

			w.init(nil)
			b.storeCode(&w, &c)

			for _, s := range used {
				w.write(uint32(c.codes[s]), uint(c.lengths[s]))
			}

			var d losslessDecoder

			d.br.init(w.flush())

			tree, err := d.readHuffmanCode(size)
			if err != nil {
				t.Fatalf("size %d hist %d: readHuffmanCode: %v", size, hi, err)
			}

			for _, s := range used {
				d.br.fill()

				if got := tree.readSymbol(&d.br); int(got) != s {
					t.Fatalf("size %d hist %d: symbol %d decoded as %d", size, hi, s, got)
				}
			}

			if d.br.overrun() {
				t.Fatalf("size %d hist %d: overrun", size, hi)
			}
		}
	}
}

func rgbaPSNR(t *testing.T, got image.Image, want *image.NRGBA) float64 {
	t.Helper()

	b := want.Bounds()
	sum := 0.0

	for y := range b.Dy() {
		for x := range b.Dx() {
			gr, gg, gb, _ := got.At(x, y).RGBA()
			wr, wg, wb, _ := want.At(x, y).RGBA()

			for _, d := range []float64{
				float64(gr>>8) - float64(wr>>8),
				float64(gg>>8) - float64(wg>>8),
				float64(gb>>8) - float64(wb>>8),
			} {
				sum += d * d
			}
		}
	}

	if sum == 0 {
		return 99
	}

	return 10 * math.Log10(255*255*float64(3*b.Dx()*b.Dy())/sum)
}

func TestEncodeLossyRoundTrip(t *testing.T) {
	for name, img := range testImages() {
		if strings.HasPrefix(name, "noise") {
			continue
		}

		t.Run(name, func(t *testing.T) {
			last := 0.0

			for _, q := range []int{30, 75, 95} {
				var buf bytes.Buffer

				if err := Encode(&buf, img, Options{Quality: q}); err != nil {
					t.Fatalf("q%d: encode: %v", q, err)
				}

				got, err := Decode(bytes.NewReader(buf.Bytes()), Options{ToRGBA: true})
				if err != nil {
					t.Fatalf("q%d: decode: %v", q, err)
				}

				if !got.Bounds().Eq(img.Bounds()) {
					t.Fatalf("q%d: bounds %v, want %v", q, got.Bounds(), img.Bounds())
				}

				if p := rgbaPSNR(t, got, img); p < last-2 {
					t.Errorf("q%d: PSNR %.1f dB, below %.1f at the lower quality", q, p, last)
				} else {
					last = p
				}
			}
		})
	}
}

// TestEncodeLossyAgainstDwebp requires libwebp and this decoder to read the
// same pixels out of a file this encoder wrote. The decoder is byte-exact
// against libwebp on the corpus, so the two agreeing means the bitstream says
// one thing only.
func TestEncodeLossyAgainstDwebp(t *testing.T) {
	bin := dwebpBin(t)

	dir := t.TempDir()

	for name, img := range testImages() {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer

			if err := Encode(&buf, img, Options{Quality: 80}); err != nil {
				t.Fatalf("encode: %v", err)
			}

			path := filepath.Join(dir, "enc.webp")

			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}

			want, w, h, err := dwebpPAM(bin, path, filepath.Join(dir, "ref.pam"))
			if err != nil {
				t.Fatalf("dwebp: %v", err)
			}

			got, err := Decode(bytes.NewReader(buf.Bytes()), Options{ToRGBA: true})
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			rgba, ok := got.(*image.RGBA)
			if !ok {
				t.Fatalf("decoded %T, want *image.RGBA", got)
			}

			premultiply(want[:4*w*h])

			if diff := comparePixels(rgba.Pix, want, w, h); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestEncodeAnimation(t *testing.T) {
	imgs := testImages()

	anim := &WEBP{
		Image:     []image.Image{imgs["gradient-64x48"], imgs["flat-64x48"], imgs["palette-64x48"]},
		Delay:     []int{40, 60, 80},
		LoopCount: 3,
	}

	for _, lossless := range []bool{true, false} {
		var buf bytes.Buffer

		if err := EncodeAll(&buf, anim, Options{Lossless: lossless, Quality: 90}); err != nil {
			t.Fatalf("lossless=%v: encode: %v", lossless, err)
		}

		got, err := DecodeAll(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("lossless=%v: decode: %v", lossless, err)
		}

		if len(got.Image) != len(anim.Image) {
			t.Fatalf("lossless=%v: %d frames, want %d", lossless, len(got.Image), len(anim.Image))
		}

		if !slices.Equal(got.Delay, anim.Delay) {
			t.Errorf("lossless=%v: delays %v, want %v", lossless, got.Delay, anim.Delay)
		}

		if got.LoopCount != anim.LoopCount {
			t.Errorf("lossless=%v: loop count %d, want %d", lossless, got.LoopCount, anim.LoopCount)
		}

		for i, frame := range got.Image {
			want := anim.Image[i].(*image.NRGBA)

			// Alpha is carried losslessly whether the frame is or not.
			for y := range want.Bounds().Dy() {
				for x := range want.Bounds().Dx() {
					_, _, _, a := frame.At(x, y).RGBA()

					if w := uint32(want.Pix[want.PixOffset(x, y)+3]); a>>8 != w {
						t.Fatalf("lossless=%v: frame %d alpha at %d,%d: %d, want %d",
							lossless, i, x, y, a>>8, w)
					}
				}
			}

			p := rgbaPSNR(t, frame, want)

			if lossless && p < 99 {
				t.Errorf("lossless: frame %d is not exact, PSNR %.1f dB", i, p)
			}

			if !lossless && i < 2 && p < 30 {
				t.Errorf("lossy: frame %d PSNR %.1f dB", i, p)
			}
		}
	}
}

// TestEncodeAnimationAgainstLibwebp composites an animation this encoder wrote
// with libwebp's demuxer and requires every frame to match what this package
// composites. Set ANIMREF_BIN to run it.
func TestEncodeAnimationAgainstLibwebp(t *testing.T) {
	bin := animRefBin(t)

	imgs := testImages()

	anim := &WEBP{
		Image:     []image.Image{imgs["gradient-64x48"], imgs["flat-64x48"], imgs["palette-64x48"]},
		Delay:     []int{40, 60, 80},
		LoopCount: 3,
	}

	dir := t.TempDir()

	for _, lossless := range []bool{true, false} {
		var buf bytes.Buffer

		if err := EncodeAll(&buf, anim, Options{Lossless: lossless, Quality: 90}); err != nil {
			t.Fatalf("lossless=%v: encode: %v", lossless, err)
		}

		path := filepath.Join(dir, "anim.webp")
		out := filepath.Join(dir, "ref.bin")

		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := exec.Command(bin, path, out).Run(); err != nil {
			t.Fatalf("lossless=%v: %v", lossless, err)
		}

		want, err := readAnimRef(out)
		if err != nil {
			t.Fatal(err)
		}

		got, err := DecodeAll(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("lossless=%v: decode: %v", lossless, err)
		}

		if want.loopCount != anim.LoopCount || len(want.frames) != len(anim.Image) {
			t.Fatalf("lossless=%v: libwebp read %d frames looping %d", lossless, len(want.frames), want.loopCount)
		}

		for i, ref := range want.frames {
			if want.delay[i] != anim.Delay[i] {
				t.Errorf("lossless=%v: frame %d delay %d, want %d", lossless, i, want.delay[i], anim.Delay[i])
			}

			rgba := got.Image[i].(*image.RGBA)

			if diff := comparePixels(rgba.Pix, ref, want.w, want.h); diff != "" {
				t.Fatalf("lossless=%v: frame %d: %s", lossless, i, diff)
			}
		}
	}
}

func TestEncodeAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates")
	}

	imgs := testImages()

	anim := &WEBP{
		Image: []image.Image{imgs["gradient-64x48"], imgs["palette-64x48"]},
		Delay: []int{40, 60},
	}

	lossy, err := Decode(bytes.NewReader(readFile(t, "test.webp")))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		max  float64
		fn   func()
	}{
		{"lossless", 0, func() { Encode(io.Discard, imgs["gradient-64x48"], Options{Lossless: true}) }},
		{"lossless alpha", 0, func() { Encode(io.Discard, imgs["palette-64x48"], Options{Lossless: true}) }},
		{"lossy", 0, func() { Encode(io.Discard, imgs["gradient-64x48"], Options{Quality: 75}) }},
		{"lossy alpha", 0, func() { Encode(io.Discard, imgs["palette-64x48"], Options{Quality: 75}) }},
		{"lossy 4:2:0 in", 0, func() { Encode(io.Discard, lossy, Options{Quality: 75}) }},
		{"animation", 0, func() { EncodeAll(io.Discard, anim, Options{Quality: 75}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn()

			n := testing.AllocsPerRun(5, tt.fn)
			if n > tt.max {
				t.Errorf("%v allocations, want at most %v", n, tt.max)
			}

			t.Logf("%v allocations", n)
		})
	}
}

func fuzzImage(w, h uint8, pix []byte) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, int(w)%64+1, int(h)%64+1))

	if len(pix) == 0 {
		return m
	}

	for i := range m.Pix {
		m.Pix[i] = pix[i%len(pix)]
	}

	return m
}

func FuzzEncodeLossless(f *testing.F) {
	f.Add(uint8(1), uint8(1), []byte{0})
	f.Add(uint8(16), uint8(16), []byte{1, 2, 3, 4})
	f.Add(uint8(63), uint8(7), []byte{255, 0, 128, 255, 9})

	f.Fuzz(func(t *testing.T, w, h uint8, pix []byte) {
		src := fuzzImage(w, h, pix)

		var buf bytes.Buffer

		if err := Encode(&buf, src, Options{Lossless: true, Exact: true}); err != nil {
			t.Fatalf("encode: %v", err)
		}

		got, err := Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}

		out, ok := got.(*image.NRGBA)
		if !ok {
			t.Fatalf("decoded %T, want *image.NRGBA", got)
		}

		if !out.Bounds().Eq(src.Bounds()) {
			t.Fatalf("bounds %v, want %v", out.Bounds(), src.Bounds())
		}

		for y := range src.Bounds().Dy() {
			row := src.Pix[src.PixOffset(0, y):][:4*src.Bounds().Dx()]

			if have := out.Pix[out.PixOffset(0, y):][:len(row)]; !bytes.Equal(have, row) {
				t.Fatalf("row %d differs", y)
			}
		}
	})
}

func FuzzEncodeLossy(f *testing.F) {
	f.Add(uint8(1), uint8(1), uint8(75), []byte{0})
	f.Add(uint8(16), uint8(16), uint8(0), []byte{1, 2, 3, 4})
	f.Add(uint8(63), uint8(7), uint8(100), []byte{255, 0, 128, 255, 9})

	f.Fuzz(func(t *testing.T, w, h, quality uint8, pix []byte) {
		src := fuzzImage(w, h, pix)

		var buf bytes.Buffer

		if err := Encode(&buf, src, Options{Quality: int(quality) % 101}); err != nil {
			t.Fatalf("encode: %v", err)
		}

		got, err := Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}

		if !got.Bounds().Eq(src.Bounds()) {
			t.Fatalf("bounds %v, want %v", got.Bounds(), src.Bounds())
		}
	})
}

func FuzzEncodeAll(f *testing.F) {
	f.Add(uint8(8), uint8(8), true, []byte{1, 2, 3, 4}, []byte{9})
	f.Add(uint8(3), uint8(5), false, []byte{0}, []byte{255, 1})

	f.Fuzz(func(t *testing.T, w, h uint8, lossless bool, first, second []byte) {
		anim := &WEBP{
			Image: []image.Image{fuzzImage(w, h, first), fuzzImage(w, h, second)},
			Delay: []int{10, 20},
		}

		var buf bytes.Buffer

		if err := EncodeAll(&buf, anim, Options{Lossless: lossless}); err != nil {
			t.Fatalf("encode: %v", err)
		}

		got, err := DecodeAll(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}

		if len(got.Image) != len(anim.Image) {
			t.Fatalf("%d frames, want %d", len(got.Image), len(anim.Image))
		}
	})
}
