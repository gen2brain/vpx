//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

#define PASS \
	VADDVV  V2, V0, V4  \
	VSUBVV  V2, V0, V5  \
	VMULHVX X14, V1, V6 \
	VMULHVX X13, V3, V7 \
	VSUBVV  V7, V6, V6  \
	VSUBVV  V3, V1, V7  \
	VADDVV  V7, V6, V6  \
	VMULHVX X13, V1, V8 \
	VMULHVX X14, V3, V9 \
	VADDVV  V9, V8, V8  \
	VADDVV  V3, V1, V9  \
	VADDVV  V9, V8, V8  \
	VADDVV  V8, V4, V10 \
	VADDVV  V6, V5, V11 \
	VSUBVV  V6, V5, V2  \
	VSUBVV  V8, V4, V3  \
	VMVVV   V10, V0     \
	VMVVV   V11, V1

// func transformRVV(in *int16, dst *byte)
TEXT ·transformRVV(SB), NOSPLIT, $32-16
	MOV in+0(FP), X10
	MOV dst+8(FP), X11
	MOV $tmp-32(SP), X12

	MOV $20091, X13
	MOV $-30068, X14

	MOV $4, X5
	VSETVLI X5, E16, M1, TA, MA, X6

	VLE16V (X10), V0
	ADD    $8, X10, X7
	VLE16V (X7), V1
	ADD    $16, X10, X7
	VLE16V (X7), V2
	ADD    $24, X10, X7
	VLE16V (X7), V3

	PASS

	VSE16V V0, (X12)
	ADD    $8, X12, X7
	VSE16V V1, (X7)
	ADD    $16, X12, X7
	VSE16V V2, (X7)
	ADD    $24, X12, X7
	VSE16V V3, (X7)

	MOV     $8, X8
	VLSE16V (X12), X8, V0
	ADD     $2, X12, X7
	VLSE16V (X7), X8, V1
	ADD     $4, X12, X7
	VLSE16V (X7), X8, V2
	ADD     $6, X12, X7
	VLSE16V (X7), X8, V3

	VADDVI $4, V0, V0

	PASS

	VSRAVI $3, V0, V0
	VSRAVI $3, V1, V1
	VSRAVI $3, V2, V2
	VSRAVI $3, V3, V3

	MOV $32, X8
	MOV $255, X9

	VSETVLI X5, E8, M1, TA, MA, X6
	VLSE8V  (X11), X8, V12
	ADD     $1, X11, X7
	VLSE8V  (X7), X8, V13
	ADD     $2, X11, X7
	VLSE8V  (X7), X8, V14
	ADD     $3, X11, X7
	VLSE8V  (X7), X8, V15

	VSETVLI  X5, E16, M1, TA, MA, X6
	VZEXTVF2 V12, V16
	VZEXTVF2 V13, V18
	VZEXTVF2 V14, V20
	VZEXTVF2 V15, V22
	VADDVV   V0, V16, V16
	VADDVV   V1, V18, V18
	VADDVV   V2, V20, V20
	VADDVV   V3, V22, V22
	VMAXVX   X0, V16, V16
	VMAXVX   X0, V18, V18
	VMAXVX   X0, V20, V20
	VMAXVX   X0, V22, V22
	VMINVX   X9, V16, V16
	VMINVX   X9, V18, V18
	VMINVX   X9, V20, V20
	VMINVX   X9, V22, V22

	VSETVLI X5, E8, M1, TA, MA, X6
	VNSRLWI $0, V16, V12
	VNSRLWI $0, V18, V13
	VNSRLWI $0, V20, V14
	VNSRLWI $0, V22, V15

	VSSE8V V12, X8, (X11)
	ADD    $1, X11, X7
	VSSE8V V13, X8, (X7)
	ADD    $2, X11, X7
	VSSE8V V14, X8, (X7)
	ADD    $3, X11, X7
	VSSE8V V15, X8, (X7)
	RET

// func transformDCRVV(in *int16, dst *byte)
TEXT ·transformDCRVV(SB), NOSPLIT, $0-16
	MOV in+0(FP), X10
	MOV dst+8(FP), X11

	MOVH  (X10), X12
	ADD   $4, X12
	SLL   $48, X12
	SRA   $48, X12
	SRA   $3, X12

	MOV  $4, X5
	VSETVLI X5, E8, M1, TA, MA, X6

	MOV  $255, X9
	BGE  X12, X0, positive

	SUB  X12, X0, X12
	BLT  X9, X12, negclamp
	JMP  negloop

negclamp:
	MOV  X9, X12

negloop:
	MOV  $4, X7

negrow:
	VLE8V    (X11), V1
	VSSUBUVX X12, V1, V1
	VSE8V    V1, (X11)
	ADD      $32, X11
	SUB      $1, X7
	BNE      X7, X0, negrow
	RET

positive:
	BLT  X9, X12, posclamp
	JMP  posloop

posclamp:
	MOV  X9, X12

posloop:
	MOV  $4, X7

posrow:
	VLE8V    (X11), V1
	VSADDUVX X12, V1, V1
	VSE8V    V1, (X11)
	ADD      $32, X11
	SUB      $1, X7
	BNE      X7, X0, posrow
	RET
