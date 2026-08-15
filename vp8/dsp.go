package vp8

var (
	vFilter16Asm  func(p []byte, off, stride, limit, ilevel, hevThresh int)
	vFilter16iAsm func(p []byte, off, stride, limit, ilevel, hevThresh int)
	vFilter8Asm   func(u, v []byte, off, stride, limit, ilevel, hevThresh int)
	vFilter8iAsm  func(u, v []byte, off, stride, limit, ilevel, hevThresh int)

	hFilter16Asm  func(p []byte, off, stride, limit, ilevel, hevThresh int)
	hFilter16iAsm func(p []byte, off, stride, limit, ilevel, hevThresh int)
	hFilter8Asm   func(u, v []byte, off, stride, limit, ilevel, hevThresh int)
	hFilter8iAsm  func(u, v []byte, off, stride, limit, ilevel, hevThresh int)

	transformAsm   func(in []int16, b []byte, off, two int)
	transformDCAsm func(in []int16, b []byte, off int)

	sixtapHAsm func(dst []byte, dOff, dStride int, src []byte, sOff, sStride, w, h int, f *[6]int16)
	sixtapVAsm func(dst []byte, dOff, dStride int, src []byte, sOff, sStride, w, h int, f *[6]int16)

	sseAsm        func(a, b []byte, off, size int) int
	fTransformAsm func(src, ref []byte, sOff, rOff int, out []int16)
)

func init() {
	dspInit()
}
