//go:build arm64 && !noasm

#include "textflag.h"

// func matchLengthNEON(a, b *uint32, limit int) int
TEXT ·matchLengthNEON(SB), NOSPLIT, $0-32
	MOVD a+0(FP), R0
	MOVD b+8(FP), R1
	MOVD limit+16(FP), R2

	MOVD $0, R3

loop:
	SUB  R3, R2, R4
	CMP  $4, R4
	BLT  tail

	VLD1  (R0), [V0.S4]
	VLD1  (R1), [V1.S4]
	VCMEQ V1.S4, V0.S4, V2.S4
	VMOV  V2.D[0], R5
	VMOV  V2.D[1], R6
	AND   R6, R5, R7
	CMN   $1, R7
	BNE   tail

	ADD $16, R0
	ADD $16, R1
	ADD $4, R3
	B   loop

tail:
	CMP  R2, R3
	BGE  done

	MOVWU (R0), R5
	MOVWU (R1), R6
	CMP   R6, R5
	BNE   done

	ADD $4, R0
	ADD $4, R1
	ADD $1, R3
	B   tail

done:
	MOVD R3, ret+24(FP)
	RET
