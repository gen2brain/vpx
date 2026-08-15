package vp8

import "math"

// EncodeOptions are the lossy encoding parameters.
type EncodeOptions struct {
	// Quality in the range [0,100].
	Quality int
	// Method is the quality/speed trade-off in the range [0,6].
	Method  int
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
	i4x4   bool
	inter  bool
	mode   uint8
	mv     mv
	imodes [16]uint8
}

// Encoder encodes VP8 frames. The zero value is ready to use, and reusing one
// across frames reuses its buffers and its reference frames.
const (
	maxPartition0   = 1 << 19
	maxI4HeaderBits = 256 * 16 * 16
	minI4HeaderBits = 256
)

type Encoder struct {
	rec   Decoder
	proba proba
	src   *Picture

	mbW, mbH int
	baseQ    int

	y1, y2, uv qmatrix
	lambdaMode int
	lambdaUV   int

	lambdaTrellisI4  int
	lambdaTrellisI16 int

	i4HeaderBits int
	p0Limit      int

	filterLevel int
	useSkip     bool
	skipProb    uint8

	info []mbInfo
	ctx  []mbCtx

	hdr boolEnc
	tok boolEnc
	out []byte

	sc         [yuvSize]uint8
	lv         mbLevels
	pipe       *encPipeline
	topB       []uint8
	leftB      [4]uint8
	tryI4      bool
	rdUV       bool
	keyFrame   bool
	probIntra  uint8
	trellis    bool
	tokens     tokenBuf
	probaNew   [numSlots]uint8
	probaFlat  [numSlots]uint8
	i4Levels   [16]int16
	i4Coeffs   [16]int16
	rdI16      bool
	saved      i16State
	savedUV    uvState
	interSaved i16State
	intraSaved i16State
	threads    int
	dc         [16]int16
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
	e.tryI4 = o.Method >= 3
	e.rdUV = o.Method >= 5
	e.rdI16 = o.Method >= 5
	e.trellis = o.Method >= 5
	e.baseQ = qualityToQuant(o.Quality)

	e.y1.q[0] = uint32(dcTable[e.baseQ])
	e.y1.q[1] = uint32(acTable[e.baseQ])
	e.y2.q[0] = uint32(dcTable[e.baseQ]) * 2
	e.y2.q[1] = max(uint32(int(acTable[e.baseQ])*101581>>16), 8)
	e.uv.q[0] = uint32(dcTable[clampQ(e.baseQ, 117)])
	e.uv.q[1] = uint32(acTable[e.baseQ])

	qi4 := e.y1.expand(0)
	quv := e.uv.expand(2)
	qi16 := e.y2.expand(1)

	e.lambdaMode = qi4 * qi4 >> 7
	e.lambdaUV = 3 * quv * quv >> 6

	e.lambdaTrellisI4 = max(7*qi4*qi4>>4, 1)
	e.lambdaTrellisI16 = max(qi16*qi16>>3, 1)

	e.i4HeaderBits = maxI4HeaderBits

	if e.p0Limit == 0 {
		e.p0Limit = maxPartition0
	}

	qstep := int(acTable[e.baseQ]) >> 2
	level := int(levelsFromDelta[0][min(qstep, len(levelsFromDelta[0])-1)]) * 300 / 256

	e.filterLevel = min(level, 63)
	if e.filterLevel < 2 {
		e.filterLevel = 0
	}

}

func (e *Encoder) allocFrame() {
	e.rec.hdr = FrameHeader{KeyFrame: e.keyFrame, Show: true, Width: e.src.Width, Height: e.src.Height}
	e.rec.mbW, e.rec.mbH = e.mbW, e.mbH

	e.rec.sixtap = true
	e.rec.seg = segmentHeader{}
	e.rec.filter = filterHeader{level: e.filterLevel}

	e.rec.precomputeFilterStrengths()

	e.rec.alloc()

	e.rec.newIdx = e.rec.freeBuffer()
	e.rec.refCnt[e.rec.newIdx] = 1

	e.rec.frames[e.rec.newIdx].alloc(e.mbW, e.mbH, e.src.Width, e.src.Height)
	e.rec.pic = e.rec.frames[e.rec.newIdx].pic
}

func (e *Encoder) alloc() {
	if cap(e.info) < e.mbW*e.mbH {
		e.info = make([]mbInfo, e.mbW*e.mbH)
	}

	e.info = e.info[:e.mbW*e.mbH]

	if cap(e.ctx) < e.mbW+1 {
		e.ctx = make([]mbCtx, e.mbW+1)
	}

	e.ctx = e.ctx[:e.mbW+1]

	if cap(e.topB) < 4*e.mbW {
		e.topB = make([]uint8, 4*e.mbW)
	}

	e.topB = e.topB[:4*e.mbW]

	clear(e.topB)

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

func (e *Encoder) pickChromaMode(mbX, mbY int, m *mbData, lv *mbLevels) (uint8, uint32) {
	b := e.rec.yuv[:]

	best, bestScore := 0, math.MaxInt

	if !e.rdUV {
		for mode := range 4 {
			k := checkMode(mbX, mbY, mode)

			predChroma8[k](b, uOff)
			predChroma8[k](b, vOff)

			score := 256*(sse(e.sc[:], b, uOff, 8)+sse(e.sc[:], b, vOff, 8)) +
				e.lambdaUV*fixedCostsUV[mode]

			if score < bestScore {
				best, bestScore = mode, score
			}
		}

		k := checkMode(mbX, mbY, best)

		predChroma8[k](b, uOff)
		predChroma8[k](b, vOff)

		return uint8(best), e.transformChroma(m, lv)
	}

	for mode := range 4 {
		k := checkMode(mbX, mbY, mode)

		predChroma8[k](b, uOff)
		predChroma8[k](b, vOff)

		bits := e.transformChroma(m, lv)

		doUVTransform(bits, m.coeffs[16*16:], b, uOff)
		doUVTransform(bits>>8, m.coeffs[20*16:], b, vOff)

		score := 256*(sse(e.sc[:], b, uOff, 8)+sse(e.sc[:], b, vOff, 8)) +
			e.lambdaUV*fixedCostsUV[mode]

		var nz [2][2]int

		for p := range 2 {
			for i := range 4 {
				x, y := i&1, i>>1

				ctx := 0
				if x > 0 {
					ctx += nz[y][0]
				}

				if y > 0 {
					ctx += nz[0][x]
				}

				n := 16 + 4*p + i
				score += e.lambdaUV * coeffCost(&e.proba.bandsPtr[2], ctx, 0, lv.levels[n][:], lv.nz[n])

				nz[y][x] = b2i(lv.nz[n] > 0)
			}
		}

		if score < bestScore {
			best, bestScore = mode, score

			e.savedUV.store(m, lv, bits)
		}
	}

	return uint8(best), e.savedUV.restore(m, lv)
}

func (e *Encoder) transformLuma(m *mbData, lv *mbLevels, trellis bool) uint32 {
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

	var tnz, lnz [4]int

	for n := range 16 {
		blk := m.coeffs[16*n : 16*n+16]

		if trellis {
			x, y := n&3, n>>2

			lv.nz[n] = e.trellisQuantize(blk, lv.levels[n][:], &e.y1, 0, tnz[x]+lnz[y], 1, e.lambdaTrellisI16)

			tnz[x] = b2i(lv.nz[n] > 1)
			lnz[y] = tnz[x]
		} else {
			lv.nz[n] = quantizeBlock(blk, lv.levels[n][:], &e.y1, 1)
		}

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
	e.writeMB(mbX, lv, e.info[mbY*e.mbW+mbX])
}

func (e *Encoder) analyzeMB(mbX, mbY int, lv *mbLevels) {
	m := &e.rec.mb

	clear(m.coeffs[:])

	m.isI4x4 = false
	m.segment = 0
	m.refFrame = refIntra
	m.mode = 0
	m.needClamp = false

	intra := e.lumaI16(mbX, mbY, m, lv)

	if e.tryI4 {
		e.saved.store(m, lv)

		limit := intra + e.lambdaMode*(probCost(145, 1)-probCost(145, 0))

		if i4, bits, ok := e.pickIntra4(mbX, mbY, m, lv, limit); ok && i4 < limit {
			m.isI4x4 = true
			m.nonZeroY = bits
			lv.nz[y2Block] = 0
			intra = i4
		} else {
			e.saved.restore(m, lv)
		}
	}

	if !m.isI4x4 {
		top := e.topB[4*mbX : 4*mbX+4 : 4*mbX+4]

		for i := range 4 {
			top[i] = m.imodes[0]
			e.leftB[i] = m.imodes[0]
		}

		if e.trellis {
			predLuma16[checkMode(mbX, mbY, int(m.imodes[0]))](e.rec.yuv[:], yOff)
			m.nonZeroY = e.transformLuma(m, lv, true)
		}
	}

	if !e.keyFrame && e.tryInter(mbX, mbY, m, lv, intra) {
		e.finishMB(mbX, mbY, m, lv)

		return
	}

	m.uvMode, m.nonZeroUV = e.pickChromaMode(mbX, mbY, m, lv)

	e.finishMB(mbX, mbY, m, lv)
}

func (e *Encoder) tryInter(mbX, mbY int, m *mbData, lv *mbLevels, intra int) bool {
	e.intraSaved.store(m, lv)

	i4x4, imodes := m.isI4x4, m.imodes

	if e.pickInter(mbX, mbY, m, lv) < intra {
		m.nonZeroY = e.transformLuma(m, lv, e.trellis)
		m.nonZeroUV = e.transformChroma(m, lv)

		return true
	}

	e.intraSaved.restore(m, lv)

	m.isI4x4, m.imodes = i4x4, imodes
	m.refFrame = refIntra
	m.mode = 0
	m.needClamp = false

	*e.rec.modeAt(mbX, mbY) = modeInfo{}

	return false
}

func (e *Encoder) finishMB(mbX, mbY int, m *mbData, lv *mbLevels) {
	skip := m.nonZeroY|m.nonZeroUV == 0 && lv.nz[y2Block] == 0

	m.skip = skip
	m.skipped = skip

	info := mbInfo{ymode: m.imodes[0], uvMode: m.uvMode, skip: skip, i4x4: m.isI4x4}

	if m.inter() {
		info = mbInfo{inter: true, mode: m.mode, mv: e.rec.modeAt(mbX, mbY).mv, skip: skip}
	} else if m.isI4x4 {
		info.imodes = m.imodes
	}

	e.info[mbY*e.mbW+mbX] = info
}

type uvState struct {
	coeffs [128]int16
	levels [8][16]int16
	nz     [8]int
	bits   uint32
}

func (s *uvState) store(m *mbData, lv *mbLevels, bits uint32) {
	copy(s.coeffs[:], m.coeffs[16*16:])
	copy(s.levels[:], lv.levels[16:])
	copy(s.nz[:], lv.nz[16:])

	s.bits = bits
}

func (s *uvState) restore(m *mbData, lv *mbLevels) uint32 {
	copy(m.coeffs[16*16:], s.coeffs[:])
	copy(lv.levels[16:], s.levels[:])
	copy(lv.nz[16:], s.nz[:])

	return s.bits
}

type i16State struct {
	coeffs [384]int16
	levels [y2Block + 1][16]int16
	nz     [y2Block + 1]int
	nonZY  uint32
	ymode  uint8
}

func (s *i16State) store(m *mbData, lv *mbLevels) {
	s.coeffs = m.coeffs
	s.levels = lv.levels
	s.nz = lv.nz
	s.nonZY = m.nonZeroY
	s.ymode = m.imodes[0]
}

func (s *i16State) restore(m *mbData, lv *mbLevels) {
	m.coeffs = s.coeffs
	lv.levels = s.levels
	lv.nz = s.nz
	m.nonZeroY = s.nonZY
	m.imodes[0] = s.ymode
}

func (e *Encoder) lumaI16(mbX, mbY int, m *mbData, lv *mbLevels) int {
	if !e.rdI16 {
		m.imodes[0] = e.pickLumaMode(mbX, mbY)
		m.nonZeroY = e.transformLuma(m, lv, false)

		return e.scoreIntra16(m, lv)
	}

	b := e.rec.yuv[:]
	best := math.MaxInt

	for mode := range 4 {
		predLuma16[checkMode(mbX, mbY, mode)](b, yOff)

		m.imodes[0] = uint8(mode)
		m.nonZeroY = e.transformLuma(m, lv, false)

		if s := e.scoreIntra16(m, lv); s < best {
			best = s
			e.saved.store(m, lv)
		}
	}

	e.saved.restore(m, lv)

	return best
}

func (e *Encoder) scoreIntra16(m *mbData, lv *mbLevels) int {
	return e.lumaScore(m, lv, fixedCostsI16[m.imodes[0]])
}

func (e *Encoder) lumaScore(m *mbData, lv *mbLevels, rate int) int {
	b := e.rec.yuv[:]

	bits := m.nonZeroY

	for n := range 16 {
		doTransform(bits, m.coeffs[n*16:n*16+16], b, yOff+scan[n])
		bits <<= 2
	}

	rate += coeffCost(&e.proba.bandsPtr[1], 0, 0, lv.levels[y2Block][:], lv.nz[y2Block])

	for n := range 16 {
		rate += coeffCost(&e.proba.bandsPtr[0], 0, 1, lv.levels[n][:], lv.nz[n])
	}

	return 256*sse(e.sc[:], b, yOff, 16) + e.lambdaMode*rate
}

func (e *Encoder) writeMB(mbX int, lv *mbLevels, info mbInfo) {
	if info.skip {
		top := &e.ctx[1+mbX]
		left := &e.ctx[0]

		dcT, dcL := top.nzDC, left.nzDC

		*top = mbCtx{}
		*left = mbCtx{}

		if info.i4x4 {
			top.nzDC, left.nzDC = dcT, dcL
		}

		return
	}

	e.putResiduals(mbX, lv, info.i4x4)
}

func b2i(v bool) int {
	if v {
		return 1
	}

	return 0
}

func (e *Encoder) putResiduals(mbX int, lv *mbLevels, i4x4 bool) {
	top := &e.ctx[1+mbX]
	left := &e.ctx[0]

	first := 0
	acType := 3

	if !i4x4 {
		e.recordCoeffs(1, int(top.nzDC)+int(left.nzDC), 0, lv.levels[y2Block][:], lv.nz[y2Block])

		nzDC := uint8(0)
		if lv.nz[y2Block] > 0 {
			nzDC = 1
		}

		top.nzDC, left.nzDC = nzDC, nzDC

		first = 1
		acType = 0
	}

	tnz := top.nz & 0x0f
	lnz := left.nz & 0x0f
	n := 0

	for range 4 {
		l := lnz & 1

		for range 4 {
			ctx := int(l) + int(tnz&1)

			e.recordCoeffs(acType, ctx, first, lv.levels[n][:], lv.nz[n])

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

				e.recordCoeffs(2, ctx, 0, lv.levels[n][:], lv.nz[n])

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

	clear(e.topB)

	for i, m := range e.info {
		mbX := i % e.mbW

		if mbX == 0 {
			e.leftB = [4]uint8{}
		}

		if e.useSkip {
			w.putBool(m.skip, e.skipProb)
		}

		top := e.topB[4*mbX : 4*mbX+4 : 4*mbX+4]

		if m.i4x4 {
			w.putBit(0, 145)

			for n := range 16 {
				x, y := n&3, n>>2
				mode := m.imodes[n]

				putBMode(w, &bModeProbs[top[x]][e.leftB[y]], mode)

				top[x] = mode
				e.leftB[y] = mode
			}
		} else {
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

			for j := range 4 {
				top[j] = m.ymode
				e.leftB[j] = m.ymode
			}
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

	e.putTokenProbs()

	w.putFlag(e.useSkip)

	if e.useSkip {
		w.putBits(uint32(e.skipProb), 8)
	}
}

func (e *Encoder) putTokenProbs() {
	w := &e.hdr

	for t := range numBlockTypes {
		for b := range numBands {
			for c := range numCtx {
				for p := range numProbas {
					fresh := e.probaNew[slotIndex(t, bandFirst[b], c)+p]

					if fresh == 0 {
						w.putBit(0, coeffUpdateProbs[t][b][c][p])

						continue
					}

					w.putBit(1, coeffUpdateProbs[t][b][c][p])
					w.putBits(uint32(fresh), 8)
				}
			}
		}
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

func (e *Encoder) Release() {
	e.src = nil
	e.out = e.out[:0]
}

// Encode writes src as a key frame. Every reference buffer is refreshed, so the
// frame stands alone and any following inter frame predicts from it.
func (e *Encoder) Encode(src *Picture, o EncodeOptions) ([]byte, error) {
	return e.encodeFrame(src, o, true)
}

// EncodeInter writes src as an inter frame predicted from the frame last
// encoded. It fails with [ErrInvalid] before any key frame, or if src does not
// have the dimensions of that key frame.
func (e *Encoder) EncodeInter(src *Picture, o EncodeOptions) ([]byte, error) {
	return e.encodeFrame(src, o, false)
}

func (e *Encoder) encodeFrame(src *Picture, o EncodeOptions, key bool) ([]byte, error) {
	if src.Width <= 0 || src.Height <= 0 || src.Width >= maxDimension || src.Height >= maxDimension {
		return nil, ErrInvalid
	}

	if !key && (e.rec.allocW != src.Width || e.rec.allocH != src.Height) {
		return nil, ErrInvalid
	}

	e.src = src
	e.mbW = (src.Width + 15) / 16
	e.mbH = (src.Height + 15) / 16
	e.keyFrame = key

	if key {
		e.rec.lastIdx, e.rec.goldenIdx, e.rec.altIdx = 0, 0, 0
		e.rec.refCnt = [numFrameBuffers]int{1, 0, 0, 0}
	}

	e.setup(o)
	e.allocFrame()

	if e.keyFrame {
		e.rec.resetEntropy()
		e.proba.reset()
	}

	saved := e.proba.bands

	var header []byte

	for {
		e.alloc()
		e.proba.bands = saved
		e.tokens.reset()

		e.encodeRows()

		e.updateProbas()
		e.skipProbability()
		e.interProbability()

		e.hdr.init(e.hdr.buf)

		if e.keyFrame {
			e.putHeader()
			e.putModes()
		} else {
			e.putInterHeader()
			e.putInterModes()
		}

		header = e.hdr.finish()

		if len(header) < e.p0Limit {
			break
		}

		if !e.tryI4 {
			return nil, ErrUnsupported
		}

		e.i4HeaderBits = min(e.i4HeaderBits>>1, 2048*(e.p0Limit*7/8)/(e.mbW*e.mbH))
		if e.i4HeaderBits < minI4HeaderBits {
			e.tryI4 = false
		}
	}

	e.tok.init(e.tok.buf)
	e.replayTokens()

	tokens := e.tok.finish()

	tag := uint32(1<<4 | len(header)<<5)
	if !e.keyFrame {
		tag |= 1
	}

	e.rec.refreshGolden = true
	e.rec.refreshAlt = true
	e.rec.refreshLast = true
	e.rec.copyGolden = 0
	e.rec.copyAlt = 0
	e.rec.rotateBuffers()

	out := e.out[:0]

	out = append(out, byte(tag), byte(tag>>8), byte(tag>>16))

	if e.keyFrame {
		out = append(out, startCode[0], startCode[1], startCode[2])
		out = append(out, byte(src.Width), byte(src.Width>>8), byte(src.Height), byte(src.Height>>8))
	}

	out = append(out, header...)
	out = append(out, tokens...)

	e.out = out

	return out, nil
}

type mbLevels struct {
	levels [y2Block + 1][16]int16
	nz     [y2Block + 1]int
}
