package binreader

import (
	"bufio"
	"errors"
)

// Перечисление типов Endianness
type Endian byte

const (
	BIG Endian = iota
	LITTLE
)

type BinReader struct {
	src          *bufio.Reader //Источник для чтения
	end          Endian        //Endianness
	isHuffStream bool          //Флаг вычисления битового потока (для пропуска нулей в 0xFF-0x00)
	curByte      byte          //Текущее значение байта для побитового чтения
	bitCount     byte          //Счетчик бит в текущем байте
}

// Конструктор объекта BinReader на расположение source
func BinReaderInit(source *bufio.Reader) *BinReader {
	var reader BinReader
	reader.src = source
	reader.end = BIG
	reader.isHuffStream = false
	reader.curByte = 0
	reader.bitCount = 0
	return &reader
}

// Изменить endian объекта BinReader
func (b *BinReader) SetEndian(end Endian) {
	b.end = end
	if end == BIG {
		b.bitCount = 0
	} else {
		b.bitCount = 8
	}
}

// Запуск чтения битового потока Хаффмана
func (b *BinReader) HuffStreamStart() {
	b.bitCount = 0
	b.isHuffStream = true
}

// Отключение чтения битового потока Хаффмана
func (b *BinReader) HuffStreamEnd() {
	b.isHuffStream = false
}

// Чтение одного байта
func (b *BinReader) GetByte() (byte, error) {
	var ans byte
	var err error
	if ans, err = b.src.ReadByte(); err != nil {
		return 0, errors.New("Can't read a byte\n" + err.Error())
	}

	if b.isHuffStream && b.curByte == 0xFF && ans == 0x00 {
		if ans, err = b.src.ReadByte(); err != nil {
			return 0, errors.New("Can't read a byte\n" + err.Error())
		}
	}

	b.curByte = ans
	return ans, nil
}

// Чтение двух байт
func (b *BinReader) GetWord() (uint16, error) {
	var ans uint16

	if temp, err := b.GetByte(); err != nil {
		return 0, errors.New("Can't read a word\n" + err.Error())
	} else {
		ans = uint16(temp)
	}

	if b.end == BIG {
		ans = ans << 8
		if temp, err := b.GetByte(); err != nil {
			return 0, errors.New("Can't read a word\n" + err.Error())
		} else {
			ans += uint16(temp)
		}
	} else {
		if temp, err := b.GetByte(); err != nil {
			return 0, errors.New("Can't read a word\n" + err.Error())
		} else {
			cur := uint16(temp)
			cur <<= 8
			ans += cur
		}
	}
	return ans, nil
}

// Получение следующего байта без смещения указателя
func (b *BinReader) GetNextByte() (byte, error) {
	if ans, err := b.src.Peek(1); err != nil {
		return 0, errors.New("Can't check a next byte\n" + err.Error())
	} else {
		return ans[0], nil
	}
}

// Чтение байта по 4бита
func (b *BinReader) Get4Bit() (byte, byte, error) {
	if temp, err := b.GetByte(); err != nil {
		return 0, 0, errors.New("Can't read a 4-bits pair\n" + err.Error())
	} else {
		return temp >> 4, temp & 0xF, nil
	}
}

// Чтение одного бита
func (b *BinReader) GetBit() (byte, error) {
	if b.end == BIG {
		if b.bitCount == 0 {
			if _, err := b.GetByte(); err != nil {
				return 0, errors.New("Can't read a bit\n" + err.Error())
			}
			b.bitCount = 8
		}
		b.bitCount--
		temp := b.curByte >> b.bitCount
		return temp & 1, nil
	} else {
		if b.bitCount == 8 {
			if _, err := b.GetByte(); err != nil {
				return 0, errors.New("Can't read a bit\n" + err.Error())
			}
			b.bitCount = 0
		}
		temp := b.curByte >> b.bitCount
		b.bitCount++
		return temp & 1, nil
	}
}

// Чтение n бит
func (b *BinReader) GetBits(n byte) (uint16, error) {
	if n == 0 {
		return 0, nil
	}
	var ans uint16
	for range n {
		ans = ans << 1
		if temp, err := b.GetBit(); err != nil {
			return 0, errors.New("Can't read bits array\n" + err.Error())
		} else {
			ans += uint16(temp)
		}
	}
	return ans, nil
}

// Пропуск оставшихся бит в байте
func (b *BinReader) BitsAlign() error {
	if b.bitCount > 0 && b.bitCount < 8 {
		if _, err := b.GetByte(); err != nil {
			return errors.New("Can't align bits\n" + err.Error())
		}
		if b.end == BIG {
			b.bitCount = 8
		} else {
			b.bitCount = 0
		}
	}
	return nil
}

// Чтение n байт
func (b *BinReader) GetArray(n uint16) ([]byte, error) {
	res := make([]byte, n)
	for i := range n {
		if temp, err := b.GetByte(); err != nil {
			return nil, errors.New("Can't read a byte array\n" + err.Error())
		} else {
			res[i] = temp
		}
	}
	return res, nil
}
