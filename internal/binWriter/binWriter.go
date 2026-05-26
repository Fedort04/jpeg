package binwriter

import (
	"bufio"
	"bytes"
	"errors"
)

// Данные хранятся во внутреннем буфере до вызова Close()
type BinWriter struct {
	w    *bufio.Writer // направление записи
	buf  byte          // текущий накапливаемый байт для битов
	bits byte          // количество накопленных бито

	buffer *bytes.Buffer //буфер для временного объекта, который позволяет его слить с другим
}

// Конструктор объекта BinWriter
func BinWriterInit(source *bufio.Writer) *BinWriter {
	var res BinWriter
	res.w = source
	res.buf = 0
	res.bits = 0
	return &res
}

// Конструктор временного объекта BinWriter, который затем предполагается влить в глобальный BinWriter
func LocalBinWriterInit(buf *bytes.Buffer) *BinWriter {
	return &BinWriter{
		w:      bufio.NewWriter(buf),
		buffer: buf,
	}
}

// MergeFrom переносит все данные из src без выравнивания.
// src должен быть создан через LocalBinWriterInit().
func (b *BinWriter) MergeFrom(src *BinWriter) error {
	if err := src.w.Flush(); err != nil {
		return err
	}
	if src.buffer == nil {
		return errors.New("Src must be created with LocalBinWriterInit()")
	}

	data := src.buffer.Bytes()
	for _, i := range data {
		if err := b.WriteBits(b.CreateBitsArray(uint16(i), 8)); err != nil {
			return err
		}
	}
	src.buffer.Reset()

	// Переносим незавершённые биты src
	for i := byte(0); i < src.bits; i++ {
		bit := (src.buf >> (7 - i)) & 1
		if err := b.WriteBit(bit == 1); err != nil {
			return err
		}
	}
	src.buf = 0
	src.bits = 0

	return nil
}

// Записывает накопленный байт (если есть незавершённые биты) в bufio.Writer
func (b *BinWriter) FlushBits() error {
	if b.bits > 0 {
		if err := b.w.WriteByte(b.buf); err != nil {
			return errors.New("Can't flush bits\n" + err.Error())
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
	if err := b.w.WriteByte(c); err != nil {
		return errors.New("Can't write a byte\n" + err.Error())
	}
	return nil
}

// Записывает два байта
func (b *BinWriter) WriteWord(val uint16) error {
	if err := b.FlushBits(); err != nil {
		return err
	}
	// Big-Endian
	if err := b.w.WriteByte(byte(val >> 8)); err != nil {
		return errors.New("Can't write a word\n" + err.Error())
	}
	if err := b.w.WriteByte(byte(val)); err != nil {
		return errors.New("Can't write a word\n" + err.Error())
	}
	return nil
}

// Записывает один бит в буфер. Когда буфер готов, то байт записывается в файл
func (b *BinWriter) WriteBit(bit bool) error {
	if bit {
		b.buf |= 1 << (7 - b.bits)
	}
	b.bits++
	if b.bits == 8 {
		if err := b.w.WriteByte(b.buf); err != nil {
			return errors.New("Can't write a bit\n" + err.Error())
		}
		if b.buf == 0xFF && b.buffer == nil { //После xFF записать 00 (если не локальный)
			if err := b.w.WriteByte(0); err != nil {
				return errors.New("Can't write a bit\n" + err.Error())
			}
		}
		b.buf = 0
		b.bits = 0
	}
	return nil
}

// Создает массив битов, обрезая старшие биты в val
func (b *BinWriter) CreateBitsArray(val uint16, len byte) []bool {
	if len > 16 {
		return nil
	}

	result := make([]bool, len)
	for i := range len {
		bit := (val >> (len - i - 1)) & 1
		result[i] = bit == 1
	}
	return result
}

// Записывает массив битов (первый элемент - старший бит)
func (b *BinWriter) WriteBits(bits []bool) error {
	for _, bit := range bits {
		if err := b.WriteBit(bit); err != nil {
			return errors.New("Can't write a bits array\n" + err.Error())
		}
	}
	return nil
}

// Записывает массив байт
func (b *BinWriter) WriteArray(data []byte) error {
	if err := b.FlushBits(); err != nil {
		return err
	}
	if _, err := b.w.Write(data); err != nil {
		return errors.New("Can't write a byte array\n" + err.Error())
	}
	return nil
}

// Запись байта парой из 4бит
func (b *BinWriter) Write4Bit(left byte, right byte) error {
	res := left << 4
	res += right
	if err := b.WriteByte(res); err != nil {
		return errors.New("Can't write a 4-bits pair\n" + err.Error())
	}
	return nil
}

// Завершить запись файла (данные записываются в память)
func (b *BinWriter) Close() error {
	if err := b.FlushBits(); err != nil {
		return err
	}
	return b.w.Flush()
}
