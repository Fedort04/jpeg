package huffman

import (
	"errors"
	"fmt"
	binreader "jpeg/internal/binReader"
	"slices"
	"sort"
)

const NumHuffCodesLen = 16 //Количество длин кодов Хаффмана
const maxNumHuffSym = 176  //Максимальное количество символов в таблице Хаффмана
const maxSymJPEG = 256     // символы 0..255, 256 – резервный

// Структура таблицы Хаффмана
type HuffTable struct {
	offset     []byte   // Количество символов по длине для вычисления кодов
	symbols    []byte   // Символы в таблице
	codes      []uint16 // Коды для символов
	codeLength []byte   // Длина кодов для символов
}

// Подсчет кодов всех длин
func countBits(codesize []int) []int {
	bits := make([]int, 33)
	for v := range maxSymJPEG + 1 {
		if L := codesize[v]; L > 0 {
			bits[L]++
		}
	}
	return bits
}

// Оптимизация таблицы Хаффмана
func adjustBits(bits []int) {
	for i := 32; i > 16; i-- {
		for bits[i] > 0 {
			j := i - 2
			for bits[j] == 0 {
				j--
			}
			bits[i] -= 2
			bits[i-1] += 1
			bits[j+1] += 2
			bits[j] -= 1
		}
	}
}

// MakeHuffTable строит таблицу Хаффмана по алгоритму JPEG (Annex K.2 / libjpeg)
func MakeHuffTable(hist map[uint16]int) ([]byte, []byte) {

	freq := make([]int, maxSymJPEG+1)
	for sym, f := range hist {
		freq[sym] = f
	}
	freq[maxSymJPEG] = 1

	codesize := make([]int, maxSymJPEG+1)
	others := make([]int, maxSymJPEG+1)
	for i := range others {
		others[i] = -1
	}

	for {
		v1, v2 := -1, -1
		min1, min2 := int(^uint(0)>>1), int(^uint(0)>>1)
		for v, f := range freq {
			if f > 0 {
				if f < min1 || (f == min1 && v > v1) {
					min2, v2 = min1, v1
					min1, v1 = f, v
				} else if f < min2 || (f == min2 && v > v2) {
					min2, v2 = f, v
				}
			}
		}
		if v2 == -1 {
			break
		}

		freq[v1] += freq[v2]
		freq[v2] = 0

		for v := v1; v != -1; v = others[v] {
			codesize[v]++
		}
		for v := v2; v != -1; v = others[v] {
			codesize[v]++
		}
		last := v1
		for others[last] != -1 {
			last = others[last]
		}
		others[last] = v2
	}

	bits := countBits(codesize)
	adjustBits(bits)

	// Сортируем реальные символы по частоте (убывание) и по возрастанию значения символа
	type symFreq struct {
		sym  uint16
		freq int
	}
	syms := make([]symFreq, 0, len(hist))
	syms = append(syms, symFreq{sym: maxSymJPEG, freq: 0})
	for sym, f := range hist {
		syms = append(syms, symFreq{sym, f})
	}
	sort.Slice(syms, func(i, j int) bool {
		if syms[i].freq != syms[j].freq {
			return syms[i].freq > syms[j].freq
		}
		return syms[i].sym < syms[j].sym
	})

	// Назначаем длины кодов согласно bits
	newCodeLen := make([]int, maxSymJPEG+1)
	idx := 0
	for length := 1; length <= NumHuffCodesLen; length++ {
		count := bits[length]
		for count > 0 {
			newCodeLen[syms[idx].sym] = length
			idx++
			count--
		}
	}
	reserveLen := newCodeLen[maxSymJPEG]
	bits[reserveLen]--
	newCodeLen[maxSymJPEG] = 0

	// Формируем HUFFVAL: символы, упорядоченные по длине, для одинаковой длины – по символу
	symbols := []byte{}
	for length := 1; length <= NumHuffCodesLen; length++ {
		// Собираем символы этой длины в порядке возрастания символа
		var symsAtLen []uint16
		for _, s := range syms {
			if newCodeLen[s.sym] == length {
				symsAtLen = append(symsAtLen, s.sym)
			}
		}
		sort.Slice(symsAtLen, func(i, j int) bool { return symsAtLen[i] < symsAtLen[j] })
		for _, s := range symsAtLen {
			symbols = append(symbols, byte(s))
		}
	}

	// Формируем BITS для выходной таблицы (длины 1..16)
	bitsBytes := make([]byte, NumHuffCodesLen)
	for i := 1; i <= NumHuffCodesLen; i++ {
		bitsBytes[i-1] = byte(bits[i])
	}

	return bitsBytes, symbols
}

// Декодирование из битового потока значений Хаффмана с помощью binReader
func (h *HuffTable) DecodeHuff(reader *binreader.BinReader) (uint16, error) {
	var code uint16
	codeLen := 0
	for {
		code = code << 1
		temp, err := reader.GetBit()
		if err != nil {
			return 0, errors.New("Huffman bit-reading error: can't find a symbol")
		}

		code += uint16(temp)
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
func RecoverHuffTable(offset []byte, symbols []byte) (*HuffTable, error) {
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
		return nil, 0, errors.New("Huffman table recovery error: invalid bits array")
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
	sr := &binreader.StickyReader{Reader: reader}
	sr.GetWord()
	tc, th := sr.Get4Bit()
	bits := sr.GetArray(NumHuffCodesLen)
	if sr.Err != nil {
		return 0, 0, nil, sr.Err
	}

	offset, sumElem, err := OffsetCreate(bits)
	if err != nil {
		return 0, 0, nil, err
	}

	symbols, err := reader.GetArray(uint16(sumElem))
	if err != nil {
		return 0, 0, nil, err
	}

	huff, err := RecoverHuffTable(offset, symbols)
	return tc, th, huff, err
}
