package webp

type lbitReader struct {
	buf  []byte
	pos  int
	bits uint64
	n    uint
	eof  bool
}

func (r *lbitReader) init(b []byte) {
	r.buf = b
	r.pos = 0
	r.bits = 0
	r.n = 0
	r.eof = false
}

func (r *lbitReader) fill() {
	for r.n <= 56 {
		if r.pos < len(r.buf) {
			r.bits |= uint64(r.buf[r.pos]) << r.n
		} else {
			r.eof = true
		}

		r.pos++
		r.n += 8
	}
}

func (r *lbitReader) overrun() bool {
	return 8*r.pos-int(r.n) > 8*len(r.buf)
}

func (r *lbitReader) peek(n uint) uint32 {
	return uint32(r.bits & (1<<n - 1))
}

func (r *lbitReader) consume(n uint) {
	r.bits >>= n
	r.n -= n
}

func (r *lbitReader) read(n uint) uint32 {
	if r.n < n {
		r.fill()
	}

	v := r.peek(n)
	r.consume(n)

	return v
}
