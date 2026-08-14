## vpx
[![Status](https://github.com/gen2brain/vpx/actions/workflows/test.yml/badge.svg)](https://github.com/gen2brain/vpx/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/gen2brain/vpx.svg)](https://pkg.go.dev/github.com/gen2brain/vpx)

[WebP](https://en.wikipedia.org/wiki/WebP) image decoder and encoder in pure Go.

A port of [libwebp](https://chromium.googlesource.com/webm/libwebp), byte-exact against it.
No CGo, no dependencies.

**Under construction.** The decoder is done and byte-exact against libwebp. The encoder
writes lossy, lossless, alpha and animation, and libwebp reads every file it produces, but it
is bigger than `cwebp` at the same quality and the SIMD kernels are not written yet.

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

The `vp8` package decodes the bitstream on its own, a frame at a time, and decodes video as
well as the key frames a still is made of:

```go
d := vp8.Decoder{}

pic, err := d.DecodeFrame(data)
```

A frame the stream marks as not shown updates the references and returns a nil picture. Every
one of the 61 VP8 test vector streams decodes to the same bytes `vpxdec` produces.

### Encoding

```go
err := webp.Encode(w, img, webp.Options{Quality: 90})
```

`Options` selects the quality, the lossless mode, the speed/size method, and whether the RGB
of fully transparent pixels is preserved. `Options.Lossless` writes VP8L, which is exact.
`webp.EncodeAll` writes an animation.

The `vp8` package encodes a key frame on its own:

```go
e := vp8.Encoder{}

data, err := e.Encode(pic, vp8.EncodeOptions{Quality: 90})
```

### Supported

Lossy (VP8) and lossless (VP8L) still images, alpha in both, animation with its disposal and
blending, and the `VP8X` extended container with its ICC, Exif and XMP chunks. Every one of
those decodes byte-exactly against libwebp.

The encoder writes lossy and lossless stills, alpha, and animations. libwebp decodes what it
writes to the same pixels this package does.

### License

The decoder and encoder are a port of libwebp, with parts taken from
[webpkit](https://github.com/P4suta/webpkit). Both notices are in [COPYING](COPYING).

This project is an implementation of a codec. It gives you no special rights on the VP8
patents. Please read the [WebM patent grant](PATENTS) that applies to the VP8 specification
and codec.
