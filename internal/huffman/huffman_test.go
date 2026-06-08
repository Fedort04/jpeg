package huffman

import (
	"bufio"
	"bytes"
	binreader "jpeg/internal/binReader"
	binwriter "jpeg/internal/binWriter"
	"math/rand"
	"slices"
	"testing"
)

// randomHistogram генерирует случайную гистограмму для тестирования.
func randomHistogram(nSymbols int, maxFreq int) map[uint16]int {
	if nSymbols > maxSymJPEG {
		nSymbols = maxSymJPEG
	}
	hist := make(map[uint16]int)
	for range nSymbols {
		sym := uint16(rand.Intn(maxSymJPEG))
		freq := rand.Intn(maxFreq) + 1
		hist[sym] = freq
	}
	if _, ok := hist[maxSymJPEG]; !ok && len(hist) < maxSymJPEG {
		hist[maxSymJPEG] = 1
	}
	return hist
}

// isPrefix проверяет, является ли код a (длины lenA) префиксом кода b (длины lenB).
func isPrefix(a uint16, lenA byte, b uint16, lenB byte) bool {
	if lenA == 0 || lenB == 0 || lenA > lenB {
		return false
	}
	shift := lenB - lenA
	prefixOfB := b >> shift
	return prefixOfB == a
}

// intToBits преобразует код (целое число) и его длину в срез bool.
func intToBits(code uint16, length byte) []bool {
	if length == 0 {
		return []bool{}
	}
	bits := make([]bool, length)
	for i := range length {
		mask := uint16(1) << (length - 1 - i)
		bits[i] = (code & mask) != 0
	}
	return bits
}

// Проверка характеристик при генерации таблицы
func TestMakeHuffTable_Properties(t *testing.T) {
	hist := randomHistogram(20, 1000)
	bits, symbols := MakeHuffTable(hist)
	if len(bits) != 16 {
		t.Fatal("bits length != 16")
	}
	sum := 0
	for _, b := range bits {
		sum += int(b)
	}
	if sum != len(symbols) {
		t.Errorf("sum(bits)=%d, len(symbols)=%d", sum, len(symbols))
	}
	offset, _, _ := OffsetCreate(bits)
	huff, _ := RecoverHuffTable(offset, symbols)
	for sym := range hist {
		_, _, err := huff.GetCodeBySym(byte(sym))
		if err != nil && sym != maxSymJPEG {
			t.Errorf("символ %d не найден в таблице", sym)
		}
	}
	// Проверка префиксности
	codes := make([]uint16, len(huff.codes))
	copy(codes, huff.codes)
	slices.Sort(codes)
	for i := 0; i < len(codes)-1; i++ {
		for j := i + 1; j < len(codes); j++ {
			if isPrefix(codes[i], huff.codeLength[i], codes[j], huff.codeLength[j]) {
				t.Errorf("код %b (len %d) является префиксом %b (len %d)", codes[i], huff.codeLength[i], codes[j], huff.codeLength[j])
			}
		}
	}
}

// Создание таблицы -> запись -> чтение (дб то же самое)
func TestHuff_Roundtrip(t *testing.T) {
	hist := map[uint16]int{1: 10, 2: 5, 3: 3, 4: 1}
	bits, syms := MakeHuffTable(hist)
	off, _, _ := OffsetCreate(bits)
	huff, _ := RecoverHuffTable(off, syms)

	var seq []byte
	for _, s := range syms {
		seq = append(seq, s)
	}

	var buf bytes.Buffer
	w := binwriter.BinWriterInit(bufio.NewWriter(&buf))
	for _, sym := range seq {
		code, len, _ := huff.GetCodeBySym(sym)
		w.WriteBits(intToBits(code, len))
	}
	w.Close()

	r := binreader.BinReaderInit(bufio.NewReader(&buf))
	r.SetEndian(binreader.BIG)
	var decoded []byte
	for range seq {
		v, err := huff.DecodeHuff(r)
		if err != nil {
			t.Fatal(err)
		}
		decoded = append(decoded, byte(v))
	}
	if !bytes.Equal(seq, decoded) {
		t.Errorf("roundtrip failed: %v vs %v", seq, decoded)
	}
}

// Сверка с эталоной таблицей
//
//	A:0 (длина 2)
//	B:2 (длина 3)
//	C:3 (длина 3)
//	D:8 (длина 4)
//	E:9 (длина 4)
//	F:10 (длина 4)
//	G:22 (длина 5)
//	H:23 (длина 5)
//	I:24 (длина 5)
//	J:25 (длина 5)
func TestSmallTable10Symbols(t *testing.T) {
	bits := []byte{0, 1, 2, 3, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	symbols := []byte{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J'}

	// 2. Восстанавливаем таблицу
	offset, sum, err := OffsetCreate(bits)
	if err != nil {
		t.Fatalf("OffsetCreate error: %v", err)
	}
	if int(sum) != len(symbols) {
		t.Fatalf("sum=%d, len(symbols)=%d", sum, len(symbols))
	}
	huff, err := RecoverHuffTable(offset, symbols)
	if err != nil {
		t.Fatalf("RecoverHuffTable error: %v", err)
	}

	expectedCodes := map[byte]uint16{
		'A': 0, 'B': 2, 'C': 3, 'D': 8, 'E': 9, 'F': 10, 'G': 22, 'H': 23, 'I': 24, 'J': 25,
	}
	expectedLengths := map[byte]byte{
		'A': 2, 'B': 3, 'C': 3, 'D': 4, 'E': 4, 'F': 4, 'G': 5, 'H': 5, 'I': 5, 'J': 5,
	}

	// Проверяем, что восстановленные коды совпадают с ожидаемыми
	for _, sym := range symbols {
		code, length, err := huff.GetCodeBySym(sym)
		if err != nil {
			t.Errorf("GetCodeBySym(%c): %v", sym, err)
			continue
		}
		if code != expectedCodes[sym] {
			t.Errorf("символ %c: code=%d, ожидается %d", sym, code, expectedCodes[sym])
		}
		if length != expectedLengths[sym] {
			t.Errorf("символ %c: length=%d, ожидается %d", sym, length, expectedLengths[sym])
		}
	}

	data := []byte{0x13, 0x89, 0xAB, 0x5F, 0x19}
	reader := binreader.BinReaderInit(bufio.NewReader(bytes.NewReader(data)))
	reader.SetEndian(binreader.BIG)

	expectedSeq := []byte{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J'}
	for i, exp := range expectedSeq {
		dec, err := huff.DecodeHuff(reader)
		if err != nil {
			t.Fatalf("DecodeHuff на позиции %d: %v", i, err)
		}
		if byte(dec) != exp {
			t.Errorf("позиция %d: декодировано %c, ожидалось %c", i, dec, exp)
		}
	}
}
