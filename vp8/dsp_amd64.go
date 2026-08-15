//go:build amd64 && !noasm

package vp8

var hasAVX2 = cpuidAVX2()

func cpuidAVX2() bool

//go:noescape
func vFilter16SSE(p *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func vFilter16iSSE(p *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func vFilter8SSE(u, v *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func vFilter8iSSE(u, v *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func hFilter16SSE(p *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func hFilter16iSSE(p *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func hFilter8SSE(u, v *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func hFilter8iSSE(u, v *byte, stride, limit, ilevel, hevThresh int)

//go:noescape
func transformSSE(in *int16, dst *byte, two int)

//go:noescape
func transformDCSSE(in *int16, dst *byte)

//go:noescape
func sixtapHAVX2(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)

//go:noescape
func sixtapVAVX2(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)

//go:noescape
func sixtapHSSE(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)

//go:noescape
func sixtapVSSE(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)

//go:noescape
func sseSSE(a, b *byte, size int) int

//go:noescape
func fTransformSSE(src, ref *byte, out *int16)

//go:noescape
func quantizeSSE(in, out *int16, m *qmatrix)

func dspInit() {
	vFilter16Asm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		vFilter16SSE(&p[off], stride, limit, ilevel, hevThresh)
	}

	vFilter16iAsm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		vFilter16iSSE(&p[off], stride, limit, ilevel, hevThresh)
	}

	vFilter8Asm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		vFilter8SSE(&u[off], &v[off], stride, limit, ilevel, hevThresh)
	}

	vFilter8iAsm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		vFilter8iSSE(&u[off], &v[off], stride, limit, ilevel, hevThresh)
	}

	hFilter16Asm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		hFilter16SSE(&p[off], stride, limit, ilevel, hevThresh)
	}

	hFilter16iAsm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		hFilter16iSSE(&p[off], stride, limit, ilevel, hevThresh)
	}

	hFilter8Asm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		hFilter8SSE(&u[off], &v[off], stride, limit, ilevel, hevThresh)
	}

	hFilter8iAsm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		hFilter8iSSE(&u[off], &v[off], stride, limit, ilevel, hevThresh)
	}

	transformAsm = func(in []int16, b []byte, off, two int) {
		transformSSE(&in[0], &b[off], two)
	}

	transformDCAsm = func(in []int16, b []byte, off int) {
		transformDCSSE(&in[0], &b[off])
	}

	sixtapHAsm = func(dst []byte, dOff, dStride int, src []byte, sOff, sStride, w, h int, f *[6]int16) {
		if sOff < 2 || len(src)-sOff < (h-1)*sStride+w+14 || len(dst)-dOff < (h-1)*dStride+w {
			sixtapHGo(dst, dOff, dStride, src, sOff, sStride, w, h, f)

			return
		}

		if hasAVX2 {
			sixtapHAVX2(&dst[dOff], dStride, &src[sOff], sStride, w, h, &f[0])

			return
		}

		sixtapHSSE(&dst[dOff], dStride, &src[sOff], sStride, w, h, &f[0])
	}

	sseAsm = func(a, b []byte, off, size int) int {
		return sseSSE(&a[off], &b[off], size)
	}

	fTransformAsm = func(src, ref []byte, sOff, rOff int, out []int16) {
		fTransformSSE(&src[sOff], &ref[rOff], &out[0])
	}

	quantizeAsm = func(in, out []int16, m *qmatrix) {
		quantizeSSE(&in[0], &out[0], m)
	}

	sixtapVAsm = func(dst []byte, dOff, dStride int, src []byte, sOff, sStride, w, h int, f *[6]int16) {
		if sOff < 2*sStride || len(src)-sOff < (h+2)*sStride+w+8 || len(dst)-dOff < (h-1)*dStride+w {
			sixtapVGo(dst, dOff, dStride, src, sOff, sStride, w, h, f)

			return
		}

		if hasAVX2 {
			sixtapVAVX2(&dst[dOff], dStride, &src[sOff], sStride, w, h, &f[0])

			return
		}

		sixtapVSSE(&dst[dOff], dStride, &src[sOff], sStride, w, h, &f[0])
	}
}
