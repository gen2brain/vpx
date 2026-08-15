package vp8

import "math"

// EncodeOptions are the lossy encoding parameters.
type EncodeOptions struct {
	// Quality in the range [0,100].
	Quality int
	// Method is the quality/speed trade-off in the range [0,6].
	Method int
	// Threads bounds the goroutines encoding one frame. Zero means GOMAXPROCS,
	// one encodes serially. A frame is encoded as a macroblock stage and a
	// token stage running concurrently, so nothing above two helps.
	Threads int
}

const y2Block = 24

var (
	fixedCostsI16 = [4]int{663, 919, 872, 919}
	fixedCostsUV  = [4]int{302, 984, 439, 642}
)

type mbInfo struct {
	ymode  uint8
	uvMode uint8
	skip   bool
}

// Encoder encodes VP8 key frames. The zero value is ready to use, and reusing
// one across frames reuses its buffers.
type Encoder struct {
	rec   Decoder
	proba proba
	src   *Picture

	mbW, mbH int
	baseQ    int

	y1, y2, uv qmatrix
	lambdaMode int
	lambdaUV   int

	filterLevel int
	useSkip     bool
	skipProb    uint8

	info []mbInfo
	ctx  []mbCtx

	buf frameBuffer

	hdr boolEnc
	tok boolEnc
	out []byte

	sc      [yuvSize]uint8
	lv      mbLevels
	pipe    *encPipeline
	threads int
	dc      [16]int16
}

func qualityToQuant(quality int) int {
	c := float64(quality) / 100

	linear := c * 2 / 3
	if c >= 0.75 {
		linear = 2*c - 1
	}

	return clampQ(int(127*(1-math.Pow(linear, 1.0/3))), 127)
}

func (e *Encoder) setup(o EncodeOptions) {
	e.threads = o.Threads
	e.baseQ = qualityToQuant(o.Quality)

	e.y1.q[0] = uint32(dcTable[e.baseQ])
	e.y1.q[1] = uint32(acTable[e.baseQ])
	e.y2.q[0] = uint32(dcTable[e.baseQ]) * 2
	e.y2.q[1] = max(uint32(int(acTable[e.baseQ])*101581>>16), 8)
	e.uv.q[0] = uint32(dcTable[clampQ(e.baseQ, 117)])
	e.uv.q[1] = uint32(acTable[e.baseQ])

	qi4 := e.y1.expand(0)
	quv := e.uv.expand(2)

	e.y2.expand(1)

	e.lambdaMode = qi4 * qi4 >> 7
	e.lambdaUV = 3 * quv * quv >> 6

	qstep := int(acTable[e.baseQ]) >> 2
	level := int(levelsFromDelta[0][min(qstep, len(levelsFromDelta[0])-1)]) * 300 / 256

	e.filterLevel = min(level, 63)
	if e.filterLevel < 2 {
		e.filterLevel = 0
	}

	e.proba.reset()
}

func (e *Encoder) alloc() {
	e.rec.hdr = FrameHeader{KeyFrame: true, Show: true, Width: e.src.Width, Height: e.src.Height}
	e.rec.mbW, e.rec.mbH = e.mbW, e.mbH

	e.buf.alloc(e.mbW, e.mbH, e.src.Width, e.src.Height)
	e.rec.pic = e.buf.pic
	e.rec.allocRows()

	if cap(e.info) < e.mbW*e.mbH {
		e.info = make([]mbInfo, e.mbW*e.mbH)
	}

	e.info = e.info[:e.mbW*e.mbH]

	if cap(e.ctx) < e.mbW+1 {
		e.ctx = make([]mbCtx, e.mbW+1)
	}

	e.ctx = e.ctx[:e.mbW+1]

	clear(e.ctx)
}

func loadPlane(dst []byte, off int, src []byte, stride, x0, y0, size, w, h int) {
	for j := range size {
		row := src[min(y0+j, h-1)*stride:]
		d := dst[off+j*bps : off+j*bps+size]

		if x0+size <= w {
			copy(d, row[x0:x0+size])

			continue
		}

		for i := range size {
			d[i] = row[min(x0+i, w-1)]
		}
	}
}

func (e *Encoder) loadSource(mbX, mbY int) {
	uvW, uvH := (e.src.Width+1)/2, (e.src.Height+1)/2

	loadPlane(e.sc[:], yOff, e.src.Y, e.src.YStride, mbX*16, mbY*16, 16, e.src.Width, e.src.Height)
	loadPlane(e.sc[:], uOff, e.src.U, e.src.UVStride, mbX*8, mbY*8, 8, uvW, uvH)
	loadPlane(e.sc[:], vOff, e.src.V, e.src.UVStride, mbX*8, mbY*8, 8, uvW, uvH)
}

func sse(a, b []byte, off, size int) int {
	if sseAsm != nil && off >= 0 && len(a)-off >= (size-1)*bps+size && len(b)-off >= (size-1)*bps+size {
		return sseAsm(a, b, off, size)
	}

	return sseGo(a, b, off, size)
}

func sseGo(a, b []byte, off, size int) int {
	total := 0

	for j := range size {
		x := a[off+j*bps : off+j*bps+size]
		y := b[off+j*bps : off+j*bps+size]

		for i := range size {
			d := int(x[i]) - int(y[i])
			total += d * d
		}
	}

	return total
}

func (e *Encoder) pickLumaMode(mbX, mbY int) uint8 {
	b := e.rec.yuv[:]

	best, bestScore := 0, math.MaxInt

	for mode := range 4 {
		predLuma16[checkMode(mbX, mbY, mode)](b, yOff)

		score := 256*sse(e.sc[:], b, yOff, 16) + e.lambdaMode*fixedCostsI16[mode]
		if score < bestScore {
			best, bestScore = mode, score
		}
	}

	predLuma16[checkMode(mbX, mbY, best)](b, yOff)

	return uint8(best)
}

func (e *Encoder) pickChromaMode(mbX, mbY int) uint8 {
	b := e.rec.yuv[:]

	best, bestScore := 0, math.MaxInt

	for mode := range 4 {
		m := checkMode(mbX, mbY, mode)

		predChroma8[m](b, uOff)
		predChroma8[m](b, vOff)

		score := 256*(sse(e.sc[:], b, uOff, 8)+sse(e.sc[:], b, vOff, 8)) +
			e.lambdaUV*fixedCostsUV[mode]

		if score < bestScore {
			best, bestScore = mode, score
		}
	}

	m := checkMode(mbX, mbY, best)

	predChroma8[m](b, uOff)
	predChroma8[m](b, vOff)

	return uint8(best)
}

func (e *Encoder) transformLuma(m *mbData, lv *mbLevels) uint32 {
	b := e.rec.yuv[:]

	for n := 0; n < 16; n += 2 {
		fTransform2(e.sc[:], b, yOff+scan[n], yOff+scan[n], m.coeffs[16*n:])
	}

	fTransformWHT(m.coeffs[:], e.dc[:])

	lv.nz[y2Block] = quantizeBlock(e.dc[:], lv.levels[y2Block][:], &e.y2, 0)

	if lv.nz[y2Block] > 1 {
		transformWHT(e.dc[:], m.coeffs[:])
	} else {
		dc0 := (e.dc[0] + 3) >> 3

		for i := 0; i < 16*16; i += 16 {
			m.coeffs[i] = dc0
		}
	}

	bits := uint32(0)

	for n := range 16 {
		blk := m.coeffs[16*n : 16*n+16]

		lv.nz[n] = quantizeBlock(blk, lv.levels[n][:], &e.y1, 1)
		bits = nzCodeBits(bits, lv.nz[n], blk[0] != 0)
	}

	return bits
}

func (e *Encoder) transformChroma(m *mbData, lv *mbLevels) uint32 {
	b := e.rec.yuv[:]
	planes := [2]int{uOff, vOff}

	bits := uint32(0)

	for p, base := range planes {
		plane := uint32(0)

		for i := range 4 {
			n := 16 + 4*p + i
			off := base + (i&1)*4 + (i>>1)*4*bps
			blk := m.coeffs[n*16 : n*16+16]

			fTransform(e.sc[:], b, off, off, blk)

			lv.nz[n] = quantizeBlock(blk, lv.levels[n][:], &e.uv, 0)
			plane = nzCodeBits(plane, lv.nz[n], blk[0] != 0)
		}

		bits |= plane << (8 * p)
	}

	return bits
}

func (e *Encoder) codeMB(mbX, mbY int, lv *mbLevels) {
	e.analyzeMB(mbX, mbY, lv)
	e.writeMB(mbX, lv, e.info[mbY*e.mbW+mbX].skip)
}

// analyzeMB picks the modes, transforms and quantizes. It reconstructs through
// e.rec and produces the levels the token writer consumes, touching nothing the
// writer owns.
func (e *Encoder) analyzeMB(mbX, mbY int, lv *mbLevels) {
	m := &e.rec.mb

	clear(m.coeffs[:])

	m.isI4x4 = false
	m.segment = 0
	m.imodes[0] = e.pickLumaMode(mbX, mbY)
	m.nonZeroY = e.transformLuma(m, lv)

	m.uvMode = e.pickChromaMode(mbX, mbY)
	m.nonZeroUV = e.transformChroma(m, lv)

	skip := m.nonZeroY|m.nonZeroUV == 0 && lv.nz[y2Block] == 0

	m.skip = skip
	m.skipped = skip

	e.info[mbY*e.mbW+mbX] = mbInfo{ymode: m.imodes[0], uvMode: m.uvMode, skip: skip}
}

// writeMB is the token stage. It owns tok and ctx, and reads only the levels
// and the skip flag analyzeMB produced.
func (e *Encoder) writeMB(mbX int, lv *mbLevels, skip bool) {
	if skip {
		e.ctx[1+mbX] = mbCtx{}
		e.ctx[0] = mbCtx{}

		return
	}

	e.putResiduals(mbX, lv)
}

func b2i(v bool) int {
	if v {
		return 1
	}

	return 0
}

func putLargeValue(w *boolEnc, p *[numProbas]uint8, v int32) {
	if v < 5 {
		w.put(0, p[3])
		w.flushIf()

		if v == 2 {
			w.put(0, p[4])
			w.flushIf()

			return
		}

		w.put(1, p[4])
		w.flushIf()
		w.put(int(v-3), p[5])
		w.flushIf()

		return
	}

	w.put(1, p[3])
	w.flushIf()

	if v < 11 {
		w.put(0, p[6])
		w.flushIf()

		if v < 7 {
			w.put(0, p[7])
			w.flushIf()
			w.put(int(v-5), 159)
			w.flushIf()

			return
		}

		w.put(1, p[7])
		w.flushIf()
		w.put(int(v-7)>>1, 165)
		w.flushIf()
		w.put(int(v-7)&1, 145)
		w.flushIf()

		return
	}

	w.put(1, p[6])
	w.flushIf()

	cat := 0
	for cat < 3 && v >= 3+8<<(cat+1) {
		cat++
	}

	bit1 := cat >> 1

	w.put(bit1, p[8])
	w.flushIf()
	w.put(cat&1, p[9+bit1])
	w.flushIf()

	probs := catProbs[cat]
	v -= 3 + 8<<cat

	for i, prob := range probs {
		w.put(int(v>>(len(probs)-1-i))&1, prob)
		w.flushIf()
	}
}

func putCoeffs(w *boolEnc, bands *[17]*bandProbs, ctx, first int, levels []int16, nz int) {
	p := &bands[first][ctx]

	for n := first; n < 16; {
		if n >= nz {
			w.put(0, p[0])
			w.flushIf()

			return
		}

		w.put(1, p[0])
		w.flushIf()

		for levels[n] == 0 {
			w.put(0, p[1])
			w.flushIf()
			n++
			p = &bands[n][0]
		}

		w.put(1, p[1])
		w.flushIf()

		v := int32(levels[n])
		neg := v < 0

		if neg {
			v = -v
		}

		next := bands[n+1]

		if v == 1 {
			w.put(0, p[2])
			w.flushIf()
			p = &next[1]
		} else {
			w.put(1, p[2])
			w.flushIf()
			putLargeValue(w, p, v)
			p = &next[2]
		}

		w.put(b2i(neg), 0x80)
		w.flushIf()

		n++
	}
}

func (e *Encoder) putResiduals(mbX int, lv *mbLevels) {
	w := &e.tok
	top := &e.ctx[1+mbX]
	left := &e.ctx[0]

	putCoeffs(w, &e.proba.bandsPtr[1], int(top.nzDC)+int(left.nzDC), 0, lv.levels[y2Block][:], lv.nz[y2Block])

	nzDC := uint8(0)
	if lv.nz[y2Block] > 0 {
		nzDC = 1
	}

	top.nzDC, left.nzDC = nzDC, nzDC

	const first = 1

	acBands := &e.proba.bandsPtr[0]

	tnz := top.nz & 0x0f
	lnz := left.nz & 0x0f
	n := 0

	for range 4 {
		l := lnz & 1

		for range 4 {
			ctx := int(l) + int(tnz&1)

			putCoeffs(w, acBands, ctx, first, lv.levels[n][:], lv.nz[n])

			l = 0
			if lv.nz[n] > first {
				l = 1
			}

			tnz = tnz>>1 | l<<7
			n++
		}

		tnz >>= 4
		lnz = lnz>>1 | l<<7
	}

	outTNZ := uint32(tnz)
	outLNZ := uint32(lnz >> 4)

	n = 16

	for ch := 0; ch < 4; ch += 2 {
		tnz := top.nz >> (4 + ch)
		lnz := left.nz >> (4 + ch)

		for range 2 {
			l := lnz & 1

			for range 2 {
				ctx := int(l) + int(tnz&1)

				putCoeffs(w, &e.proba.bandsPtr[2], ctx, 0, lv.levels[n][:], lv.nz[n])

				l = 0
				if lv.nz[n] > 0 {
					l = 1
				}

				tnz = tnz>>1 | l<<3
				n++
			}

			tnz >>= 2
			lnz = lnz>>1 | l<<5
		}

		outTNZ |= uint32(tnz) << 4 << ch
		outLNZ |= uint32(lnz&0xf0) << ch
	}

	top.nz = uint8(outTNZ)
	left.nz = uint8(outLNZ)
}

func (e *Encoder) putModes() {
	w := &e.hdr

	for _, m := range e.info {
		if e.useSkip {
			w.putBool(m.skip, e.skipProb)
		}

		w.putBit(1, 145)

		switch m.ymode {
		case dcPred:
			w.putBit(0, 156)
			w.putBit(0, 163)
		case vPred:
			w.putBit(0, 156)
			w.putBit(1, 163)
		case hPred:
			w.putBit(1, 156)
			w.putBit(0, 128)
		default:
			w.putBit(1, 156)
			w.putBit(1, 128)
		}

		switch m.uvMode {
		case dcPred:
			w.putBit(0, 142)
		case vPred:
			w.putBit(1, 142)
			w.putBit(0, 114)
		case hPred:
			w.putBit(1, 142)
			w.putBit(1, 114)
			w.putBit(0, 183)
		default:
			w.putBit(1, 142)
			w.putBit(1, 114)
			w.putBit(1, 183)
		}
	}
}

func (e *Encoder) putHeader() {
	w := &e.hdr

	w.putBits(0, 2)

	w.putFlag(false)

	w.putFlag(false)
	w.putBits(uint32(e.filterLevel), 6)
	w.putBits(0, 3)
	w.putFlag(false)

	w.putBits(0, 2)

	w.putBits(uint32(e.baseQ), 7)

	for range 5 {
		w.putFlag(false)
	}

	w.putFlag(true)

	for t := range numBlockTypes {
		for b := range numBands {
			for c := range numCtx {
				for p := range numProbas {
					w.putBit(0, coeffUpdateProbs[t][b][c][p])
				}
			}
		}
	}

	w.putFlag(e.useSkip)

	if e.useSkip {
		w.putBits(uint32(e.skipProb), 8)
	}
}

func (e *Encoder) skipProbability() {
	skips := 0

	for _, m := range e.info {
		if m.skip {
			skips++
		}
	}

	total := len(e.info)

	e.useSkip = true
	e.skipProb = uint8((total - skips) * 255 / total)
}

// Release drops the encoder's reference to the picture it last encoded, so a
// pooled encoder does not keep the caller's input alive.
func (e *Encoder) Release() {
	e.src = nil
	e.out = e.out[:0]
}

// Encode writes src as a VP8 key frame. The bitstream it returns is owned by
// the encoder and is valid until the next call.
func (e *Encoder) Encode(src *Picture, o EncodeOptions) ([]byte, error) {
	if src.Width <= 0 || src.Height <= 0 || src.Width >= maxDimension || src.Height >= maxDimension {
		return nil, ErrInvalid
	}

	e.src = src
	e.mbW = (src.Width + 15) / 16
	e.mbH = (src.Height + 15) / 16

	e.setup(o)
	e.alloc()

	e.tok.init(e.tok.buf)

	e.encodeRows()

	tokens := e.tok.finish()

	e.skipProbability()

	e.hdr.init(e.hdr.buf)
	e.putHeader()
	e.putModes()

	header := e.hdr.finish()

	tag := uint32(1<<4 | len(header)<<5)

	out := e.out[:0]

	out = append(out, byte(tag), byte(tag>>8), byte(tag>>16))
	out = append(out, startCode[0], startCode[1], startCode[2])
	out = append(out, byte(src.Width), byte(src.Width>>8), byte(src.Height), byte(src.Height>>8))
	out = append(out, header...)
	out = append(out, tokens...)

	e.out = out

	return out, nil
}

type mbLevels struct {
	levels [y2Block + 1][16]int16
	nz     [y2Block + 1]int
}
