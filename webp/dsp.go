package webp

var (
	matchLengthAsm func(a, b []uint32, limit int) int
	argbToRGBAAsm  func(dst []byte, px []uint32)
)

func init() {
	dspInit()
}
