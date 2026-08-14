package webp

import (
	"encoding/binary"
	"errors"
)

// ErrInvalid is returned for a file that is not a well formed WebP.
var ErrInvalid = errors.New("webp: invalid file")

// ErrUnsupported is returned for a file this package cannot decode but which is
// otherwise well formed. A caller with another decoder to fall back on should
// test for this one rather than for [ErrInvalid].
var ErrUnsupported = errors.New("webp: unsupported feature")

const (
	fccRIFF = "RIFF"
	fccWEBP = "WEBP"
	fccVP8  = "VP8 "
	fccVP8L = "VP8L"
	fccVP8X = "VP8X"
	fccALPH = "ALPH"
	fccANIM = "ANIM"
	fccANMF = "ANMF"
	fccICCP = "ICCP"
	fccEXIF = "EXIF"
	fccXMP  = "XMP "
)

const (
	flagAnimation = 1 << 1
	flagAlpha     = 1 << 4
)

const (
	riffHeaderSize  = 12
	chunkHeaderSize = 8
	vp8xPayloadSize = 10
	animPayloadSize = 6
	anmfHeaderSize  = 16
	bitstreamHeader = 16

	maxChunkPayload = 0xfffffffe
	maxCanvasSize   = 1 << 24
	maxImageArea    = 1 << 32
	maxStillArea    = 1 << 28
	maxCanvasArea   = 1 << 26
)

type fourCC [4]byte

func (f fourCC) is(s string) bool { return string(f[:]) == s }

type chunk struct {
	id   fourCC
	off  int
	size int
}

func (c chunk) valid() bool { return c.id != fourCC{} }

type frame struct {
	x, y, w, h int
	duration   int
	blend      bool
	dispose    bool
	alpha      chunk
	image      chunk
}

type container struct {
	src *source

	extended bool
	flags    byte
	width    int
	height   int

	iccp chunk
	exif chunk
	xmp  chunk

	alpha chunk
	image chunk

	loopCount int
	frames    []frame
}

func (c *container) animated() bool { return c.flags&flagAnimation != 0 }

func (c *container) hasAlpha() bool { return c.flags&flagAlpha != 0 }

func (c *container) payload(ch chunk) ([]byte, error) {
	if !ch.valid() {
		return nil, nil
	}

	return c.src.at(ch.off, ch.size)
}

func uint24(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
}

func parse(s *source) (*container, error) {
	hdr, err := s.header(0, riffHeaderSize)
	if err != nil {
		return nil, ErrInvalid
	}

	if string(hdr[0:4]) != fccRIFF || string(hdr[8:12]) != fccWEBP {
		return nil, ErrInvalid
	}

	size := binary.LittleEndian.Uint32(hdr[4:8])
	if size < 4 || uint64(size) > maxChunkPayload {
		return nil, ErrInvalid
	}

	end := s.size
	if n := riffHeaderSize + int(size) - 4; n < end {
		end = n
	}

	c := &container{src: s, loopCount: 1}

	first, next, err := s.nextChunk(riffHeaderSize, end)
	if err != nil {
		return nil, err
	}

	switch string(first.id[:]) {
	case fccVP8, fccVP8L:
		c.image = first

		return c, nil
	case fccVP8X:
		if err := c.parseExtended(first, next, end); err != nil {
			return nil, err
		}

		return c, nil
	}

	return nil, ErrInvalid
}

func (c *container) parseExtended(hdr chunk, off, end int) error {
	if hdr.size < vp8xPayloadSize {
		return ErrInvalid
	}

	b, err := c.src.header(hdr.off, vp8xPayloadSize)
	if err != nil {
		return err
	}

	c.extended = true
	c.flags = b[0]
	c.width = int(uint24(b[4:7])) + 1
	c.height = int(uint24(b[7:10])) + 1

	if c.width > maxCanvasSize || c.height > maxCanvasSize {
		return ErrInvalid
	}

	if uint64(c.width)*uint64(c.height) >= maxImageArea {
		return ErrInvalid
	}

	var alpha chunk

	for off < end {
		ch, next, err := c.src.nextChunk(off, end)
		if err != nil {
			return err
		}

		off = next

		switch string(ch.id[:]) {
		case fccICCP:
			if !c.iccp.valid() {
				c.iccp = ch
			}
		case fccEXIF:
			if !c.exif.valid() {
				c.exif = ch
			}
		case fccXMP:
			if !c.xmp.valid() {
				c.xmp = ch
			}
		case fccANIM:
			if ch.size < animPayloadSize {
				return ErrInvalid
			}

			b, err := c.src.header(ch.off, animPayloadSize)
			if err != nil {
				return err
			}

			c.loopCount = int(binary.LittleEndian.Uint16(b[4:6]))
		case fccANMF:
			f, err := c.parseFrame(ch)
			if err != nil {
				return err
			}

			if c.animated() {
				c.frames = append(c.frames, f)
			}
		case fccALPH:
			if !alpha.valid() {
				alpha = ch
			}
		case fccVP8, fccVP8L:
			if !c.image.valid() {
				c.image = ch
				c.alpha = alpha
			}
		}
	}

	if !c.image.valid() && len(c.frames) == 0 {
		return ErrInvalid
	}

	return nil
}

func (c *container) parseFrame(ch chunk) (frame, error) {
	if ch.size < anmfHeaderSize {
		return frame{}, ErrInvalid
	}

	b, err := c.src.header(ch.off, anmfHeaderSize)
	if err != nil {
		return frame{}, err
	}

	f := frame{
		x:        2 * int(uint24(b[0:3])),
		y:        2 * int(uint24(b[3:6])),
		w:        1 + int(uint24(b[6:9])),
		h:        1 + int(uint24(b[9:12])),
		duration: int(uint24(b[12:15])),
		blend:    b[15]&2 == 0,
		dispose:  b[15]&1 != 0,
	}

	if uint64(f.w)*uint64(f.h) >= maxImageArea {
		return frame{}, ErrInvalid
	}

	off, end := ch.off+anmfHeaderSize, ch.off+ch.size

	for off < end {
		sub, next, err := c.src.nextChunk(off, end)
		if err != nil {
			return frame{}, err
		}

		off = next

		switch string(sub.id[:]) {
		case fccALPH:
			if !f.alpha.valid() {
				f.alpha = sub
			}
		case fccVP8, fccVP8L:
			if !f.image.valid() {
				f.image = sub
			}
		}
	}

	if !f.image.valid() {
		return frame{}, ErrInvalid
	}

	return f, nil
}
