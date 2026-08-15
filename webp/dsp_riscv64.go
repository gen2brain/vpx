//go:build riscv64 && riscv64.rva23u64 && !noasm

package webp

//go:noescape
func matchLengthRVV(a, b *uint32, limit int) int

//go:noescape
func argbToRGBARVV(dst *byte, px *uint32, n int)

//go:noescape
func upsample16RVV(top, cur, out *byte)

//go:noescape
func yuvToRGBA32RVV(dst, y, u, v *byte)

func dspInit() {
	matchLengthAsm = func(a, b []uint32, limit int) int {
		return matchLengthRVV(&a[0], &b[0], limit)
	}

	argbToRGBAAsm = func(dst []byte, px []uint32) {
		argbToRGBARVV(&dst[0], &px[0], len(px))
	}

	upsample16Asm = func(top, cur, out []byte) {
		_, _ = top[16], cur[16]
		_ = out[63]

		upsample16RVV(&top[0], &cur[0], &out[0])
	}

	yuvToRGBA32Asm = func(dst, y, u, v []byte) {
		_, _ = y[31], u[31]
		_, _ = v[31], dst[127]

		yuvToRGBA32RVV(&dst[0], &y[0], &u[0], &v[0])
	}
}
