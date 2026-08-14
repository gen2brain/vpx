## vpx
[![Status](https://github.com/gen2brain/vpx/actions/workflows/test.yml/badge.svg)](https://github.com/gen2brain/vpx/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/gen2brain/vpx.svg)](https://pkg.go.dev/github.com/gen2brain/vpx)

[WebP](https://en.wikipedia.org/wiki/WebP) image decoder and encoder in pure Go.

A port of [libwebp](https://chromium.googlesource.com/webm/libwebp), byte-exact against it.
No CGo, no dependencies.

SIMD support for amd64 (AVX2), arm64 (NEON) and riscv64 (RVV, with `GORISCV64=rva23u64`).
Build with `-tags noasm` for pure Go everywhere.

**Under construction.** The decoder is done and byte-exact against libwebp; the encoder is
not written yet. `docs/PLAN.md` has the design and the milestone table, `docs/STATUS.md` where
it stands.

### Decoding

```go
img, err := webp.Decode(r)
```

`webp.Decode` returns the planes the bitstream carries, `*image.NYCbCrA` in 4:2:0 for a lossy
image and `*image.NRGBA` for a lossless one, and registers itself with `image.RegisterFormat`.
An animation frame is composited and comes back as `*image.RGBA`. `webp.DecodeAll` returns
every frame with its delay.

`Options.ToRGBA` forces `*image.RGBA` with premultiplied alpha and `Options.ToYCbCr` forces
`*image.NYCbCrA`, whichever the image is natively. `Options.AutoRotate` applies the EXIF
orientation, and `webp.DecodeExif` reads the metadata without decoding pixels.

A reader that is also an `io.ReaderAt` and an `io.Seeker`, such as an `*os.File`, is addressed
by range rather than read into memory: `DecodeConfig` on a 47 KB file touches 46 bytes of it,
and `Decode` never reads metadata it does not need.

The `vp8` package decodes the bitstream on its own, a frame at a time:

```go
d := vp8.Decoder{}

pic, err := d.DecodeFrame(data)
```

### Encoding

```go
err := webp.Encode(w, img, &webp.Options{Quality: 90})
```

`Options` selects the quality, the lossless mode, the speed/size method, and whether the RGB
of fully transparent pixels is preserved. `webp.EncodeAll` writes an animation.

### Supported

Lossy (VP8) and lossless (VP8L) still images, alpha in both, animation with its disposal and
blending, and the `VP8X` extended container with its ICC, Exif and XMP chunks. Every one of
those decodes byte-exactly against libwebp.

### License

The decoder and encoder are a port of libwebp, with parts taken from
[webpkit](https://github.com/P4suta/webpkit). Both notices are in [COPYING](COPYING).

This project is an implementation of a codec. It gives you no special rights on the VP8
patents. Please read the [WebM patent grant](PATENTS) that applies to the VP8 specification
and codec.
