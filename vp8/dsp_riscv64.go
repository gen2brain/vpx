//go:build riscv64 && riscv64.rva23u64 && !noasm

package vp8

//go:noescape
func filterRVV(p *byte, tap, walk, n, limit, ilevel, hevThresh, six int)

//go:noescape
func transformRVV(in *int16, dst *byte)

//go:noescape
func transformDCRVV(in *int16, dst *byte)

//go:noescape
func sixtapRVV(dst *byte, dStride int, src *byte, sStride, step, w, h int, f *int16)

func dspInit() {
	vFilter16Asm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		filterRVV(&p[off], stride, 1, 16, limit, ilevel, hevThresh, 1)
	}

	vFilter16iAsm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		for range 3 {
			off += 4 * stride
			filterRVV(&p[off], stride, 1, 16, limit, ilevel, hevThresh, 0)
		}
	}

	vFilter8Asm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		filterRVV(&u[off], stride, 1, 8, limit, ilevel, hevThresh, 1)
		filterRVV(&v[off], stride, 1, 8, limit, ilevel, hevThresh, 1)
	}

	vFilter8iAsm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		filterRVV(&u[off+4*stride], stride, 1, 8, limit, ilevel, hevThresh, 0)
		filterRVV(&v[off+4*stride], stride, 1, 8, limit, ilevel, hevThresh, 0)
	}

	hFilter16Asm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		filterRVV(&p[off], 1, stride, 16, limit, ilevel, hevThresh, 1)
	}

	hFilter16iAsm = func(p []byte, off, stride, limit, ilevel, hevThresh int) {
		for range 3 {
			off += 4
			filterRVV(&p[off], 1, stride, 16, limit, ilevel, hevThresh, 0)
		}
	}

	hFilter8Asm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		filterRVV(&u[off], 1, stride, 8, limit, ilevel, hevThresh, 1)
		filterRVV(&v[off], 1, stride, 8, limit, ilevel, hevThresh, 1)
	}

	hFilter8iAsm = func(u, v []byte, off, stride, limit, ilevel, hevThresh int) {
		filterRVV(&u[off+4], 1, stride, 8, limit, ilevel, hevThresh, 0)
		filterRVV(&v[off+4], 1, stride, 8, limit, ilevel, hevThresh, 0)
	}

	transformAsm = func(in []int16, b []byte, off, two int) {
		transformRVV(&in[0], &b[off])

		if two != 0 {
			transformRVV(&in[16], &b[off+4])
		}
	}

	transformDCAsm = func(in []int16, b []byte, off int) {
		transformDCRVV(&in[0], &b[off])
	}

	sixtapHAsm = func(dst []byte, dOff, dStride int, src []byte, sOff, sStride, w, h int, f *[6]int16) {
		if sOff < 2 || len(src)-sOff < (h-1)*sStride+w+4 || len(dst)-dOff < (h-1)*dStride+w {
			sixtapHGo(dst, dOff, dStride, src, sOff, sStride, w, h, f)

			return
		}

		sixtapRVV(&dst[dOff], dStride, &src[sOff], sStride, 1, w, h, &f[0])
	}

	sixtapVAsm = func(dst []byte, dOff, dStride int, src []byte, sOff, sStride, w, h int, f *[6]int16) {
		if sOff < 2*sStride || len(src)-sOff < (h+2)*sStride+w || len(dst)-dOff < (h-1)*dStride+w {
			sixtapVGo(dst, dOff, dStride, src, sOff, sStride, w, h, f)

			return
		}

		sixtapRVV(&dst[dOff], dStride, &src[sOff], sStride, sStride, w, h, &f[0])
	}
}
