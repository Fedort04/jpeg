package binreader

import "testing"

const source = "../pics/Aqours.jpg" //Файл с данными, по которому делается тест

func TestGetByte(t *testing.T) {
	reader, err := BinReaderInit(source, BIG)
	if err != nil {
		t.Fatal("BinReaderInit -> error", err.Error())
	}
	if temp := reader.GetByte(); temp != 0xFF {
		t.Fatal("Read:", temp, "Expect:", 0xFF)
	}
	if temp := reader.GetByte(); temp != 0xD8 {
		t.Fatal("Read:", temp, "Expect:", 0xD8)
	}
}

func TestGetWord(t *testing.T) {
	reader, err := BinReaderInit(source, BIG)
	if err != nil {
		t.Fatal("BinReaderInit -> error", err.Error())
	}
	if temp := reader.GetWord(); temp != 0xFFD8 {
		t.Fatal("Read:", temp, "Expect:", 0xFFD8)
	}
	reader, err = BinReaderInit(source, LITTLE)
	if err != nil {
		t.Fatal("BinReaderInit -> error", err.Error())
	}
	if temp := reader.GetWord(); temp != 0xD8FF {
		t.Fatal("Read:", temp, "Expect:", 0xD8FF)
	}
}

func TestGetArray(t *testing.T) {
	reader, err := BinReaderInit(source, BIG)
	if err != nil {
		t.Fatal("BinReaderInit -> error", err.Error())
	}
	temp := reader.GetArray(4)
	res := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	for i := range 4 {
		if temp[i] != res[i] {
			t.Fatal("Read:", temp[i], "Expect:", res[i])
		}
	}
}

func TestGet4Bit(t *testing.T) {
	reader, err := BinReaderInit(source, BIG)
	if err != nil {
		t.Fatal("BinReaderInit -> error", err.Error())
	}
	first, second := reader.Get4Bit()
	if first != 0xF || second != 0xF {
		t.Fatal("Read:", first, "-", second, "Expect: 15-15")
	}
}

func TestGetBit(t *testing.T) {
	reader, err := BinReaderInit(source, BIG)
	if err != nil {
		t.Fatal("BinReaderInit -> error", err.Error())
	}
	reader.GetByte()
	answer := []byte{1, 1, 0, 1, 1, 0, 0, 0}
	for i := range 8 {
		if temp := reader.GetBit(); temp != answer[i] {
			t.Fatal("Read:", temp, "Expect:", answer[i])
		}
	}
	reader.GetByte()
	reader.SetEndian(LITTLE)
	answer = []byte{0, 0, 0, 0, 0, 1, 1, 1}
	for i := range 8 {
		if temp := reader.GetBit(); temp != answer[i] {
			t.Fatal("Read:", temp, "Expect:", answer[i])
		}
	}
}

func TestGetBits(t *testing.T) {
	reader, err := BinReaderInit(source, BIG)
	if err != nil {
		t.Fatal("BinReaderInit -> error", err.Error())
	}
	reader.GetByte()
	if temp := reader.GetBits(7); temp != 0b1101100 {
		t.Fatal("Read:", temp, "Expect:", 0b0101100)
	}
	reader.SetEndian(LITTLE)
	reader.GetByte()
	if temp := reader.GetBits(7); temp != 0b11 {
		t.Fatal("Read:", temp, "Expect:", 0b11)
	}
}

func TestBitsAlign(t *testing.T) {
	reader, err := BinReaderInit(source, BIG)
	if err != nil {
		t.Fatal("BinReaderInit -> error", err.Error())
	}
	reader.GetBit()
	reader.BitsAlign()
	if temp := reader.GetBits(3); temp != 0b110 {
		t.Fatal("Read:", temp, "Expect:", 0b110)
	}
}

func TestGetNextByte(t *testing.T) {
	reader, err := BinReaderInit(source, BIG)
	if err != nil {
		t.Fatal("BinReaderInit -> error", err.Error())
	}
	reader.GetByte()
	if temp := reader.GetNextByte(); temp != reader.GetByte() {
		t.Fatal("Read:", temp, "Expect:", 0xD8)
	}
}
