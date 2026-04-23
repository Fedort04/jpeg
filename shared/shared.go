package shared

import (
	"jpeg/internal/mcu"
	"math"
)

type Image = [][]Rgb

// Структура для хранения данных в RGB формате
type Rgb struct {
	R byte
	G byte
	B byte
}

// Перевод в YCbCr пространство по указателю
func (cur *Rgb) ToYCbCr(res *YCbCr) {
	res.Y = Clamp255Float(0.299*float64(cur.R) + 0.587*float64(cur.G) + 0.114*float64(cur.B))
	res.Cb = Clamp255Float(-0.168736*float64(cur.R) - 0.331264*float64(cur.G) + 0.5*float64(cur.B) + 128)
	res.Cr = Clamp255Float(0.5*float64(cur.R) - 0.418688*float64(cur.G) - 0.081312*float64(cur.B) + 128)
}

// Generic функция для создания матриц
func CreateMatrix[T any](height int, width int) [][]T {
	res := make([][]T, height)
	for i := range height {
		res[i] = make([]T, width)
	}
	return res
}

const rgbDelta = 128 //Константа, которая прибавляется при переводе в RGB

// Проверка в диапазоне 0-255 (int)
func Clamp255Int(val int) byte {
	min := 0
	max := 255
	if val < min {
		return byte(min)
	}
	if val > max {
		return byte(max)
	}
	return byte(val)
}

// Проверка в диапазоне 0-255 (float)
func Clamp255Float(val float64) float32 {
	min := float64(0)
	max := float64(255)
	if val < min {
		return float32(min)
	}
	if val > max {
		return float32(max)
	}
	return float32(val)
}

type YCbCrMatrix = [][]YCbCr

// Структура для хранения данных в YCbCr формате
type YCbCr struct {
	Y  float32
	Cb float32
	Cr float32
}

// Перевод в RGB пространство по указателю
func (cur *YCbCr) ToRGB(res *Rgb) {
	cur.Y += rgbDelta
	cur.Cb += rgbDelta
	cur.Cr += rgbDelta
	res.R = Clamp255Int(int(math.Round(float64(cur.Y) + 1.402*float64((float64(cur.Cr)-rgbDelta)))))
	res.G = Clamp255Int(int(math.Round(float64(cur.Y) - 0.34414*float64((float64(cur.Cb)-rgbDelta)) - 0.71414*float64((float64(cur.Cr)-rgbDelta)))))
	res.B = Clamp255Int(int(math.Round(float64(cur.Y) + 1.772*float64((float64(cur.Cb)-rgbDelta)))))
}

// Создание пустого блока размерами [height][width] из MCU(8х8) в YCbCr
func CreateYCbCrBlock(height byte, width byte) [][]YCbCrMatrix {
	res := make([][]YCbCrMatrix, height)
	for i := range height {
		res[i] = make([]YCbCrMatrix, width)
		for j := range width {
			res[i][j] = CreateMatrix[YCbCr](mcu.UnitRowCount, mcu.UnitColCount)
		}
	}
	return res
}

// Маркеры всех используемых заголовков
const (
	SOI   uint16 = 0xFFD8
	EOI   uint16 = 0xFFD9
	SOF0  uint16 = 0xFFC0
	SOF2  uint16 = 0xFFC2
	APP0  uint16 = 0xFFE0
	APP15 uint16 = 0xFFEF
	DQT   uint16 = 0xFFDB
	DHT   uint16 = 0xFFC4
	SOS   uint16 = 0xFFDA
	DRI   uint16 = 0xFFDD
	RST0  uint16 = 0xFFD0
	RST7  uint16 = 0xFFD7
)

const NumOfTables = 4   //Максимальное количество таблиц
const NumOfChannels = 3 //Максимальное количество цветовых компонент
const MaxComps = 3      //Максимальное количество компонент
const MinMatrixSize = 8 //Размерность mcu
const SizeOfTable = 64  //Количество элементов в одной таблице квантования
