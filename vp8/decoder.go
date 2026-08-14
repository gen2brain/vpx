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

	useSkipProb bool
	skipProb    uint8

	numParts int
	parts    [maxPartitions]boolDec
	br       boolDec

	mbW, mbH int

	filterType int
	fStrengths [numSegments][2]fInfo
	fInfoRow   []uint8

	// SizeLimit bounds the pixel area a frame may allocate. Zero means
	// DefaultFrameSizeLimit.
	SizeLimit int

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

	if !h.Show {
		return ErrUnsupported
	}

	limit := d.SizeLimit
	if limit <= 0 {
		limit = DefaultFrameSizeLimit
	}

	if h.Width*h.Height > limit {
		return ErrUnsupported
	}

	d.hdr = h
	d.mbW = (h.Width + 15) / 16
	d.mbH = (h.Height + 15) / 16

	b := data[h.size():]
	if h.PartSize > len(b) {
		return ErrInvalid
	}

	d.br.init(b[:h.PartSize])

	d.colorSpace = int(d.br.getBits(1))
	d.clampType = int(d.br.getBits(1))

	d.seg = segmentHeader{}
	d.proba.segments = [numSegments - 1]uint8{255, 255, 255}

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

	d.br.getFlag()

	d.parseProba()

	if d.br.eof {
		return ErrInvalid
	}

	d.precomputeFilterStrengths()

	return nil
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
					v := coeffProbs[t][b][c][p]
					if d.br.getBit(coeffUpdateProbs[t][b][c][p]) != 0 {
						v = uint8(d.br.getBits(8))
					}

					d.proba.bands[t][b][c][p] = v
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
