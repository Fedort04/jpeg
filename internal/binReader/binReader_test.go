package binreader

import (
	"bufio"
	"bytes"
	"testing"
)

func TestHuffStreamStart(t *testing.T) {
	t.Run("Запуск потока Хаффмана", func(t *testing.T) {
		buf := &bytes.Buffer{}
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.HuffStreamStart()
		if !reader.isHuffStream {
			t.Error("isHuffStream должен быть true")
		}
		if reader.bitCount != 0 {
			t.Errorf("bitCount = %d, ожидался 0", reader.bitCount)
		}
	})

	t.Run("Запуск потока Хаффмана после чтения байтов", func(t *testing.T) {
		data := []byte{0xFF, 0x00, 0xAA}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.HuffStreamStart()
		b1, err := reader.GetByte()
		if err != nil {
			t.Fatalf("GetByte error: %v", err)
		}
		if b1 != 0xFF {
			t.Errorf("Первый байт = %x, ожидался %x", b1, 0xFF)
		}
		b2, err := reader.GetByte()
		if err != nil {
			t.Fatalf("GetByte error: %v", err)
		}
		// После 0xFF идёт 0x00, который должен быть пропущен, поэтому следующий байт = 0xAA
		if b2 != 0xAA {
			t.Errorf("Второй байт = %x, ожидался %x", b2, 0xAA)
		}
	})
}

func TestHuffStreamEnd(t *testing.T) {
	t.Run("Отключение потока Хаффмана", func(t *testing.T) {
		buf := &bytes.Buffer{}
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.HuffStreamStart()
		reader.HuffStreamEnd()
		if reader.isHuffStream {
			t.Error("isHuffStream должен быть false")
		}
	})

	t.Run("Отключение после чтения с пропуском", func(t *testing.T) {
		data := []byte{0xFF, 0x00, 0xAA}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.HuffStreamStart()
		b1, _ := reader.GetByte() // 0xFF
		reader.HuffStreamEnd()
		// Теперь 0x00 не должен пропускаться
		b2, err := reader.GetByte()
		if err != nil {
			t.Fatalf("GetByte error: %v", err)
		}
		if b2 != 0x00 {
			t.Errorf("После отключения получен %x, ожидался 0x00", b2)
		}
		if b1 != 0xFF {
			t.Errorf("Первый байт = %x, ожидался 0xFF", b1)
		}
	})
}

func TestGetByte(t *testing.T) {
	t.Run("Чтение байта", func(t *testing.T) {
		data := []byte{0x12, 0x34}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))

		b, err := reader.GetByte()
		if err != nil {
			t.Fatalf("GetByte error: %v", err)
		}
		if b != 0x12 {
			t.Errorf("Получен %x, ожидался %x", b, 0x12)
		}

		b, err = reader.GetByte()
		if err != nil {
			t.Fatalf("GetByte error: %v", err)
		}
		if b != 0x34 {
			t.Errorf("Получен %x, ожидался %x", b, 0x34)
		}
	})
}

func TestGetWord(t *testing.T) {
	t.Run("Чтение слова", func(t *testing.T) {
		data := []byte{0x12, 0x34}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.SetEndian(BIG)

		word, err := reader.GetWord()
		if err != nil {
			t.Fatalf("GetWord error: %v", err)
		}
		expected := uint16(0x1234)
		if word != expected {
			t.Errorf("Получено %#x, ожидалось %#x", word, expected)
		}
	})
}

func TestGetNextByte(t *testing.T) {
	t.Run("Получение следующего байта без смещения", func(t *testing.T) {
		data := []byte{0xAB, 0xCD}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))

		next, err := reader.GetNextByte()
		if err != nil {
			t.Fatalf("GetNextByte error: %v", err)
		}
		if next != 0xAB {
			t.Errorf("Получен %x, ожидался %x", next, 0xAB)
		}

		b, err := reader.GetByte()
		if err != nil {
			t.Fatalf("GetByte error: %v", err)
		}
		if b != 0xAB {
			t.Errorf("После GetNextByte GetByte вернул %x, ожидался %x", b, 0xAB)
		}
	})
}

func TestGet4Bit(t *testing.T) {
	t.Run("Разделение байта на полубайты", func(t *testing.T) {
		data := []byte{0xAB}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))

		left, right, err := reader.Get4Bit()
		if err != nil {
			t.Fatalf("Get4Bit error: %v", err)
		}
		if left != 0xA {
			t.Errorf("Левый полубайт = %x, ожидался %x", left, 0xA)
		}
		if right != 0xB {
			t.Errorf("Правый полубайт = %x, ожидался %x", right, 0xB)
		}
	})

	t.Run("Отдельно 0x0FF0", func(t *testing.T) {
		data := []byte{0x0F, 0xF0}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))

		l1, r1, err := reader.Get4Bit()
		if err != nil {
			t.Fatalf("Get4Bit error: %v", err)
		}
		if l1 != 0x0 || r1 != 0xF {
			t.Errorf("Первая пара: left=%x right=%x, ожидались 0x0 и 0xF", l1, r1)
		}

		l2, r2, err := reader.Get4Bit()
		if err != nil {
			t.Fatalf("Get4Bit error: %v", err)
		}
		if l2 != 0xF || r2 != 0x0 {
			t.Errorf("Вторая пара: left=%x right=%x, ожидались 0xF и 0x0", l2, r2)
		}
	})
}

func TestGetBit(t *testing.T) {
	t.Run("Побитовое чтение", func(t *testing.T) {
		data := []byte{0xAB}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.SetEndian(BIG)

		bits := make([]byte, 8)
		var err error
		for i := range 8 {
			bits[i], err = reader.GetBit()
			if err != nil {
				t.Fatalf("GetBit error on bit %d: %v", i, err)
			}
		}
		expected := []byte{1, 0, 1, 0, 1, 0, 1, 1}
		for i := range 8 {
			if bits[i] != expected[i] {
				t.Errorf("Бит %d = %d, ожидался %d", i, bits[i], expected[i])
			}
		}
	})

	t.Run("Чтение через границу байта", func(t *testing.T) {
		data := []byte{0xFF, 0x80} // 11111111 10000000
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.SetEndian(BIG)

		for i := range 9 {
			bit, err := reader.GetBit()
			if err != nil {
				t.Fatalf("GetBit error: %v", err)
			}
			if bit != 1 {
				t.Errorf("Бит %d = %d, ожидался 1", i, bit)
			}
		}
		// Тут должен быть нолик
		bit, err := reader.GetBit()
		if err != nil {
			t.Fatalf("GetBit error: %v", err)
		}
		if bit != 0 {
			t.Errorf("10-й бит = %d, ожидался 0", bit)
		}
	})
}

func TestGetBits(t *testing.T) {

	t.Run("Чтение 5 бит из одного байта", func(t *testing.T) {
		// Байт 0x6A = 01101010, первые 5 бит: 01101 (13)
		data := []byte{0x6A}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.SetEndian(BIG)

		val, err := reader.GetBits(5)
		if err != nil {
			t.Fatalf("GetBits error: %v", err)
		}
		expected := uint16(0b01101) // 13
		if val != expected {
			t.Errorf("Получено %d, ожидалось %d", val, expected)
		}
		remaining, _ := reader.GetBits(3)
		if remaining != 0b010 {
			t.Errorf("Оставшиеся 3 бита = %d, ожидались 2", remaining)
		}
	})

	t.Run("Чтение битов через несколько байтов", func(t *testing.T) {
		// Байты: 0x01 (00000001), 0xFF (11111111)
		// Результат: 000000011111 = 0b11111 = 31
		data := []byte{0x01, 0xFF}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.SetEndian(BIG)

		val, err := reader.GetBits(12)
		if err != nil {
			t.Fatalf("GetBits error: %v", err)
		}
		expected := uint16(0b000000011111) // 31
		if val != expected {
			t.Errorf("Получено %d, ожидалось %d", val, expected)
		}
	})
}

func TestBitsAlign(t *testing.T) {
	t.Run("Выравнивание после частичного чтения битов", func(t *testing.T) {
		// 3 байта: 0xAB, 0xCD, 0xEF
		data := []byte{0xAB, 0xCD, 0xEF}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.SetEndian(BIG)

		// 3 бита (101)
		bits, err := reader.GetBits(3)
		if err != nil {
			t.Fatalf("GetBits error: %v", err)
		}
		if bits != 0b101 {
			t.Errorf("Прочитано %d, ожидалось 5", bits)
		}

		err = reader.BitsAlign()
		if err != nil {
			t.Fatalf("BitsAlign error: %v", err)
		}

		// Теперь читаем байт — должен быть 0xEF
		b, err := reader.GetByte()
		if err != nil {
			t.Fatalf("GetByte error: %v", err)
		}
		if b != 0xEF {
			t.Errorf("После выравнивания получен %x, ожидался %x", b, 0xEF)
		}
	})

	t.Run("Выравнивание при нулевом смещении", func(t *testing.T) {
		data := []byte{0x12, 0x34, 0x36}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))
		reader.SetEndian(BIG)

		b1, _ := reader.GetByte()
		if b1 != 0x12 {
			t.Fatalf("Первый байт = %x", b1)
		}
		// Теперь bitCount = 0, выравнивание должно просто перейти к следующему байту
		err := reader.BitsAlign()
		if err != nil {
			t.Fatalf("BitsAlign error: %v", err)
		}
		b2, err := reader.GetWord()
		if err != nil {
			t.Fatalf("GetByte error: %v", err)
		}
		if b2 != 0x3436 {
			t.Errorf("После выравнивания получен %x, ожидался %x", b2, 0x3436)
		}
	})
}

func TestGetArray(t *testing.T) {
	t.Run("Чтение среза байт", func(t *testing.T) {
		data := []byte{0x01, 0x02, 0x03, 0x04}
		buf := bytes.NewBuffer(data)
		reader := BinReaderInit(bufio.NewReader(buf))

		arr, err := reader.GetArray(4)
		if err != nil {
			t.Fatalf("GetArray error: %v", err)
		}
		if len(arr) != 4 {
			t.Errorf("Длина массива = %d, ожидалась 4", len(arr))
		}
		for i := 0; i < 4; i++ {
			if arr[i] != data[i] {
				t.Errorf("arr[%d] = %x, ожидался %x", i, arr[i], data[i])
			}
		}
	})
}
