//go:build amd64 && !noasm

#include "textflag.h"

DATA argbR<>+0(SB)/8, $0x000000ff000000ff
DATA argbR<>+8(SB)/8, $0x000000ff000000ff
GLOBL argbR<>(SB), RODATA|NOPTR, $16

DATA argbB<>+0(SB)/8, $0x00ff000000ff0000
DATA argbB<>+8(SB)/8, $0x00ff000000ff0000
GLOBL argbB<>(SB), RODATA|NOPTR, $16

DATA argbGA<>+0(SB)/8, $0xff00ff00ff00ff00
DATA argbGA<>+8(SB)/8, $0xff00ff00ff00ff00
GLOBL argbGA<>(SB), RODATA|NOPTR, $16

DATA argbShuf<>+0(SB)/8, $0x0704050603000102
DATA argbShuf<>+8(SB)/8, $0x0f0c0d0e0b08090a
DATA argbShuf<>+16(SB)/8, $0x0704050603000102
DATA argbShuf<>+24(SB)/8, $0x0f0c0d0e0b08090a
GLOBL argbShuf<>(SB), RODATA|NOPTR, $32

// func argbToRGBASSE(dst *byte, px *uint32, n int)
TEXT ·argbToRGBASSE(SB), NOSPLIT, $0-24
	MOVQ dst+0(FP), DI
	MOVQ px+8(FP), SI
	MOVQ n+16(FP), CX

	MOVOU argbR<>(SB), X5
	MOVOU argbB<>(SB), X6
	MOVOU argbGA<>(SB), X7

	CMPQ CX, $4
	JLT  tail

loop:
	MOVOU (SI), X0
	MOVO  X0, X1
	MOVO  X0, X2
	PSRLL $16, X1
	PSLLL $16, X2
	PAND  X5, X1
	PAND  X6, X2
	PAND  X7, X0
	POR   X1, X0
	POR   X2, X0
	MOVOU X0, (DI)

	ADDQ $16, SI
	ADDQ $16, DI
	SUBQ $4, CX
	CMPQ CX, $4
	JGE  loop

tail:
	TESTQ CX, CX
	JLE   done

one:
	MOVL   (SI), AX
	BSWAPL AX
	RORL   $8, AX
	MOVL   AX, (DI)

	ADDQ $4, SI
	ADDQ $4, DI
	DECQ CX
	JNZ  one

done:
	RET

// func argbToRGBAAVX2(dst *byte, px *uint32, n int)
TEXT ·argbToRGBAAVX2(SB), NOSPLIT, $0-24
	MOVQ dst+0(FP), DI
	MOVQ px+8(FP), SI
	MOVQ n+16(FP), CX

	VMOVDQU argbShuf<>(SB), Y7

	CMPQ CX, $8
	JLT  half

loop8:
	VMOVDQU (SI), Y0
	VPSHUFB Y7, Y0, Y0
	VMOVDQU Y0, (DI)

	ADDQ $32, SI
	ADDQ $32, DI
	SUBQ $8, CX
	CMPQ CX, $8
	JGE  loop8

half:
	CMPQ CX, $4
	JLT  tail

	VMOVDQU (SI), X0
	VPSHUFB X7, X0, X0
	VMOVDQU X0, (DI)

	ADDQ $16, SI
	ADDQ $16, DI
	SUBQ $4, CX

tail:
	VZEROUPPER
	TESTQ CX, CX
	JLE   done

one:
	MOVL   (SI), AX
	BSWAPL AX
	RORL   $8, AX
	MOVL   AX, (DI)

	ADDQ $4, SI
	ADDQ $4, DI
	DECQ CX
	JNZ  one

done:
	RET
