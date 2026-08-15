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

type encPipeline struct {
	rows [pipeDepth][]mbLevels
	free chan int
	tok  chan rowMsg
	wg   sync.WaitGroup
}

func (e *Encoder) workers() int {
	n := e.threads
	if n == 0 {
		n = runtime.GOMAXPROCS(0)
	}

	return max(n, 1)
}

func (e *Encoder) pipelined() bool {
	return e.workers() > 1 && e.mbH >= pipeMinRows && e.mbW*e.mbH >= pipeMinMBs
}

func (e *Encoder) preparePipeline() *encPipeline {
	p := e.pipe

	if p == nil {
		p = &encPipeline{
			free: make(chan int, pipeDepth),
			tok:  make(chan rowMsg, pipeDepth),
		}

		e.pipe = p
	}

	for i := range p.rows {
		if cap(p.rows[i]) < e.mbW {
			p.rows[i] = make([]mbLevels, e.mbW)
		}

		p.rows[i] = p.rows[i][:e.mbW]
	}

	for len(p.free) > 0 {
		<-p.free
	}

	for len(p.tok) > 0 {
		<-p.tok
	}

	for i := range pipeDepth {
		p.free <- i
	}

	return p
}

func (e *Encoder) writeRows(p *encPipeline) {
	defer p.wg.Done()

	for range e.mbH {
		msg := <-p.tok
		lv := p.rows[msg.slot]

		e.ctx[0] = mbCtx{}

		for mbX := range e.mbW {
			e.writeMB(mbX, &lv[mbX], e.info[msg.row*e.mbW+mbX])
		}

		p.free <- msg.slot
	}
}

func (e *Encoder) encodeRows() {
	if !e.pipelined() {
		for mbY := range e.mbH {
			e.rec.initScanline()
			e.rec.initRowContext(mbY)

			e.ctx[0] = mbCtx{}
			e.leftB = [4]uint8{}

			for mbX := range e.mbW {
				e.loadSource(mbX, mbY)
				e.rec.loadNeighbours(mbX, mbY)
				e.codeMB(mbX, mbY, &e.lv)
				e.rec.reconstructMB(&e.rec.mb, mbX, mbY)

				if e.rec.filterType > 0 {
					e.rec.fInfoRow[mbX] = e.rec.mb.filterFlags()
				}
			}

			if e.rec.filterType > 0 {
				e.rec.filterRow(e.rec.fInfoRow, mbY)
			}
		}

		e.rec.frames[e.rec.newIdx].extend(e.mbW, e.mbH)

		return
	}

	p := e.preparePipeline()

	p.wg.Add(1)

	go e.writeRows(p)

	for mbY := range e.mbH {
		slot := <-p.free
		lv := p.rows[slot]

		e.rec.initScanline()
		e.rec.initRowContext(mbY)

		e.leftB = [4]uint8{}

		for mbX := range e.mbW {
			e.loadSource(mbX, mbY)
			e.rec.loadNeighbours(mbX, mbY)
			e.analyzeMB(mbX, mbY, &lv[mbX])
			e.rec.reconstructMB(&e.rec.mb, mbX, mbY)

			if e.rec.filterType > 0 {
				e.rec.fInfoRow[mbX] = e.rec.mb.filterFlags()
			}
		}

		if e.rec.filterType > 0 {
			e.rec.filterRow(e.rec.fInfoRow, mbY)
		}

		p.tok <- rowMsg{row: mbY, slot: slot}
	}

	p.wg.Wait()

	e.rec.frames[e.rec.newIdx].extend(e.mbW, e.mbH)
}
