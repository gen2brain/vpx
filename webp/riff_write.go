package webp

import (
	"encoding/binary"
	"io"
	"slices"
)

type chunkOut struct {
	id      string
	payload []byte
}

func putUint24(b []byte, v int) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
}

func appendChunk(b []byte, id string, payload []byte) []byte {
	var hdr [chunkHeaderSize]byte

	copy(hdr[:], id)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(len(payload)))

	b = append(b, hdr[:]...)
	b = append(b, payload...)

	if len(payload)&1 == 1 {
		b = append(b, 0)
	}

	return b
}

func writeFile(w io.Writer, hdr []byte, chunks []chunkOut) error {
	size := 4

	for _, c := range chunks {
		if len(c.payload) > maxChunkPayload {
			return ErrEncode
		}

		size += chunkHeaderSize + len(c.payload) + len(c.payload)&1
	}

	copy(hdr[0:], fccRIFF)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(size))
	copy(hdr[8:], fccWEBP)

	if _, err := w.Write(hdr[:riffHeaderSize]); err != nil {
		return err
	}

	for _, c := range chunks {
		copy(hdr[0:], c.id)
		binary.LittleEndian.PutUint32(hdr[4:], uint32(len(c.payload)))

		if _, err := w.Write(hdr[:chunkHeaderSize]); err != nil {
			return err
		}

		if _, err := w.Write(c.payload); err != nil {
			return err
		}

		if len(c.payload)&1 == 1 {
			if _, err := w.Write(hdr[:1]); err != nil {
				return err
			}
		}
	}

	return nil
}

func vp8xPayload(b []byte, width, height int, flags byte) []byte {
	b = b[:vp8xPayloadSize]

	clear(b)

	b[0] = flags

	putUint24(b[4:7], width-1)
	putUint24(b[7:10], height-1)

	return b
}

func animPayload(b []byte, loopCount int) []byte {
	b = b[:animPayloadSize]

	clear(b)

	binary.LittleEndian.PutUint16(b[4:6], uint16(loopCount))

	return b
}

func appendANMF(dst []byte, f frame, chunks []chunkOut) []byte {
	off := len(dst)

	dst = slices.Grow(dst, anmfHeaderSize)[:off+anmfHeaderSize]

	b := dst[off:]

	clear(b)

	putUint24(b[0:3], f.x/2)
	putUint24(b[3:6], f.y/2)
	putUint24(b[6:9], f.w-1)
	putUint24(b[9:12], f.h-1)
	putUint24(b[12:15], f.duration)

	if !f.blend {
		b[15] |= 2
	}

	if f.dispose {
		b[15] |= 1
	}

	for _, c := range chunks {
		dst = appendChunk(dst, c.id, c.payload)
	}

	return dst
}
