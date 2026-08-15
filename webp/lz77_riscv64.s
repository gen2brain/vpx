//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

// func matchLengthRVV(a, b *uint32, limit int) int
TEXT ·matchLengthRVV(SB), NOSPLIT, $0-32
	MOV a+0(FP), X10
	MOV b+8(FP), X11
	MOV limit+16(FP), X12

	MOV $0, X13

loop:
	SUB X13, X12, X14
	BEQ X14, X0, done

	VSETVLI X14, E32, M1, TA, MA, X15

	VLE32V  (X10), V1
	VLE32V  (X11), V2
	VMSNEVV V2, V1, V0
	VFIRSTM V0, X16

	BGE X16, X0, found

	SLLI $2, X15, X17
	ADD  X17, X10
	ADD  X17, X11
	ADD  X15, X13
	JMP  loop

found:
	ADD X16, X13

done:
	MOV X13, ret+24(FP)
	RET
