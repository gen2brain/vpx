//go:build amd64 && !noasm

#include "textflag.h"

// func matchLengthSSE(a, b *uint32, limit int) int
TEXT ·matchLengthSSE(SB), NOSPLIT, $0-32
	MOVQ a+0(FP), SI
	MOVQ b+8(FP), DI
	MOVQ limit+16(FP), CX

	XORQ DX, DX

loop:
	MOVQ  CX, R8
	SUBQ  DX, R8
	CMPQ  R8, $4
	JLT   tail

	MOVOU    (SI)(DX*4), X0
	MOVOU    (DI)(DX*4), X1
	PCMPEQL  X1, X0
	PMOVMSKB X0, AX
	CMPL     AX, $0xffff
	JNE      diff

	ADDQ $4, DX
	JMP  loop

diff:
	NOTL AX
	BSFL AX, AX
	SHRL $2, AX
	ADDQ AX, DX
	MOVQ DX, ret+24(FP)
	RET

tail:
	CMPQ DX, CX
	JGE  done

	MOVL (SI)(DX*4), AX
	CMPL AX, (DI)(DX*4)
	JNE  done

	INCQ DX
	JMP  tail

done:
	MOVQ DX, ret+24(FP)
	RET

// func matchLengthAVX2(a, b *uint32, limit int) int
TEXT ·matchLengthAVX2(SB), NOSPLIT, $0-32
	MOVQ a+0(FP), SI
	MOVQ b+8(FP), DI
	MOVQ limit+16(FP), CX

	XORQ DX, DX

loop8:
	MOVQ CX, R8
	SUBQ DX, R8
	CMPQ R8, $8
	JLT  tail8

	VMOVDQU   (SI)(DX*4), Y0
	VMOVDQU   (DI)(DX*4), Y1
	VPCMPEQD  Y1, Y0, Y0
	VPMOVMSKB Y0, AX
	CMPL      AX, $-1
	JNE       diff8

	ADDQ $8, DX
	JMP  loop8

diff8:
	NOTL AX
	BSFL AX, AX
	SHRL $2, AX
	ADDQ AX, DX
	VZEROUPPER
	MOVQ DX, ret+24(FP)
	RET

tail8:
	VZEROUPPER

tailloop:
	CMPQ DX, CX
	JGE  done8

	MOVL (SI)(DX*4), AX
	CMPL AX, (DI)(DX*4)
	JNE  done8

	INCQ DX
	JMP  tailloop

done8:
	MOVQ DX, ret+24(FP)
	RET
