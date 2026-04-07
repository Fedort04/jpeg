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
		{"Нулевое значение", 0x0000, []byte{0x00, 0x00}},
		{"Минимальное значение", 0x0001, []byte{0x00, 0x01}},
		{"Максимальное значение", 0xFFFF, []byte{0xFF, 0xFF}},
		{"Среднее значение", 0x1234, []byte{0x12, 0x34}},
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

func TestWriteBits(t *testing.T) {
	buf := &bytes.Buffer{}
	w := BinWriterInit(bufio.NewWriter(buf))

	bits := []bool{true, false, true, false, true, true, false, false}
	err := w.WriteBits(bits)
	if err != nil {
		t.Fatalf("WriteBits вернул ошибку: %v", err)
	}
	w.Close()

	expected := []byte{0xAC} // 10101100
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("Ожидалось % X, получено % X", expected, buf.Bytes())
	}
}

func TestWriteBytes(t *testing.T) {
	buf := &bytes.Buffer{}
	w := BinWriterInit(bufio.NewWriter(buf))

	testData := []byte{0x01, 0x02, 0x03, 0xFF, 0x00}
	err := w.WriteBytes(testData)
	if err != nil {
		t.Fatalf("WriteBytes вернул ошибку: %v", err)
	}
	w.Close()

	if !bytes.Equal(buf.Bytes(), testData) {
		t.Errorf("Ожидалось % X, получено % X", testData, buf.Bytes())
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

func TestFlushBehavior(t *testing.T) {
	buf := &bytes.Buffer{}
	w := BinWriterInit(bufio.NewWriter(buf))

	w.WriteBit(true)
	w.WriteBit(false)
	w.WriteBit(true)

	err := w.Close()
	if err != nil {
		t.Fatalf("Flush вернул ошибку: %v", err)
	}

	expected := []byte{0xA0} // 10100000
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("После Flush ожидалось % X, получено % X", expected, buf.Bytes())
	}

	w.WriteByte(0xFF)
	w.Close()

	expected = []byte{0xA0, 0xFF}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("После продолжения записи ожидалось % X, получено % X", expected, buf.Bytes())
	}
}

func TestMultipleWrites(t *testing.T) {
	buf := &bytes.Buffer{}
	w := BinWriterInit(bufio.NewWriter(buf))

	// Записываем сложную структуру
	data := []struct {
		op  string
		val interface{}
	}{
		{"byte", byte(0x01)},
		{"bits", []bool{true, false}},
		{"twoBytes", uint16(0xABCD)},
		{"bits", []bool{true, true, true, false}},
		{"bytes", []byte{0xDE, 0xAD, 0xBE, 0xEF}},
	}

	for _, d := range data {
		switch d.op {
		case "byte":
			w.WriteByte(d.val.(byte))
		case "bits":
			w.WriteBits(d.val.([]bool))
		case "twoBytes":
			w.WriteWord(d.val.(uint16))
		case "bytes":
			w.WriteBytes(d.val.([]byte))
		}
	}
	w.Close()

	if buf.Len() == 0 {
		t.Error("Не было записано ни одного байта")
	}

	t.Logf("Записано %d байт: % X", buf.Len(), buf.Bytes())
}
