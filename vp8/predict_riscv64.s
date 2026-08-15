//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

// func trueMotionRVV(b *byte, stride, size int)
TEXT ·trueMotionRVV(SB), NOSPLIT, $0-24
	MOV b+0(FP), X10
	MOV stride+8(FP), X11
	MOV size+16(FP), X12

	SUB   X11, X10, X13
	MOVBU -1(X13), X14
	MOV   $255, X15
	MOV   X12, X16

	VSETVLI  X12, E8, M1, TA, MA, X6
	VLE8V    (X13), V1

	VSETVLI  X12, E16, M1, TA, MA, X6
	VZEXTVF2 V1, V2

row:
	MOVBU  -1(X10), X17
	VMVVX  X17, V4
	VADDVV V2, V4, V4
	VSUBVX X14, V4, V4
	VMAXVX X0, V4, V4
	VMINVX X15, V4, V4

	VSETVLI X12, E8, M1, TA, MA, X6
	VNSRLWI $0, V4, V6
	VSE8V   V6, (X10)

	VSETVLI X12, E16, M1, TA, MA, X6

	ADD X11, X10
	SUB $1, X16
	BNE X16, X0, row
	RET
