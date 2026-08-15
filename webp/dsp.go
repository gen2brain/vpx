package webp

var matchLengthAsm func(a, b []uint32, limit int) int

func init() {
	dspInit()
}
