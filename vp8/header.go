package vp8

const (
	frameTagSize    = 3
	keyFrameHdrSize = 7
	maxDimension    = 1 << 14
)

var startCode = [3]byte{0x9d, 0x01, 0x2a}

// FrameHeader is the part of a frame header that is not boolean coded: the
// frame tag of RFC 6386 §9.1, and on a key frame the picture header of §9.2.
type FrameHeader struct {
	KeyFrame bool
	Profile  int
	Show     bool
	PartSize int
	Width    int
	Height   int
	XScale   int
	YScale   int
}

func (h FrameHeader) size() int {
	if h.KeyFrame {
		return frameTagSize + keyFrameHdrSize
	}

	return frameTagSize
}

type segmentHeader struct {
	enabled        bool
	updateMap      bool
	absoluteDelta  bool
	quantizer      [numSegments]int8
	filterStrength [numSegments]int8
}

func (h *segmentHeader) parse(d *boolDec, p *proba) {
	h.enabled = d.getFlag()
	if !h.enabled {
		return
	}

	h.updateMap = d.getFlag()

	if d.getFlag() {
		h.absoluteDelta = d.getFlag()

		for i := range numSegments {
			h.quantizer[i] = int8(d.flagged(7))
		}

		for i := range numSegments {
			h.filterStrength[i] = int8(d.flagged(6))
		}
	}

	if !h.updateMap {
		return
	}

	for i := range p.segments {
		p.segments[i] = 255
		if d.getFlag() {
			p.segments[i] = uint8(d.getBits(8))
		}
	}
}

type filterHeader struct {
	simple    bool
	level     int
	sharpness int
	useDelta  bool
	refDelta  [numRefLFDeltas]int8
	modeDelta [numModeLFDeltas]int8
}

func (h *filterHeader) parse(d *boolDec) {
	*h = filterHeader{}

	h.simple = d.getFlag()
	h.level = int(d.getBits(6))
	h.sharpness = int(d.getBits(3))
	h.useDelta = d.getFlag()

	if !h.useDelta || !d.getFlag() {
		return
	}

	for i := range h.refDelta {
		if d.getFlag() {
			h.refDelta[i] = int8(d.getSignedBits(6))
		}
	}

	for i := range h.modeDelta {
		if d.getFlag() {
			h.modeDelta[i] = int8(d.getSignedBits(6))
		}
	}
}

// ParseFrameHeader reads the uncompressed header off the front of a frame: the
// frame tag every frame carries, and the picture header a key frame adds. An
// inter frame parses to a header with no dimensions, because it has none; only
// [Decoder.DecodeFrame] refuses to decode one.
func ParseFrameHeader(b []byte) (FrameHeader, error) {
	var h FrameHeader

	if len(b) < frameTagSize {
		return h, ErrInvalid
	}

	bits := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16

	h.KeyFrame = bits&1 == 0
	h.Profile = int(bits>>1) & 7
	h.Show = bits>>4&1 != 0
	h.PartSize = int(bits >> 5)

	if h.Profile > 3 {
		return h, ErrInvalid
	}

	if !h.KeyFrame {
		return h, nil
	}

	if len(b) < frameTagSize+keyFrameHdrSize {
		return h, ErrInvalid
	}

	b = b[frameTagSize:]

	if b[0] != startCode[0] || b[1] != startCode[1] || b[2] != startCode[2] {
		return h, ErrInvalid
	}

	h.Width = int(uint16(b[3])|uint16(b[4])<<8) & 0x3fff
	h.XScale = int(b[4] >> 6)
	h.Height = int(uint16(b[5])|uint16(b[6])<<8) & 0x3fff
	h.YScale = int(b[6] >> 6)

	if h.Width == 0 || h.Height == 0 {
		return h, ErrInvalid
	}

	return h, nil
}
