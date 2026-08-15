//go:build arm64 && !noasm

#include "textflag.h"

#define SQXTUN(n, d)  WORD $(0x2E212800 | ((n) << 5) | (d))
#define SQXTUN2(n, d) WORD $(0x6E212800 | ((n) << 5) | (d))

// func trueMotionNEON(b *byte, stride, size int)
TEXT ·trueMotionNEON(SB), NOSPLIT, $0-24
	MOVD b+0(FP), R0
	MOVD stride+8(FP), R1
	MOVD size+16(FP), R2
	MOVD R2, R7

	SUB   R1, R0, R3
	MOVBU -1(R3), R4
	VDUP  R4, V2.H8

	CMP $16, R2
	BEQ top16

	VLD1  (R3), [V0.D1]
	VUXTL V0.B8, V0.H8
	B     rows

top16:
	VLD1   (R3), [V0.B16]
	VUXTL2 V0.B16, V1.H8
	VUXTL  V0.B8, V0.H8

rows:
	MOVBU -1(R0), R4
	VDUP  R4, V3.H8

	VADD V0.H8, V3.H8, V4.H8
	VSUB V2.H8, V4.H8, V4.H8

	CMP $16, R2
	BEQ row16

	SQXTUN(4, 5)

	CMP $4, R2
	BEQ store4

	VST1 [V5.D1], (R0)
	B    next

store4:
	VMOV V5.S[0], R5
	MOVW R5, (R0)
	B    next

row16:
	VADD V1.H8, V3.H8, V6.H8
	VSUB V2.H8, V6.H8, V6.H8
	SQXTUN(4, 5)
	SQXTUN2(6, 5)
	VST1 [V5.B16], (R0)

next:
	ADD  R1, R0
	SUB  $1, R7
	CBNZ R7, rows
	RET
