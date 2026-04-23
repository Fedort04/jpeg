package encoder

import (
	"jpeg/internal/mcu"
	"jpeg/shared"
)

// Структура для хранения и обработки одной матрицы данных
type componentMatrix struct {
	data [][]float32
}

// Копирует данные матрицы src в матрицу dst
func copyToMatrix[T any](src [][]T, dst *[][]T) {
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

// Перевод изображения в YCbCr
func (jpeg *Encoder) convertToYCbCr() {
	data := *jpeg.data
	jpeg.imgHeight = uint16(len(data))
	jpeg.imgWidth = uint16(len(data[0]))
	jpeg.img = shared.CreateMatrix[shared.YCbCr](int(jpeg.imgHeight), int(jpeg.imgWidth))

	for i := range jpeg.imgHeight {
		for j := range jpeg.imgWidth {
			data[i][j].ToYCbCr(&jpeg.img[i][j])
		}
	}
}

// Вычисление максмальных значений факторов
func (jpeg *Encoder) setMaxComps() {
	jpeg.maxH = max(jpeg.Yh, jpeg.Ch)
	jpeg.maxV = max(jpeg.Yv, jpeg.Cv)
}

// Разложение блока данных на матрицы цветов
// Возвращает массив с матрицами компонент для текущего блока (YCbCr)
func (jpeg *Encoder) blockDecompose(blockH uint16, blockW uint16) []componentMatrix {
	res := make([]componentMatrix, jpeg.Yh*jpeg.Yv+2*jpeg.Ch*jpeg.Cv)

	// Сначала люма
	for curV := range jpeg.Yv {
		for curH := range jpeg.Yh {
			var curMatrix componentMatrix
			curMatrix.data = shared.CreateMatrix[float32](shared.MinMatrixSize, shared.MinMatrixSize)

			amendV := uint16(curV) * shared.MinMatrixSize
			amendH := uint16(curH) * shared.MinMatrixSize
			curStartRowPix := blockH*uint16(jpeg.blockVSize) + amendV
			curStartColPix := blockW*uint16(jpeg.blockHSize) + amendH
			for row := curStartRowPix; row < curStartRowPix+shared.MinMatrixSize; row++ {
				for col := curStartColPix; col < curStartColPix+shared.MinMatrixSize; col++ {

				}
			}
			res = append(res, curMatrix)
		}
	}

	// Затем цвет
	return res
}

// Chroma subsample
// Возвращает матрицу обработанных блоков в массивы матриц данных (например для 2:1:1 -> 6 матриц)
func (jpeg *Encoder) subsample() [][][]componentMatrix {
	jpeg.setMaxComps()

	res := make([][][]componentMatrix, 0)

	numOfMCUHeightReal := (jpeg.imgHeight + (mcu.UnitRowCount - 1)) / (mcu.UnitRowCount)
	numOfMCUHeight := numOfMCUHeightReal + numOfMCUHeightReal%uint16(jpeg.maxV)
	numOfMCUWidthReal := (jpeg.imgWidth + (mcu.UnitColCount - 1)) / (mcu.UnitColCount)
	numOfMCUWidth := numOfMCUWidthReal + numOfMCUWidthReal%uint16(jpeg.maxH)
	jpeg.numBlocksHeight = numOfMCUHeight / uint16(jpeg.maxV)
	jpeg.numBlocksWidth = numOfMCUWidth / uint16(jpeg.maxH)
	jpeg.blockVSize = shared.MinMatrixSize * jpeg.maxV
	jpeg.blockHSize = shared.MinMatrixSize * jpeg.maxH

	for blockH := range jpeg.numBlocksHeight {
		res = append(res, make([][]componentMatrix, jpeg.numBlocksWidth))
		for blockW := range jpeg.numBlocksWidth {
			res[blockH] = append(res[blockH], jpeg.blockDecompose(blockH, blockW))
		}
	}

	return res
}
