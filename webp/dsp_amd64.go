//go:build amd64 && !noasm

package webp

var hasAVX2 = cpuidAVX2()

func cpuidAVX2() bool

//go:noescape
func matchLengthSSE(a, b *uint32, limit int) int

//go:noescape
func matchLengthAVX2(a, b *uint32, limit int) int

func dspInit() {
	matchLengthAsm = func(a, b []uint32, limit int) int {
		if hasAVX2 {
			return matchLengthAVX2(&a[0], &b[0], limit)
		}

		return matchLengthSSE(&a[0], &b[0], limit)
	}
}
