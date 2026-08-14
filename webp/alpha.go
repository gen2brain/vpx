package webp

const alphaHeaderLen = 1

const (
	alphaNoCompression = iota
	alphaLossless
	numAlphaMethods
)

const (
	filterNone = iota
	filterHorizontal
	filterVertical
	filterGradient
	numFilters
)

const alphaPreprocessedLevels = 1

func gradient(a, b, c int) int {
	g := a + b - c

	if g&^0xff == 0 {
		return g
	}

	if g < 0 {
		return 0
	}

	return 255
}

func horizontalUnfilter(prev, in, out []byte) {
	pred := byte(0)
	if prev != nil {
		pred = prev[0]
	}

	for i := range out {
		out[i] = pred + in[i]
		pred = out[i]
	}
}

func verticalUnfilter(prev, in, out []byte) {
	if prev == nil {
		horizontalUnfilter(nil, in, out)

		return
	}

	for i := range out {
		out[i] = prev[i] + in[i]
	}
}

func gradientUnfilter(prev, in, out []byte) {
	if prev == nil {
		horizontalUnfilter(nil, in, out)

		return
	}

	topLeft, left := prev[0], prev[0]

	for i := range out {
		top := prev[i]
		left = in[i] + byte(gradient(int(left), int(top), int(topLeft)))
		topLeft = top
		out[i] = left
	}
}

var unfilters = [numFilters]func(prev, in, out []byte){
	nil, horizontalUnfilter, verticalUnfilter, gradientUnfilter,
}

type alphaHeader struct {
	method        int
	filter        int
	preprocessing int
}

func parseAlphaHeader(b []byte) (alphaHeader, error) {
	var h alphaHeader

	if len(b) <= alphaHeaderLen {
		return h, ErrInvalid
	}

	h.method = int(b[0] & 3)
	h.filter = int(b[0] >> 2 & 3)
	h.preprocessing = int(b[0] >> 4 & 3)

	if b[0]>>6 != 0 || h.method >= numAlphaMethods ||
		h.preprocessing > alphaPreprocessedLevels {
		return h, ErrInvalid
	}

	return h, nil
}

func decodeAlpha(ll *losslessDecoder, chunk, dst []byte, stride, w, h, dither int) error {
	if chunk == nil {
		for i := range dst {
			dst[i] = 0xff
		}

		return nil
	}

	hdr, err := parseAlphaHeader(chunk)
	if err != nil {
		return err
	}

	data := chunk[alphaHeaderLen:]

	if hdr.method == alphaLossless {
		px, err := decodeVP8LAlpha(ll, data, w, h)
		if err != nil {
			return err
		}

		for y := range h {
			row := dst[y*stride : y*stride+w]
			src := px[y*w : y*w+w]

			for x, v := range src {
				row[x] = uint8(v >> 8)
			}
		}
	} else {
		if len(data) < w*h {
			return ErrInvalid
		}

		for y := range h {
			copy(dst[y*stride:y*stride+w], data[y*w:y*w+w])
		}
	}

	if unfilter := unfilters[hdr.filter]; unfilter != nil {
		var prev []byte

		for y := range h {
			row := dst[y*stride : y*stride+w]
			unfilter(prev, row, row)
			prev = row
		}
	}

	if hdr.preprocessing == alphaPreprocessedLevels {
		dequantizeLevels(dst, stride, w, h, dither)
	}

	return nil
}
