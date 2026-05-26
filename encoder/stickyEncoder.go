package encoder

import (
	"jpeg/internal/huffman"
	"jpeg/internal/mcu"
)

type stickyEncoder struct {
	encoder *Encoder
	err     error
}

func (se *stickyEncoder) dataUnitEncode(dataUnit []int16, channel byte) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.dataUnitEncode(dataUnit, channel)
}

func (se *stickyEncoder) restartIncrement() {
	if se.err != nil {
		return
	}
	se.err = se.encoder.restartIncrement()
}

func (se *stickyEncoder) encodeDC(val int16, table *huffman.HuffTable, ch byte) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.encodeDC(val, table, ch)
}

func (se *stickyEncoder) encodeAC(dataUnit []int16, ss byte, send byte, acTable *huffman.HuffTable) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.encodeAC(dataUnit, ss, send, acTable)
}

func (se *stickyEncoder) writeStartImg() {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeStartImg()
}

func (se *stickyEncoder) writeApp() {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeApp()
}

func (se *stickyEncoder) writeQuantTable(quantTable []byte, compId byte) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeQuantTable(quantTable, compId)
}

func (se *stickyEncoder) writeFrameHeader(isProgressive bool) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeFrameHeader(isProgressive)
}

func (se *stickyEncoder) writeHuffTable(class byte, id byte, bits []byte, symbols []byte) *huffman.HuffTable {
	if se.err != nil {
		return nil
	}
	var res *huffman.HuffTable
	res, se.err = se.encoder.writeHuffTable(class, id, bits, symbols)
	return res
}

func (se *stickyEncoder) writeDri() {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeDri()
}

func (se *stickyEncoder) writeComponent(c *component) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeComponent(c)
}

func (se *stickyEncoder) encodeSymbol(symbol byte, table *huffman.HuffTable) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.encodeSymbol(symbol, table)
}

func (se *stickyEncoder) encodeAddVal(val int16, ssss byte) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.encodeAddVal(val, ssss)
}

func (se *stickyEncoder) writeHeader(isProgressive bool) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeHeader(isProgressive)
}

func (se *stickyEncoder) writeBaselineScanHeader(blocks [][]mcu.CodingBlock) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeBaselineScanHeader(blocks)
}

func (se *stickyEncoder) writeProgressiveScan(blocks [][]mcu.CodingBlock, head *scanHeader) bool {
	if se.err != nil {
		return false
	}
	res, err := se.encoder.writeProgressiveScan(blocks, head)
	se.err = err
	return res
}

func (se *stickyEncoder) writeRefinementScan(blocks [][]mcu.CodingBlock, head *scanHeader) bool {
	if se.err != nil {
		return false
	}
	res, err := se.encoder.writeRefinementScan(blocks, head)
	se.err = err
	return res
}

func (se *stickyEncoder) writeSos(config *scanHeader) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeSos(config)
}

func (se *stickyEncoder) writeEndImg() {
	if se.err != nil {
		return
	}
	se.err = se.encoder.writeEndImg()
}

func (se *stickyEncoder) encodeEOB(table *huffman.HuffTable) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.encodeEOB(table)
}
