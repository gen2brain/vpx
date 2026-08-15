package webp

var (
	matchLengthAsm func(a, b []uint32, limit int) int
	argbToRGBAAsm  func(dst []byte, px []uint32)
	upsample16Asm  func(top, cur, out []byte)
	yuvToRGBA32Asm func(dst, y, u, v []byte)
)

func init() {
	dspInit()
}
