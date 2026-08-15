//go:build amd64 && !noasm

#include "textflag.h"

#define BCASTB(mem, x) \
	MOVBLZX   mem, AX \
	MOVQ      AX, x   \
	PUNPCKLWL x, x    \
	PSHUFD    $0, x, x

// func trueMotionSSE(b *byte, stride, size int)
TEXT ·trueMotionSSE(SB), NOSPLIT, $0-24
	MOVQ b+0(FP), SI
	MOVQ stride+8(FP), BX
	MOVQ size+16(FP), CX
	MOVQ CX, R8

	MOVQ SI, DI
	SUBQ BX, DI

	BCASTB(-1(DI), X2)

	PXOR X7, X7

	CMPQ R8, $16
	JE   top16

	MOVQ      (DI), X0
	PUNPCKLBW X7, X0
	JMP       rows

top16:
	MOVOU     (DI), X0
	MOVOU     X0, X1
	PUNPCKLBW X7, X0
	PUNPCKHBW X7, X1

rows:
	BCASTB(-1(SI), X3)

	MOVOU X3, X4
	PADDW X0, X3
	PSUBW X2, X3

	CMPQ R8, $16
	JE   row16

	PACKUSWB X3, X3

	CMPQ R8, $4
	JE   store4

	MOVQ X3, (SI)
	JMP  next

store4:
	MOVL X3, (SI)
	JMP  next

row16:
	PADDW    X1, X4
	PSUBW    X2, X4
	PACKUSWB X4, X3
	MOVOU    X3, (SI)

next:
	ADDQ BX, SI
	DECQ CX
	JNZ  rows
	RET
