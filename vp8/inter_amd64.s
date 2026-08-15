//go:build amd64 && !noasm

#include "textflag.h"

DATA krnd<>+0(SB)/8, $0x0040004000400040
DATA krnd<>+8(SB)/8, $0x0040004000400040
GLOBL krnd<>(SB), RODATA|NOPTR, $16

#define BCASTW(mem, x) \
	MOVWLSX mem, AX \
	MOVQ    AX, x   \
	PUNPCKLWL x, x  \
	PSHUFD  $0, x, x

#define LOADCOEF \
	MOVQ f+48(FP), R8 \
	BCASTW(0(R8), X10) \
	BCASTW(2(R8), X11) \
	BCASTW(4(R8), X12) \
	BCASTW(6(R8), X13) \
	BCASTW(8(R8), X14) \
	BCASTW(10(R8), X15) \
	PXOR  X6, X6       \
	MOVOU krnd<>(SB), X7

#define TAP(mem, coef) \
	MOVQ      mem, X4  \
	PUNPCKLBW X6, X4   \
	PMULLW    coef, X4 \
	PADDSW    X4, X3

// func sixtapVSSE(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)
TEXT ·sixtapVSSE(SB), NOSPLIT, $0-56
	LOADCOEF

	MOVQ dst+0(FP), DI
	MOVQ dStride+8(FP), DX
	MOVQ src+16(FP), SI
	MOVQ sStride+24(FP), BX
	MOVQ w+32(FP), CX
	MOVQ h+40(FP), R8

	MOVQ BX, R14
	ADDQ BX, R14
	ADDQ BX, R14
	MOVQ R14, R15
	ADDQ BX, R15
	ADDQ BX, R15

	MOVQ BX, AX
	ADDQ AX, AX
	SUBQ AX, SI

vrow:
	XORQ R9, R9

vgroup:
	LEAQ (SI)(R9*1), R11

	MOVQ      (R11), X3
	PUNPCKLBW X6, X3
	PMULLW    X10, X3

	TAP((R11)(BX*1), X11)
	TAP((R11)(BX*4), X14)
	TAP((R11)(R15*1), X15)
	TAP((R11)(BX*2), X12)
	TAP((R11)(R14*1), X13)

	PADDSW   X7, X3
	PSRAW    $7, X3
	PACKUSWB X3, X3

	CMPQ CX, $4
	JE   vstore4

	MOVQ X3, (DI)(R9*1)
	JMP  vnext

vstore4:
	MOVL X3, (DI)(R9*1)

vnext:
	ADDQ $8, R9
	CMPQ R9, CX
	JLT  vgroup

	ADDQ BX, SI
	ADDQ DX, DI
	DECQ R8
	JNZ  vrow
	RET

#define HTAP(sh, coef) \
	MOVOU  X0, X4    \
	PSRLDQ $sh, X4   \
	MOVOU  X1, X5    \
	PSLLDQ $(16-sh), X5 \
	POR    X5, X4    \
	PMULLW coef, X4  \
	PADDSW X4, X3

// func sixtapHSSE(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)
TEXT ·sixtapHSSE(SB), NOSPLIT, $0-56
	LOADCOEF

	MOVQ dst+0(FP), DI
	MOVQ dStride+8(FP), DX
	MOVQ src+16(FP), SI
	MOVQ sStride+24(FP), BX
	MOVQ w+32(FP), CX
	MOVQ h+40(FP), R8

hrow:
	XORQ R9, R9

hgroup:
	LEAQ (SI)(R9*1), R11
	MOVOU -2(R11), X0
	MOVOU X0, X1
	PUNPCKLBW X6, X0
	PUNPCKHBW X6, X1

	MOVOU  X0, X3
	PMULLW X10, X3

	HTAP(2, X11)
	HTAP(8, X14)
	HTAP(10, X15)
	HTAP(4, X12)
	HTAP(6, X13)

	PADDSW   X7, X3
	PSRAW    $7, X3
	PACKUSWB X3, X3

	CMPQ CX, $4
	JE   hstore4

	MOVQ X3, (DI)(R9*1)
	JMP  hnext

hstore4:
	MOVL X3, (DI)(R9*1)

hnext:
	ADDQ $8, R9
	CMPQ R9, CX
	JLT  hgroup

	ADDQ BX, SI
	ADDQ DX, DI
	DECQ R8
	JNZ  hrow
	RET
