package binwriter

import (
	"bufio"
)

// Данные хранятся во внутреннем буфере до вызова Close()
type BinWriter struct {
	w    *bufio.Writer // направление записи
	buf  byte          // текущий накапливаемый байт для битов
	bits byte          // количество накопленных бито
}

// Конструктор объекта BinWriter
func BinWriterInit(source *bufio.Writer) *BinWriter {
	var res BinWriter
	res.w = source
	res.buf = 0
	res.bits = 0
	return &res
}

// Записывает накопленный байт (если есть незавершённые биты) в bufio.Writer
func (b *BinWriter) FlushBits() error {
	if b.bits > 0 {
		if err := b.w.WriteByte(b.buf); err != nil {
			return err
		}
		b.buf = 0
		b.bits = 0
	}
	return nil
}

// Записывает один байт
func (b *BinWriter) WriteByte(c byte) error {
	if err := b.FlushBits(); err != nil {
		return err
	}
	return b.w.WriteByte(c)
}

// Записывает два байта
func (b *BinWriter) WriteWord(val uint16) error {
	if err := b.FlushBits(); err != nil {
		return err
	}
	// Big-Endian
	if err := b.w.WriteByte(byte(val >> 8)); err != nil {
		return err
	}
	return b.w.WriteByte(byte(val))
}

// Записывает один бит в буфер. Когда буфер готов, то байт записывается в файл
func (b *BinWriter) WriteBit(bit bool) error {
	if bit {
		b.buf |= 1 << (7 - b.bits)
	}
	b.bits++
	if b.bits == 8 {
		if err := b.w.WriteByte(b.buf); err != nil {
			return err
		}
		b.buf = 0
		b.bits = 0
	}
	return nil
}

// Записывает массив битов
func (b *BinWriter) WriteBits(bits []bool) error {
	for _, bit := range bits {
		if err := b.WriteBit(bit); err != nil {
			return err
		}
	}
	return nil
}

// Записывает массив байт
func (b *BinWriter) WriteBytes(data []byte) error {
	if err := b.FlushBits(); err != nil {
		return err
	}
	_, err := b.w.Write(data)
	return err
}

// Завершить запись файла (данные записываются в память)
func (b *BinWriter) Close() error {
	if err := b.FlushBits(); err != nil {
		return err
	}
	return b.w.Flush()
}
