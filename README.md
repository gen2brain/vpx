## vpx
[![Status](https://github.com/gen2brain/vpx/actions/workflows/test.yml/badge.svg)](https://github.com/gen2brain/vpx/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/gen2brain/vpx.svg)](https://pkg.go.dev/github.com/gen2brain/vpx)

[WebP](https://en.wikipedia.org/wiki/WebP) image decoder and encoder, and a [VP8](https://en.wikipedia.org/wiki/VP8)
video codec, in pure Go.

A port of [libwebp](https://chromium.googlesource.com/webm/libwebp), byte-exact against it. No CGo, no dependencies.

SIMD support for amd64 (SSE2, AVX2), arm64 (NEON) and riscv64 (RVV, with `GORISCV64=rva23u64`).
Build with `-tags noasm` for pure Go everywhere.

### Decoding

```go
img, err := webp.Decode(r)
```

`webp.Decode` returns the planes the bitstream carries, `*image.NYCbCrA` in 4:2:0 for a lossy image and `*image.NRGBA` for a lossless one,
and registers itself with `image.RegisterFormat`. An animation frame is composited and comes back as `*image.RGBA`.

The `vp8` package decodes the bitstream on its own, a frame at a time, and decodes video as well as
the key frames a still is made of:

```go
d := vp8.Decoder{}

pic, err := d.DecodeFrame(data)
```

### Encoding

```go
err := webp.Encode(w, img, webp.EncodeOptions{Quality: 90})
```

`EncodeOptions.Lossless` writes VP8L, which is exact. `webp.EncodeAll` writes an animation.
The `vp8` package encodes video, a key frame and then frames predicted from it:

```go
e := vp8.Encoder{}

key, err := e.Encode(pic, vp8.EncodeOptions{Quality: 90})
next, err := e.EncodeInter(pic2, vp8.EncodeOptions{Quality: 90})
```

There is no container that carries an inter frame yet, so a video stream needs a muxer of your own.

### Supported

Lossy (VP8) and lossless (VP8L) still images, alpha in both, animation with its disposal and blending, and the `VP8X`
extended container with its ICC, Exif and XMP chunks. The encoder writes all of those, plus VP8 video. Every path is
checked against the reference: stills decode byte-exactly against libwebp and libwebp reads back every one this package
writes, all VP8 test vector streams decode to the bytes `vpxdec` produces, and libvpx decodes the encoded video
bit-exactly.

Decoding is 1.09x to 1.21x slower than libwebp and lossy encoding 1.11x slower; lossless encoding is faster, by 1.07x and 1.9x on the two files measured.
Threading worth 1.41x on a 1080p still and 1.55x on a lossy encode, and every encode path allocates nothing per call.

### License

The decoder and encoder are a port of libwebp, BSD-3-Clause, in [COPYING](COPYING), with parts
taken from [webpkit](https://github.com/P4suta/webpkit), whose notice is in
[COPYING.webpkit](COPYING.webpkit).

This project is an implementation of a codec. It gives you no special rights on the VP8
patents. Please read the [WebM patent grant](PATENTS) that applies to the VP8 specification
and codec.
