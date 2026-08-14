package webp

import (
	"bytes"
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
	for _, name := range []string{"test.webp", "simple-rgb.webp", "lossy_alpha.webp", "simple.webp", "palette.webp"} {
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
