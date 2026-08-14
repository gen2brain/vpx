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
}
