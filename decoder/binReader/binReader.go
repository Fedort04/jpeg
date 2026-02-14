package binreader

import (
	"bufio"
	"log"
	"os"
)

// Перечисление типов Endianness
type Endian byte

const (
	BIG Endian = iota
	LITTLE
)

type BinReader struct {
	src          *bufio.Reader //Источник для чтения
	curFile      *os.File
	end          Endian //Endianness
	isHuffStream bool   //Флаг вычисления битового потока (для пропуска нулей в 0xFF-0x00)
	curByte      byte   //Текущее значение байта для побитового чтения
	bitCount     byte   //Счетчик бит в текущем байте
}

// func BinReaderInit(source string, end Endian) (*BinReader, error) {
// 	var reader BinReader
// 	temp, err := os.Open(source)
// 	if err != nil {
// 		return nil, err
// 	}
// 	reader.curFile = temp
// 	reader.src = bufio.NewReader(temp)
// 	reader.end = end
// 	reader.isHuffStream = false
// 	reader.curByte = 0
// 	reader.bitCount = 0
// 	return &reader, nil
// }

// Инициализация объекта BinReader на расположение source
func BinReaderInit(source *bufio.Reader) *BinReader {
	var reader BinReader
	reader.src = source
	reader.end = BIG
	reader.isHuffStream = false
	reader.curByte = 0
	reader.bitCount = 0
	return &reader
}

// Деструктор объекта
// func (b *BinReader) Close() error {
// 	return b.curFile.Close()
// }

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
func (b *BinReader) GetByte() byte {
	ans, err := b.src.ReadByte()
	if err != nil {
		log.Panic(err.Error())
	}

	if b.isHuffStream && b.curByte == 0xFF && ans == 0x00 {
		ans, err = b.src.ReadByte()
		if err != nil {
			log.Panic(err.Error())
		}
	}

	b.curByte = ans
	return ans
}

// Чтение двух байт
func (b *BinReader) GetWord() uint16 {

	ans := uint16(b.GetByte())
	if b.end == BIG {
		ans = ans << 8
		ans += uint16(b.GetByte())
	} else {
		temp := uint16(b.GetByte())
		temp = temp << 8
		ans += temp
	}
	return ans
}

// Получение следующего байта без смещения указателя
func (b *BinReader) GetNextByte() byte {
	ans, err := b.src.Peek(1)
	if err != nil {
		log.Fatal("GetNextByte -> error", err.Error())
	}
	return ans[0]
}

// Чтение байта по 4бита
func (b *BinReader) Get4Bit() (byte, byte) {
	temp := b.GetByte()
	return temp >> 4, temp & 0xF
}

// Чтение одного бита
func (b *BinReader) GetBit() byte {
	if b.end == BIG {
		if b.bitCount == 0 {
			b.GetByte()
			b.bitCount = 8
		}
		b.bitCount--
		temp := b.curByte >> b.bitCount
		return temp & 1
	} else {
		if b.bitCount == 8 {
			b.GetByte()
			b.bitCount = 0
		}
		temp := b.curByte >> b.bitCount
		b.bitCount++
		return temp & 1
	}
}

// Чтение n бит
func (b *BinReader) GetBits(n byte) uint16 {
	if n == 0 {
		return 0
	}
	var ans uint16
	for range n {
		ans = ans << 1
		ans += uint16(b.GetBit())
	}
	return ans
}

// Пропуск оставшихся бит в байте
func (b *BinReader) BitsAlign() {
	b.GetByte()
	if b.end == BIG {
		b.bitCount = 8
	} else {
		b.bitCount = 0
	}
}

// Чтение n байт
func (b *BinReader) GetArray(n uint16) []byte {
	res := make([]byte, n)
	for i := range n {
		res[i] = b.GetByte()
	}
	return res
}

// Декодирование символа EOB
func (b *BinReader) DecodeEndOfBand(count byte) uint16 {
	var ans uint16
	ans = 1 << count
	ans += b.GetBits(count)
	return ans
}
