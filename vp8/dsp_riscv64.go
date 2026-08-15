//go:build riscv64 && riscv64.rva23u64 && !noasm

package vp8

//go:noescape
func filterRVV(p *byte, tap, walk, n, limit, ilevel, hevThresh, six int)

//go:noescape
func transformRVV(in *int16, dst *byte)

//go:noescape
func transformDCRVV(in *int16, dst *byte)

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
}
