//go:build amd64 && !noasm

#include "textflag.h"

DATA m80<>+0(SB)/8, $0x8080808080808080
DATA m80<>+8(SB)/8, $0x8080808080808080
GLOBL m80<>(SB), RODATA|NOPTR, $16

DATA mfe<>+0(SB)/8, $0xfefefefefefefefe
DATA mfe<>+8(SB)/8, $0xfefefefefefefefe
GLOBL mfe<>(SB), RODATA|NOPTR, $16

DATA m3<>+0(SB)/8, $0x0303030303030303
DATA m3<>+8(SB)/8, $0x0303030303030303
GLOBL m3<>(SB), RODATA|NOPTR, $16

DATA m4<>+0(SB)/8, $0x0404040404040404
DATA m4<>+8(SB)/8, $0x0404040404040404
GLOBL m4<>(SB), RODATA|NOPTR, $16

DATA m64<>+0(SB)/8, $0x4040404040404040
DATA m64<>+8(SB)/8, $0x4040404040404040
GLOBL m64<>(SB), RODATA|NOPTR, $16

DATA m9<>+0(SB)/8, $0x0900090009000900
DATA m9<>+8(SB)/8, $0x0900090009000900
GLOBL m9<>(SB), RODATA|NOPTR, $16

DATA m63<>+0(SB)/8, $0x003f003f003f003f
DATA m63<>+8(SB)/8, $0x003f003f003f003f
GLOBL m63<>(SB), RODATA|NOPTR, $16

#define SETUP \
	VMOVDQU32    m80<>(SB), X16 \
	VMOVDQU32    m3<>(SB), X17  \
	VMOVDQU32    m4<>(SB), X18  \
	VMOVDQU32    m64<>(SB), X19 \
	VMOVDQU32    mfe<>(SB), X20 \
	VMOVDQU32    m9<>(SB), X25  \
	VMOVDQU32    m63<>(SB), X26 \
	VPXORD       X21, X21, X21  \
	VPBROADCASTB AX, X23        \
	VPBROADCASTB R11, X22       \
	VPBROADCASTB R12, X24

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
	VPCMPUB  $2, X22, X8, K1  \
	ABSDU(X2, X5, X10, X12)   \
	VPANDD   X20, X12, X12    \
	VPSRLW   $1, X12, X12     \
	ABSDU(X3, X4, X10, X13)   \
	VPADDUSB X13, X13, X13    \
	VPADDUSB X12, X13, X13    \
	VPCMPUB  $2, X23, X13, K2 \
	KANDW    K2, K1, K1

#define NOTHEV \
	ABSDU(X2, X3, X10, X9)  \
	ABSDU(X5, X4, X10, X11) \
	VPMAXUB X11, X9, X9     \
	VPCMPUB $2, X24, X9, K2 \
	KNOTW   K2, K3

#define SSHIFT3(x, t0, t1) \
	VPUNPCKLBW x, X21, t0  \
	VPUNPCKHBW x, X21, t1  \
	VPSRAW     $11, t0, t0 \
	VPSRAW     $11, t1, t1 \
	VPACKSSWB  t1, t0, x

#define SIMPLEFILTER(f) \
	VPADDSB X17, f, X14    \
	VPADDSB X18, f, X15    \
	SSHIFT3(X14, X27, X28) \
	SSHIFT3(X15, X27, X28) \
	VPSUBSB X15, X4, X4    \
	VPADDSB X14, X3, X3

#define UPDATE2(pi, qi) \
	VPSRAW    $7, X11, X9  \
	VPSRAW    $7, X12, X13 \
	VPACKSSWB X13, X9, X9  \
	VPADDSB   X9, pi, pi   \
	VPSUBSB   X9, qi, qi   \
	VPXORD    X16, pi, pi  \
	VPXORD    X16, qi, qi

#define DOFILTER6 \
	NOTHEV                     \
	KANDW      K1, K3, K4      \
	KANDW      K1, K2, K5      \
	VPXORD     X16, X1, X1     \
	VPXORD     X16, X2, X2     \
	VPXORD     X16, X3, X3     \
	VPXORD     X16, X4, X4     \
	VPXORD     X16, X5, X5     \
	VPXORD     X16, X6, X6     \
	VPSUBSB    X5, X2, X12     \
	VPSUBSB    X3, X4, X13     \
	VPADDSB    X13, X12, X12   \
	VPADDSB    X13, X12, X12   \
	VPADDSB    X13, X12, X12   \
	VMOVDQU8.Z X12, K4, X13    \
	SIMPLEFILTER(X13)          \
	VMOVDQU8.Z X12, K5, X13    \
	VPUNPCKLBW X13, X21, X14   \
	VPUNPCKHBW X13, X21, X15   \
	VPMULHW    X25, X14, X14   \
	VPMULHW    X25, X15, X15   \
	VPADDW     X26, X14, X11   \
	VPADDW     X26, X15, X12   \
	UPDATE2(X1, X6)            \
	VPADDW     X14, X11, X11   \
	VPADDW     X15, X12, X12   \
	UPDATE2(X2, X5)            \
	VPADDW     X14, X11, X11   \
	VPADDW     X15, X12, X12   \
	UPDATE2(X3, X4)

#define DOFILTER4 \
	NOTHEV                      \
	VPXORD    X16, X2, X2       \
	VPXORD    X16, X3, X3       \
	VPXORD    X16, X4, X4       \
	VPXORD    X16, X5, X5       \
	VPSUBSB.Z X5, X2, K3, X13   \
	VPSUBSB   X3, X4, X12       \
	VPADDSB   X12, X13, X13     \
	VPADDSB   X12, X13, X13     \
	VPADDSB.Z X12, X13, K1, X13 \
	VPADDSB   X17, X13, X14     \
	VPADDSB   X18, X13, X15     \
	SSHIFT3(X14, X27, X28)      \
	SSHIFT3(X15, X27, X28)      \
	VPADDSB   X14, X3, X3       \
	VPSUBSB   X15, X4, X4       \
	VPXORD    X16, X3, X3       \
	VPXORD    X16, X4, X4       \
	VPADDB    X16, X15, X14     \
	VPAVGB    X21, X14, X14     \
	VPSUBB    X19, X14, X14     \
	VPSUBSB   X14, X5, K2, X5   \
	VPADDSB   X14, X2, K2, X2   \
	VPXORD    X16, X2, X2       \
	VPXORD    X16, X5, X5

#define LOAD8X4(base, outp, outq, t0, t1) \
	MOVL        (base), outp     \
	MOVL        (base)(BX*4), t0 \
	VPUNPCKLDQ  t0, outp, outp   \
	LEAQ        (base)(BX*2), R9 \
	MOVL        (R9), t0         \
	MOVL        (R9)(BX*4), t1   \
	VPUNPCKLDQ  t1, t0, t0       \
	VPUNPCKLQDQ t0, outp, outp   \
	LEAQ        (base)(BX*1), R9 \
	MOVL        (R9), outq       \
	MOVL        (R9)(BX*4), t0   \
	VPUNPCKLDQ  t0, outq, outq   \
	LEAQ        (R9)(BX*2), R10  \
	MOVL        (R10), t0        \
	MOVL        (R10)(BX*4), t1  \
	VPUNPCKLDQ  t1, t0, t0       \
	VPUNPCKLQDQ t0, outq, outq   \
	VPUNPCKHBW  outq, outp, t0   \
	VPUNPCKLBW  outq, outp, outp \
	VPUNPCKHWD  t0, outp, t1     \
	VPUNPCKLWD  t0, outp, outp   \
	VPUNPCKHDQ  t1, outp, outq   \
	VPUNPCKLDQ  t1, outp, outp

#define LOAD16X4(r0, r8, o0, o1, o2, o3) \
	LOAD8X4(r0, o0, o2, X10, X11) \
	LOAD8X4(r8, o1, o3, X10, X11) \
	VPUNPCKHQDQ o1, o0, X10       \
	VPUNPCKLQDQ o1, o0, o0        \
	VMOVDQA32   X10, o1           \
	VPUNPCKHQDQ o3, o2, X10       \
	VPUNPCKLQDQ o3, o2, o2        \
	VMOVDQA32   X10, o3

#define STORE4X4(x, ptr) \
	MOVL    x, (ptr) \
	VPSRLDQ $4, x, x \
	ADDQ    BX, ptr  \
	MOVL    x, (ptr) \
	VPSRLDQ $4, x, x \
	ADDQ    BX, ptr  \
	MOVL    x, (ptr) \
	VPSRLDQ $4, x, x \
	ADDQ    BX, ptr  \
	MOVL    x, (ptr)

#define STORE16X4(i0, i1, i2, i3, r0, r8) \
	VPUNPCKLBW i1, i0, X8   \
	VPUNPCKHBW i1, i0, X9   \
	VPUNPCKLBW i3, i2, X10  \
	VPUNPCKHBW i3, i2, X11  \
	VPUNPCKHWD X10, X8, X12 \
	VPUNPCKLWD X10, X8, X8  \
	VPUNPCKHWD X11, X9, X13 \
	VPUNPCKLWD X11, X9, X9  \
	STORE4X4(X8, r0)        \
	ADDQ       BX, r0       \
	STORE4X4(X12, r0)       \
	STORE4X4(X9, r8)        \
	ADDQ       BX, r8       \
	STORE4X4(X13, r8)

// func vFilter16AVX512(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter16AVX512(SB), NOSPLIT, $0-40
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

// func vFilter16iAVX512(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter16iAVX512(SB), NOSPLIT, $0-40
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

	VMOVDQA32 X4, X0
	VMOVDQA32 X5, X1
	VMOVDQA32 X6, X2
	VMOVDQA32 X7, X3

	DECQ CX
	JNZ  loop16i
	VZEROUPPER
	RET

#define LOADUV(x) \
	VMOVQ       (SI), x    \
	VMOVQ       (R8), X9   \
	VPUNPCKLQDQ X9, x, x   \
	ADDQ        BX, SI     \
	ADDQ        BX, R8

#define STOREUV(x) \
	VMOVQ   x, (SI)  \
	VPSRLDQ $8, x, x \
	VMOVQ   x, (R8)  \
	ADDQ    BX, SI   \
	ADDQ    BX, R8

// func vFilter8AVX512(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter8AVX512(SB), NOSPLIT, $0-48
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

// func vFilter8iAVX512(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter8iAVX512(SB), NOSPLIT, $0-48
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

// func hFilter16AVX512(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter16AVX512(SB), NOSPLIT, $0-40
	MOVQ p+0(FP), SI
	MOVQ stride+8(FP), BX
	MOVQ limit+16(FP), AX
	MOVQ ilevel+24(FP), R11
	MOVQ hevThresh+32(FP), R12
	SETUP

	MOVQ BX, DX
	SHLQ $3, DX

	LEAQ -4(SI), CX
	LEAQ (CX)(DX*1), DI
	LOAD16X4(CX, DI, X0, X1, X2, X3)
	MAXDIFF1

	LEAQ (SI)(DX*1), DI
	LOAD16X4(SI, DI, X4, X5, X6, X7)
	MAXDIFF2

	COMPLEXMASK
	DOFILTER6

	MOVQ p+0(FP), SI
	MOVQ BX, DX
	SHLQ $3, DX
	LEAQ -4(SI), CX
	LEAQ (CX)(DX*1), DI
	STORE16X4(X0, X1, X2, X3, CX, DI)

	MOVQ p+0(FP), SI
	LEAQ (SI)(DX*1), DI
	STORE16X4(X4, X5, X6, X7, SI, DI)
	VZEROUPPER
	RET

// func hFilter16iAVX512(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter16iAVX512(SB), NOSPLIT, $0-40
	MOVQ p+0(FP), SI
	MOVQ stride+8(FP), BX
	MOVQ limit+16(FP), AX
	MOVQ ilevel+24(FP), R11
	MOVQ hevThresh+32(FP), R12
	SETUP
	MOVQ $3, R13

	MOVQ BX, DX
	SHLQ $3, DX

	LEAQ (SI)(DX*1), DI
	LOAD16X4(SI, DI, X0, X1, X2, X3)

loop16ih:
	MAXDIFF1

	ADDQ $4, SI
	LEAQ (SI)(DX*1), DI
	LOAD16X4(SI, DI, X4, X5, X6, X7)
	MAXDIFF2

	COMPLEXMASK
	DOFILTER4

	LEAQ -2(SI), CX
	LEAQ (CX)(DX*1), DI
	STORE16X4(X2, X3, X4, X5, CX, DI)

	VMOVDQA32 X4, X0
	VMOVDQA32 X5, X1
	VMOVDQA32 X6, X2
	VMOVDQA32 X7, X3

	DECQ R13
	JNZ  loop16ih
	VZEROUPPER
	RET

// func hFilter8AVX512(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter8AVX512(SB), NOSPLIT, $0-48
	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ stride+16(FP), BX
	MOVQ limit+24(FP), AX
	MOVQ ilevel+32(FP), R11
	MOVQ hevThresh+40(FP), R12
	SETUP

	LEAQ -4(SI), CX
	LEAQ -4(R8), DI
	LOAD16X4(CX, DI, X0, X1, X2, X3)
	MAXDIFF1

	LOAD16X4(SI, R8, X4, X5, X6, X7)
	MAXDIFF2

	COMPLEXMASK
	DOFILTER6

	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	LEAQ -4(SI), CX
	LEAQ -4(R8), DI
	STORE16X4(X0, X1, X2, X3, CX, DI)

	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	STORE16X4(X4, X5, X6, X7, SI, R8)
	VZEROUPPER
	RET

// func hFilter8iAVX512(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter8iAVX512(SB), NOSPLIT, $0-48
	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ stride+16(FP), BX
	MOVQ limit+24(FP), AX
	MOVQ ilevel+32(FP), R11
	MOVQ hevThresh+40(FP), R12
	SETUP

	LOAD16X4(SI, R8, X0, X1, X2, X3)
	MAXDIFF1

	ADDQ $4, SI
	ADDQ $4, R8
	LOAD16X4(SI, R8, X4, X5, X6, X7)
	MAXDIFF2

	COMPLEXMASK
	DOFILTER4

	LEAQ -2(SI), CX
	LEAQ -2(R8), DI
	STORE16X4(X2, X3, X4, X5, CX, DI)
	VZEROUPPER
	RET
