package webp

import (
	"cmp"
	"math/bits"
	"slices"
)

const codeLengthDepthLimit = 7

type huffCode struct {
	lengths []uint8
	codes   []uint16
}

type huffNode struct {
	count uint32
	value int32
	left  int32
	right int32
}

type huffToken struct {
	code  uint8
	extra uint8
}

type huffBuilder struct {
	nodes  []huffNode
	rle    []uint8
	tokens []huffToken
	clHist [codeLengthCodes]uint32
	clLen  [codeLengthCodes]uint8
	clCode [codeLengthCodes]uint16
}

func (b *huffBuilder) build(hist []uint32, limit int, c *huffCode) {
	n := len(hist)

	if cap(b.rle) < n {
		b.rle = make([]uint8, n)
	}

	b.rle = b.rle[:n]
	clear(b.rle)

	if cap(b.nodes) < 3*n {
		b.nodes = make([]huffNode, 3*n)
	}

	b.nodes = b.nodes[:3*n]

	optimizeForRle(hist, b.rle)
	generateTree(hist, limit, b.nodes, c.lengths)
	assignCodes(c.lengths, c.codes)
}

func collapseToAverage(a, b uint32) bool {
	d := int(a) - int(b)

	return d < 4 && d > -4
}

func optimizeForRle(counts []uint32, goodForRle []uint8) {
	length := len(counts)

	for {
		if length == 0 {
			return
		}

		if counts[length-1] != 0 {
			break
		}

		length--
	}

	symbol := counts[0]
	stride := 0

	for i := range length + 1 {
		if i == length || counts[i] != symbol {
			if (symbol == 0 && stride >= 5) || (symbol != 0 && stride >= 7) {
				for k := range stride {
					goodForRle[i-k-1] = 1
				}
			}

			stride = 1

			if i != length {
				symbol = counts[i]
			}
		} else {
			stride++
		}
	}

	run := 0
	limit := counts[0]
	sum := uint32(0)

	for i := range length + 1 {
		if i == length || goodForRle[i] != 0 || (i != 0 && goodForRle[i-1] != 0) ||
			!collapseToAverage(counts[i], limit) {
			if run >= 4 || (run >= 3 && sum == 0) {
				count := (sum + uint32(run)/2) / uint32(run)

				if count == 0 {
					count = 1
				}

				if sum == 0 {
					count = 0
				}

				for k := range run {
					counts[i-k-1] = count
				}
			}

			run = 0
			sum = 0

			switch {
			case i < length-3:
				limit = (counts[i] + counts[i+1] + counts[i+2] + counts[i+3] + 2) / 4
			case i < length:
				limit = counts[i]
			default:
				limit = 0
			}
		}

		run++

		if i != length {
			sum += counts[i]

			if run >= 4 {
				limit = (sum + uint32(run)/2) / uint32(run)
			}
		}
	}
}

func setBitDepths(node *huffNode, pool []huffNode, depths []uint8, level int) {
	if node.left < 0 {
		depths[node.value] = uint8(level)

		return
	}

	setBitDepths(&pool[node.left], pool, depths, level+1)
	setBitDepths(&pool[node.right], pool, depths, level+1)
}

func generateTree(hist []uint32, limit int, nodes []huffNode, depths []uint8) {
	clear(depths)

	size := 0
	most := uint32(0)

	for _, c := range hist {
		if c != 0 {
			size++
			most = max(most, c)
		}
	}

	if size == 0 {
		return
	}

	tree, pool := nodes[:size], nodes[size:]

	for countMin := uint32(1); ; countMin *= 2 {
		clear(depths)

		n := 0

		for j, c := range hist {
			if c == 0 {
				continue
			}

			tree[n] = huffNode{count: max(c, countMin), value: int32(j), left: -1, right: -1}
			n++
		}

		slices.SortFunc(tree, func(x, y huffNode) int {
			if c := cmp.Compare(y.count, x.count); c != 0 {
				return c
			}

			return cmp.Compare(x.value, y.value)
		})

		if n == 1 {
			depths[tree[0].value] = 1

			return
		}

		np := 0

		for n > 1 {
			pool[np] = tree[n-1]
			np++
			pool[np] = tree[n-2]
			np++

			count := pool[np-1].count + pool[np-2].count
			n -= 2

			k := 0

			for ; k < n; k++ {
				if tree[k].count <= count {
					break
				}
			}

			copy(tree[k+1:n+1], tree[k:n])
			tree[k] = huffNode{count: count, value: -1, left: int32(np - 1), right: int32(np - 2)}
			n++
		}

		setBitDepths(&tree[0], pool, depths, 0)

		depth := 0

		for _, d := range depths {
			depth = max(depth, int(d))
		}

		if depth <= limit || countMin >= most {
			return
		}
	}
}

func reverseBits(n uint8, v uint16) uint16 {
	return bits.Reverse16(v) >> (16 - n)
}

func assignCodes(lengths []uint8, codes []uint16) {
	var hist [maxCodeLength + 1]uint16

	for _, l := range lengths {
		hist[l]++
	}

	hist[0] = 0

	var next [maxCodeLength + 1]uint16

	code := uint16(0)

	for i := 1; i <= maxCodeLength; i++ {
		code = (code + hist[i-1]) << 1
		next[i] = code
	}

	for i, l := range lengths {
		codes[i] = reverseBits(l, next[l])
		next[l]++
	}
}

func codeRepeatedValues(tokens []huffToken, reps int, value, prev uint8) []huffToken {
	if value != prev {
		tokens = append(tokens, huffToken{code: value})
		reps--
	}

	for reps >= 1 {
		switch {
		case reps < 3:
			for range reps {
				tokens = append(tokens, huffToken{code: value})
			}

			return tokens
		case reps < 7:
			return append(tokens, huffToken{code: 16, extra: uint8(reps - 3)})
		default:
			tokens = append(tokens, huffToken{code: 16, extra: 3})
			reps -= 6
		}
	}

	return tokens
}

func codeRepeatedZeros(tokens []huffToken, reps int) []huffToken {
	for reps >= 1 {
		switch {
		case reps < 3:
			for range reps {
				tokens = append(tokens, huffToken{})
			}

			return tokens
		case reps < 11:
			return append(tokens, huffToken{code: 17, extra: uint8(reps - 3)})
		case reps < 139:
			return append(tokens, huffToken{code: 18, extra: uint8(reps - 11)})
		default:
			tokens = append(tokens, huffToken{code: 18, extra: 0x7f})
			reps -= 138
		}
	}

	return tokens
}

func (b *huffBuilder) treeTokens(lengths []uint8) []huffToken {
	tokens := b.tokens[:0]
	prev := uint8(8)

	for i := 0; i < len(lengths); {
		v := lengths[i]
		k := i + 1

		for k < len(lengths) && lengths[k] == v {
			k++
		}

		if v == 0 {
			tokens = codeRepeatedZeros(tokens, k-i)
		} else {
			tokens = codeRepeatedValues(tokens, k-i, v, prev)
			prev = v
		}

		i = k
	}

	b.tokens = tokens

	return tokens
}

var codeLengthExtraBits = [codeLengthCodes]uint{16: 2, 17: 3, 18: 7}

func (b *huffBuilder) storeCode(w *lbitWriter, c *huffCode) {
	var symbols [2]int

	count := 0

	for i, l := range c.lengths {
		if l == 0 {
			continue
		}

		if count < 2 {
			symbols[count] = i
		}

		count++

		if count == 3 {
			break
		}
	}

	switch {
	case count == 0:
		w.write(0x01, 4)
	case count <= 2 && symbols[0] < 1<<8 && symbols[1] < 1<<8:
		w.write(1, 1)
		w.write(uint32(count-1), 1)

		if symbols[0] <= 1 {
			w.write(0, 1)
			w.write(uint32(symbols[0]), 1)
		} else {
			w.write(1, 1)
			w.write(uint32(symbols[0]), 8)
		}

		if count == 2 {
			w.write(uint32(symbols[1]), 8)
		}
	default:
		b.storeFullCode(w, c)
	}
}

func (b *huffBuilder) storeFullCode(w *lbitWriter, c *huffCode) {
	w.write(0, 1)

	tokens := b.treeTokens(c.lengths)

	clear(b.clHist[:])

	for _, t := range tokens {
		b.clHist[t.code]++
	}

	cl := huffCode{lengths: b.clLen[:], codes: b.clCode[:]}

	b.build(b.clHist[:], codeLengthDepthLimit, &cl)

	toStore := codeLengthCodes
	for toStore > 4 && cl.lengths[codeLengthOrder[toStore-1]] == 0 {
		toStore--
	}

	w.write(uint32(toStore-4), 4)

	for i := range toStore {
		w.write(uint32(cl.lengths[codeLengthOrder[i]]), 3)
	}

	clearIfSingle(&cl)

	trimmed := len(tokens)
	zeroBits := 0

	for i := len(tokens) - 1; i >= 0; i-- {
		ix := tokens[i].code

		if ix != 0 && ix != 17 && ix != 18 {
			break
		}

		trimmed--
		zeroBits += int(cl.lengths[ix]) + int(codeLengthExtraBits[ix])
	}

	writeTrimmed := trimmed > 1 && zeroBits > 12
	length := len(tokens)

	w.write(b2u(writeTrimmed), 1)

	if writeTrimmed {
		length = trimmed

		if trimmed == 2 {
			w.write(0, 3+2)
		} else {
			pairs := (bits.Len32(uint32(trimmed-2))-1)/2 + 1

			w.write(uint32(pairs-1), 3)
			w.write(uint32(trimmed-2), uint(2*pairs))
		}
	}

	for _, t := range tokens[:length] {
		ix := t.code

		w.write(uint32(cl.codes[ix]), uint(cl.lengths[ix]))
		w.write(uint32(t.extra), codeLengthExtraBits[ix])
	}
}

func clearIfSingle(c *huffCode) {
	count := 0

	for _, l := range c.lengths {
		if l != 0 {
			count++

			if count > 1 {
				return
			}
		}
	}

	clear(c.lengths)
	clear(c.codes)
}

func b2u(b bool) uint32 {
	if b {
		return 1
	}

	return 0
}
