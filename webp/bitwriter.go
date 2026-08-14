package webp

import "encoding/binary"

type lbitWriter struct {
	buf  []byte
	bits uint64
	n    uint
}

func (w *lbitWriter) init(buf []byte) {
	w.buf = buf[:0]
	w.bits = 0
	w.n = 0
}

func (w *lbitWriter) write(v uint32, n uint) {
	if w.n >= 32 {
		w.buf = binary.LittleEndian.AppendUint32(w.buf, uint32(w.bits))
		w.bits >>= 32
		w.n -= 32
	}

	w.bits |= (uint64(v) & (1<<n - 1)) << w.n
	w.n += n
}

func (w *lbitWriter) flush() []byte {
	for w.n > 0 {
		w.buf = append(w.buf, byte(w.bits))
		w.bits >>= 8

		if w.n < 8 {
			break
		}

		w.n -= 8
	}

	w.bits = 0
	w.n = 0

	return w.buf
}
