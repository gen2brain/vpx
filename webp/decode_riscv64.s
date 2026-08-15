//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

// func argbToRGBARVV(dst *byte, px *uint32, n int)
TEXT ·argbToRGBARVV(SB), NOSPLIT, $0-24
	MOV dst+0(FP), X10
	MOV px+8(FP), X11
	MOV n+16(FP), X12

	MOV $0x000000ff, X14
	MOV $0x00ff0000, X15
	MOV $0xff00ff00, X16

loop:
	BEQ X12, X0, done

	VSETVLI X12, E32, M1, TA, MA, X13

	VLE32V (X11), V1
	VSRLVI $16, V1, V2
	VSLLVI $16, V1, V3
	VANDVX X14, V2, V2
	VANDVX X15, V3, V3
	VANDVX X16, V1, V1
	VORVV  V2, V1, V1
	VORVV  V3, V1, V1
	VSE32V V1, (X10)

	SLLI $2, X13, X17
	ADD  X17, X10
	ADD  X17, X11
	SUB  X13, X12
	JMP  loop

done:
	RET
