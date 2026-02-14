package huffman

import (
	"errors"
	binreader "jpeg/decoder/binReader"
)

const NumHuffCodesLen = 16 //Количество длин кодов Хаффмана
const maxNumHuffSym = 176  //Максимальное количество символов в таблице Хаффмана

// Структура таблицы Хаффмана
type HuffTable struct {
	offset  []byte   // Количество символов по длине для вычисления кодов
	symbols []byte   // Символы в таблице
	codes   []uint16 //Коды для символов
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
func makeHuffTable(offset []byte, symbols []byte) (*HuffTable, error) {
	if offset[NumHuffCodesLen] > maxNumHuffSym {
		return nil, errors.New("Huffman recovery error: too much symbols")
	}
	var ans HuffTable
	ans.offset = offset
	ans.symbols = symbols
	ans.codes = make([]uint16, offset[NumHuffCodesLen])
	var code uint16
	for i := range NumHuffCodesLen {
		for j := ans.offset[i]; j < ans.offset[i+1]; j++ {
			ans.codes[j] = code
			code++
		}
		code = code << 1
	}
	return &ans, nil
}

// Чтение и конструирование таблиц Хаффмана
// Возвращает tc, th, уже готовую таблицу
func ReadHuffTable(reader *binreader.BinReader) (byte, byte, *HuffTable, error) {
	reader.GetWord()
	tc, th := reader.Get4Bit()
	offset := make([]byte, NumHuffCodesLen+1)
	var sumElem byte //Количество символов
	//Запись offset
	for i := 1; i < NumHuffCodesLen+1; i++ {
		sumElem += reader.GetByte()
		offset[i] = sumElem
	}
	symbols := make([]byte, sumElem)
	//Чтение символов
	for i := range sumElem {
		symbols[i] = reader.GetByte()
	}
	huff, err := makeHuffTable(offset, symbols)
	return tc, th, huff, err
}
