//go:build arm64 && !noasm

package webp

//go:noescape
func matchLengthNEON(a, b *uint32, limit int) int

func dspInit() {
	matchLengthAsm = func(a, b []uint32, limit int) int {
		return matchLengthNEON(&a[0], &b[0], limit)
	}
}
