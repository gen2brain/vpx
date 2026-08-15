//go:build arm64 && !noasm

#include "textflag.h"

#define SMULL(m, n, d)  WORD $(0x0E60C000 | ((m) << 16) | ((n) << 5) | (d))
#define SMULL2(m, n, d) WORD $(0x4E60C000 | ((m) << 16) | ((n) << 5) | (d))
#define SSHRH(k, n, d)  WORD $(0x4F000400 | ((32 - (k)) << 16) | ((n) << 5) | (d))
#define SQXTUN(n, d)    WORD $(0x2E212800 | ((n) << 5) | (d))

#define MULHI(a, k, dname) \
	SMULL(k, a, 20)     \
	SMULL2(k, a, 21)    \
	VUZP2 V21.H8, V20.H8, dname

#define PASS \
	VADD V2.H8, V0.H8, V4.H8  \
	VSUB V2.H8, V0.H8, V5.H8  \
	MULHI(1, 13, V6.H8)       \
	MULHI(3, 12, V7.H8)       \
	VSUB V7.H8, V6.H8, V6.H8  \
	VSUB V3.H8, V1.H8, V7.H8  \
	VADD V7.H8, V6.H8, V6.H8  \
	MULHI(1, 12, V8.H8)       \
	MULHI(3, 13, V9.H8)       \
	VADD V9.H8, V8.H8, V8.H8  \
	VADD V3.H8, V1.H8, V9.H8  \
	VADD V9.H8, V8.H8, V8.H8  \
	VADD V8.H8, V4.H8, V10.H8 \
	VADD V6.H8, V5.H8, V11.H8 \
	VSUB V6.H8, V5.H8, V2.H8  \
	VSUB V8.H8, V4.H8, V3.H8  \
	VMOV V10.B16, V0.B16      \
	VMOV V11.B16, V1.B16

#define TRANSPOSE \
	VZIP1 V1.H8, V0.H8, V4.H8  \
	VZIP1 V3.H8, V2.H8, V5.H8  \
	VZIP2 V1.H8, V0.H8, V6.H8  \
	VZIP2 V3.H8, V2.H8, V7.H8  \
	VZIP1 V5.S4, V4.S4, V8.S4  \
	VZIP1 V7.S4, V6.S4, V9.S4  \
	VZIP2 V5.S4, V4.S4, V10.S4 \
	VZIP2 V7.S4, V6.S4, V11.S4 \
	VZIP1 V9.D2, V8.D2, V0.D2  \
	VZIP2 V9.D2, V8.D2, V1.D2  \
	VZIP1 V11.D2, V10.D2, V2.D2 \
	VZIP2 V11.D2, V10.D2, V3.D2

#define ADDSTORE(t, off) \
	VLD1 (R2), [V14.D1]       \
	VUXTL V14.B8, V14.H8      \
	VADD  t.H8, V14.H8, V14.H8 \
	SQXTUN(14, 14)            \
	VST1 [V14.D1], (R2)

// func transformNEON(in *int16, dst *byte, two int)
TEXT ·transformNEON(SB), NOSPLIT, $0-24
	MOVD in+0(FP), R0
	MOVD dst+8(FP), R1
	MOVD two+16(FP), R3

	MOVD $20091, R4
	VDUP R4, V12.H8
	MOVD $-30068, R4
	VDUP R4, V13.H8

	VLD1 (R0), [V0.D1]
	ADD  $8, R0, R5
	VLD1 (R5), [V1.D1]
	ADD  $16, R0, R5
	VLD1 (R5), [V2.D1]
	ADD  $24, R0, R5
	VLD1 (R5), [V3.D1]

	CBZ  R3, one

	ADD  $32, R0, R5
	VLD1 (R5), [V4.D1]
	VMOV V4.D[0], V0.D[1]
	ADD  $40, R0, R5
	VLD1 (R5), [V4.D1]
	VMOV V4.D[0], V1.D[1]
	ADD  $48, R0, R5
	VLD1 (R5), [V4.D1]
	VMOV V4.D[0], V2.D[1]
	ADD  $56, R0, R5
	VLD1 (R5), [V4.D1]
	VMOV V4.D[0], V3.D[1]

one:
	PASS
	TRANSPOSE

	MOVD  $4, R4
	VDUP  R4, V14.H8
	VADD  V14.H8, V0.H8, V0.H8

	PASS

	SSHRH(3, 0, 0)
	SSHRH(3, 1, 1)
	SSHRH(3, 2, 2)
	SSHRH(3, 3, 3)

	TRANSPOSE

	MOVD R1, R2
	CBZ  R3, storeone

	ADDSTORE(V0, 0)
	ADD  $32, R1, R2
	ADDSTORE(V1, 32)
	ADD  $64, R1, R2
	ADDSTORE(V2, 64)
	ADD  $96, R1, R2
	ADDSTORE(V3, 96)
	RET

storeone:
	VLD1  (R2), V14.S[0]
	VUXTL V14.B8, V14.H8
	VADD  V0.H8, V14.H8, V14.H8
	SQXTUN(14, 14)
	VMOV  V14.S[0], R4
	MOVW  R4, (R2)

	ADD   $32, R1, R2
	VLD1  (R2), V14.S[0]
	VUXTL V14.B8, V14.H8
	VADD  V1.H8, V14.H8, V14.H8
	SQXTUN(14, 14)
	VMOV  V14.S[0], R4
	MOVW  R4, (R2)

	ADD   $64, R1, R2
	VLD1  (R2), V14.S[0]
	VUXTL V14.B8, V14.H8
	VADD  V2.H8, V14.H8, V14.H8
	SQXTUN(14, 14)
	VMOV  V14.S[0], R4
	MOVW  R4, (R2)

	ADD   $96, R1, R2
	VLD1  (R2), V14.S[0]
	VUXTL V14.B8, V14.H8
	VADD  V3.H8, V14.H8, V14.H8
	SQXTUN(14, 14)
	VMOV  V14.S[0], R4
	MOVW  R4, (R2)
	RET

// func transformDCNEON(in *int16, dst *byte)
TEXT ·transformDCNEON(SB), NOSPLIT, $0-16
	MOVD in+0(FP), R0
	MOVD dst+8(FP), R1

	MOVH  (R0), R4
	ADD   $4, R4
	SXTH  R4, R4
	ASR   $3, R4, R4

	VDUP  R4, V0.H8

	MOVD  $4, R5

dcloop:
	VLD1  (R1), V1.S[0]
	VUXTL V1.B8, V1.H8
	VADD  V0.H8, V1.H8, V1.H8
	SQXTUN(1, 1)
	VMOV  V1.S[0], R4
	MOVW  R4, (R1)
	ADD   $32, R1
	SUB   $1, R5
	CBNZ  R5, dcloop
	RET
