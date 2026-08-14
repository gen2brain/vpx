package webp

import (
	"image"

	"github.com/gen2brain/vpx/vp8"
)

const (
	yuvFix2  = 6
	yuvMask2 = 256<<yuvFix2 - 1
)

func multHi(v, coeff int) int { return v * coeff >> 8 }

func yuvClip8(v int) byte {
	if v&^yuvMask2 == 0 {
		return byte(v >> yuvFix2)
	}

	if v < 0 {
		return 0
	}

	return 255
}

func yuvToR(y, v int) byte { return yuvClip8(multHi(y, 19077) + multHi(v, 26149) - 14234) }

func yuvToG(y, u, v int) byte {
	return yuvClip8(multHi(y, 19077) - multHi(u, 6419) - multHi(v, 13320) + 8708)
}

func yuvToB(y, u int) byte { return yuvClip8(multHi(y, 19077) + multHi(u, 33050) - 17685) }

func yuvToRGB(dst []byte, y, u, v int) {
	dst[0] = yuvToR(y, v)
	dst[1] = yuvToG(y, u, v)
	dst[2] = yuvToB(y, u)
}

func loadUV(u, v byte) uint32 { return uint32(u) | uint32(v)<<16 }

func upsampleRGB(dst []byte, y int, uv uint32) {
	yuvToRGB(dst, y, int(uv&0xff), int(uv>>16))
}

func upsamplePair(topY, bottomY, topU, topV, curU, curV, topDst, bottomDst []byte, n int) {
	last := (n - 1) >> 1

	tlUV := loadUV(topU[0], topV[0])
	lUV := loadUV(curU[0], curV[0])

	upsampleRGB(topDst, int(topY[0]), (3*tlUV+lUV+0x00020002)>>2)

	if bottomY != nil {
		upsampleRGB(bottomDst, int(bottomY[0]), (3*lUV+tlUV+0x00020002)>>2)
	}

	for x := 1; x <= last; x++ {
		tUV := loadUV(topU[x], topV[x])
		uv := loadUV(curU[x], curV[x])

		avg := tlUV + tUV + lUV + uv + 0x00080008
		diag12 := (avg + 2*(tUV+lUV)) >> 3
		diag03 := (avg + 2*(tlUV+uv)) >> 3

		upsampleRGB(topDst[(2*x-1)*4:], int(topY[2*x-1]), (diag12+tlUV)>>1)
		upsampleRGB(topDst[2*x*4:], int(topY[2*x]), (diag03+tUV)>>1)

		if bottomY != nil {
			upsampleRGB(bottomDst[(2*x-1)*4:], int(bottomY[2*x-1]), (diag03+lUV)>>1)
			upsampleRGB(bottomDst[2*x*4:], int(bottomY[2*x]), (diag12+uv)>>1)
		}

		tlUV = tUV
		lUV = uv
	}

	if n&1 != 0 {
		return
	}

	upsampleRGB(topDst[(n-1)*4:], int(topY[n-1]), (3*tlUV+lUV+0x00020002)>>2)

	if bottomY != nil {
		upsampleRGB(bottomDst[(n-1)*4:], int(bottomY[n-1]), (3*lUV+tlUV+0x00020002)>>2)
	}
}

func upsampleFrame(dst []byte, stride int, pic *vp8.Picture) {
	w, h := pic.Width, pic.Height

	row := func(i int) []byte { return dst[i*stride:] }
	yRow := func(i int) []byte { return pic.Y[i*pic.YStride:] }
	uRow := func(i int) []byte { return pic.U[i*pic.UVStride:] }
	vRow := func(i int) []byte { return pic.V[i*pic.UVStride:] }

	upsamplePair(yRow(0), nil, uRow(0), vRow(0), uRow(0), vRow(0), row(0), nil, w)

	for y := 0; y+2 < h; y += 2 {
		k := y / 2
		upsamplePair(yRow(y+1), yRow(y+2), uRow(k), vRow(k), uRow(k+1), vRow(k+1),
			row(y+1), row(y+2), w)
	}

	if h&1 == 0 && h > 1 {
		k := (h - 1) / 2
		upsamplePair(yRow(h-1), nil, uRow(k), vRow(k), uRow(k), vRow(k), row(h-1), nil, w)
	}
}

const (
	yuvFix  = 16
	yuvHalf = 1 << (yuvFix - 1)
)

func rgbToY(r, g, b, rounding int) byte {
	return byte((16839*r + 33059*g + 6420*b + rounding + 16<<yuvFix) >> yuvFix)
}

func clipUV(uv, rounding int) byte {
	uv = (uv + rounding + 128<<(yuvFix+2)) >> (yuvFix + 2)

	if uv&^0xff == 0 {
		return byte(uv)
	}

	if uv < 0 {
		return 0
	}

	return 255
}

func rgbToU(r, g, b, rounding int) byte {
	return clipUV(-9719*r-19081*g+28800*b, rounding)
}

func rgbToV(r, g, b, rounding int) byte {
	return clipUV(28800*r-24116*g-4684*b, rounding)
}

func argbRowToY(src []uint32, y []byte) {
	for i, p := range src {
		y[i] = rgbToY(int(p>>16&0xff), int(p>>8&0xff), int(p&0xff), yuvHalf)
	}
}

func argbRowToUV(src []uint32, u, v []byte, store bool) {
	const rounding = yuvHalf << 2

	n := len(src) >> 1

	for i := range n {
		v0, v1 := src[2*i], src[2*i+1]

		r := int(v0>>15&0x1fe) + int(v1>>15&0x1fe)
		g := int(v0>>7&0x1fe) + int(v1>>7&0x1fe)
		b := int(v0<<1&0x1fe) + int(v1<<1&0x1fe)

		tu, tv := rgbToU(r, g, b, rounding), rgbToV(r, g, b, rounding)

		if store {
			u[i], v[i] = tu, tv
		} else {
			u[i] = byte((int(u[i]) + int(tu) + 1) >> 1)
			v[i] = byte((int(v[i]) + int(tv) + 1) >> 1)
		}
	}

	if len(src)&1 == 0 {
		return
	}

	v0 := src[2*n]

	r := int(v0 >> 14 & 0x3fc)
	g := int(v0 >> 6 & 0x3fc)
	b := int(v0 << 2 & 0x3fc)

	tu, tv := rgbToU(r, g, b, rounding), rgbToV(r, g, b, rounding)

	if store {
		u[n], v[n] = tu, tv
	} else {
		u[n] = byte((int(u[n]) + int(tu) + 1) >> 1)
		v[n] = byte((int(v[n]) + int(tv) + 1) >> 1)
	}
}

func argbToYUVA(px []uint32, w, h int, img *image.NYCbCrA) {
	for y := range h {
		src := px[y*w : (y+1)*w]

		argbRowToY(src, img.Y[y*img.YStride:])

		off := (y >> 1) * img.CStride
		argbRowToUV(src, img.Cb[off:], img.Cr[off:], y&1 == 0)

		a := img.A[y*img.AStride:]
		for i, p := range src {
			a[i] = byte(p >> 24)
		}
	}
}
