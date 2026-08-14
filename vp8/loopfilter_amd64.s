//go:build amd64 && !noasm

#include "textflag.h"

DATA k80<>+0(SB)/8, $0x8080808080808080
DATA k80<>+8(SB)/8, $0x8080808080808080
GLOBL k80<>(SB), RODATA|NOPTR, $16

DATA kfe<>+0(SB)/8, $0xfefefefefefefefe
DATA kfe<>+8(SB)/8, $0xfefefefefefefefe
GLOBL kfe<>(SB), RODATA|NOPTR, $16

DATA k3<>+0(SB)/8, $0x0303030303030303
DATA k3<>+8(SB)/8, $0x0303030303030303
GLOBL k3<>(SB), RODATA|NOPTR, $16

DATA k4<>+0(SB)/8, $0x0404040404040404
DATA k4<>+8(SB)/8, $0x0404040404040404
GLOBL k4<>(SB), RODATA|NOPTR, $16

DATA k64<>+0(SB)/8, $0x4040404040404040
DATA k64<>+8(SB)/8, $0x4040404040404040
GLOBL k64<>(SB), RODATA|NOPTR, $16

DATA k9<>+0(SB)/8, $0x0900090009000900
DATA k9<>+8(SB)/8, $0x0900090009000900
GLOBL k9<>(SB), RODATA|NOPTR, $16

DATA k63<>+0(SB)/8, $0x003f003f003f003f
DATA k63<>+8(SB)/8, $0x003f003f003f003f
GLOBL k63<>(SB), RODATA|NOPTR, $16

#define BCAST(mem, x) \
	MOVQ      mem, AX  \
	MOVQ      AX, x    \
	PUNPCKLBW x, x     \
	PUNPCKLWL x, x     \
	PSHUFD    $0, x, x

#define ABSD(a, b, t, d) \
	MOVOU   a, t   \
	PSUBUSB b, t   \
	MOVOU   b, d   \
	PSUBUSB a, d   \
	POR     t, d

#define SSHIFT3(x, t0, t1) \
	PXOR      t0, t0  \
	MOVOU     t0, t1  \
	PUNPCKLBW x, t0   \
	PUNPCKHBW x, t1   \
	PSRAW     $11, t0 \
	PSRAW     $11, t1 \
	MOVOU     t0, x   \
	PACKSSWB  t1, x

#define MAXDIFF1 \
	ABSD(X2, X3, X10, X8)   \
	ABSD(X0, X1, X10, X11)  \
	PMAXUB X11, X8          \
	ABSD(X1, X2, X10, X11)  \
	PMAXUB X11, X8

#define MAXDIFF2 \
	ABSD(X5, X4, X10, X11)  \
	PMAXUB X11, X8          \
	ABSD(X7, X6, X10, X11)  \
	PMAXUB X11, X8          \
	ABSD(X6, X5, X10, X11)  \
	PMAXUB X11, X8

#define COMPLEXMASK(ilevel, limit) \
	BCAST(ilevel, X10)      \
	PSUBUSB X10, X8         \
	PXOR    X11, X11        \
	PCMPEQB X11, X8         \
	ABSD(X2, X5, X10, X12)  \
	MOVOU   kfe<>(SB), X11  \
	PAND    X11, X12        \
	PSRLW   $1, X12         \
	ABSD(X3, X4, X10, X13)  \
	PADDUSB X13, X13        \
	PADDUSB X12, X13        \
	BCAST(limit, X10)       \
	PSUBUSB X10, X13        \
	PXOR    X11, X11        \
	PCMPEQB X11, X13        \
	PAND    X13, X8

#define NOTHEV(hev) \
	ABSD(X2, X3, X10, X9)   \
	ABSD(X5, X4, X10, X11)  \
	PMAXUB  X11, X9         \
	BCAST(hev, X10)         \
	PSUBUSB X10, X9         \
	PXOR    X11, X11        \
	PCMPEQB X11, X9

#define BASEDELTA \
	MOVOU   X2, X12  \
	PSUBSB  X5, X12  \
	MOVOU   X4, X13  \
	PSUBSB  X3, X13  \
	PADDSB  X13, X12 \
	PADDSB  X13, X12 \
	PADDSB  X13, X12

#define SIMPLEFILTER(f) \
	MOVOU   f, X14         \
	MOVOU   k3<>(SB), X10  \
	PADDSB  X10, X14       \
	MOVOU   f, X15         \
	MOVOU   k4<>(SB), X10  \
	PADDSB  X10, X15       \
	SSHIFT3(X14, X10, X11) \
	SSHIFT3(X15, X10, X11) \
	PSUBSB  X15, X4        \
	PADDSB  X14, X3

#define UPDATE2(pi, qi) \
	MOVOU    X11, X9       \
	PSRAW    $7, X9        \
	MOVOU    X12, X13      \
	PSRAW    $7, X13       \
	PACKSSWB X13, X9       \
	PADDSB   X9, pi        \
	PSUBSB   X9, qi        \
	MOVOU    k80<>(SB), X10 \
	PXOR     X10, pi       \
	PXOR     X10, qi

#define DOFILTER6(hev) \
	NOTHEV(hev)             \
	MOVOU   k80<>(SB), X10  \
	PXOR    X10, X1         \
	PXOR    X10, X2         \
	PXOR    X10, X3         \
	PXOR    X10, X4         \
	PXOR    X10, X5         \
	PXOR    X10, X6         \
	BASEDELTA               \
	MOVOU   X9, X13         \
	PANDN   X8, X13         \
	PAND    X12, X13        \
	SIMPLEFILTER(X13)       \
	MOVOU   X9, X13         \
	PAND    X8, X13         \
	PAND    X12, X13        \
	PXOR    X10, X10        \
	MOVOU   X10, X14        \
	PUNPCKLBW X13, X14      \
	MOVOU   X10, X15        \
	PUNPCKHBW X13, X15      \
	MOVOU   k9<>(SB), X10   \
	PMULHW  X10, X14        \
	PMULHW  X10, X15        \
	MOVOU   k63<>(SB), X10  \
	MOVOU   X14, X11        \
	PADDW   X10, X11        \
	MOVOU   X15, X12        \
	PADDW   X10, X12        \
	UPDATE2(X1, X6)         \
	PADDW   X14, X11        \
	PADDW   X15, X12        \
	UPDATE2(X2, X5)         \
	PADDW   X14, X11        \
	PADDW   X15, X12        \
	UPDATE2(X3, X4)

#define DOFILTER4(hev) \
	NOTHEV(hev)             \
	MOVOU   k80<>(SB), X10  \
	PXOR    X10, X2         \
	PXOR    X10, X3         \
	PXOR    X10, X4         \
	PXOR    X10, X5         \
	MOVOU   X2, X12         \
	PSUBSB  X5, X12         \
	MOVOU   X9, X13         \
	PANDN   X12, X13        \
	MOVOU   X4, X12         \
	PSUBSB  X3, X12         \
	PADDSB  X12, X13        \
	PADDSB  X12, X13        \
	PADDSB  X12, X13        \
	PAND    X8, X13         \
	MOVOU   X13, X14        \
	MOVOU   k3<>(SB), X10   \
	PADDSB  X10, X14        \
	MOVOU   X13, X15        \
	MOVOU   k4<>(SB), X10   \
	PADDSB  X10, X15        \
	SSHIFT3(X14, X10, X11)  \
	SSHIFT3(X15, X10, X11)  \
	PADDSB  X14, X3         \
	PSUBSB  X15, X4         \
	MOVOU   k80<>(SB), X10  \
	PXOR    X10, X3         \
	PXOR    X10, X4         \
	MOVOU   X15, X14        \
	PADDB   X10, X14        \
	PXOR    X11, X11        \
	PAVGB   X11, X14        \
	MOVOU   k64<>(SB), X10  \
	PSUBB   X10, X14        \
	PAND    X9, X14         \
	PSUBSB  X14, X5         \
	PADDSB  X14, X2         \
	MOVOU   k80<>(SB), X10  \
	PXOR    X10, X2         \
	PXOR    X10, X5

// func vFilter16SSE(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter16SSE(SB), NOSPLIT, $0-40
	MOVQ p+0(FP), SI
	MOVQ stride+8(FP), BX

	MOVQ BX, DX
	SHLQ $2, DX
	SUBQ DX, SI

	MOVOU (SI), X0
	ADDQ  BX, SI
	MOVOU (SI), X1
	ADDQ  BX, SI
	MOVOU (SI), X2
	ADDQ  BX, SI
	MOVOU (SI), X3
	ADDQ  BX, SI
	MOVOU (SI), X4
	ADDQ  BX, SI
	MOVOU (SI), X5
	ADDQ  BX, SI
	MOVOU (SI), X6
	ADDQ  BX, SI
	MOVOU (SI), X7

	MAXDIFF1
	MAXDIFF2
	COMPLEXMASK(ilevel+24(FP), limit+16(FP))
	DOFILTER6(hevThresh+32(FP))

	MOVQ p+0(FP), DI
	MOVQ BX, DX
	ADDQ BX, DX
	ADDQ BX, DX
	SUBQ DX, DI

	MOVOU X1, (DI)
	ADDQ  BX, DI
	MOVOU X2, (DI)
	ADDQ  BX, DI
	MOVOU X3, (DI)
	ADDQ  BX, DI
	MOVOU X4, (DI)
	ADDQ  BX, DI
	MOVOU X5, (DI)
	ADDQ  BX, DI
	MOVOU X6, (DI)
	RET

// func vFilter16iSSE(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter16iSSE(SB), NOSPLIT, $0-40
	MOVQ p+0(FP), SI
	MOVQ stride+8(FP), BX
	MOVQ $3, CX

	MOVOU (SI), X0
	ADDQ  BX, SI
	MOVOU (SI), X1
	ADDQ  BX, SI
	MOVOU (SI), X2
	ADDQ  BX, SI
	MOVOU (SI), X3
	ADDQ  BX, SI

loop16i:
	MOVOU (SI), X4
	ADDQ  BX, SI
	MOVOU (SI), X5
	ADDQ  BX, SI
	MOVOU (SI), X6
	ADDQ  BX, SI
	MOVOU (SI), X7
	ADDQ  BX, SI

	MAXDIFF1
	MAXDIFF2
	COMPLEXMASK(ilevel+24(FP), limit+16(FP))
	DOFILTER4(hevThresh+32(FP))

	MOVQ  SI, DI
	MOVQ  BX, DX
	SHLQ  $2, DX
	SUBQ  DX, DI
	MOVQ  BX, DX
	ADDQ  BX, DX
	SUBQ  DX, DI

	MOVOU X2, (DI)
	ADDQ  BX, DI
	MOVOU X3, (DI)
	ADDQ  BX, DI
	MOVOU X4, (DI)
	ADDQ  BX, DI
	MOVOU X5, (DI)

	MOVOU X4, X0
	MOVOU X5, X1
	MOVOU X6, X2
	MOVOU X7, X3

	DECQ CX
	JNZ  loop16i
	RET

// func vFilter8SSE(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter8SSE(SB), NOSPLIT, $0-48
	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ stride+16(FP), BX

	MOVQ BX, DX
	SHLQ $2, DX
	SUBQ DX, SI
	SUBQ DX, R8

#define LOADUV(x) \
	MOVQ       (SI), x  \
	MOVQ       (R8), X9 \
	PUNPCKLQDQ X9, x    \
	ADDQ       BX, SI   \
	ADDQ       BX, R8

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
	COMPLEXMASK(ilevel+32(FP), limit+24(FP))
	DOFILTER6(hevThresh+40(FP))

	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ BX, DX
	ADDQ BX, DX
	ADDQ BX, DX
	SUBQ DX, SI
	SUBQ DX, R8

#define STOREUV(x) \
	MOVQ   x, (SI)  \
	PSRLDQ $8, x    \
	MOVQ   x, (R8)  \
	ADDQ   BX, SI   \
	ADDQ   BX, R8

	STOREUV(X1)
	STOREUV(X2)
	STOREUV(X3)
	STOREUV(X4)
	STOREUV(X5)
	STOREUV(X6)
	RET

// func vFilter8iSSE(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·vFilter8iSSE(SB), NOSPLIT, $0-48
	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ stride+16(FP), BX

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
	COMPLEXMASK(ilevel+32(FP), limit+24(FP))
	DOFILTER4(hevThresh+40(FP))

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
	RET

#define LOAD8X4(base, outp, outq, t0, t1, t2) \
	MOVL       (base), outp     \
	MOVL       (base)(BX*4), t0 \
	PUNPCKLLQ  t0, outp         \
	LEAQ       (base)(BX*2), R9 \
	MOVL       (R9), t0         \
	MOVL       (R9)(BX*4), t1   \
	PUNPCKLLQ  t1, t0           \
	PUNPCKLQDQ t0, outp         \
	LEAQ       (base)(BX*1), R9 \
	MOVL       (R9), outq       \
	MOVL       (R9)(BX*4), t0   \
	PUNPCKLLQ  t0, outq         \
	LEAQ       (R9)(BX*2), R10  \
	MOVL       (R10), t0        \
	MOVL       (R10)(BX*4), t1  \
	PUNPCKLLQ  t1, t0           \
	PUNPCKLQDQ t0, outq         \
	MOVOU      outp, t0         \
	PUNPCKLBW  outq, outp       \
	PUNPCKHBW  outq, t0         \
	MOVOU      outp, t1         \
	PUNPCKLWL  t0, outp         \
	PUNPCKHWL  t0, t1           \
	MOVOU      outp, t2         \
	PUNPCKLLQ  t1, outp         \
	MOVOU      t2, outq         \
	PUNPCKHLQ  t1, outq

#define LOAD16X4(r0, r8, o0, o1, o2, o3) \
	LOAD8X4(r0, o0, o2, X10, X11, X12) \
	LOAD8X4(r8, o1, o3, X10, X11, X12) \
	MOVOU      o0, X10   \
	PUNPCKLQDQ o1, o0    \
	PUNPCKHQDQ o1, X10   \
	MOVOU      X10, o1   \
	MOVOU      o2, X10   \
	PUNPCKLQDQ o3, o2    \
	PUNPCKHQDQ o3, X10   \
	MOVOU      X10, o3

#define STORE4X4(x, ptr) \
	MOVL   x, (ptr) \
	PSRLDQ $4, x    \
	ADDQ   BX, ptr  \
	MOVL   x, (ptr) \
	PSRLDQ $4, x    \
	ADDQ   BX, ptr  \
	MOVL   x, (ptr) \
	PSRLDQ $4, x    \
	ADDQ   BX, ptr  \
	MOVL   x, (ptr)

#define STORE16X4(i0, i1, i2, i3, r0, r8) \
	MOVOU     i0, X8       \
	PUNPCKLBW i1, X8       \
	MOVOU     i0, X9       \
	PUNPCKHBW i1, X9       \
	MOVOU     i2, X10      \
	PUNPCKLBW i3, X10      \
	MOVOU     i2, X11      \
	PUNPCKHBW i3, X11      \
	MOVOU     X8, X12      \
	PUNPCKLWL X10, X8      \
	PUNPCKHWL X10, X12     \
	MOVOU     X9, X13      \
	PUNPCKLWL X11, X9      \
	PUNPCKHWL X11, X13     \
	STORE4X4(X8, r0)       \
	ADDQ      BX, r0       \
	STORE4X4(X12, r0)      \
	STORE4X4(X9, r8)       \
	ADDQ      BX, r8       \
	STORE4X4(X13, r8)

// func hFilter16SSE(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter16SSE(SB), NOSPLIT, $0-40
	MOVQ p+0(FP), SI
	MOVQ stride+8(FP), BX

	MOVQ BX, DX
	SHLQ $3, DX

	LEAQ -4(SI), CX
	LEAQ (CX)(DX*1), DI
	LOAD16X4(CX, DI, X0, X1, X2, X3)
	MAXDIFF1

	LEAQ (SI)(DX*1), DI
	LOAD16X4(SI, DI, X4, X5, X6, X7)
	MAXDIFF2

	COMPLEXMASK(ilevel+24(FP), limit+16(FP))
	DOFILTER6(hevThresh+32(FP))

	MOVQ p+0(FP), SI
	MOVQ BX, DX
	SHLQ $3, DX
	LEAQ -4(SI), CX
	LEAQ (CX)(DX*1), DI
	STORE16X4(X0, X1, X2, X3, CX, DI)

	MOVQ p+0(FP), SI
	LEAQ (SI)(DX*1), DI
	STORE16X4(X4, X5, X6, X7, SI, DI)
	RET

// func hFilter16iSSE(p *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter16iSSE(SB), NOSPLIT, $0-40
	MOVQ p+0(FP), SI
	MOVQ stride+8(FP), BX
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

	COMPLEXMASK(ilevel+24(FP), limit+16(FP))
	DOFILTER4(hevThresh+32(FP))

	LEAQ -2(SI), CX
	LEAQ (CX)(DX*1), DI
	STORE16X4(X2, X3, X4, X5, CX, DI)

	MOVOU X4, X0
	MOVOU X5, X1
	MOVOU X6, X2
	MOVOU X7, X3

	DECQ R13
	JNZ  loop16ih
	RET

// func hFilter8SSE(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter8SSE(SB), NOSPLIT, $0-48
	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ stride+16(FP), BX

	LEAQ -4(SI), CX
	LEAQ -4(R8), DI
	LOAD16X4(CX, DI, X0, X1, X2, X3)
	MAXDIFF1

	LOAD16X4(SI, R8, X4, X5, X6, X7)
	MAXDIFF2

	COMPLEXMASK(ilevel+32(FP), limit+24(FP))
	DOFILTER6(hevThresh+40(FP))

	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	LEAQ -4(SI), CX
	LEAQ -4(R8), DI
	STORE16X4(X0, X1, X2, X3, CX, DI)

	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	STORE16X4(X4, X5, X6, X7, SI, R8)
	RET

// func hFilter8iSSE(u, v *byte, stride, limit, ilevel, hevThresh int)
TEXT ·hFilter8iSSE(SB), NOSPLIT, $0-48
	MOVQ u+0(FP), SI
	MOVQ v+8(FP), R8
	MOVQ stride+16(FP), BX

	LOAD16X4(SI, R8, X0, X1, X2, X3)
	MAXDIFF1

	ADDQ $4, SI
	ADDQ $4, R8
	LOAD16X4(SI, R8, X4, X5, X6, X7)
	MAXDIFF2

	COMPLEXMASK(ilevel+32(FP), limit+24(FP))
	DOFILTER4(hevThresh+40(FP))

	LEAQ -2(SI), CX
	LEAQ -2(R8), DI
	STORE16X4(X2, X3, X4, X5, CX, DI)
	RET
