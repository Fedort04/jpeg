package binreader

// Идиома sticky error. При возникновении ошибки вызовы функций игнорируются до тех пор, пока не будет сброшен флаг ошибки
type StickyReader struct {
	Reader *BinReader
	Err    error
}

func (sr *StickyReader) GetByte() byte {
	if sr.Err != nil {
		return 0
	}
	var val byte
	val, sr.Err = sr.Reader.GetByte()
	return val
}

func (sr *StickyReader) GetWord() uint16 {
	if sr.Err != nil {
		return 0
	}
	var val uint16
	val, sr.Err = sr.Reader.GetWord()
	return val
}

func (sr *StickyReader) GetNextByte() byte {
	if sr.Err != nil {
		return 0
	}
	var val byte
	val, sr.Err = sr.Reader.GetNextByte()
	return val
}

func (sr *StickyReader) Get4Bit() (byte, byte) {
	if sr.Err != nil {
		return 0, 0
	}
	var left, right byte
	left, right, sr.Err = sr.Reader.Get4Bit()
	return left, right
}

func (sr *StickyReader) GetBit() byte {
	if sr.Err != nil {
		return 0
	}
	var val byte
	val, sr.Err = sr.Reader.GetBit()
	return val
}

func (sr *StickyReader) GetBits(n byte) uint16 {
	if sr.Err != nil {
		return 0
	}
	var val uint16
	val, sr.Err = sr.Reader.GetBits(n)
	return val
}

func (sr *StickyReader) BitsAlign() {
	if sr.Err != nil {
		return
	}
	sr.Err = sr.Reader.BitsAlign()
}

func (sr *StickyReader) GetArray(n uint16) []byte {
	if sr.Err != nil {
		return nil
	}
	var val []byte
	val, sr.Err = sr.Reader.GetArray(n)
	return val
}
