package encoder

import (
	"bufio"
	"bytes"
	binwriter "jpeg/encoder/binWriter"
	"log"
)

type Image = byte

const samplePrecision = 8 //Глубина цвета

type Encoder struct {
	Data            Image    //Данные изображения
	QuantTableY     [][]byte //Таблица квантования для яркости
	QuantTableColor [][]byte //Таблица квантования для цвета
	Yh              byte     //Горизонтальный фактор яркости (по умолчанию 2)
	Yv              byte     //Вертикальный фактор яркости (по умолчанию 2)
	Ch              byte     //Горизонтальный фактор цвета (по умолчанию 1)
	Cb              byte     //Вертикальный фактор цвета (по умолчанию 1)
	RestartInterval byte     //Интервал перезапуска дельта кодирования (по умолчанию 5)

	// Не используется при Baseline кодировании
	Yspectral []byte //SpectralSelection яркости (по умолчанию [0, 5, 63])
	Cspectral []byte //SpectralSelection цвета (по умолчанию [0, 63])
	Yapprox   byte   //Аппроксимация яркости (по умолчанию 2)
	Capprox   byte   //Аппроксимация цвета (по умолчанию 1)
}

func CreateEncoder(dest *bufio.Writer, data Image, quantTableY [][]byte, quantTableColor [][]byte) (*Encoder, error) {
	return nil, nil
}

// По вызову функции выполняется Baseline кодирование
func (encoder *Encoder) StartBaseline(numOfRows uint16) (bool, error) {
	return true, nil
}

// По вызову функции выполняется Progressive кодирование
func (encoder *Encoder) StartProgressive(numOfScans byte) (bool, error) {
	return true, nil
}

func Encode() {
	buf := &bytes.Buffer{}
	w := binwriter.BinWriterInit(bufio.NewWriter(buf))

	number := []bool{true, false, true, true, false, true, false, false}
	for _, bit := range number {
		err := w.WriteBit(bit)
		if err != nil {
			log.Fatalf("WriteBit вернул ошибку: %v", err)
		}
	}
	w.Close()
}
