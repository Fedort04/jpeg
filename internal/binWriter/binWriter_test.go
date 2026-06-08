package binwriter

import (
	"bufio"
	"bytes"
	"testing"
)

func TestWriteByte(t *testing.T) {
	buf := &bytes.Buffer{}
	w := BinWriterInit(bufio.NewWriter(buf))

	testByte := byte(0xAB)
	err := w.WriteByte(testByte)
	if err != nil {
		t.Fatalf("WriteByte вернул ошибку: %v", err)
	}

	w.Close()

	expected := []byte{0xAB}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("Ожидалось % X, получено % X", expected, buf.Bytes())
	}
}

func TestWriteWord(t *testing.T) {
	tests := []struct {
		name     string
		value    uint16
		expected []byte
	}{

		{"Число 256", 0x0100, []byte{0x01, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			w := BinWriterInit(bufio.NewWriter(buf))

			err := w.WriteWord(tt.value)
			if err != nil {
				t.Fatalf("WriteWord вернул ошибку: %v", err)
			}
			w.Close()

			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("Для %#06x ожидалось % X, получено % X",
					tt.value, tt.expected, buf.Bytes())
			}
		})
	}
}

func TestWriteBit(t *testing.T) {
	tests := []struct {
		name     string
		bits     []bool
		expected []byte
	}{
		{
			"Один бит (нужно выравнивание)",
			[]bool{true},
			[]byte{0x80}, // 10000000
		},
		{
			"Четыре бита",
			[]bool{true, false, true, false},
			[]byte{0xA0}, // 10100000
		},
		{
			"Восемь битов (полный байт)",
			[]bool{true, false, true, false, true, false, true, false},
			[]byte{0xAA}, // 10101010
		},
		{
			"Девять битов (два байта)",
			[]bool{true, false, true, false, true, false, true, false, true},
			[]byte{0xAA, 0x80}, // 10101010 10000000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			w := BinWriterInit(bufio.NewWriter(buf))

			for _, bit := range tt.bits {
				err := w.WriteBit(bit)
				if err != nil {
					t.Fatalf("WriteBit вернул ошибку: %v", err)
				}
			}
			w.Close()

			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("Ожидалось % X, получено % X", tt.expected, buf.Bytes())
			}
		})
	}
}

func TestWriteBitsVariants(t *testing.T) {
	tests := []struct {
		name  string
		calls []struct {
			val byte
			len byte
		}
		expected []byte
	}{
		{
			name: "len=8 – полный байт",
			calls: []struct {
				val byte
				len byte
			}{{0x12, 8}},
			expected: []byte{0x12},
		},
		{
			name: "len=4 – нулевые младшие биты",
			calls: []struct {
				val byte
				len byte
			}{{0xA0, 4}},
			expected: []byte{0x00},
		},
		{
			name: "len=3 – три бита 110",
			calls: []struct {
				val byte
				len byte
			}{{0xF6, 3}},
			expected: []byte{0xC0},
		},
		{
			name: "len=7 – семь бит из 0xAA",
			calls: []struct {
				val byte
				len byte
			}{{0xAA, 7}},
			expected: []byte{0x54},
		},
		{
			name: "три вызова подряд: 4 бита + 3 бита + 2 бита (больше байта)",
			calls: []struct {
				val byte
				len byte
			}{
				{0x0A, 4}, // 1010
				{0x04, 3}, // 100
				{0x03, 2}, // 11
			},
			expected: []byte{0xA9, 0b10000000}, // 1010 100 11 -> 10101001 10000000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			w := BinWriterInit(bufio.NewWriter(buf))

			for _, call := range tt.calls {
				bits := w.CreateBitsArray(uint16(call.val), call.len)
				err := w.WriteBits(bits)
				if err != nil {
					t.Fatalf("WriteBits вернул ошибку: %v", err)
				}
			}
			w.Close()

			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("Ожидалось % X, получено % X", tt.expected, buf.Bytes())
			}
		})
	}
}

func TestWrite4Bit(t *testing.T) {
	tests := []struct {
		name     string
		left     byte
		right    byte
		expected []byte
	}{
		{"Оба нуля", 0x0, 0x0, []byte{0x00}},
		{"Левая часть 0xF, правая 0x0", 0xF, 0x0, []byte{0xF0}},
		{"Обе части 0xF", 0xF, 0xF, []byte{0xFF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			w := BinWriterInit(bufio.NewWriter(buf))

			err := w.Write4Bit(tt.left, tt.right)
			if err != nil {
				t.Fatalf("Write4Bit вернул ошибку: %v", err)
			}
			w.Close()

			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("Для left=%#x, right=%#x ожидалось % X, получено % X",
					tt.left, tt.right, tt.expected, buf.Bytes())
			}
		})
	}
}

func TestMixedWrite(t *testing.T) {
	buf := &bytes.Buffer{}
	w := BinWriterInit(bufio.NewWriter(buf))

	// Записываем: 2 бита, байт, 4 бита, два байта
	// Ожидаемый результат:
	// 2 бита: 10
	// байт: AB
	// 4 бита: 1100
	// два байта: 1234

	err := w.WriteBit(true) // 1
	err = w.WriteBit(false) // 0 10xxxxxx

	err = w.WriteByte(0xAB)
	if err != nil {
		t.Fatalf("Ошибка при смешанной записи: %v", err)
	}

	err = w.WriteBit(true)  // 1
	err = w.WriteBit(true)  // 1
	err = w.WriteBit(false) // 0
	err = w.WriteBit(false) // 0 -> 1100xxxx

	err = w.WriteWord(0x1234)
	if err != nil {
		t.Fatalf("Ошибка при смешанной записи: %v", err)
	}

	w.Close()

	expected := []byte{0x80, 0xAB, 0xC0, 0x12, 0x34}

	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("Ожидалось % X, получено % X", expected, buf.Bytes())
	}
}

func TestMergeInto(t *testing.T) {
	tests := []struct {
		name     string
		ops1     func(w *BinWriter)
		ops2     func(w *BinWriter)
		expected []byte
	}{
		{
			name:     "Только байты",
			ops1:     func(w *BinWriter) { w.WriteArray([]byte{0x01, 0x02}) },
			ops2:     func(w *BinWriter) { w.WriteArray([]byte{0x03, 0x04}) },
			expected: []byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			name: "Биты с проверкой на FF",
			ops1: func(w *BinWriter) {
				w.WriteByte(0x0F)
				w.WriteBits(w.CreateBitsArray(0b11110011, 8))
				w.WriteBits(w.CreateBitsArray(0b111, 3))
			},
			ops2: func(w *BinWriter) {
				w.WriteBits(w.CreateBitsArray(0b11111, 5))
				w.WriteBits(w.CreateBitsArray(0b11000000, 8))
			},
			expected: []byte{0x0F, 0xF3, 0xFF, 0x00, 0xC0},
		},
		{
			name: "Много нулей в начале",
			ops1: func(w *BinWriter) {
				w.WriteBits(w.CreateBitsArray(0b0, 4))
			},
			ops2: func(w *BinWriter) {
				w.WriteByte(0x3f)
			},
			expected: []byte{0b00000011, 0b11110000},
		},
		{
			name: "Дополнение 0xFF00 при использовании трех буферов",
			ops1: func(w *BinWriter) {
				w.WriteBits(w.CreateBitsArray(0b0, 1))
				buf2 := &bytes.Buffer{}
				w2 := LocalBinWriterInit(buf2)
				w2.WriteBits(w2.CreateBitsArray(0xff, 8))
				w.MergeFrom(w2)
			},
			ops2: func(w *BinWriter) {
				w.WriteBits(w.CreateBitsArray(0xb, 4))
			},
			expected: []byte{0b01111111, 0b11011000},
		},
		{
			name:     "Биты без выравнивания",
			ops1:     func(w *BinWriter) { w.WriteBits([]bool{true, false, true}) /*101*/ },
			ops2:     func(w *BinWriter) { w.WriteBits([]bool{false, true, true}) /*011*/ },
			expected: []byte{0xAC}, //10101100
		},
		{
			name: "Байты и биты, затем снова байты",
			ops1: func(w *BinWriter) {
				w.WriteByte(0xFF)
			},
			ops2: func(w *BinWriter) {
				w.WriteWord(0xAACC)
				w.WriteBits([]bool{true, true})
				w.WriteBits(w.CreateBitsArray(0xDB, 8))
			},
			expected: []byte{0xFF, 0xAA, 0xCC, 0xF6, 0xC0},
		},
		{
			name: "Один объект в другой, а затем его в основной",
			ops1: func(w *BinWriter) {
				w.WriteBits([]bool{true, false, false, true})
			},
			ops2: func(w *BinWriter) {
				w.WriteBits([]bool{true, false})
				buf2 := &bytes.Buffer{}
				w2 := LocalBinWriterInit(buf2)
				w2.WriteBits([]bool{false, true, false})
				w.MergeFrom(w2)
			},
			expected: []byte{0x99, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf1 := &bytes.Buffer{}
			w1 := BinWriterInit(bufio.NewWriter(buf1))
			tt.ops1(w1)

			buf2 := &bytes.Buffer{}
			w2 := LocalBinWriterInit(buf2)
			tt.ops2(w2)

			err := w1.MergeFrom(w2)
			if err != nil {
				t.Fatalf("MergeInto error: %v", err)
			}

			w1.Close()

			result := buf1.Bytes()
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("Expected % X, got % X", tt.expected, result)
			}
		})
	}
}
