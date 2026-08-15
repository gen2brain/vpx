//go:build riscv64 && riscv64.rva23u64 && !noasm

package webp

//go:noescape
func matchLengthRVV(a, b *uint32, limit int) int

//go:noescape
func argbToRGBARVV(dst *byte, px *uint32, n int)

func dspInit() {
	matchLengthAsm = func(a, b []uint32, limit int) int {
		return matchLengthRVV(&a[0], &b[0], limit)
	}

	argbToRGBAAsm = func(dst []byte, px []uint32) {
		argbToRGBARVV(&dst[0], &px[0], len(px))
	}
}
