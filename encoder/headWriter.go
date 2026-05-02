package encoder

import "jpeg/internal/huffman"

// StickyError реализация для записи заголовка
type headWriter struct {
	w interface {
		writeStartImg() error
		writeApp() error
		writeQuantTable([]byte, byte) error
		writeFrameHeader(bool) error
		writeHuffTable(byte, byte, []byte, []byte) (*huffman.HuffTable, error)
		writeDri() error
		writeComponent(*component) error
	}
	err error
}

func (hw *headWriter) writeStartImg() {
	if hw.err != nil {
		return
	}
	hw.err = hw.w.writeStartImg()
}

func (hw *headWriter) writeApp() {
	if hw.err != nil {
		return
	}
	hw.err = hw.w.writeApp()
}

func (hw *headWriter) writeQuantTable(quantTable []byte, compId byte) {
	if hw.err != nil {
		return
	}
	hw.err = hw.w.writeQuantTable(quantTable, compId)
}

func (hw *headWriter) writeFrameHeader(isProgressive bool) {
	if hw.err != nil {
		return
	}
	hw.err = hw.w.writeFrameHeader(isProgressive)
}

func (hw *headWriter) writeHuffTable(class byte, id byte, bits []byte, symbols []byte) *huffman.HuffTable {
	if hw.err != nil {
		return nil
	}
	var res *huffman.HuffTable
	res, hw.err = hw.w.writeHuffTable(class, id, bits, symbols)
	return res
}

func (hw *headWriter) writeDri() {
	if hw.err != nil {
		return
	}
	hw.err = hw.w.writeDri()
}

func (hw *headWriter) writeComponent(c *component) {
	if hw.err != nil {
		return
	}
	hw.err = hw.w.writeComponent(c)
}
