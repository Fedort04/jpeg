package encoder

import (
	"bufio"
	"jpeg/internal/mcu"
	"jpeg/shared"
)

const samplePrecision = 8 //Глубина цвета

// Типы форматов прореживания
type EncodeFormat byte

const (
	Without    EncodeFormat = iota //4:4:4
	Horizontal                     //4:2:2 вертикальный
	Vertival                       //4:2:2 горизонтальный
	Both                           //4:2:0
)

type Encoder struct {
	RestartInterval byte         //Интервал перезапуска дельта кодирования (по умолчанию 5)
	Format          EncodeFormat //Формат прореживания (по умолчанию 4:2:0)

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
	yh              byte               //Горизонтальный фактор яркости
	yv              byte               //Вертикальный фактор яркости
	ch              byte               //Горизонтальный фактор цвета
	cv              byte               //Вертикальный фактор цвета
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
	shared.CopyToMatrix(quantTableY, &encoder.quantTableY)
	shared.CopyToMatrix(quantTableColor, &encoder.quantTableColor)
	encoder.Format = Both
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
	encoder.factorUpdate()
	encoder.convertToYCbCr()
	encoder.blockSubsample()
	return true, nil
}

// Создание единичной таблицы квантования
func CreateOneTable() [][]byte {
	table := make([][]byte, mcu.UnitRowCount)

	for i := range mcu.UnitRowCount {
		row := make([]byte, mcu.UnitColCount)
		for j := range mcu.UnitColCount {
			row[j] = 1
		}
		table[i] = row
	}

	return table
}
