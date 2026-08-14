package vp8

const (
	numSegments     = 4
	numRefLFDeltas  = 4
	numModeLFDeltas = 4
	numBlockTypes   = 4
	numBands        = 8
	numCtx          = 3
	numProbas       = 11
	maxPartitions   = 8
)

type proba struct {
	segments [numSegments - 1]uint8
	bands    [numBlockTypes][numBands]bandProbs
	bandsPtr [numBlockTypes][17]*bandProbs
}

type entropy struct {
	bands   [numBlockTypes][numBands]bandProbs
	mvProbs mvProbs
	yProbs  [4]uint8
	uvProbs [3]uint8
}

func (d *Decoder) saveEntropy() entropy {
	return entropy{
		bands:   d.proba.bands,
		mvProbs: d.mvProbs,
		yProbs:  d.yProbs,
		uvProbs: d.uvProbs,
	}
}

func (d *Decoder) restoreEntropy(e entropy) {
	d.proba.bands = e.bands
	d.mvProbs = e.mvProbs
	d.yProbs = e.yProbs
	d.uvProbs = e.uvProbs
}

func (d *Decoder) resetEntropy() {
	d.proba.reset()

	d.mvProbs = mvProbs(mvDefaultProbs)
	d.yProbs = yModeProbs
	d.uvProbs = uvModeProbs
}

func (p *proba) reset() {
	for t := range numBlockTypes {
		for b := range numBands {
			for c := range numCtx {
				p.bands[t][b][c] = coeffProbs[t][b][c]
			}
		}

		for b := range 17 {
			p.bandsPtr[t][b] = &p.bands[t][coeffBands[b]]
		}
	}
}

// Decoder decodes VP8 frames. The zero value is ready to use, and reusing one
// across frames reuses its buffers.
type Decoder struct {
	hdr    FrameHeader
	seg    segmentHeader
	filter filterHeader
	quant  quantHeader
	dqm    [numSegments]quantMatrix
	proba  proba

	colorSpace int
	clampType  int
	sixtap     bool
	fullPixel  int

	useSkipProb bool
	skipProb    uint8

	numParts int
	parts    [maxPartitions]boolDec
	br       boolDec

	mbW, mbH       int
	width, height  int
	allocW, allocH int

	filterType int
	fStrengths [numSegments][numRefFrames][4]fInfo
	fInfoRow   []uint8

	// SizeLimit bounds the pixel area a frame may allocate. Zero means
	// DefaultFrameSizeLimit.
	SizeLimit int

	frames    [numFrameBuffers]frameBuffer
	refCnt    [numFrameBuffers]int
	lastIdx   int
	goldenIdx int
	altIdx    int
	newIdx    int
	modes     []modeInfo
	segmap    []uint8

	refreshGolden bool
	refreshAlt    bool
	refreshLast   bool
	refreshProbs  bool
	copyGolden    int
	copyAlt       int
	signBias      [numRefFrames]bool
	probIntra     uint8
	probLast      uint8
	probGF        uint8

	mvProbs mvProbs
	yProbs  [4]uint8
	uvProbs [3]uint8
	saved   entropy

	mb         mbData
	mbCtx      []mbCtx
	intraT     []uint8
	intraL     [4]uint8
	topSamples []topSample

	yuv [yuvSize]uint8
	pic Picture
}

// Release drops the decoder's references to the frame it last decoded, so a
// pooled decoder does not keep the caller's input alive. The buffers it
// allocated for the picture are kept.
func (d *Decoder) Release() {
	d.br = boolDec{}

	clear(d.parts[:])
}

func (d *Decoder) parseHeader(data []byte) error {
	h, err := ParseFrameHeader(data)
	if err != nil {
		return err
	}

	if h.KeyFrame {
		limit := d.SizeLimit
		if limit <= 0 {
			limit = DefaultFrameSizeLimit
		}

		if h.Width*h.Height > limit {
			return ErrUnsupported
		}

		d.mbW = (h.Width + 15) / 16
		d.mbH = (h.Height + 15) / 16
		d.width, d.height = h.Width, h.Height
	} else if d.mbW == 0 {
		return ErrInvalid
	}

	h.Width, h.Height = d.width, d.height
	d.hdr = h

	d.sixtap = h.Profile == 0
	d.fullPixel = -1

	if h.Profile == 3 {
		d.fullPixel = ^7
	}

	b := data[h.size():]
	if h.PartSize > len(b) {
		return ErrInvalid
	}

	d.br.init(b[:h.PartSize])

	if h.KeyFrame {
		d.colorSpace = int(d.br.getBits(1))
		d.clampType = int(d.br.getBits(1))

		d.seg = segmentHeader{}
		d.filter = filterHeader{}
		d.proba.segments = [numSegments - 1]uint8{255, 255, 255}
		d.signBias = [numRefFrames]bool{}

		d.resetEntropy()
	}

	d.seg.parse(&d.br, &d.proba)
	d.filter.parse(&d.br)

	if d.br.eof {
		return ErrInvalid
	}

	if err := d.parsePartitions(b[h.PartSize:]); err != nil {
		return err
	}

	d.quant.parse(&d.br)
	d.deriveQuant()

	d.refreshGolden = true
	d.refreshAlt = true
	d.copyGolden = 0
	d.copyAlt = 0

	if !h.KeyFrame {
		d.refreshGolden = d.br.getFlag()
		d.refreshAlt = d.br.getFlag()

		if !d.refreshGolden {
			d.copyGolden = int(d.br.getBits(2))
		}

		if !d.refreshAlt {
			d.copyAlt = int(d.br.getBits(2))
		}

		d.signBias[refGolden] = d.br.getFlag()
		d.signBias[refAltRef] = d.br.getFlag()
	}

	d.refreshProbs = d.br.getFlag()

	if !d.refreshProbs {
		d.saved = d.saveEntropy()
	}

	d.refreshLast = h.KeyFrame || d.br.getFlag()

	d.parseProba()

	if !h.KeyFrame {
		d.parseInterProba()
	}

	if d.br.eof {
		return ErrInvalid
	}

	d.precomputeFilterStrengths()

	return nil
}

func (d *Decoder) parseInterProba() {
	d.probIntra = uint8(d.br.getBits(8))
	d.probLast = uint8(d.br.getBits(8))
	d.probGF = uint8(d.br.getBits(8))

	if d.br.getFlag() {
		for i := range d.yProbs {
			d.yProbs[i] = uint8(d.br.getBits(8))
		}
	}

	if d.br.getFlag() {
		for i := range d.uvProbs {
			d.uvProbs[i] = uint8(d.br.getBits(8))
		}
	}

	d.br.readMVProbs(&d.mvProbs)
}

func (d *Decoder) parsePartitions(b []byte) error {
	d.numParts = 1 << d.br.getBits(2)
	last := d.numParts - 1

	if len(b) < 3*last {
		return ErrInvalid
	}

	sizes, rest := b[:3*last], b[3*last:]

	for p := range last {
		n := int(sizes[3*p]) | int(sizes[3*p+1])<<8 | int(sizes[3*p+2])<<16
		if n > len(rest) {
			n = len(rest)
		}

		d.parts[p].init(rest[:n])
		rest = rest[n:]
	}

	if len(rest) == 0 {
		return ErrInvalid
	}

	d.parts[last].init(rest)

	return nil
}

func (d *Decoder) parseProba() {
	for t := range numBlockTypes {
		for b := range numBands {
			for c := range numCtx {
				for p := range numProbas {
					if d.br.getBit(coeffUpdateProbs[t][b][c][p]) != 0 {
						d.proba.bands[t][b][c][p] = uint8(d.br.getBits(8))
					}
				}
			}
		}

		for b := range 17 {
			d.proba.bandsPtr[t][b] = &d.proba.bands[t][coeffBands[b]]
		}
	}

	d.useSkipProb = d.br.getFlag()
	if d.useSkipProb {
		d.skipProb = uint8(d.br.getBits(8))
	}
}
