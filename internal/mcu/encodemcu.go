package mcu

import "math"

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

// Структура для хранения и обработки одного блока subsample данных
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
func (block *BlockRaw) Quantization(tableY, tableColor [][]byte) {
	for _, arr := range block.Y {
		for _, elm := range arr {
			elm.quantization(tableY)
		}
	}
	block.Cb.quantization(tableColor)
	block.Cr.quantization(tableColor)
}

// Зиг-заг преобразование для всего блока
func (block *BlockRaw) ZigZag(maxH, maxV byte) CodingBlock {
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
