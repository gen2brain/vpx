//go:build arm64 && !noasm

package vp8

//go:noescape
func vFilter16NEON(p *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func vFilter16iNEON(p *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func vFilter8NEON(u, v *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func vFilter8iNEON(u, v *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func hFilter16NEON(p *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func hFilter16iNEON(p *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func hFilter8NEON(u, v *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func hFilter8iNEON(u, v *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func transformNEON(in *int16, dst *byte, two int)

//go:noescape
func transformDCNEON(in *int16, dst *byte)

//go:noescape
func sixtapHNEON(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)

//go:noescape
func sixtapVNEON(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)

//go:noescape
func sseNEON(a, b *byte, size int) int

//go:noescape
func fTransformNEON(src, ref *byte, out *int16)

func dspInit() {
	vFilter16Asm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		vFilter16NEON(&p[off], stride, limit, ilevel, hevThresh)
	}

	vFilter16iAsm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		vFilter16iNEON(&p[off], stride, limit, ilevel, hevThresh)
	}

	vFilter8Asm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		vFilter8NEON(&u[off], &v[off], stride, limit, ilevel, hevThresh)
	}

	vFilter8iAsm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		vFilter8iNEON(&u[off], &v[off], stride, limit, ilevel, hevThresh)
	}

	hFilter16Asm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		hFilter16NEON(&p[off], stride, limit, ilevel, hevThresh)
	}

	hFilter16iAsm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		hFilter16iNEON(&p[off], stride, limit, ilevel, hevThresh)
	}

	hFilter8Asm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		hFilter8NEON(&u[off], &v[off], stride, limit, ilevel, hevThresh)
	}

	hFilter8iAsm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		hFilter8iNEON(&u[off], &v[off], stride, limit, ilevel, hevThresh)
	}

	transformAsm = func(in []int16, b []byte, off, two int) {
		transformNEON(&in[0], &b[off], two)
	}

	transformDCAsm = func(in []int16, b []byte, off int) {
		transformDCNEON(&in[0], &b[off])
	}

	sseAsm = func(a, b []byte, off, size int) int {
		return sseNEON(&a[off], &b[off], size)
	}

	fTransformAsm = func(src, ref []byte, sOff, rOff int, out []int16) {
		fTransformNEON(&src[sOff], &ref[rOff], &out[0])
	}

	sixtapHAsm = func(dst []byte, dOff, dStride int, src []byte, sOff, sStride, w, h int, f *[6]int16) {
		if sOff < 2 || len(src)-sOff < (h-1)*sStride+w+30 || len(dst)-dOff < (h-1)*dStride+w {
			sixtapHGo(dst, dOff, dStride, src, sOff, sStride, w, h, f)

			return
		}

		sixtapHNEON(&dst[dOff], dStride, &src[sOff], sStride, w, h, &f[0])
	}

	sixtapVAsm = func(dst []byte, dOff, dStride int, src []byte, sOff, sStride, w, h int, f *[6]int16) {
		if sOff < 2*sStride || len(src)-sOff < (h+2)*sStride+w+8 || len(dst)-dOff < (h-1)*dStride+w {
			sixtapVGo(dst, dOff, dStride, src, sOff, sStride, w, h, f)

			return
		}

		sixtapVNEON(&dst[dOff], dStride, &src[sOff], sStride, w, h, &f[0])
	}
}
