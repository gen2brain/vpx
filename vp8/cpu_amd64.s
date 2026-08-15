//go:build amd64 && !noasm

#include "textflag.h"

// func cpuidAVX2() bool
TEXT ·cpuidAVX2(SB), NOSPLIT, $0-1
	MOVL $0, AX
	CPUID
	CMPL AX, $7
	JL   nope

	MOVL $1, AX
	MOVL $0, CX
	CPUID
	BTL  $27, CX
	JNC  nope

	MOVL $0, CX
	XGETBV
	ANDL $6, AX
	CMPL AX, $6
	JNE  nope

	MOVL $7, AX
	MOVL $0, CX
	CPUID
	BTL  $5, BX
	JNC  nope

	MOVB $1, ret+0(FP)
	RET

nope:
	MOVB $0, ret+0(FP)
	RET

// func cpuidAVX512() bool
TEXT ·cpuidAVX512(SB), NOSPLIT, $0-1
	MOVL $0, AX
	CPUID
	CMPL AX, $7
	JL   no512

	MOVL $1, AX
	MOVL $0, CX
	CPUID
	BTL  $27, CX
	JNC  no512

	MOVL $0, CX
	XGETBV
	ANDL $0xe6, AX
	CMPL AX, $0xe6
	JNE  no512

	MOVL $7, AX
	MOVL $0, CX
	CPUID
	BTL  $16, BX
	JNC  no512
	BTL  $30, BX
	JNC  no512
	BTL  $31, BX
	JNC  no512

	MOVB $1, ret+0(FP)
	RET

no512:
	MOVB $0, ret+0(FP)
	RET
