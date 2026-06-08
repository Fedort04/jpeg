package huffman

import (
	"errors"
	"fmt"
	binreader "jpeg/internal/binReader"
	"math"
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

// Создание цепочек символов для формирования кодов
func chainsCreate(freq, codesize, mergeChain []int) {
	for {
		node1, node2 := -1, -1
		minFreq1, minFreq2 := math.MaxInt, math.MaxInt
		for v, f := range freq {
			if f > 0 {
				if f < minFreq1 || (f == minFreq1 && v > node1) {
					minFreq2, node2 = minFreq1, node1
					minFreq1, node1 = f, v
				} else if f < minFreq2 || (f == minFreq2 && v > node2) {
					minFreq2, node2 = f, v
				}
			}
		}
		if node2 == -1 {
			break
		}

		freq[node1] += freq[node2]
		freq[node2] = 0

		for v := node1; v != -1; v = mergeChain[v] {
			codesize[v]++
		}
		for v := node2; v != -1; v = mergeChain[v] {
			codesize[v]++
		}
		last := node1
		for mergeChain[last] != -1 {
			last = mergeChain[last]
		}
		mergeChain[last] = node2
	}
}

// Пара для сортировки символов
type symFreq struct {
	sym  uint16
	freq int
}

// Сортируем реальные символы по частоте (убывание) и по возрастанию значения символа
func symbolSort(hist map[uint16]int) []symFreq {
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
	return syms
}

// Обновляем длины кодов согласно bits
func updateCodeLen(bits []int, syms []symFreq) []int {
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
	return newCodeLen
}

// Пара для сортировки HUFFVAL
type symLen struct {
	sym uint16
	len int
}

// Формирование массива HUFFVAL (символы, упорядоченные по длине, для одинаковой длины – по символу)
func makeHUFFVAL(hist map[uint16]int, newCodeLen []int) []byte {
	symbols := []byte{}
	sorted := make([]symLen, 0, len(hist))
	for sym := range hist {
		if l := newCodeLen[sym]; l > 0 {
			sorted = append(sorted, symLen{sym, l})
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].len != sorted[j].len {
			return sorted[i].len < sorted[j].len
		}
		return sorted[i].sym < sorted[j].sym
	})
	for _, s := range sorted {
		symbols = append(symbols, byte(s.sym))
	}
	return symbols
}

// Подсчет кодов всех длин
func countBits(codesize []int) []int {
	bits := make([]int, NumHuffCodesLen*2+1)
	for v := range maxSymJPEG + 1 {
		if L := codesize[v]; L > 0 {
			bits[L]++
		}
	}
	return bits
}

// Оптимизация таблицы Хаффмана
func adjustBits(bits []int) {
	for i := NumHuffCodesLen * 2; i > NumHuffCodesLen; i-- {
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
	mergeChain := make([]int, maxSymJPEG+1)
	for i := range mergeChain {
		mergeChain[i] = -1
	}

	chainsCreate(freq, codesize, mergeChain)

	bits := countBits(codesize)
	adjustBits(bits)

	syms := symbolSort(hist)

	newCodeLen := updateCodeLen(bits, syms)

	// Формируем HUFFVAL: символы, упорядоченные по длине, для одинаковой длины – по символу
	symbols := makeHUFFVAL(hist, newCodeLen)

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
		if codeLen > NumHuffCodesLen {
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
func RecoverHuffTable(offset, symbols []byte) (*HuffTable, error) {
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
