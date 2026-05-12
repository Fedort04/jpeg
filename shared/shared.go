package shared

import (
	"math"
)

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

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
func CreateYCbCrBlock(height byte, width byte, unitRowCount int, unitColCount int) [][]YCbCrMatrix {
	res := make([][]YCbCrMatrix, height)
	for i := range height {
		res[i] = make([]YCbCrMatrix, width)
		for j := range width {
			res[i][j] = CreateMatrix[YCbCr](unitRowCount, unitColCount)
		}
	}
	return res
}

// Вычисление категории в соответствии с F.1
func FindCategory(val int16) byte {
	if val == 0 {
		return 0
	}

	abs := int16(math.Abs(float64(val)))
	n := byte(0)
	for (1 << n) <= abs {
		n++
	}
	return n
}

// Асболютное значение
func Abs[T Number](val T) T {
	return T(math.Abs(float64(val)))
}

// Проверка, был ли значимым этот коэфф в предыдущем скане аппроксимации
// Передача оригинального значения
func CheckHistory(val int16, app byte) bool {
	return Abs(val)>>app == 1
}

// Копирует данные матрицы src в матрицу dst
func CopyToMatrix[T any](src [][]T, dst *[][]T) {
	if src == nil {
		*dst = nil
		return
	}

	newMatrix := make([][]T, len(src))

	for i := range len(src) {
		if src[i] != nil {
			newMatrix[i] = make([]T, len(src[i]))
			copy(newMatrix[i], src[i])
		} else {
			newMatrix[i] = nil
		}
	}

	*dst = newMatrix
}

// Вычисление среднего значения двух чисел
func Average[T Number](a, b T) T {
	return T(float64(a+b) / 2)
}

// Умножение каждого элемента матрицы на число
func MultMatrixOnNumber[T Number](matrix [][]T, number T) [][]T {
	res := CreateMatrix[T](len(matrix), len(matrix[0]))
	for i := range len(matrix) {
		for j := range len(matrix[i]) {
			res[i][j] = matrix[i][j] * number
		}
	}
	return res
}

// map функция для матрицы. Применяет f() к каждому элементу матрицы в порядке слева-направо сверху-вниз
func MatrixMap[T any](matrix [][]T, f func(elm *T)) {
	for _, row := range matrix {
		for _, elm := range row {
			f(&elm)
		}
	}
}

// Слияние двух ассоциативных массивов в левый аргумент с суммированием конфликтов
func MergeInto[T Number](left, right map[T]int) {
	for k, v := range right {
		left[k] += v
	}
}

// map функция для матрицы. Применяет f() к каждому элементу матрицы в порядке слева-направо сверху-вниз
// Вариант, который обрабатывает возникающие ошибки
func MatrixMapError[T any](matrix [][]T, f func(elm *T) error) error {
	for _, row := range matrix {
		for _, elm := range row {
			if err := f(&elm); err != nil {
				return err
			}
		}
	}
	return nil
}

// map функция для матрицы. Применяет f() к каждому элементу матрицы в порядке слева-направо сверху-вниз
// Вариант, который обрабатывает возникающие ошибки и выполняет заданное количество строк
func MatrixMapRows[T any](matrix [][]T, startRow, numOfRows uint16, f func(elm *T) error) error {
	for i := startRow; i < startRow+numOfRows; i++ {
		if i >= uint16(len(matrix)) {
			return nil
		}

		row := matrix[i]
		for _, elm := range row {
			if err := f(&elm); err != nil {
				return err
			}
		}
	}
	return nil
}

// Сравнение двух слайсов
func CompareSlices[T Number](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// @todo полезный, но не подходит((
// Копирует данные части src в dst
// hPos; vPos характеризуют верхний левый угол части, а height; width размер части
// func CopyPartToMatrix[T any, M constraints.Integer](dst *[][]T, src [][]T, hPos M, vPos M, height M, width M) error {
// 	if src == nil {
// 		*dst = nil
// 		return errors.New("src is nil")
// 	}

// 	newMatrix := make([][]T, height)

// 	for i := range height {
// 		if src[hPos+i] != nil {
// 			newMatrix[i] = make([]T, width)

// 			for j := range width {
// 				newMatrix[i][j] = src[hPos+i][vPos+j]
// 			}
// 		} else {
// 			return errors.New("Index height range out of src array")
// 		}
// 	}
// 	return nil
// }

// Маркеры всех используемых заголовков
const (
	SOI   uint16 = 0xFFD8
	EOI   uint16 = 0xFFD9
	SOF0  uint16 = 0xFFC0 //Baseline
	SOF2  uint16 = 0xFFC2 //Progressive
	APP0  uint16 = 0xFFE0
	APP15 uint16 = 0xFFEF
	DQT   uint16 = 0xFFDB
	DHT   uint16 = 0xFFC4
	SOS   uint16 = 0xFFDA
	DRI   uint16 = 0xFFDD
	RST0  uint16 = 0xFFD0
	RST7  uint16 = 0xFFD7
)

const EndOfBlock = 0x00   //Конец блока AC
const ZRL = 0xF0          //группа из 16 нулей
const NumOfRstMarkers = 8 //Количество RST маркеров
const NumOfTables = 4     //Максимальное количество таблиц
const NumOfChannels = 3   //Максимальное количество цветовых компонент
const MaxComps = 3        //Максимальное количество компонент
const SizeOfTable = 64    //Количество элементов в одной таблице квантования
