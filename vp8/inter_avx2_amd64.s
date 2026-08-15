//go:build amd64 && !noasm

#include "textflag.h"

DATA krnd2<>+0(SB)/8, $0x0040004000400040
DATA krnd2<>+8(SB)/8, $0x0040004000400040
DATA krnd2<>+16(SB)/8, $0x0040004000400040
DATA krnd2<>+24(SB)/8, $0x0040004000400040
GLOBL krnd2<>(SB), RODATA|NOPTR, $32

#define LOADCOEF2 \
	MOVQ f+48(FP), R8      \
	VPBROADCASTW 0(R8), Y10  \
	VPBROADCASTW 2(R8), Y11  \
	VPBROADCASTW 4(R8), Y12  \
	VPBROADCASTW 6(R8), Y13  \
	VPBROADCASTW 8(R8), Y14  \
	VPBROADCASTW 10(R8), Y15 \
	VMOVDQU krnd2<>(SB), Y7

#define TAP2(mem, coef) \
	VPMOVZXBW mem, Y4     \
	VPMULLW   coef, Y4, Y4 \
	VPADDSW   Y4, Y3, Y3

#define FINISH2 \
	VPADDSW    Y7, Y3, Y3   \
	VPSRAW     $7, Y3, Y3   \
	VPACKUSWB  Y3, Y3, Y3   \
	VPERMQ     $0xD8, Y3, Y3

// func sixtapVAVX2(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)
TEXT ·sixtapVAVX2(SB), NOSPLIT, $0-56
	LOADCOEF2

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

vrow2:
	LEAQ (SI), R11

	VPMOVZXBW (R11), Y3
	VPMULLW   Y10, Y3, Y3

	TAP2((R11)(BX*1), Y11)
	TAP2((R11)(BX*4), Y14)
	TAP2((R11)(R15*1), Y15)
	TAP2((R11)(BX*2), Y12)
	TAP2((R11)(R14*1), Y13)

	FINISH2

	CMPQ CX, $16
	JE   vstore16

	CMPQ CX, $8
	JE   vstore8

	MOVL X3, (DI)
	JMP  vnext2

vstore8:
	MOVQ X3, (DI)
	JMP  vnext2

vstore16:
	VMOVDQU X3, (DI)

vnext2:
	ADDQ BX, SI
	ADDQ DX, DI
	DECQ R8
	JNZ  vrow2

	VZEROUPPER
	RET

// func sixtapHAVX2(dst *byte, dStride int, src *byte, sStride, w, h int, f *int16)
TEXT ·sixtapHAVX2(SB), NOSPLIT, $0-56
	LOADCOEF2

	MOVQ dst+0(FP), DI
	MOVQ dStride+8(FP), DX
	MOVQ src+16(FP), SI
	MOVQ sStride+24(FP), BX
	MOVQ w+32(FP), CX
	MOVQ h+40(FP), R8

hrow2:
	VPMOVZXBW -2(SI), Y3
	VPMULLW   Y10, Y3, Y3

	TAP2(-1(SI), Y11)
	TAP2(2(SI), Y14)
	TAP2(3(SI), Y15)
	TAP2(0(SI), Y12)
	TAP2(1(SI), Y13)

	FINISH2

	CMPQ CX, $16
	JE   hstore16

	CMPQ CX, $8
	JE   hstore8

	MOVL X3, (DI)
	JMP  hnext2

hstore8:
	MOVQ X3, (DI)
	JMP  hnext2

hstore16:
	VMOVDQU X3, (DI)

hnext2:
	ADDQ BX, SI
	ADDQ DX, DI
	DECQ R8
	JNZ  hrow2

	VZEROUPPER
	RET
