package webp

const (
	vp8lSignature    = 0x2f
	vp8lVersion      = 0
	vp8lHeaderSize   = 5
	vp8lMaxDimension = 1 << 14
)

const (
	greenTree = iota
	redTree
	blueTree
	alphaTree
	distTree
	treesPerGroup
)

const (
	codeLengthCodes  = 19
	numLiteralCodes  = 256
	numLengthCodes   = 24
	numDistanceCodes = 40
	codeToPlaneCodes = 120
)

var alphabetSize = [treesPerGroup]int{
	numLiteralCodes + numLengthCodes,
	numLiteralCodes,
	numLiteralCodes,
	numLiteralCodes,
	numDistanceCodes,
}

type vp8lHeader struct {
	width, height int
	hasAlpha      bool
}

func (r *lbitReader) readVP8LHeader() (vp8lHeader, error) {
	var h vp8lHeader

	h.width = int(r.read(14)) + 1
	h.height = int(r.read(14)) + 1
	h.hasAlpha = r.read(1) != 0

	if r.read(3) != vp8lVersion {
		return h, ErrUnsupported
	}

	if r.overrun() {
		return h, ErrInvalid
	}

	return h, nil
}

func parseVP8LHeader(b []byte) (vp8lHeader, error) {
	if len(b) < vp8lHeaderSize || b[0] != vp8lSignature {
		return vp8lHeader{}, ErrInvalid
	}

	var r lbitReader

	r.init(b[1:])

	return r.readVP8LHeader()
}

type colorCache struct {
	bits uint
	data []uint32
}

func (c *colorCache) index(argb uint32) uint32 {
	return argb * 0x1e35a7bd >> (32 - c.bits)
}

func (c *colorCache) insert(argb uint32) {
	c.data[c.index(argb)] = argb
}

func (c *colorCache) init(bits uint) {
	c.bits = bits

	if cap(c.data) < 1<<bits {
		c.data = make([]uint32, 1<<bits)
	}

	c.data = c.data[:1<<bits]

	clear(c.data)
}

type huffGroup struct {
	trees [treesPerGroup]huffTree
}

type huffmanInfo struct {
	xsize  int
	bits   uint
	mask   int
	image  []uint16
	groups []huffGroup
	cache  *colorCache
}

func (h *huffmanInfo) index(x, y int) int {
	if h.bits == 0 {
		return 0
	}

	return int(h.image[(y>>h.bits)*h.xsize+(x>>h.bits)])
}

type losslessDecoder struct {
	br         lbitReader
	width      int
	height     int
	transforms [numTransforms]*transform
	order      []int
	buf        []uint32
	tmp        []uint32
	arena      huffArena
	lenScratch []uint16
}

func (d *losslessDecoder) reset() {
	d.transforms = [numTransforms]*transform{}
	d.order = d.order[:0]
	d.arena.reset()
}

func (d *losslessDecoder) release() {
	d.reset()
	d.br = lbitReader{}
}

func grow(b []uint32, n int) []uint32 {
	if cap(b) < n {
		return make([]uint32, n)
	}

	return b[:n]
}

func decodeVP8L(d *losslessDecoder, data []byte) ([]uint32, int, int, error) {
	if len(data) < 1 || data[0] != vp8lSignature {
		return nil, 0, 0, ErrInvalid
	}

	d.br.init(data[1:])

	h, err := d.br.readVP8LHeader()
	if err != nil {
		return nil, 0, 0, err
	}

	px, err := d.decode(h.width, h.height)
	if err != nil {
		return nil, 0, 0, err
	}

	return px, h.width, h.height, nil
}

func decodeVP8LAlpha(d *losslessDecoder, data []byte, width, height int) ([]uint32, error) {
	d.br.init(data)

	return d.decode(width, height)
}

func (d *losslessDecoder) decode(width, height int) ([]uint32, error) {
	if width <= 0 || height <= 0 || width > vp8lMaxDimension || height > vp8lMaxDimension {
		return nil, ErrInvalid
	}

	d.reset()

	d.width, d.height = width, height

	xsize, err := d.readTransforms()
	if err != nil {
		return nil, err
	}

	px, err := d.decodeImageStream(xsize, height, true)
	if err != nil {
		return nil, err
	}

	w := xsize

	for i := len(d.order) - 1; i >= 0; i-- {
		t := d.transforms[d.order[i]]

		switch t.kind {
		case predictorTransform:
			applyPredictor(px, w, height, t.bits, t.data)
		case crossColorTransform:
			applyCrossColor(px, w, height, t.bits, t.data)
		case subtractGreenTransform:
			applySubtractGreen(px)
		case colorIndexingTransform:
			d.tmp = grow(d.tmp, d.width*height)
			px = applyColorIndexing(px, d.tmp, d.width, height, t.bits, t.data)
			w = d.width
		}
	}

	if d.br.overrun() {
		return nil, ErrInvalid
	}

	return px, nil
}

func (d *losslessDecoder) readTransforms() (int, error) {
	xsize := d.width

	for d.br.read(1) == 1 {
		kind := int(d.br.read(2))

		if d.transforms[kind] != nil {
			return 0, ErrInvalid
		}

		t := &transform{kind: kind}

		switch kind {
		case predictorTransform, crossColorTransform:
			t.bits = int(d.br.read(3)) + 2

			bw := subSampleSize(xsize, t.bits)
			bh := subSampleSize(d.height, t.bits)

			data, err := d.decodeImageStream(bw, bh, false)
			if err != nil {
				return 0, err
			}

			t.data = data
		case subtractGreenTransform:
		case colorIndexingTransform:
			t.size = int(d.br.read(8)) + 1

			table, err := d.decodeImageStream(t.size, 1, false)
			if err != nil {
				return 0, err
			}

			switch {
			case t.size <= 2:
				t.bits = 3
			case t.size <= 4:
				t.bits = 2
			case t.size <= 16:
				t.bits = 1
			}

			t.data = expandColorMap(table, t.bits)
			xsize = subSampleSize(xsize, t.bits)
		}

		d.transforms[kind] = t
		d.order = append(d.order, kind)

		if d.br.overrun() {
			return 0, ErrInvalid
		}
	}

	return xsize, nil
}

func expandColorMap(table []uint32, bits int) []uint32 {
	out := make([]uint32, 1<<(8>>bits))

	for i := range table {
		if i >= len(out) {
			break
		}

		out[i] = table[i]

		if i > 0 {
			out[i] = addPixels(out[i], out[i-1])
		}
	}

	return out
}

func addPixels(a, b uint32) uint32 {
	alpha := (a & 0xff000000) + (b & 0xff000000)
	red := (a & 0x00ff0000) + (b & 0x00ff0000)
	green := (a & 0x0000ff00) + (b & 0x0000ff00)
	blue := (a & 0x000000ff) + (b & 0x000000ff)

	return alpha&0xff000000 | red&0x00ff0000 | green&0x0000ff00 | blue&0x000000ff
}

func (d *losslessDecoder) decodeImageStream(xsize, ysize int, isARGB bool) ([]uint32, error) {
	if xsize <= 0 || ysize <= 0 {
		return nil, ErrInvalid
	}

	cache, err := d.readColorCache()
	if err != nil {
		return nil, err
	}

	info, err := d.readHuffmanCodes(isARGB, xsize, ysize, cache)
	if err != nil {
		return nil, err
	}

	var px []uint32

	if isARGB {
		d.buf = grow(d.buf, xsize*ysize)
		px = d.buf
	} else {
		px = make([]uint32, xsize*ysize)
	}

	if err := d.decodeImageData(xsize, ysize, info, px); err != nil {
		return nil, err
	}

	return px, nil
}

func (d *losslessDecoder) readColorCache() (*colorCache, error) {
	if d.br.read(1) == 0 {
		return nil, nil
	}

	bits := uint(d.br.read(4))
	if bits < 1 || bits > 11 {
		return nil, ErrInvalid
	}

	return &colorCache{bits: bits, data: make([]uint32, 1<<bits)}, nil
}

func (d *losslessDecoder) readHuffmanCodes(readMeta bool, xsize, ysize int, cache *colorCache) (*huffmanInfo, error) {
	info := &huffmanInfo{cache: cache, mask: -1}
	groups := 1

	if readMeta && d.br.read(1) == 1 {
		info.bits = uint(d.br.read(3)) + 2
		info.xsize = subSampleSize(xsize, int(info.bits))
		info.mask = 1<<info.bits - 1

		data, err := d.decodeImageStream(info.xsize, subSampleSize(ysize, int(info.bits)), false)
		if err != nil {
			return nil, err
		}

		info.image = make([]uint16, len(data))

		for i, v := range data {
			code := uint16(v >> 8)
			info.image[i] = code

			if int(code)+1 > groups {
				groups = int(code) + 1
			}
		}
	}

	if d.br.overrun() {
		return nil, ErrInvalid
	}

	info.groups = make([]huffGroup, groups)

	for g := range info.groups {
		for j := range treesPerGroup {
			size := alphabetSize[j]
			if j == greenTree && cache != nil {
				size += 1 << cache.bits
			}

			tree, err := d.readHuffmanCode(size)
			if err != nil {
				return nil, err
			}

			info.groups[g].trees[j] = tree
		}
	}

	return info, nil
}

func (d *losslessDecoder) readHuffmanCode(size int) (huffTree, error) {
	if d.br.read(1) == 1 {
		n := d.br.read(1) + 1
		wide := d.br.read(1)

		zero := uint16(d.br.read(uint(1 + 7*wide)))
		if int(zero) >= size {
			return huffTree{}, ErrInvalid
		}

		if n == 1 {
			return singleTree(zero), nil
		}

		one := uint16(d.br.read(8))
		if int(one) >= size {
			return huffTree{}, ErrInvalid
		}

		return d.arena.twoTree(zero, one), nil
	}

	var codeLengths [codeLengthCodes]uint16

	n := 4 + int(d.br.read(4))
	for i := range n {
		codeLengths[codeLengthOrder[i]] = uint16(d.br.read(3))
	}

	lengths, err := d.readCodeLengths(codeLengths[:], size)
	if err != nil {
		return huffTree{}, err
	}

	return d.arena.buildTree(lengths)
}

var (
	repeatExtraBits = [3]uint{2, 3, 7}
	repeatOffsets   = [3]int{3, 3, 11}
)

func (d *losslessDecoder) readCodeLengths(codeLengths []uint16, size int) ([]uint16, error) {
	table, err := d.arena.buildTree(codeLengths)
	if err != nil {
		return nil, err
	}

	maxSymbol := size

	if d.br.read(1) == 1 {
		bits := uint(2 + 2*d.br.read(3))

		v := int(d.br.read(bits))
		if v > size-2 {
			return nil, ErrInvalid
		}

		maxSymbol = 2 + v
	}

	if cap(d.lenScratch) < size {
		d.lenScratch = make([]uint16, size)
	}

	lengths := d.lenScratch[:size]
	clear(lengths)

	prev := uint16(8)
	symbol := 0

	for symbol < size && maxSymbol > 0 {
		maxSymbol--

		d.br.fill()

		if d.br.overrun() {
			return nil, ErrInvalid
		}

		code := table.readSymbol(&d.br)

		if code < 16 {
			lengths[symbol] = code
			symbol++

			if code != 0 {
				prev = code
			}

			continue
		}

		slot := int(code) - 16
		if slot > 2 {
			return nil, ErrInvalid
		}

		repeat := int(d.br.read(repeatExtraBits[slot])) + repeatOffsets[slot]
		if symbol+repeat > size {
			return nil, ErrInvalid
		}

		length := uint16(0)
		if code == 16 {
			length = prev
		}

		for range repeat {
			lengths[symbol] = length
			symbol++
		}
	}

	return lengths, nil
}

func prefixValue(r *lbitReader, code int) int {
	if code < 4 {
		return code + 1
	}

	extra := uint((code - 2) >> 1)
	offset := (2 + code&1) << extra

	return offset + int(r.read(extra)) + 1
}

func planeCodeToDistance(xsize, code int) int {
	if code > codeToPlaneCodes {
		return code - codeToPlaneCodes
	}

	p := int(codeToPlane[code-1])
	dist := (p>>4)*xsize + 8 - p&0xf

	return max(dist, 1)
}

func (d *losslessDecoder) decodeImageData(width, height int, info *huffmanInfo, px []uint32) error {
	n := width * height
	i := 0

	var group *huffGroup

	cache := info.cache
	nextBlock := 0
	x, y := 0, 0

	if info.bits == 0 {
		group = &info.groups[0]
		nextBlock = n
	}

	for i < n {
		d.br.fill()

		if d.br.overrun() {
			return ErrInvalid
		}

		if i >= nextBlock {
			nextBlock = min(x|info.mask, width-1) + y*width + 1
			group = &info.groups[info.index(x, y)]
		}

		code := group.trees[greenTree].readSymbol(&d.br)

		switch {
		case code < numLiteralCodes:
			red := group.trees[redTree].readSymbol(&d.br)
			blue := group.trees[blueTree].readSymbol(&d.br)

			if d.br.n < 16 {
				d.br.fill()
			}

			alpha := group.trees[alphaTree].readSymbol(&d.br)

			argb := uint32(alpha)<<24 | uint32(red)<<16 | uint32(code)<<8 | uint32(blue)
			px[i] = argb
			i++

			x++
			if x == width {
				x, y = 0, y+1
			}

			if cache != nil {
				cache.insert(argb)
			}
		case int(code) < numLiteralCodes+numLengthCodes:
			length := prefixValue(&d.br, int(code)-numLiteralCodes)

			distSym := group.trees[distTree].readSymbol(&d.br)
			dist := planeCodeToDistance(width, prefixValue(&d.br, int(distSym)))

			if i < dist || n-i < length {
				return ErrInvalid
			}

			dst := px[i : i+length]

			if dist >= length {
				copy(dst, px[i-dist:])
			} else {
				copy(dst, px[i-dist:i])

				for k := dist; k < length; k *= 2 {
					copy(dst[k:], dst[:min(k, length-k)])
				}
			}

			if cache != nil {
				shift := 32 - cache.bits
				data := cache.data

				for _, v := range dst {
					data[v*0x1e35a7bd>>shift] = v
				}
			}

			i += length

			x += length
			if x >= width {
				y += x / width
				x %= width
			}
		default:
			if cache == nil {
				return ErrInvalid
			}

			key := int(code) - numLiteralCodes - numLengthCodes
			if key >= len(cache.data) {
				return ErrInvalid
			}

			px[i] = cache.data[key]
			i++

			x++
			if x == width {
				x, y = 0, y+1
			}
		}
	}

	return nil
}
