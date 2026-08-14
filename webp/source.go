package webp

import (
	"encoding/binary"
	"errors"
	"io"
)

type source struct {
	b    []byte
	r    io.ReaderAt
	size int
	// hdr backs header reads, which are consumed before the next one is made.
	hdr [anmfHeaderSize]byte
}

func memSource(b []byte) *source {
	return &source{b: b, size: len(b)}
}

func srcFor(r io.Reader) (*source, error) {
	ra, raOK := r.(io.ReaderAt)
	sk, skOK := r.(io.Seeker)

	if raOK && skOK {
		cur, err1 := sk.Seek(0, io.SeekCurrent)
		end, err2 := sk.Seek(0, io.SeekEnd)

		if err1 == nil && err2 == nil && end > cur && end-cur <= maxSourceSize {
			n := int(end - cur)

			return &source{r: io.NewSectionReader(ra, cur, int64(n)), size: n}, nil
		}
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return memSource(data), nil
}

// header reads a fixed size field into the source's own scratch, so walking a
// file costs no allocations. The result is only valid until the next call.
func (s *source) header(off, n int) ([]byte, error) {
	if s.r == nil || n > len(s.hdr) {
		return s.at(off, n)
	}

	if off < 0 || off > s.size || n > s.size-off {
		return nil, ErrInvalid
	}

	buf := s.hdr[:n]

	if err := s.readAt(buf, off); err != nil {
		return nil, err
	}

	return buf, nil
}

func (s *source) readAt(p []byte, off int) error {
	n, err := s.r.ReadAt(p, int64(off))
	if n == len(p) {
		return nil
	}

	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrInvalid
	}

	return err
}

func (s *source) at(off, n int) ([]byte, error) {
	if off < 0 || n < 0 || off > s.size || n > s.size-off {
		return nil, ErrInvalid
	}

	if s.r == nil {
		return s.b[off : off+n], nil
	}

	buf := make([]byte, n)

	if err := s.readAt(buf, off); err != nil {
		return nil, err
	}

	return buf, nil
}

func (s *source) nextChunk(off, end int) (chunk, int, error) {
	if off > end-chunkHeaderSize {
		return chunk{}, 0, ErrInvalid
	}

	h, err := s.header(off, chunkHeaderSize)
	if err != nil {
		return chunk{}, 0, err
	}

	size := binary.LittleEndian.Uint32(h[4:8])
	if uint64(size) > maxChunkPayload || uint64(size) > uint64(end-off-chunkHeaderSize) {
		return chunk{}, 0, ErrInvalid
	}

	c := chunk{off: off + chunkHeaderSize, size: int(size)}
	copy(c.id[:], h[:4])

	adv := c.off + c.size
	if adv < end {
		adv += c.size & 1
	}

	return c, adv, nil
}
