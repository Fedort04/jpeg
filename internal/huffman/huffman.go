package huffman

import (
	"errors"
	"fmt"
	binreader "jpeg/internal/binReader"
	"slices"
)

const NumHuffCodesLen = 16 //Количество длин кодов Хаффмана
const maxNumHuffSym = 176  //Максимальное количество символов в таблице Хаффмана

// Структура таблицы Хаффмана
type HuffTable struct {
	offset     []byte   // Количество символов по длине для вычисления кодов
	symbols    []byte   // Символы в таблице
	codes      []uint16 // Коды для символов
	codeLength []byte   // Длина кодов для символов
}

// Декодирование из битового потока значений Хаффмана с помощью binReader
func (h *HuffTable) DecodeHuff(reader *binreader.BinReader) (uint16, error) {
	var code uint16
	codeLen := 0
	for {
		code = code << 1
		code += uint16(reader.GetBit())
		codeLen++
		if codeLen > 16 {
			return 0, errors.New("Huffman bit-reading error: can't find a symbol")
		}
		for i := h.offset[codeLen-1]; i < h.offset[codeLen]; i++ {
			if code == h.codes[i] {
				return uint16(h.symbols[i]), nil
			}
		}
	}
}

// Восстановление кодов таблицы Хаффмана и конструирование объекта
func MakeHuffTable(offset []byte, symbols []byte) (*HuffTable, error) {
	if offset[NumHuffCodesLen] > maxNumHuffSym {
		return nil, errors.New("Huffman recovery error: too much symbols")
	}
	var ans HuffTable
	ans.offset = offset
	ans.symbols = symbols
	ans.codes = make([]uint16, offset[NumHuffCodesLen])
	ans.codeLength = make([]byte, len(ans.codes))
	var code uint16
	for i := range NumHuffCodesLen {
		for j := ans.offset[i]; j < ans.offset[i+1]; j++ {
			ans.codes[j] = code
			ans.codeLength[j] = byte(i + 1)
			code++
		}
		code = code << 1
	}
	return &ans, nil
}

// Создание массива сдвигов для восстановления таблиц
// Возвращает offset и кол-во символов
func OffsetCreate(bits []byte) ([]byte, byte, error) {
	if len(bits) != NumHuffCodesLen {
		return nil, 0, errors.New("Huffman recovery error: invalid bits array")
	}

	offset := make([]byte, NumHuffCodesLen+1)
	var sumElem byte
	for i := 1; i < NumHuffCodesLen+1; i++ {
		sumElem += bits[i-1]
		offset[i] = sumElem
	}
	return offset, sumElem, nil
}

// Получить код по символу из таблицы
// Возвращает код и его длину
func (huff *HuffTable) GetCodeBySym(sym byte) (uint16, byte, error) {
	idx := slices.Index(huff.symbols, sym)
	if idx == -1 {
		return 0, 0, fmt.Errorf("Huff table can't find symbol %X", sym)
	}
	return huff.codes[idx], huff.codeLength[idx], nil
}

// Чтение и конструирование таблиц Хаффмана
// Возвращает tc (класс таблицы), th(id таблицы), уже готовую таблицу
func ReadHuffTable(reader *binreader.BinReader) (byte, byte, *HuffTable, error) {
	reader.GetWord()
	tc, th := reader.Get4Bit()
	bits := reader.GetArray(NumHuffCodesLen)
	offset, sumElem, err := OffsetCreate(bits)
	if err != nil {
		return 0, 0, nil, err
	}

	symbols := reader.GetArray(uint16(sumElem))
	huff, err := MakeHuffTable(offset, symbols)
	return tc, th, huff, err
}
