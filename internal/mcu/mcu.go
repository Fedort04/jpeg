package mcu

import (
	"jpeg/shared"
	"math"
)

// Последовательность зиг-зага
var zigZagTable = [8][8]byte{
	{0, 1, 5, 6, 14, 15, 27, 28},
	{2, 4, 7, 13, 16, 26, 29, 42},
	{3, 8, 12, 17, 25, 30, 41, 43},
	{9, 11, 18, 24, 31, 40, 44, 53},
	{10, 19, 23, 32, 39, 45, 52, 54},
	{20, 22, 33, 38, 46, 51, 55, 60},
	{21, 34, 37, 47, 50, 56, 59, 61},
	{35, 36, 48, 49, 57, 58, 62, 63},
}

const DCTQuantCoeff = 4 // Коэффициент для таблиц квантования

// Таблица с коэффициентами в ОДКП
var idctTable = [8][8]float64{
	{0.707107, 0.707107, 0.707107, 0.707107, 0.707107, 0.707107, 0.707107, 0.707107},
	{0.980785, 0.831470, 0.555570, 0.195090, -0.195090, -0.555570, -0.831470, -0.980785},
	{0.923880, 0.382683, -0.382683, -0.923880, -0.923880, -0.382683, 0.382683, 0.923880},
	{0.831470, -0.195090, -0.980785, -0.555570, 0.555570, 0.980785, 0.195090, -0.831470},
	{0.707107, -0.707107, -0.707107, 0.707107, 0.707107, -0.707107, -0.707107, 0.707107},
	{0.555570, -0.980785, 0.195090, 0.831470, -0.831470, -0.195090, 0.980785, -0.555570},
	{0.382683, -0.923880, 0.923880, -0.382683, -0.382683, 0.923880, -0.923880, 0.382683},
	{0.195090, -0.555570, 0.831470, -0.980785, 0.980785, -0.831470, 0.555570, -0.195090},
}

const UnitRowCount = 8 //Количество строк в mcu
const UnitColCount = 8 //Количество столбцов в mcu

type Channel byte

const (
	Y  Channel = 0
	Cb Channel = 1
	Cr Channel = 2
)

// Структура для MCU
type MCU struct {
	//Пиксели текущего блока (коэффициенты из потока)
	Y  []int16 //Коэффициент из потока
	Cb []int16 //Коэффициент из потока
	Cr []int16 //Коэффициент из потока
}

// Конструткор MCU
func MakeMCU() MCU {
	var res MCU
	res.Y = make([]int16, UnitRowCount*UnitColCount)
	res.Cb = make([]int16, UnitRowCount*UnitColCount)
	res.Cr = make([]int16, UnitRowCount*UnitColCount)
	return res
}

// Сoздание пустой матрицы MCU
func CreateMCUMatrix(MCUsHeight uint16, MCUsWidth uint16) [][]MCU {
	blocks := make([][]MCU, MCUsHeight)
	for i := range MCUsHeight {
		blocks[i] = make([]MCU, MCUsWidth)
		for j := range MCUsWidth {
			blocks[i][j] = MakeMCU()
		}
	}
	return blocks
}

// Копирование значений текущего MCU в dst
func (unit *MCU) Copy(dst *MCU) {
	copy(dst.Y, unit.Y)
	copy(dst.Cb, unit.Cb)
	copy(dst.Cr, unit.Cr)
}

// Деквантование
// Передается номер канала ch и таблица квантования для него
func (unit *MCU) Dequant(quantTable []byte, ch Channel) {
	switch ch {
	case Y:
		for i := range unit.Y {
			unit.Y[i] = unit.Y[i] * int16(quantTable[i])
		}
	case Cb:
		for i := range unit.Cb {
			unit.Cb[i] = unit.Cb[i] * int16(quantTable[i])
		}
	case Cr:
		for i := range unit.Cr {
			unit.Cr[i] = unit.Cr[i] * int16(quantTable[i])
		}
	}
}

// Зиг-заг преобразование (в матрицу)
func zigZagMatrix(unit []int16) [][]int16 {
	//Создание матрицы
	res := make([][]int16, UnitRowCount)
	for i := range UnitRowCount {
		res[i] = make([]int16, UnitColCount)
		for j := range UnitColCount {
			res[i][j] = unit[zigZagTable[i][j]]
		}
	}
	return res
}

// Обратное дискретно-косинусное преобразование
func idctCalc(unit [][]int16) [][]float32 {
	res := make([][]float32, UnitRowCount)
	for i := range UnitRowCount {
		res[i] = make([]float32, UnitColCount)
	}
	for x := range UnitRowCount {
		for y := range UnitColCount {
			sum := 0.0
			for u := range UnitRowCount {
				for v := range UnitColCount {
					sum += float64(unit[u][v]) * idctTable[u][x] * idctTable[v][y]
				}
			}
			res[x][y] = float32(0.25 * sum)
		}
	}
	return res
}

// Обратное дискретно-косинусное преобразование канала ch
// Используя ее создается блок MCU, который обрабатывается до ргб и записывается в результат
func (unit *MCU) InverseCosin(ch Channel) [][]float32 {
	switch ch {
	case Y:
		return idctCalc(zigZagMatrix(unit.Y))
	case Cb:
		return idctCalc(zigZagMatrix(unit.Cb))
	case Cr:
		return idctCalc(zigZagMatrix(unit.Cr))
	default:
		return nil
	}
}

// Один MCU в сыром виде
type RawMCU struct {
	Data [][]float32
}

// Дискретно-косинусное преобразование над одной MCU
func (mcu *RawMCU) dct() {
	for i := range UnitRowCount {
		for j := range UnitColCount {
			mcu.Data[i][j] -= 128.0
		}
	}

	var tmp [UnitRowCount][UnitColCount]float64

	for row := range UnitRowCount {
		for u := range UnitColCount {
			var sum float64
			for col, val := range mcu.Data[row] {
				sum += float64(val) * idctTable[u][col]
			}
			tmp[row][u] = sum
		}
	}

	for v := range UnitRowCount {
		for u := range UnitColCount {
			var sum float64
			for row := range UnitRowCount {
				sum += tmp[row][u] * idctTable[v][row]
			}
			mcu.Data[v][u] = float32(math.Round(sum))
		}
	}
}

// Квантование сырых данных
func (mcu *RawMCU) quantization(quantTable [][]byte) {
	for i, row := range mcu.Data {
		for j, elm := range row {
			mcu.Data[i][j] = float32(math.Round(float64(elm / float32(quantTable[i][j]))))
		}
	}
}

// Зиг-заг преобразование (в строку)
func ZigZagRow[T ~int16 | ~byte, P ~byte | ~float32](data [][]P) []T {
	result := make([]T, UnitRowCount*UnitColCount)
	for row := range UnitRowCount {
		for col := range UnitColCount {
			idx := zigZagTable[row][col]
			result[idx] = T(data[row][col])
		}
	}
	return result
}

// Структура для хранеия и обработки одного блока subsample данных
// Содержит сырые данные после subsample для baseline кодирования
type BlockRaw struct {
	Y  [][]RawMCU
	Cb RawMCU
	Cr RawMCU
}

// Дискретно-косинусное преобразование над блоком MCU
func (block *BlockRaw) DCT() {
	for _, arr := range block.Y {
		for _, elm := range arr {
			elm.dct()
		}
	}
	block.Cb.dct()
	block.Cr.dct()
}

// Квантование блока MCU
func (block *BlockRaw) Quantization(tableY [][]byte, tableColor [][]byte) {
	for _, arr := range block.Y {
		for _, elm := range arr {
			elm.quantization(tableY)
		}
	}
	block.Cb.quantization(tableColor)
	block.Cr.quantization(tableColor)
}

// Зиг-заг преобразование для всего блока
func (block *BlockRaw) ZigZag(maxH byte, maxV byte) CodingBlock {
	var res CodingBlock
	res.Y = make([][]int16, maxH*maxV)

	temp := byte(0)
	for _, arr := range block.Y {
		for _, elm := range arr {
			res.Y[temp] = ZigZagRow[int16](elm.Data)
			temp++
		}
	}
	res.Cb = ZigZagRow[int16](block.Cb.Data)
	res.Cr = ZigZagRow[int16](block.Cr.Data)

	return res
}

// Блок, подготовленный для кодирования
type CodingBlock struct {
	Y  [][]int16
	Cb []int16
	Cr []int16
}

// Получение гистоаграммы из массива row в отрезке ss-se
func channelHist(row []int16, ss byte, se byte) map[uint16]int {
	res := make(map[uint16]int)
	var zeroCounter byte

	for k := ss; k <= se; k++ {
		val := row[k]
		if val == 0 {
			zeroCounter++
			continue
		}

		for zeroCounter >= 16 {
			zeroCounter -= 16
			res[shared.ZRL]++
		}

		ssss := shared.FindCategory(val)
		rs := uint16((zeroCounter << 4) | ssss)
		res[rs]++
		zeroCounter = 0
	}

	if zeroCounter > 0 {
		res[shared.EndOfBlock]++
	}
	return res
}

// Гистограмма частоты для прогрессива
func ChannelHistProg(row []int16, ss byte, se byte, eobCounter *int) map[uint16]int {
	res := make(map[uint16]int)
	allZero := true
	for k := ss; k <= se; k++ {
		if row[k] != 0 {
			allZero = false
			break
		}
	}

	if allZero {
		*eobCounter++
		return res
	}

	if *eobCounter != 0 {
		ssss := shared.FindCategory(int16(*eobCounter)) - 1
		res[uint16(ssss<<4)]++
		*eobCounter = 0
	}

	var zeroCounter byte

	for k := ss; k <= se; k++ {
		val := row[k]
		if val == 0 {
			zeroCounter++
			continue
		}

		for zeroCounter >= 16 {
			zeroCounter -= 16
			res[shared.ZRL]++
		}

		ssss := shared.FindCategory(val)
		rs := uint16((zeroCounter << 4) | ssss)
		res[rs]++
		zeroCounter = 0
	}

	if zeroCounter > 0 {
		res[shared.EndOfBlock]++
	}
	return res
}

// Получение гистограммы частоты встречаемых символов канала ch в отрезке ss-se
func (block *CodingBlock) GetChannelHist(ch byte, ss byte, se byte) map[uint16]int {
	res := make(map[uint16]int)
	channel := Channel(ch)

	switch channel {
	case Y:
		for _, row := range block.Y {
			shared.MergeInto(res, channelHist(row, ss, se))
		}
	default:
		shared.MergeInto(res, channelHist(block.Cb, ss, se))
		shared.MergeInto(res, channelHist(block.Cr, ss, se))
	}

	return res
}
