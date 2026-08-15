//go:build amd64 && !noasm

#include "textflag.h"

DATA a80<>+0(SB)/8, $0x8080808080808080
DATA a80<>+8(SB)/8, $0x8080808080808080
GLOBL a80<>(SB), RODATA|NOPTR, $16

DATA afe<>+0(SB)/8, $0xfefefefefefefefe
DATA afe<>+8(SB)/8, $0xfefefefefefefefe
GLOBL afe<>(SB), RODATA|NOPTR, $16

DATA a3<>+0(SB)/8, $0x0303030303030303
DATA a3<>+8(SB)/8, $0x0303030303030303
GLOBL a3<>(SB), RODATA|NOPTR, $16

DATA a4<>+0(SB)/8, $0x0404040404040404
DATA a4<>+8(SB)/8, $0x0404040404040404
GLOBL a4<>(SB), RODATA|NOPTR, $16

DATA a64<>+0(SB)/8, $0x4040404040404040
DATA a64<>+8(SB)/8, $0x4040404040404040
GLOBL a64<>(SB), RODATA|NOPTR, $16

DATA a9<>+0(SB)/8, $0x0900090009000900
DATA a9<>+8(SB)/8, $0x0900090009000900
GLOBL a9<>(SB), RODATA|NOPTR, $16

DATA a63<>+0(SB)/8, $0x003f003f003f003f
DATA a63<>+8(SB)/8, $0x003f003f003f003f
GLOBL a63<>(SB), RODATA|NOPTR, $16

// The three thresholds are broadcast once into the frame, so every later use is
// a memory operand instead of the five instruction sequence SSE2 needs.
#define SETUP \
	VMOVD        AX, X0   \
	VPBROADCASTB X0, X0   \
	VMOVDQU      X0, 0(SP) \
	VMOVD        R11, X0  \
	VPBROADCASTB X0, X0   \
	VMOVDQU      X0, 16(SP) \
	VMOVD        R12, X0  \
	VPBROADCASTB X0, X0   \
	VMOVDQU      X0, 32(SP)

#define ABSDU(a, b, t, d) \
	VPSUBUSB b, a, t \
	VPSUBUSB a, b, d \
	VPMAXUB  t, d, d

#define MAXDIFF1 \
	ABSDU(X2, X3, X10, X8)  \
	ABSDU(X0, X1, X10, X11) \
	VPMAXUB X11, X8, X8     \
	ABSDU(X1, X2, X10, X11) \
	VPMAXUB X11, X8, X8

#define MAXDIFF2 \
	ABSDU(X5, X4, X10, X11) \
	VPMAXUB X11, X8, X8     \
	ABSDU(X7, X6, X10, X11) \
	VPMAXUB X11, X8, X8     \
	ABSDU(X6, X5, X10, X11) \
	VPMAXUB X11, X8, X8

#define COMPLEXMASK \
	VPSUBUSB 16(SP), X8, X8   \
	VPXOR    X11, X11, X11    \
	VPCMPEQB X11, X8, X8      \
	ABSDU(X2, X5, X10, X12)   \
	VPAND    afe<>(SB), X12, X12 \
	VPSRLW   $1, X12, X12     \
	ABSDU(X3, X4, X10, X13)   \
	VPADDUSB X13, X13, X13    \
	VPADDUSB X12, X13, X13    \
	VPSUBUSB 0(SP), X13, X13  \
	VPCMPEQB X11, X13, X13    \
	VPAND    X13, X8, X8

#define NOTHEV \
	ABSDU(X2, X3, X10, X9)  \
	ABSDU(X5, X4, X10, X11) \
	VPMAXUB  X11, X9, X9    \
	VPSUBUSB 32(SP), X9, X9 \
	VPXOR    X11, X11, X11  \
	VPCMPEQB X11, X9, X9

// SSHIFT3 is the signed >>3 of a byte lane. VPMOVSXBW reaches it in one
// instruction where SSE2 unpacks against zero and shifts by eleven.
#define SSHIFT3(x, t0, t1) \
	VPMOVSXBW x, t0     \
	VPSRLDQ   $8, x, t1 \
	VPMOVSXBW t1, t1    \
	VPSRAW    $3, t0, t0 \
	VPSRAW    $3, t1, t1 \
	VPACKSSWB t1, t0, x

#define SIMPLEFILTER(f) \
	VPADDSB a3<>(SB), f, X14 \
	VPADDSB a4<>(SB), f, X15 \
	SSHIFT3(X14, X10, X11)   \
	SSHIFT3(X15, X10, X11)   \
	VPSUBSB X15, X4, X4      \
	VPADDSB X14, X3, X3

#define UPDATE2(pi, qi) \
	VPSRAW    $7, X11, X10 \
	VPSRAW    $7, X12, X13 \
	VPACKSSWB X13, X10, X10 \
	VPADDSB   X10, pi, pi  \
	VPSUBSB   X10, qi, qi  \
	VPXOR     a80<>(SB), pi, pi \
	VPXOR     a80<>(SB), qi, qi

#define DOFILTER6 \
	NOTHEV                       \
	VPXOR      a80<>(SB), X1, X1 \
	VPXOR      a80<>(SB), X2, X2 \
	VPXOR      a80<>(SB), X3, X3 \
	VPXOR      a80<>(SB), X4, X4 \
	VPXOR      a80<>(SB), X5, X5 \
	VPXOR      a80<>(SB), X6, X6 \
	VPSUBSB    X5, X2, X12       \
	VPSUBSB    X3, X4, X13       \
	VPADDSB    X13, X12, X12     \
	VPADDSB    X13, X12, X12     \
	VPADDSB    X13, X12, X12     \
	VPANDN     X8, X9, X13       \
	VPAND      X12, X13, X13     \
	SIMPLEFILTER(X13)            \
	VPAND      X8, X9, X13       \
	VPAND      X12, X13, X13     \
	VPXOR      X11, X11, X11     \
	VPUNPCKLBW X13, X11, X14     \
	VPUNPCKHBW X13, X11, X15     \
	VPMULHW    a9<>(SB), X14, X14 \
	VPMULHW    a9<>(SB), X15, X15 \
	VPADDW     a63<>(SB), X14, X11 \
	VPADDW     a63<>(SB), X15, X12 \
	UPDATE2(X1, X6)              \
	VPADDW     X14, X11, X11     \
	VPADDW     X15, X12, X12     \
	UPDATE2(X2, X5)              \
	VPADDW     X14, X11, X11     \
	VPADDW     X15, X12, X12     \
	UPDATE2(X3, X4)

#define DOFILTER4 \
	NOTHEV                       \
	VPXOR   a80<>(SB), X2, X2    \
	VPXOR   a80<>(SB), X3, X3    \
	VPXOR   a80<>(SB), X4, X4    \
	VPXOR   a80<>(SB), X5, X5    \
	VPSUBSB X5, X2, X12          \
	VPANDN  X12, X9, X13         \
	VPSUBSB X3, X4, X12          \
	VPADDSB X12, X13, X13        \
	VPADDSB X12, X13, X13        \
	VPADDSB X12, X13, X13        \
	VPAND   X8, X13, X13         \
	VPADDSB a3<>(SB), X13, X14   \
	VPADDSB a4<>(SB), X13, X15   \
	SSHIFT3(X14, X10, X11)       \
	SSHIFT3(X15, X10, X11)       \
	VPADDSB X14, X3, X3          \
	VPSUBSB X15, X4, X4          \
	VPXOR   a80<>(SB), X3, X3    \
	VPXOR   a80<>(SB), X4, X4    \
	VPADDB  a80<>(SB), X15, X14  \
	VPXOR   X11, X11, X11        \
	VPAVGB  X11, X14, X14        \
	VPSUBB  a64<>(SB), X14, X14  \
	VPAND   X9, X14, X14         \
	VPSUBSB X14, X5, X5          \
	VPADDSB X14, X2, X2          \
	VPXOR   a80<>(SB), X2, X2    \
	VPXOR   a80<>(SB), X5, X5

// VPINSRD takes its dword straight from memory, so a column of eight rows



// func vFilter16AVX2(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter16AVX2(SB), NOSPLIT, $48-40
	MOVQ p+0(FP), SI
	MOVQ stride+8(FP), BX
	MOVQ limit+16(FP), AX
	MOVQ ilevel+24(FP), R11
	MOVQ hevThresh+32(FP), R12
	SETUP

	MOVQ BX, DX
	SHLQ $2, DX
	SUBQ DX, SI

	VMOVDQU (SI), X0
	ADDQ    BX, SI
	VMOVDQU (SI), X1
	ADDQ    BX, SI
	VMOVDQU (SI), X2
	ADDQ    BX, SI
	VMOVDQU (SI), X3
	ADDQ    BX, SI
	VMOVDQU (SI), X4
	ADDQ    BX, SI
	VMOVDQU (SI), X5
	ADDQ    BX, SI
	VMOVDQU (SI), X6
	ADDQ    BX, SI
	VMOVDQU (SI), X7

	MAXDIFF1
	MAXDIFF2
	COMPLEXMASK
	DOFILTER6

	MOVQ p+0(FP), DI
	MOVQ BX, DX
	ADDQ BX, DX
	ADDQ BX, DX
	SUBQ DX, DI

	VMOVDQU X1, (DI)
	ADDQ    BX, DI
	VMOVDQU X2, (DI)
	ADDQ    BX, DI
	VMOVDQU X3, (DI)
	ADDQ    BX, DI
	VMOVDQU X4, (DI)
	ADDQ    BX, DI
	VMOVDQU X5, (DI)
	ADDQ    BX, DI
	VMOVDQU X6, (DI)
	VZEROUPPER
	RET

// func vFilter16iAVX2(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter16iAVX2(SB), NOSPLIT, $48-40
	MOVQ p+0(FP), SI
	MOVQ stride+8(FP), BX
	MOVQ limit+16(FP), AX
	MOVQ ilevel+24(FP), R11
	MOVQ hevThresh+32(FP), R12
	SETUP
	MOVQ $3, CX

	VMOVDQU (SI), X0
	ADDQ    BX, SI
	VMOVDQU (SI), X1
	ADDQ    BX, SI
	VMOVDQU (SI), X2
	ADDQ    BX, SI
	VMOVDQU (SI), X3
	ADDQ    BX, SI

loop16i:
	VMOVDQU (SI), X4
	ADDQ    BX, SI
	VMOVDQU (SI), X5
	ADDQ    BX, SI
	VMOVDQU (SI), X6
	ADDQ    BX, SI
	VMOVDQU (SI), X7
	ADDQ    BX, SI

	MAXDIFF1
	MAXDIFF2
	COMPLEXMASK
	DOFILTER4

	MOVQ SI, DI
	MOVQ BX, DX
	SHLQ $2, DX
	SUBQ DX, DI
	MOVQ BX, DX
	ADDQ BX, DX
	SUBQ DX, DI

	VMOVDQU X2, (DI)
	ADDQ    BX, DI
	VMOVDQU X3, (DI)
	ADDQ    BX, DI
	VMOVDQU X4, (DI)
	ADDQ    BX, DI
	VMOVDQU X5, (DI)

	VMOVDQA X4, X0
	VMOVDQA X5, X1
	VMOVDQA X6, X2
	VMOVDQA X7, X3

	DECQ CX
	JNZ  loop16i
	VZEROUPPER
	RET

#define LOADUV(x) \
	VMOVQ       (SI), x  \
	VMOVQ       (R8), X9 \
	VPUNPCKLQDQ X9, x, x \
	ADDQ        BX, SI   \
	ADDQ        BX, R8

#define STOREUV(x) \
	VMOVQ   x, (SI)  \
	VPSRLDQ $8, x, x \
	VMOVQ   x, (R8)  \
	ADDQ    BX, SI   \
	ADDQ    BX, R8

// func vFilter8AVX2(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter8AVX2(SB), NOSPLIT, $48-48
	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ stride+16(FP), BX
	MOVQ limit+24(FP), AX
	MOVQ ilevel+32(FP), R11
	MOVQ hevThresh+40(FP), R12
	SETUP

	MOVQ BX, DX
	SHLQ $2, DX
	SUBQ DX, SI
	SUBQ DX, R8

	LOADUV(X0)
	LOADUV(X1)
	LOADUV(X2)
	LOADUV(X3)
	LOADUV(X4)
	LOADUV(X5)
	LOADUV(X6)
	LOADUV(X7)

	MAXDIFF1
	MAXDIFF2
	COMPLEXMASK
	DOFILTER6

	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ BX, DX
	ADDQ BX, DX
	ADDQ BX, DX
	SUBQ DX, SI
	SUBQ DX, R8

	STOREUV(X1)
	STOREUV(X2)
	STOREUV(X3)
	STOREUV(X4)
	STOREUV(X5)
	STOREUV(X6)
	VZEROUPPER
	RET

// func vFilter8iAVX2(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter8iAVX2(SB), NOSPLIT, $48-48
	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ stride+16(FP), BX
	MOVQ limit+24(FP), AX
	MOVQ ilevel+32(FP), R11
	MOVQ hevThresh+40(FP), R12
	SETUP

	LOADUV(X0)
	LOADUV(X1)
	LOADUV(X2)
	LOADUV(X3)
	LOADUV(X4)
	LOADUV(X5)
	LOADUV(X6)
	LOADUV(X7)

	MAXDIFF1
	MAXDIFF2
	COMPLEXMASK
	DOFILTER4

	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ BX, DX
	ADDQ BX, DX
	ADDQ DX, SI
	ADDQ DX, R8

	STOREUV(X2)
	STOREUV(X3)
	STOREUV(X4)
	STOREUV(X5)
	VZEROUPPER
	RET



