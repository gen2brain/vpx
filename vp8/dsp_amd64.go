//go:build amd64 && !noasm

package vp8

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
}
