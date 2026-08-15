//go:build arm64 && !noasm

package webp

//go:noescape
func matchLengthNEON(a, b *uint32, limit int) int

//go:noescape
func argbToRGBANEON(dst *byte, px *uint32, n int)

func dspInit() {
	matchLengthAsm = func(a, b []uint32, limit int) int {
		return matchLengthNEON(&a[0], &b[0], limit)
	}

	argbToRGBAAsm = func(dst []byte, px []uint32) {
		argbToRGBANEON(&dst[0], &px[0], len(px))
	}
}
