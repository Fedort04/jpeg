package encoder

import "jpeg/internal/huffman"

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

func (se *stickyEncoder) encodeAC(dataUnit []int16, acTable *huffman.HuffTable) {
	if se.err != nil {
		return
	}
	se.err = se.encoder.encodeAC(dataUnit, acTable)
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
