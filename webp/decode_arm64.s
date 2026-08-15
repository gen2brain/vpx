//go:build arm64 && !noasm

#include "textflag.h"

DATA argbShuf<>+0(SB)/8, $0x0704050603000102
DATA argbShuf<>+8(SB)/8, $0x0f0c0d0e0b08090a
GLOBL argbShuf<>(SB), RODATA|NOPTR, $16

// func argbToRGBANEON(dst *byte, px *uint32, n int)
TEXT ·argbToRGBANEON(SB), NOSPLIT, $0-24
	MOVD dst+0(FP), R0
	MOVD px+8(FP), R1
	MOVD n+16(FP), R2

	MOVD $argbShuf<>(SB), R3
	VLD1 (R3), [V7.B16]

	CMP $8, R2
	BLT half

loop8:
	VLD1.P 32(R1), [V0.B16, V1.B16]
	VTBL   V7.B16, [V0.B16], V2.B16
	VTBL   V7.B16, [V1.B16], V3.B16
	VST1.P [V2.B16, V3.B16], 32(R0)

	SUB $8, R2
	CMP $8, R2
	BGE loop8

half:
	CMP $4, R2
	BLT tail

	VLD1.P 16(R1), [V0.B16]
	VTBL   V7.B16, [V0.B16], V2.B16
	VST1.P [V2.B16], 16(R0)

	SUB $4, R2

tail:
	CBZ R2, done

one:
	MOVWU (R1), R4
	REVW  R4, R4
	RORW  $8, R4, R4
	MOVW  R4, (R0)

	ADD  $4, R1
	ADD  $4, R0
	SUB  $1, R2
	CBNZ R2, one

done:
	RET
