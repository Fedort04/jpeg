package encoder

import (
	"bufio"
	"jpeg/shared"
)

const samplePrecision = 8 //Глубина цвета

type Encoder struct {
	Yh              byte //Горизонтальный фактор яркости (по умолчанию 2)
	Yv              byte //Вертикальный фактор яркости (по умолчанию 2)
	Ch              byte //Горизонтальный фактор цвета (по умолчанию 1)
	Cv              byte //Вертикальный фактор цвета (по умолчанию 1)
	RestartInterval byte //Интервал перезапуска дельта кодирования (по умолчанию 5)

	// Не используется при Baseline кодировании
	Yspectral []byte //SpectralSelection яркости (по умолчанию [0, 5, 63])
	Cspectral []byte //SpectralSelection цвета (по умолчанию [0, 63])
	Yapprox   byte   //Аппроксимация яркости (по умолчанию 2)
	Capprox   byte   //Аппроксимация цвета (по умолчанию 1)

	//private:
	data            *shared.Image      //Данные изображения
	imgHeight       uint16             //Высота изображения
	imgWidth        uint16             //Ширина изображения
	quantTableY     [][]byte           //Таблица квантования для яркости
	quantTableColor [][]byte           //Таблица квантования для цвета
	maxH            byte               //Максимальный H фактор
	maxV            byte               //Максимальный V фактор
	numBlocksHeight uint16             //Количество блоков mcu в изображении по высоте
	numBlocksWidth  uint16             //Количество блоков mcu в изображении по ширине
	blockVSize      byte               //Размер блока по вертикали
	blockHSize      byte               //Размер блока по горизонтали
	img             shared.YCbCrMatrix //Изображение в виде YCbCr
}

// Конструктор объекта кодирования
func CreateEncoder(dest *bufio.Writer, data shared.Image, quantTableY [][]byte, quantTableColor [][]byte) (*Encoder, error) {
	var encoder Encoder
	encoder.data = &data
	copyToMatrix(quantTableY, &encoder.quantTableY)
	copyToMatrix(quantTableColor, &encoder.quantTableColor)
	encoder.Yh = 2
	encoder.Yv = 2
	encoder.Ch = 2
	encoder.Cv = 2
	encoder.RestartInterval = 5
	// Для прогрессива
	encoder.Yspectral = []byte{0, 5, 63}
	encoder.Cspectral = []byte{0, 63}
	encoder.Yapprox = 2
	encoder.Capprox = 1
	return &encoder, nil
}

// По вызову функции выполняется Baseline кодирование
func (encoder *Encoder) StartBaseline(numOfRows uint16) (bool, error) {
	return true, nil
}

// По вызову функции выполняется Progressive кодирование
func (encoder *Encoder) StartProgressive(numOfScans byte) (bool, error) {
	encoder.convertToYCbCr()
	encoder.subsample()
	return true, nil
}

// Создание единичной таблицы квантования
func CreateOneTable() [][]byte {
	table := make([][]byte, shared.MinMatrixSize)

	for i := range shared.MinMatrixSize {
		row := make([]byte, shared.MinMatrixSize)
		for j := range shared.MinMatrixSize {
			row[j] = 1
		}
		table[i] = row
	}

	return table
}
