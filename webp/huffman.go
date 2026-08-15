package webp

import "math/bits"

const (
	maxCodeLength = 15
	maxTableBits  = 10
	maxSecondary  = 4096
)

type huffTree struct {
	single    bool
	symbol    uint16
	mask      uint16
	primary   []uint16
	secondary []uint16
}

func singleTree(symbol uint16) huffTree {
	return huffTree{single: true, symbol: symbol}
}

func (a *huffArena) twoTree(zero, one uint16) huffTree {
	p := a.alloc(2)
	p[0] = 1<<12 | zero
	p[1] = 1<<12 | one

	return huffTree{mask: 1, primary: p}
}

const arenaChunk = 1 << 14

type huffArena struct {
	chunks [][]uint16
	cur    int
	off    int
	sorted []uint16
	sec    []uint16
}

func (a *huffArena) reset() {
	a.cur = 0
	a.off = 0
}

func (a *huffArena) alloc(n int) []uint16 {
	if n > arenaChunk {
		return make([]uint16, n)
	}

	for a.cur < len(a.chunks) {
		if c := a.chunks[a.cur]; a.off+n <= len(c) {
			s := c[a.off : a.off+n : a.off+n]
			a.off += n

			clear(s)

			return s
		}

		a.cur++
		a.off = 0
	}

	a.chunks = append(a.chunks, make([]uint16, arenaChunk))

	s := a.chunks[a.cur][:n:n]
	a.off = n

	return s
}

func (a *huffArena) scratch(n int) []uint16 {
	if cap(a.sorted) < n {
		a.sorted = make([]uint16, n)
	}

	return a.sorted[:n]
}

func nextCodeword(cw, size uint16) uint16 {
	if cw == size-1 {
		return cw
	}

	adv := uint(15 - bits.LeadingZeros16(cw^(size-1)))
	bit := uint16(1) << adv

	return cw&(bit-1) | bit
}

func (a *huffArena) buildTree(lengths []uint16) (huffTree, error) {
	var t huffTree

	var hist [maxCodeLength + 1]int

	numSymbols := 0

	for _, l := range lengths {
		if l > maxCodeLength {
			return t, ErrInvalid
		}

		hist[l]++

		if l != 0 {
			numSymbols++
		}
	}

	if numSymbols == 0 {
		return t, ErrInvalid
	}

	if numSymbols == 1 {
		for i, l := range lengths {
			if l != 0 {
				return singleTree(uint16(i)), nil
			}
		}
	}

	maxLength := maxCodeLength
	for maxLength > 1 && hist[maxLength] == 0 {
		maxLength--
	}

	var offsets [maxCodeLength + 2]int

	codespace := 0
	offsets[1] = hist[0]

	for i := 1; i < maxLength; i++ {
		offsets[i+1] = offsets[i] + hist[i]
		codespace = codespace<<1 + hist[i]
	}

	codespace = codespace<<1 + hist[maxLength]

	if codespace != 1<<maxLength {
		return t, ErrInvalid
	}

	sorted := a.scratch(len(lengths))
	next := offsets

	for symbol, l := range lengths {
		sorted[next[l]] = uint16(symbol)
		next[l]++
	}

	tableBits := min(maxLength, maxTableBits)
	t.mask = uint16(1)<<tableBits - 1
	t.primary = a.alloc(1 << tableBits)

	codeword := uint16(0)
	i := hist[0]

	for length := 1; length <= tableBits; length++ {
		end := 1 << length

		for range hist[length] {
			t.primary[codeword] = uint16(length)<<12 | sorted[i]
			i++
			codeword = nextCodeword(codeword, uint16(end))
		}

		if length < tableBits {
			copy(t.primary[end:2*end], t.primary[:end])
		}
	}

	if maxLength <= tableBits {
		return t, nil
	}

	if cap(a.sec) < maxSecondary {
		a.sec = make([]uint16, maxSecondary)
	}

	sec := a.sec[:0]

	subStart := 0
	subPrefix := -1

	for length := tableBits + 1; length <= maxLength; length++ {
		subSize := 1 << (length - tableBits)

		for range hist[length] {
			if prefix := int(codeword & t.mask); prefix != subPrefix {
				subPrefix = prefix
				subStart = len(sec)

				if subStart+subSize > maxSecondary {
					return t, ErrInvalid
				}

				t.primary[subPrefix] = uint16(length)<<12 | uint16(subStart)

				sec = sec[:subStart+subSize]
				clear(sec[subStart:])
			}

			sec[subStart+int(codeword>>tableBits)] = sorted[i]<<4 | uint16(length)
			i++
			codeword = nextCodeword(codeword, uint16(1)<<length)
		}

		if length < maxLength && int(codeword&t.mask) == subPrefix {
			grown := len(sec) - subStart

			if len(sec)+grown > maxSecondary {
				return t, ErrInvalid
			}

			sec = append(sec, sec[subStart:]...)
			t.primary[subPrefix] = uint16(length+1)<<12 | uint16(subStart)
		}
	}

	t.secondary = a.alloc(len(sec))
	copy(t.secondary, sec)

	return t, nil
}

func (t *huffTree) readSymbol(r *lbitReader) uint16 {
	if t.single {
		return t.symbol
	}

	v := uint16(r.bits)
	e := t.primary[v&t.mask]

	if e>>12 <= maxTableBits {
		r.consume(uint(e >> 12))

		return e & 0xfff
	}

	length := uint(e >> 12)
	m := uint16(1)<<(length-maxTableBits) - 1
	s := t.secondary[int(e&0xfff)+int(v>>maxTableBits&m)]

	r.consume(uint(s & 0xf))

	return s >> 4
}
