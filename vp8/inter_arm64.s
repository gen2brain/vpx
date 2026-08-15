//go:build arm64 && !noasm

#include "textflag.h"

#define MULH(m, n, d)   WORD $(0x4E609C00 | ((m) << 16) | ((n) << 5) | (d))
#define SQADDH(m, n, d) WORD $(0x4E600C00 | ((m) << 16) | ((n) << 5) | (d))
#define SSHRH(k, n, d)  WORD $(0x4F000400 | ((32 - (k)) << 16) | ((n) << 5) | (d))
#define SQXTUN(n, d)    WORD $(0x2E212800 | ((n) << 5) | (d))

#define LOADCOEF \
	MOVD f+48(FP), R8   \
	MOVH (R8), R9       \
	VDUP R9, V10.H8     \
	MOVH 2(R8), R9      \
	VDUP R9, V11.H8     \
	MOVH 4(R8), R9      \
	VDUP R9, V12.H8     \
	MOVH 6(R8), R9      \
	VDUP R9, V13.H8     \
	MOVH 8(R8), R9      \
	VDUP R9, V14.H8     \
	MOVH 10(R8), R9     \
	VDUP R9, V15.H8     \
	MOVD $64, R9        \
	VDUP R9, V9.H8

#define VTAP(off, coef) \
	VLD1  (R11), [V4.B8] \
	VUXTL V4.B8, V4.H8   \
	MULH(coef, 4, 4)     \
	SQADDH(4, 3, 3)

// func sixtapVNEON(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)
TEXT ·sixtapVNEON(SB), NOSPLIT, $0-56
	LOADCOEF

	MOVD dst+0(FP), R0
	MOVD dStride+8(FP), R1
	MOVD src+16(FP), R2
	MOVD sStride+24(FP), R3
	MOVD w+32(FP), R4
	MOVD h+40(FP), R5

	LSL  $1, R3, R6
	SUB  R6, R2

vrow:
	MOVD $0, R7

vgroup:
	ADD  R7, R2, R12

	MOVD  R12, R11
	VLD1  (R11), [V3.B8]
	VUXTL V3.B8, V3.H8
	MULH(10, 3, 3)

	ADD R3, R12, R11
	VTAP(1, 11)

	ADD R3, R11, R11
	ADD R3, R11, R11
	ADD R3, R11, R11
	VTAP(4, 14)

	ADD R3, R11, R11
	VTAP(5, 15)

	ADD  R3, R12, R11
	ADD  R3, R11, R11
	VTAP(2, 12)

	ADD R3, R11, R11
	VTAP(3, 13)

	SQADDH(9, 3, 3)
	SSHRH(7, 3, 3)
	SQXTUN(3, 3)

	ADD  R7, R0, R13
	CMP  $4, R4
	BEQ  vstore4

	VST1 [V3.D1], (R13)
	B    vnext

vstore4:
	VMOV V3.S[0], R14
	MOVW R14, (R13)

vnext:
	ADD  $8, R7
	CMP  R4, R7
	BLT  vgroup

	ADD  R3, R2
	ADD  R1, R0
	SUB  $1, R5
	CBNZ R5, vrow
	RET

#define HTAP(k, coef) \
	VEXT  $k, V1.B16, V0.B16, V4.B16 \
	VUXTL V4.B8, V4.H8               \
	MULH(coef, 4, 4)                 \
	SQADDH(4, 3, 3)

// func sixtapHNEON(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)
TEXT ·sixtapHNEON(SB), NOSPLIT, $0-56
	LOADCOEF

	MOVD dst+0(FP), R0
	MOVD dStride+8(FP), R1
	MOVD src+16(FP), R2
	MOVD sStride+24(FP), R3
	MOVD w+32(FP), R4
	MOVD h+40(FP), R5

hrow:
	MOVD $0, R7

hgroup:
	ADD  R7, R2, R12
	SUB  $2, R12, R11
	VLD1 (R11), [V0.B16]
	ADD  $16, R11
	VLD1 (R11), [V1.B16]

	VUXTL V0.B8, V3.H8
	MULH(10, 3, 3)

	HTAP(1, 11)
	HTAP(4, 14)
	HTAP(5, 15)
	HTAP(2, 12)
	HTAP(3, 13)

	SQADDH(9, 3, 3)
	SSHRH(7, 3, 3)
	SQXTUN(3, 3)

	ADD  R7, R0, R13
	CMP  $4, R4
	BEQ  hstore4

	VST1 [V3.D1], (R13)
	B    hnext

hstore4:
	VMOV V3.S[0], R14
	MOVW R14, (R13)

hnext:
	ADD  $8, R7
	CMP  R4, R7
	BLT  hgroup

	ADD  R3, R2
	ADD  R1, R0
	SUB  $1, R5
	CBNZ R5, hrow
	RET
