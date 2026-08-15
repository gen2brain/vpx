package vp8

import (
	"runtime"
	"sync"
)

const (
	pipeDepth   = 4
	pipeMinRows = 4
	pipeMinMBs  = 256
)

type rowMsg struct {
	row, slot int
}

type pipeline struct {
	mbs   [pipeDepth][]mbData
	flags [pipeDepth][]uint8
	free  chan int
	rec   chan rowMsg
	flt   chan rowMsg
	wg    sync.WaitGroup
	err   error
}

func (d *Decoder) workers() int {
	n := d.Threads
	if n == 0 {
		n = runtime.GOMAXPROCS(0)
	}

	return max(n, 1)
}

func (d *Decoder) pipelined() bool {
	return d.workers() > 1 && d.mbH >= pipeMinRows && d.mbW*d.mbH >= pipeMinMBs
}

func (d *Decoder) preparePipeline() *pipeline {
	p := d.pipe

	if p == nil {
		p = &pipeline{
			free: make(chan int, pipeDepth),
			rec:  make(chan rowMsg, pipeDepth),
			flt:  make(chan rowMsg, pipeDepth),
		}

		d.pipe = p
	}

	for i := range p.mbs {
		if cap(p.mbs[i]) < d.mbW {
			p.mbs[i] = make([]mbData, d.mbW)
			p.flags[i] = make([]uint8, d.mbW)
		}

		p.mbs[i] = p.mbs[i][:d.mbW]
		p.flags[i] = p.flags[i][:d.mbW]
	}

	for len(p.free) > 0 {
		<-p.free
	}

	for len(p.rec) > 0 {
		<-p.rec
	}

	for len(p.flt) > 0 {
		<-p.flt
	}

	for i := range pipeDepth {
		p.free <- i
	}

	p.err = nil

	return p
}

// parseRows is the entropy stage: the first partition for the modes, then the
// row's token partition. It owns br, parts, mbCtx, intraT, intraL and modes.
func (d *Decoder) parseRows(p *pipeline) {
	defer p.wg.Done()

	for mbY := range d.mbH {
		slot := <-p.free
		mbs := p.mbs[slot]
		flags := p.flags[slot]

		br := &d.parts[mbY&(d.numParts-1)]

		d.initScanline()

		for mbX := range d.mbW {
			m := &mbs[mbX]

			if d.hdr.KeyFrame {
				d.parseIntraMode(m, mbX, mbY)
			} else {
				d.parseInterModes(m, mbX, mbY)
			}

			if !d.decodeMB(m, br, mbX) {
				p.err = ErrInvalid
				p.rec <- rowMsg{row: -1}

				return
			}

			if d.filterType > 0 {
				flags[mbX] = m.filterFlags()
			}
		}

		if d.br.eof {
			p.err = ErrInvalid
			p.rec <- rowMsg{row: -1}

			return
		}

		p.rec <- rowMsg{row: mbY, slot: slot}
	}
}

// reconstructRows is the prediction stage. It owns yuv, mcTmp and topSamples,
// and never reads the frame it is writing: intra prediction takes its
// neighbours from topSamples, which are saved before the loop filter runs.
func (d *Decoder) reconstructRows(p *pipeline) {
	defer p.wg.Done()

	for range d.mbH {
		msg := <-p.rec

		if msg.row < 0 {
			p.flt <- msg

			return
		}

		mbs := p.mbs[msg.slot]

		d.initRowContext(msg.row)

		for mbX := range d.mbW {
			d.reconstruct(&mbs[mbX], mbX, msg.row)
		}

		p.flt <- msg
	}
}

// decodeFramePipelined overlaps the three stages, which is all VP8 allows when
// a stream carries one token partition: the tokens are then a single
// sequential bitstream and no two macroblock rows can be read at once.
func (d *Decoder) decodeFramePipelined() error {
	p := d.preparePipeline()

	p.wg.Add(2)

	go d.parseRows(p)
	go d.reconstructRows(p)

	for range d.mbH {
		msg := <-p.flt

		if msg.row < 0 {
			break
		}

		if d.filterType > 0 {
			d.filterRow(p.flags[msg.slot], msg.row)
		}

		p.free <- msg.slot
	}

	p.wg.Wait()

	if p.err != nil {
		return p.err
	}

	d.frames[d.newIdx].extend(d.mbW, d.mbH)

	return nil
}
