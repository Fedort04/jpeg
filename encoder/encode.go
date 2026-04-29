package encoder

import (
	"jpeg/internal/mcu"
	"jpeg/shared"
)

// Перевод изображения в YCbCr с расширением изображения до кратного subsample размера
func (jpeg *Encoder) convertToYCbCr() shared.YCbCrMatrix {
	data := *jpeg.data

	realHeight := uint16(len(data))
	realWidth := uint16(len(data[0]))
	jpeg.imgWidth = ((realWidth + uint16(jpeg.blockHSize) - 1) / uint16(jpeg.blockHSize)) * uint16(jpeg.blockHSize)
	jpeg.imgHeight = ((realHeight + uint16(jpeg.blockVSize) - 1) / uint16(jpeg.blockVSize)) * uint16(jpeg.blockVSize)
	img := shared.CreateMatrix[shared.YCbCr](int(jpeg.imgHeight), int(jpeg.imgWidth))

	var realI, realJ uint16
	for i := range jpeg.imgHeight {
		if i >= realHeight {
			realI = realHeight - 1
		} else {
			realI = i
		}

		for j := range jpeg.imgWidth {
			if j >= realWidth {
				realJ = realWidth - 1
			} else {
				realJ = j
			}
			//@todo здесь подумать над оптимизацией вычислений
			data[realI][realJ].ToYCbCr(&img[i][j])
		}
	}
	return img
}

// Вычисление значений факторов subsample по выбранному формату
func (jpeg *Encoder) factorUpdate() {
	jpeg.ch = 1
	jpeg.cv = 1
	switch jpeg.Format {
	case Without:
		jpeg.yh = 1
		jpeg.yv = 1
		jpeg.maxH = 1
		jpeg.maxV = 1
	case Horizontal:
		jpeg.yh = 2
		jpeg.maxH = 2
		jpeg.maxV = 1
	case Vertival:
		jpeg.yv = 2
		jpeg.maxV = 2
		jpeg.maxH = 1
	case Both:
		jpeg.yh = 2
		jpeg.maxH = 2
		jpeg.yv = 2
		jpeg.maxV = 2
	}
	jpeg.blockVSize = mcu.UnitRowCount * jpeg.maxV
	jpeg.blockHSize = mcu.UnitColCount * jpeg.maxH
}

// Характеризует часть изображения
type part struct {
	// Глобальный левый угол
	globalVPos uint16
	globalHPos uint16

	// Левый угол текущего MCU в блоке (относительно самого блока)
	// Пример: 4:2:0 блок 2х2 -> позиции от 0 до 2
	vPos byte
	hPos byte
}

// Копирует данные части img в dst (вместе с использованием subsample)
// dst матрица слайсов уже создана, просто заполняет значениями
func (jpeg *Encoder) copyImgPartToMatrix(img shared.YCbCrMatrix, dst *mcu.RawMCU, curPart part, channel mcu.Channel) {
	for i := range uint16(mcu.UnitRowCount) {
		for j := range uint16(mcu.UnitColCount) {
			// По глобальному изображению
			curV := curPart.globalVPos + i
			curH := curPart.globalHPos + j
			switch channel {
			case mcu.Y:
				dst.Data[i][j] = img[curV][curH].Y
			case mcu.Cb, mcu.Cr:
				// По текущему фрагменту
				subJ := (j + uint16(curPart.hPos)*mcu.UnitColCount) / uint16(jpeg.maxH)
				subI := (i + uint16(curPart.vPos)*mcu.UnitRowCount) / uint16(jpeg.maxV)
				remainI := curH % uint16(jpeg.maxH)
				remainJ := curV % uint16(jpeg.maxV)
				if remainI == 0 && remainJ == 0 {
					var value float32
					if channel == mcu.Cb {
						value = img[curV][curH].Cb
					} else {
						value = img[curV][curH].Cr
					}
					dst.Data[subI][subJ] = value
				}
			}
		}
	}
}

// Chroma blockSubsample
// Возвращает матрицу прореженных и структурирвованных под Baseline кодирование блоков
func (jpeg *Encoder) blockSubsample(img shared.YCbCrMatrix) [][]mcu.BlockRaw {
	jpeg.numBlocksHeight = jpeg.imgHeight / uint16(jpeg.blockVSize)
	jpeg.numBlocksWidth = jpeg.imgWidth / uint16(jpeg.blockHSize)

	res := shared.CreateMatrix[mcu.BlockRaw](int(jpeg.numBlocksHeight), int(jpeg.numBlocksWidth))

	// Для каждого блока
	for blockI := range jpeg.numBlocksHeight {
		for blockJ := range jpeg.numBlocksWidth {
			var curBlock mcu.BlockRaw

			globalVPos := blockI * uint16(jpeg.blockVSize)
			globalHPos := blockJ * uint16(jpeg.blockHSize)

			curBlock.Y = shared.CreateMatrix[mcu.RawMCU](int(jpeg.maxV), int(jpeg.maxH))
			curBlock.Cb.Data = shared.CreateMatrix[float32](mcu.UnitRowCount, mcu.UnitColCount)
			curBlock.Cr.Data = shared.CreateMatrix[float32](mcu.UnitRowCount, mcu.UnitColCount)

			for i := range uint16(jpeg.maxV) {
				for j := range uint16(jpeg.maxH) {
					// Текущие позиции в изображении
					curHpos := globalHPos + j*mcu.UnitColCount
					curVPos := globalVPos + i*mcu.UnitRowCount
					curPart := part{globalVPos: curVPos, globalHPos: curHpos, vPos: byte(i), hPos: byte(j)}

					// Обработка Y
					curBlock.Y[i][j].Data = shared.CreateMatrix[float32](mcu.UnitRowCount, mcu.UnitColCount)
					jpeg.copyImgPartToMatrix(img, &curBlock.Y[i][j], curPart, mcu.Channel(mcu.Y))

					// Обработка Cb и Cr
					jpeg.copyImgPartToMatrix(img, &curBlock.Cb, curPart, mcu.Channel(mcu.Cb))
					jpeg.copyImgPartToMatrix(img, &curBlock.Cr, curPart, mcu.Channel(mcu.Cr))
				}
			}

			res[blockI][blockJ] = curBlock
		}
	}
	return res
}

// Преобразование в вид, пригодный для кодирования
func (jpeg *Encoder) zigZag(blocks [][]mcu.BlockRaw) [][]mcu.CodingBlock {
	res := shared.CreateMatrix[mcu.CodingBlock](len(blocks), len(blocks[0]))
	for i, row := range blocks {
		for j, elm := range row {
			res[i][j] = elm.ZigZag(jpeg.maxH, jpeg.maxV)
		}
	}
	return res
}
