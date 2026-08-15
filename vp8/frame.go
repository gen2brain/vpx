package vp8

import "encoding/binary"

const (
	yuvSize = bps*17 + bps*9
	yOff    = bps + 8
	uOff    = yOff + bps*16 + bps
	vOff    = uOff + 16
)

var scan = [16]int{
	0 + 0*bps, 4 + 0*bps, 8 + 0*bps, 12 + 0*bps,
	0 + 4*bps, 4 + 4*bps, 8 + 4*bps, 12 + 4*bps,
	0 + 8*bps, 4 + 8*bps, 8 + 8*bps, 12 + 8*bps,
	0 + 12*bps, 4 + 12*bps, 8 + 12*bps, 12 + 12*bps,
}

func (d *Decoder) alloc() {
	d.allocFrames()
	d.allocRows()
}

func (d *Decoder) allocFrames() {
	if d.allocW != d.hdr.Width || d.allocH != d.hdr.Height {
		d.allocW, d.allocH = d.hdr.Width, d.hdr.Height

		for i := range d.frames {
			d.frames[i].pic = Picture{}
		}

		d.lastIdx, d.goldenIdx, d.altIdx = 0, 1, 2
		d.refCnt = [numFrameBuffers]int{1, 1, 1, 0}

		n := (d.mbW + 1) * (d.mbH + 1)

		if cap(d.modes) < n {
			d.modes = make([]modeInfo, n)
		}

		d.modes = d.modes[:n]

		clear(d.modes)

		if cap(d.segmap) < d.mbW*d.mbH {
			d.segmap = make([]uint8, d.mbW*d.mbH)
		}

		d.segmap = d.segmap[:d.mbW*d.mbH]

		clear(d.segmap)
	}
}

func (d *Decoder) allocRows() {
	if cap(d.intraT) < 4*d.mbW {
		d.intraT = make([]uint8, 4*d.mbW)
		d.mbCtx = make([]mbCtx, d.mbW+1)
		d.topSamples = make([]topSample, d.mbW)
	}

	d.intraT = d.intraT[:4*d.mbW]
	d.mbCtx = d.mbCtx[:d.mbW+1]
	d.topSamples = d.topSamples[:d.mbW]

	if cap(d.fInfoRow) < d.mbW {
		d.fInfoRow = make([]uint8, d.mbW)
	}

	d.fInfoRow = d.fInfoRow[:d.mbW]

	clear(d.intraT)
	clear(d.mbCtx)
}

func (d *Decoder) initScanline() {
	d.mbCtx[0] = mbCtx{}
	d.intraL = [4]uint8{}
}

func (d *Decoder) initRowContext(mbY int) {
	b := d.yuv[:]

	for j := range 16 {
		b[yOff+j*bps-1] = 129
	}

	for j := range 8 {
		b[uOff+j*bps-1] = 129
		b[vOff+j*bps-1] = 129
	}

	if mbY > 0 {
		b[yOff-1-bps] = 129
		b[uOff-1-bps] = 129
		b[vOff-1-bps] = 129

		return
	}

	fill(b, yOff-bps-1, 16+4+1, 127)
	fill(b, uOff-bps-1, 8+1, 127)
	fill(b, vOff-bps-1, 8+1, 127)
}

func (d *Decoder) rotateLeft() {
	b := &d.yuv

	for j := -1; j < 16; j++ {
		o := yOff + j*bps
		binary.LittleEndian.PutUint32(b[o-4:o], binary.LittleEndian.Uint32(b[o+12:o+16]))
	}

	for j := -1; j < 8; j++ {
		o := uOff + j*bps
		binary.LittleEndian.PutUint32(b[o-4:o], binary.LittleEndian.Uint32(b[o+4:o+8]))

		o = vOff + j*bps
		binary.LittleEndian.PutUint32(b[o-4:o], binary.LittleEndian.Uint32(b[o+4:o+8]))
	}
}

func doTransform(bits uint32, in []int16, b []byte, off int) {
	switch bits >> 30 {
	case 3:
		transformOne(in, b, off)
	case 2:
		transformAC3(in, b, off)
	case 1:
		transformDC(in, b, off)
	}
}

func doUVTransform(bits uint32, in []int16, b []byte, off int) {
	if bits&0xff == 0 {
		return
	}

	if bits&0xaa != 0 {
		transformUV(in, b, off)

		return
	}

	transformDCUV(in, b, off)
}

func (d *Decoder) loadNeighbours(mbX, mbY int) {
	if mbX > 0 {
		d.rotateLeft()
	}

	if mbY == 0 {
		return
	}

	b := d.yuv[:]
	top := &d.topSamples[mbX]

	copy(b[yOff-bps:yOff-bps+16], top.y[:])
	copy(b[uOff-bps:uOff-bps+8], top.u[:])
	copy(b[vOff-bps:vOff-bps+8], top.v[:])
}

func (d *Decoder) reconstruct(m *mbData, mbX, mbY int) {
	d.loadNeighbours(mbX, mbY)
	d.reconstructMB(m, mbX, mbY)
}

func (d *Decoder) reconstructMB(m *mbData, mbX, mbY int) {
	b := d.yuv[:]

	top := &d.topSamples[mbX]

	bits := m.nonZeroY

	if m.inter() {
		d.predictInter(m, mbX, mbY)

		if bits != 0 {
			for n := range 16 {
				doTransform(bits, m.coeffs[n*16:n*16+16], b, yOff+scan[n])
				bits <<= 2
			}
		}
	} else if m.isI4x4 {
		d.fillTopRight(mbX, mbY)

		for n := range 16 {
			off := yOff + scan[n]
			predLuma4[m.imodes[n]](b, off)
			doTransform(bits, m.coeffs[n*16:n*16+16], b, off)
			bits <<= 2
		}
	} else {
		predLuma16[checkMode(mbX, mbY, int(m.imodes[0]))](b, yOff)

		if bits != 0 {
			for n := range 16 {
				doTransform(bits, m.coeffs[n*16:n*16+16], b, yOff+scan[n])
				bits <<= 2
			}
		}
	}

	if !m.inter() {
		uv := checkMode(mbX, mbY, int(m.uvMode))
		predChroma8[uv](b, uOff)
		predChroma8[uv](b, vOff)
	}

	doUVTransform(m.nonZeroUV, m.coeffs[16*16:], b, uOff)
	doUVTransform(m.nonZeroUV>>8, m.coeffs[20*16:], b, vOff)

	if mbY < d.mbH-1 {
		copy(top.y[:], b[yOff+15*bps:yOff+15*bps+16])
		copy(top.u[:], b[uOff+7*bps:uOff+7*bps+8])
		copy(top.v[:], b[vOff+7*bps:vOff+7*bps+8])
	}

	d.emit(mbX, mbY)
}

func (d *Decoder) fillTopRight(mbX, mbY int) {
	b := d.yuv[:]
	off := yOff - bps + 16

	if mbY > 0 {
		if mbX >= d.mbW-1 {
			fill(b, off, 4, d.topSamples[mbX].y[15])
		} else {
			copy(b[off:off+4], d.topSamples[mbX+1].y[:4])
		}
	}

	for j := 1; j < 4; j++ {
		copy(b[off+4*j*bps:off+4*j*bps+4], b[off:off+4])
	}
}

func (d *Decoder) emit(mbX, mbY int) {
	b := &d.yuv

	ys := d.pic.YStride
	y := d.pic.Y[mbY*16*ys+mbX*16:]

	for j := range 16 {
		copy(y[j*ys:j*ys+16], b[yOff+j*bps:yOff+j*bps+16])
	}

	us := d.pic.UVStride
	off := mbY*8*us + mbX*8
	u, v := d.pic.U[off:], d.pic.V[off:]

	for j := range 8 {
		copy(u[j*us:j*us+8], b[uOff+j*bps:uOff+j*bps+8])
		copy(v[j*us:j*us+8], b[vOff+j*bps:vOff+j*bps+8])
	}
}

func (d *Decoder) decodeFrame() error {
	if d.pipelined() {
		return d.decodeFramePipelined()
	}

	for mbY := range d.mbH {
		br := &d.parts[mbY&(d.numParts-1)]

		d.initScanline()
		d.initRowContext(mbY)

		for mbX := range d.mbW {
			m := &d.mb

			if d.hdr.KeyFrame {
				d.parseIntraMode(m, mbX, mbY)
			} else {
				d.parseInterModes(m, mbX, mbY)
			}

			if !d.decodeMB(m, br, mbX) {
				return ErrInvalid
			}

			if d.filterType > 0 {
				d.fInfoRow[mbX] = m.filterFlags()
			}

			d.reconstruct(m, mbX, mbY)
		}

		if d.br.eof {
			return ErrInvalid
		}

		if d.filterType > 0 {
			d.filterRow(d.fInfoRow, mbY)
		}
	}

	d.frames[d.newIdx].extend(d.mbW, d.mbH)

	return nil
}

// DecodeFrame decodes one VP8 frame. The picture it returns is owned by the
// decoder and is valid until the next call. A frame the stream marks as not
// shown updates the references and returns a nil picture.
func (d *Decoder) DecodeFrame(data []byte) (*Picture, error) {
	if err := d.parseHeader(data); err != nil {
		return nil, err
	}

	d.alloc()

	d.newIdx = d.freeBuffer()
	d.refCnt[d.newIdx] = 1

	d.frames[d.newIdx].alloc(d.mbW, d.mbH, d.hdr.Width, d.hdr.Height)

	d.pic = d.frames[d.newIdx].pic

	if err := d.decodeFrame(); err != nil {
		return nil, err
	}

	if !d.refreshProbs {
		d.restoreEntropy(d.saved)
	}

	d.rotateBuffers()

	if !d.hdr.Show {
		return nil, nil
	}

	return &d.frames[d.newIdx].pic, nil
}
