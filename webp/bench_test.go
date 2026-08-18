package webp

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"
	"testing"
)

func benchFile(b *testing.B, name string) []byte {
	b.Helper()

	data, err := readFileBytes(name)
	if err != nil {
		b.Fatal(err)
	}

	return data
}

func BenchmarkDecode(b *testing.B) {
	for _, name := range []string{"test.webp", "simple-rgb.webp", "lossy_alpha.webp", "simple.webp", "palette.webp", "2-color.webp"} {
		data := benchFile(b, name)

		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Decode(bytes.NewReader(data)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchLarge(b *testing.B, opts EncodeOptions) []byte {
	b.Helper()

	src, err := Decode(bytes.NewReader(benchFile(b, "test.webp")))
	if err != nil {
		b.Fatal(err)
	}

	big := image.NewNRGBA(image.Rect(0, 0, 1920, 1080))

	for y := 0; y < 1080; y += 512 {
		for x := 0; x < 1920; x += 512 {
			draw.Draw(big, image.Rect(x, y, x+512, y+512), src, src.Bounds().Min, draw.Src)
		}
	}

	var buf bytes.Buffer

	if err := Encode(&buf, big, opts); err != nil {
		b.Fatal(err)
	}

	return buf.Bytes()
}

func BenchmarkDecodeLarge(b *testing.B) {
	data := benchLarge(b, EncodeOptions{Quality: 75})

	for _, n := range []int{1, 0} {
		name := "threaded"
		if n == 1 {
			name = "serial"
		}

		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Decode(bytes.NewReader(data), Options{Threads: n}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecodeRGBA(b *testing.B) {
	for _, name := range []string{"test.webp", "simple.webp"} {
		data := benchFile(b, name)

		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Decode(bytes.NewReader(data), Options{ToRGBA: true}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkEncodeLossless(b *testing.B) {
	for _, name := range []string{"simple.webp", "palette.webp", "lossless_alpha.webp"} {
		img, err := Decode(bytes.NewReader(benchFile(b, name)))
		if err != nil {
			b.Fatal(err)
		}

		b.Run(name, func(b *testing.B) {
			r := img.Bounds()

			b.SetBytes(int64(4 * r.Dx() * r.Dy()))
			b.ReportAllocs()

			for b.Loop() {
				if err := Encode(io.Discard, img, EncodeOptions{Lossless: true}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkEncodeLossy(b *testing.B) {
	for _, name := range []string{"test.webp", "simple-rgb.webp", "lossy_alpha.webp"} {
		img, err := Decode(bytes.NewReader(benchFile(b, name)))
		if err != nil {
			b.Fatal(err)
		}

		for _, n := range []int{1, 0} {
			mode := "threaded"
			if n == 1 {
				mode = "serial"
			}

			b.Run(name+"/"+mode, func(b *testing.B) {
				r := img.Bounds()

				b.SetBytes(int64(4 * r.Dx() * r.Dy()))
				b.ReportAllocs()

				for b.Loop() {
					if err := Encode(io.Discard, img, EncodeOptions{Quality: 75, Threads: n}); err != nil {
						b.Fatal(err)
					}
				}
			})
		}

		for _, m := range []int{0, 3, 6} {
			b.Run(fmt.Sprintf("%s/m%d", name, m), func(b *testing.B) {
				r := img.Bounds()

				b.SetBytes(int64(4 * r.Dx() * r.Dy()))

				for b.Loop() {
					if err := Encode(io.Discard, img, EncodeOptions{Quality: 75, Method: m}); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkDecodeConfig(b *testing.B) {
	data := benchFile(b, "test.webp")

	b.ReportAllocs()

	for b.Loop() {
		if _, err := DecodeConfig(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeAll(b *testing.B) {
	data := benchFile(b, "anim.webp")

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	for b.Loop() {
		if _, err := DecodeAll(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkColorIndexing(b *testing.B) {
	const w, h = 640, 480

	for _, bits := range []int{0, 1, 2, 3} {
		packedWidth := subSampleSize(w, bits)
		px := make([]uint32, packedWidth*h)

		for i := range px {
			px[i] = uint32(i) * 2654435761 & 0xff00
		}

		table := make([]uint32, 1<<(8>>bits))
		for i := range table {
			table[i] = uint32(i) * 0x01010101
		}

		out := make([]uint32, w*h)

		b.Run(fmt.Sprint(bits), func(b *testing.B) {
			b.SetBytes(4 * w * h)

			for b.Loop() {
				applyColorIndexing(px, out, w, h, bits, table)
			}
		})
	}
}
