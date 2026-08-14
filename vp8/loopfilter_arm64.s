//go:build arm64 && !noasm

#include "textflag.h"

#define SQADD(m, n, d)    WORD $(0x4E200C00 | ((m) << 16) | ((n) << 5) | (d))
#define SQSUB(m, n, d)    WORD $(0x4E202C00 | ((m) << 16) | ((n) << 5) | (d))
#define SSHR3(n, d)       WORD $(0x4F0D0400 | ((n) << 5) | (d))
#define SRSHR1(n, d)      WORD $(0x4F0F2400 | ((n) << 5) | (d))
#define SMLAL(m, n, d)    WORD $(0x0E208000 | ((m) << 16) | ((n) << 5) | (d))
#define SMLAL2(m, n, d)   WORD $(0x4E208000 | ((m) << 16) | ((n) << 5) | (d))
#define SQRSHRN(k, n, d)  WORD $(0x0F009C00 | ((16 - (k)) << 16) | ((n) << 5) | (d))
#define SQRSHRN2(k, n, d) WORD $(0x4F009C00 | ((16 - (k)) << 16) | ((n) << 5) | (d))

#define ABSD(a, b, t, d) \
	VUMAX a, b, t \
	VUMIN a, b, d \
	VSUB  d, t, d

#define MASKS \
	ABSD(V2.B16, V3.B16, V10.B16, V8.B16)   \
	ABSD(V0.B16, V1.B16, V10.B16, V11.B16)  \
	VUMAX V11.B16, V8.B16, V8.B16           \
	ABSD(V1.B16, V2.B16, V10.B16, V11.B16)  \
	VUMAX V11.B16, V8.B16, V8.B16           \
	ABSD(V5.B16, V4.B16, V10.B16, V11.B16)  \
	VUMAX V11.B16, V8.B16, V8.B16           \
	ABSD(V7.B16, V6.B16, V10.B16, V11.B16)  \
	VUMAX V11.B16, V8.B16, V8.B16           \
	ABSD(V6.B16, V5.B16, V10.B16, V11.B16)  \
	VUMAX V11.B16, V8.B16, V8.B16           \
	VUMIN V24.B16, V8.B16, V10.B16          \
	VCMEQ V10.B16, V8.B16, V8.B16           \
	ABSD(V3.B16, V4.B16, V10.B16, V12.B16)  \
	ABSD(V2.B16, V5.B16, V10.B16, V13.B16)  \
	VUSHR   $1, V13.B16, V13.B16            \
	VUSHLL  $0, V12.B8, V14.H8              \
	VUSHLL2 $0, V12.B16, V15.H8             \
	VUSHLL  $0, V13.B8, V16.H8              \
	VUSHLL2 $0, V13.B16, V17.H8             \
	VSHL  $1, V14.H8, V14.H8                \
	VSHL  $1, V15.H8, V15.H8                \
	VADD  V16.H8, V14.H8, V14.H8            \
	VADD  V17.H8, V15.H8, V15.H8            \
	VUMIN V25.H8, V14.H8, V16.H8            \
	VCMEQ V16.H8, V14.H8, V14.H8            \
	VUMIN V25.H8, V15.H8, V17.H8            \
	VCMEQ V17.H8, V15.H8, V15.H8            \
	VUZP1 V15.B16, V14.B16, V14.B16         \
	VAND  V14.B16, V8.B16, V8.B16           \
	ABSD(V2.B16, V3.B16, V10.B16, V11.B16)  \
	ABSD(V5.B16, V4.B16, V10.B16, V12.B16)  \
	VUMAX V12.B16, V11.B16, V11.B16         \
	VUMIN V26.B16, V11.B16, V10.B16         \
	VCMEQ V10.B16, V11.B16, V9.B16          \
	VEOR  V23.B16, V9.B16, V9.B16

#define BASEDELTA \
	SQSUB(5, 2, 10) \
	SQSUB(3, 4, 11) \
	SQADD(11, 10, 10) \
	SQADD(11, 10, 10) \
	SQADD(11, 10, 10)

#define FILTER2(delta) \
	SQADD(19, delta, 14) \
	SQADD(20, delta, 15) \
	SSHR3(14, 14)        \
	SSHR3(15, 15)        \
	SQADD(14, 3, 3)      \
	SQSUB(15, 4, 4)

#define DOFILTER6 \
	VEOR V18.B16, V1.B16, V1.B16   \
	VEOR V18.B16, V2.B16, V2.B16   \
	VEOR V18.B16, V3.B16, V3.B16   \
	VEOR V18.B16, V4.B16, V4.B16   \
	VEOR V18.B16, V5.B16, V5.B16   \
	VEOR V18.B16, V6.B16, V6.B16   \
	BASEDELTA                      \
	VAND V9.B16, V8.B16, V12.B16   \
	VAND V12.B16, V10.B16, V13.B16 \
	FILTER2(13)                    \
	VEOR V8.B16, V12.B16, V12.B16  \
	VAND V12.B16, V10.B16, V13.B16 \
	VORR V23.B16, V23.B16, V14.B16 \
	VORR V23.B16, V23.B16, V15.B16 \
	SMLAL(21, 13, 14)              \
	SMLAL2(21, 13, 15)             \
	VORR V14.B16, V14.B16, V16.B16 \
	VORR V15.B16, V15.B16, V17.B16 \
	SMLAL(22, 13, 16)              \
	SMLAL2(22, 13, 17)             \
	SQRSHRN(7, 14, 10)             \
	SQRSHRN2(7, 15, 10)            \
	SQRSHRN(6, 14, 11)             \
	SQRSHRN2(6, 15, 11)            \
	SQRSHRN(7, 16, 12)             \
	SQRSHRN2(7, 17, 12)            \
	SQADD(12, 3, 3)                \
	SQSUB(12, 4, 4)                \
	SQSUB(11, 5, 5)                \
	SQADD(11, 2, 2)                \
	SQSUB(10, 6, 6)                \
	SQADD(10, 1, 1)                \
	VEOR V18.B16, V1.B16, V1.B16   \
	VEOR V18.B16, V2.B16, V2.B16   \
	VEOR V18.B16, V3.B16, V3.B16   \
	VEOR V18.B16, V4.B16, V4.B16   \
	VEOR V18.B16, V5.B16, V5.B16   \
	VEOR V18.B16, V6.B16, V6.B16

#define DOFILTER4 \
	VEOR V18.B16, V2.B16, V2.B16   \
	VEOR V18.B16, V3.B16, V3.B16   \
	VEOR V18.B16, V4.B16, V4.B16   \
	VEOR V18.B16, V5.B16, V5.B16   \
	BASEDELTA                      \
	VAND V9.B16, V8.B16, V12.B16   \
	VAND V12.B16, V10.B16, V13.B16 \
	FILTER2(13)                    \
	VEOR V8.B16, V12.B16, V12.B16  \
	SQSUB(3, 4, 10)                \
	SQADD(10, 10, 11)              \
	SQADD(10, 11, 10)              \
	VAND V12.B16, V10.B16, V13.B16 \
	SQADD(20, 13, 14)              \
	SQADD(19, 13, 15)              \
	SSHR3(14, 14)                  \
	SSHR3(15, 15)                  \
	SRSHR1(14, 16)                 \
	SQADD(15, 3, 3)                \
	SQSUB(14, 4, 4)                \
	SQADD(16, 2, 2)                \
	SQSUB(16, 5, 5)                \
	VEOR V18.B16, V2.B16, V2.B16   \
	VEOR V18.B16, V3.B16, V3.B16   \
	VEOR V18.B16, V4.B16, V4.B16   \
	VEOR V18.B16, V5.B16, V5.B16

#define CONSTANTS(limit, ilevel, hev) \
	MOVD  limit, R4               \
	MOVD  ilevel, R5              \
	MOVD  hev, R6                 \
	VDUP  R5, V24.B16             \
	VDUP  R4, V25.H8              \
	VDUP  R6, V26.B16             \
	VMOVI $128, V18.B16           \
	VMOVI $3, V19.B16             \
	VMOVI $4, V20.B16             \
	VMOVI $9, V21.B16             \
	VMOVI $18, V22.B16            \
	VMOVI $255, V23.B16

// func vFilter16NEON(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter16NEON(SB), NOSPLIT, $0-40
	MOVD p+0(FP), R0
	MOVD stride+8(FP), R1
	CONSTANTS(limit+16(FP), ilevel+24(FP), hevThresh+32(FP))

	LSL  $2, R1, R2
	SUB  R2, R0, R3

	VLD1 (R3), [V0.B16]
	ADD  R1, R3
	VLD1 (R3), [V1.B16]
	ADD  R1, R3
	VLD1 (R3), [V2.B16]
	ADD  R1, R3
	VLD1 (R3), [V3.B16]
	ADD  R1, R3
	VLD1 (R3), [V4.B16]
	ADD  R1, R3
	VLD1 (R3), [V5.B16]
	ADD  R1, R3
	VLD1 (R3), [V6.B16]
	ADD  R1, R3
	VLD1 (R3), [V7.B16]

	MASKS
	DOFILTER6

	ADD  R1, R1, R2
	ADD  R1, R2, R2
	SUB  R2, R0, R3

	VST1 [V1.B16], (R3)
	ADD  R1, R3
	VST1 [V2.B16], (R3)
	ADD  R1, R3
	VST1 [V3.B16], (R3)
	ADD  R1, R3
	VST1 [V4.B16], (R3)
	ADD  R1, R3
	VST1 [V5.B16], (R3)
	ADD  R1, R3
	VST1 [V6.B16], (R3)
	RET

// func vFilter16iNEON(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter16iNEON(SB), NOSPLIT, $0-40
	MOVD p+0(FP), R0
	MOVD stride+8(FP), R1
	CONSTANTS(limit+16(FP), ilevel+24(FP), hevThresh+32(FP))

	MOVD $3, R7
	MOVD R0, R3

	VLD1 (R3), [V0.B16]
	ADD  R1, R3
	VLD1 (R3), [V1.B16]
	ADD  R1, R3
	VLD1 (R3), [V2.B16]
	ADD  R1, R3
	VLD1 (R3), [V3.B16]
	ADD  R1, R3

loop16i:
	VLD1 (R3), [V4.B16]
	ADD  R1, R3
	VLD1 (R3), [V5.B16]
	ADD  R1, R3
	VLD1 (R3), [V6.B16]
	ADD  R1, R3
	VLD1 (R3), [V7.B16]
	ADD  R1, R3

	MASKS
	DOFILTER4

	LSL  $2, R1, R2
	SUB  R2, R3, R8
	ADD  R1, R1, R2
	SUB  R2, R8, R8

	VST1 [V2.B16], (R8)
	ADD  R1, R8
	VST1 [V3.B16], (R8)
	ADD  R1, R8
	VST1 [V4.B16], (R8)
	ADD  R1, R8
	VST1 [V5.B16], (R8)

	VORR V4.B16, V4.B16, V0.B16
	VORR V5.B16, V5.B16, V1.B16
	VORR V6.B16, V6.B16, V2.B16
	VORR V7.B16, V7.B16, V3.B16

	SUB  $1, R7
	CBNZ R7, loop16i
	RET

#define LOADUV(x) \
	VLD1 (R3), [x.D1]      \
	VLD1 (R8), [V27.D1]    \
	VMOV V27.D[0], x.D[1]  \
	ADD  R1, R3            \
	ADD  R1, R8

#define STOREUV(x) \
	VST1 [x.D1], (R3)      \
	VMOV x.D[1], V27.D[0]  \
	VST1 [V27.D1], (R8)    \
	ADD  R1, R3            \
	ADD  R1, R8

// func vFilter8NEON(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter8NEON(SB), NOSPLIT, $0-48
	MOVD u+0(FP), R0
	MOVD v+8(FP), R9
	MOVD stride+16(FP), R1
	CONSTANTS(limit+24(FP), ilevel+32(FP), hevThresh+40(FP))

	LSL  $2, R1, R2
	SUB  R2, R0, R3
	SUB  R2, R9, R8

	LOADUV(V0)
	LOADUV(V1)
	LOADUV(V2)
	LOADUV(V3)
	LOADUV(V4)
	LOADUV(V5)
	LOADUV(V6)
	LOADUV(V7)

	MASKS
	DOFILTER6

	ADD  R1, R1, R2
	ADD  R1, R2, R2
	SUB  R2, R0, R3
	SUB  R2, R9, R8

	STOREUV(V1)
	STOREUV(V2)
	STOREUV(V3)
	STOREUV(V4)
	STOREUV(V5)
	STOREUV(V6)
	RET

// func vFilter8iNEON(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter8iNEON(SB), NOSPLIT, $0-48
	MOVD u+0(FP), R0
	MOVD v+8(FP), R9
	MOVD stride+16(FP), R1
	CONSTANTS(limit+24(FP), ilevel+32(FP), hevThresh+40(FP))

	MOVD R0, R3
	MOVD R9, R8

	LOADUV(V0)
	LOADUV(V1)
	LOADUV(V2)
	LOADUV(V3)
	LOADUV(V4)
	LOADUV(V5)
	LOADUV(V6)
	LOADUV(V7)

	MASKS
	DOFILTER4

	ADD  R1, R1, R2
	ADD  R2, R0, R3
	ADD  R2, R9, R8

	STOREUV(V2)
	STOREUV(V3)
	STOREUV(V4)
	STOREUV(V5)
	RET

#define LOAD8X4(ptr, outp, outq) \
	VLD1.P (ptr)(R1), V27.S[0]      \
	VLD1.P (ptr)(R1), V28.S[0]      \
	VLD1.P (ptr)(R1), V27.S[2]      \
	VLD1.P (ptr)(R1), V28.S[2]      \
	VLD1.P (ptr)(R1), V27.S[1]      \
	VLD1.P (ptr)(R1), V28.S[1]      \
	VLD1.P (ptr)(R1), V27.S[3]      \
	VLD1.P (ptr)(R1), V28.S[3]      \
	VZIP1 V28.B16, V27.B16, V29.B16 \
	VZIP2 V28.B16, V27.B16, V30.B16 \
	VZIP1 V30.H8, V29.H8, V31.H8    \
	VZIP2 V30.H8, V29.H8, V27.H8    \
	VZIP1 V27.S4, V31.S4, outp.S4   \
	VZIP2 V27.S4, V31.S4, outq.S4

#define LOAD16X4(p0, p8, o0, o1, o2, o3) \
	LOAD8X4(p0, o0, o2)          \
	LOAD8X4(p8, o1, o3)          \
	VORR  o0.B16, o0.B16, V27.B16 \
	VZIP1 o1.D2, V27.D2, o0.D2   \
	VZIP2 o1.D2, V27.D2, o1.D2   \
	VORR  o2.B16, o2.B16, V27.B16 \
	VZIP1 o3.D2, V27.D2, o2.D2   \
	VZIP2 o3.D2, V27.D2, o3.D2

#define STORE4X4(x, ptr) \
	VMOV x.S[0], R10 \
	MOVW R10, (ptr)  \
	ADD  R1, ptr     \
	VMOV x.S[1], R10 \
	MOVW R10, (ptr)  \
	ADD  R1, ptr     \
	VMOV x.S[2], R10 \
	MOVW R10, (ptr)  \
	ADD  R1, ptr     \
	VMOV x.S[3], R10 \
	MOVW R10, (ptr)  \
	ADD  R1, ptr

#define STORE16X4(i0, i1, i2, i3, p0, p8) \
	VZIP1 i1.B16, i0.B16, V27.B16 \
	VZIP2 i1.B16, i0.B16, V28.B16 \
	VZIP1 i3.B16, i2.B16, V29.B16 \
	VZIP2 i3.B16, i2.B16, V30.B16 \
	VZIP1 V29.H8, V27.H8, V31.H8  \
	VZIP2 V29.H8, V27.H8, V27.H8  \
	VZIP1 V30.H8, V28.H8, V29.H8  \
	VZIP2 V30.H8, V28.H8, V28.H8  \
	STORE4X4(V31, p0)             \
	STORE4X4(V27, p0)             \
	STORE4X4(V29, p8)             \
	STORE4X4(V28, p8)

// func hFilter16NEON(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter16NEON(SB), NOSPLIT, $0-40
	MOVD p+0(FP), R0
	MOVD stride+8(FP), R1
	CONSTANTS(limit+16(FP), ilevel+24(FP), hevThresh+32(FP))

	LSL  $3, R1, R2
	SUB  $4, R0, R3
	ADD  R2, R3, R8
	LOAD16X4(R3, R8, V0, V1, V2, V3)

	MOVD R0, R3
	ADD  R2, R3, R8
	LOAD16X4(R3, R8, V4, V5, V6, V7)

	MASKS
	DOFILTER6

	SUB  $4, R0, R3
	ADD  R2, R3, R8
	STORE16X4(V0, V1, V2, V3, R3, R8)

	MOVD R0, R3
	ADD  R2, R3, R8
	STORE16X4(V4, V5, V6, V7, R3, R8)
	RET

// func hFilter16iNEON(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter16iNEON(SB), NOSPLIT, $0-40
	MOVD p+0(FP), R0
	MOVD stride+8(FP), R1
	CONSTANTS(limit+16(FP), ilevel+24(FP), hevThresh+32(FP))

	MOVD $3, R7
	LSL  $3, R1, R2

	MOVD R0, R3
	ADD  R2, R3, R8
	LOAD16X4(R3, R8, V0, V1, V2, V3)

loop16ih:
	ADD  $4, R0
	MOVD R0, R3
	ADD  R2, R3, R8
	LOAD16X4(R3, R8, V4, V5, V6, V7)

	MASKS
	DOFILTER4

	SUB  $2, R0, R3
	ADD  R2, R3, R8
	STORE16X4(V2, V3, V4, V5, R3, R8)

	VORR V4.B16, V4.B16, V0.B16
	VORR V5.B16, V5.B16, V1.B16
	VORR V6.B16, V6.B16, V2.B16
	VORR V7.B16, V7.B16, V3.B16

	SUB  $1, R7
	CBNZ R7, loop16ih
	RET

// func hFilter8NEON(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter8NEON(SB), NOSPLIT, $0-48
	MOVD u+0(FP), R0
	MOVD v+8(FP), R9
	MOVD stride+16(FP), R1
	CONSTANTS(limit+24(FP), ilevel+32(FP), hevThresh+40(FP))

	SUB  $4, R0, R3
	SUB  $4, R9, R8
	LOAD16X4(R3, R8, V0, V1, V2, V3)

	MOVD R0, R3
	MOVD R9, R8
	LOAD16X4(R3, R8, V4, V5, V6, V7)

	MASKS
	DOFILTER6

	SUB  $4, R0, R3
	SUB  $4, R9, R8
	STORE16X4(V0, V1, V2, V3, R3, R8)

	MOVD R0, R3
	MOVD R9, R8
	STORE16X4(V4, V5, V6, V7, R3, R8)
	RET

// func hFilter8iNEON(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter8iNEON(SB), NOSPLIT, $0-48
	MOVD u+0(FP), R0
	MOVD v+8(FP), R9
	MOVD stride+16(FP), R1
	CONSTANTS(limit+24(FP), ilevel+32(FP), hevThresh+40(FP))

	MOVD R0, R3
	MOVD R9, R8
	LOAD16X4(R3, R8, V0, V1, V2, V3)

	ADD  $4, R0, R3
	ADD  $4, R9, R8
	LOAD16X4(R3, R8, V4, V5, V6, V7)

	MASKS
	DOFILTER4

	ADD  $2, R0, R3
	ADD  $2, R9, R8
	STORE16X4(V2, V3, V4, V5, R3, R8)
	RET
