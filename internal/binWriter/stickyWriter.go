package binwriter

// Идиома sticky error. При возникновении ошибки вызовы функций игнорируются до тех пор, пока не будет сброшен флаг ошибки
type StickyWriter struct {
	Writer *BinWriter
	Err    error
}

func (sw *StickyWriter) WriteByte(c byte) {
	if sw.Err != nil {
		return
	}
	sw.Err = sw.Writer.WriteByte(c)
}

func (sw *StickyWriter) WriteWord(word uint16) {
	if sw.Err != nil {
		return
	}
	sw.Err = sw.Writer.WriteWord(word)
}

func (sw *StickyWriter) WriteBit(bit bool) {
	if sw.Err != nil {
		return
	}
	sw.Err = sw.Writer.WriteBit(bit)
}

func (sw *StickyWriter) WriteBits(bits []bool) {
	if sw.Err != nil {
		return
	}
	sw.Err = sw.Writer.WriteBits(bits)
}

func (sw *StickyWriter) WriteArray(data []byte) {
	if sw.Err != nil {
		return
	}
	sw.Err = sw.Writer.WriteArray(data)
}

func (sw *StickyWriter) Write4Bit(left, right byte) {
	if sw.Err != nil {
		return
	}
	sw.Err = sw.Writer.Write4Bit(left, right)
}

func (sw *StickyWriter) FlushBits() {
	if sw.Err != nil {
		return
	}
	sw.Err = sw.Writer.FlushBits()
}

func (sw *StickyWriter) MergeFrom(src *BinWriter) {
	if sw.Err != nil {
		return
	}
	sw.Err = sw.Writer.MergeFrom(src)
}
