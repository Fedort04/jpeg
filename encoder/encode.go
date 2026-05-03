package encoder

import (
	"errors"
	binwriter "jpeg/internal/binWriter"
	"jpeg/internal/huffman"
	"jpeg/internal/mcu"
	"jpeg/shared"
	"math"
)

// Перевод изображения в YCbCr с расширением изображения до кратного subsample размера
func (jpeg *Encoder) convertToYCbCr() shared.YCbCrMatrix {
	data := *jpeg.data

	jpeg.realImgHeight = uint16(len(data))
	jpeg.realImgWidth = uint16(len(data[0]))
	jpeg.imgWidth = ((jpeg.realImgWidth + uint16(jpeg.blockHSize) - 1) / uint16(jpeg.blockHSize)) * uint16(jpeg.blockHSize)
	jpeg.imgHeight = ((jpeg.realImgHeight + uint16(jpeg.blockVSize) - 1) / uint16(jpeg.blockVSize)) * uint16(jpeg.blockVSize)
	img := shared.CreateMatrix[shared.YCbCr](int(jpeg.imgHeight), int(jpeg.imgWidth))

	var realI, realJ uint16
	for i := range jpeg.imgHeight {
		if i >= jpeg.realImgHeight {
			realI = jpeg.realImgHeight - 1
		} else {
			realI = i
		}

		for j := range jpeg.imgWidth {
			if j >= jpeg.realImgWidth {
				realJ = jpeg.realImgWidth - 1
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
		jpeg.yv = 1
		jpeg.maxH = 2
		jpeg.maxV = 1
	case Vertical:
		jpeg.yv = 2
		jpeg.yh = 1
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

// Запись маркера сброса дельты
func (jpeg *Encoder) writeRestart() error {
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	sw.FlushBits()

	if jpeg.restartCounter >= shared.NumOfRstMarkers {
		jpeg.restartCounter = 0
	}
	curMarker := shared.RST0 + uint16(jpeg.restartCounter)
	sw.WriteWord(curMarker)
	jpeg.restartCounter++

	if sw.Err != nil {
		return errors.New("Can't write an RST marker\n" + sw.Err.Error())
	}
	return nil
}

// Умный счетчик для перезапуска дельта-кодирования
// При необходимости сам записывает сегмент сброса дельты
func (jpeg *Encoder) restartIncrement() error {
	jpeg.mcuCounter++

	var err error
	if jpeg.RestartInterval != 0 && jpeg.mcuCounter >= jpeg.RestartInterval {
		jpeg.prev = make([]int16, shared.NumOfChannels)
		jpeg.mcuCounter = 0
		err = jpeg.writeRestart()
	}

	return err
}

// Вычисление категории в соответствии с F.1
func (jpeg *Encoder) findCategory(val int16) byte {
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

// Создание дополнительного кода для кодирования значения
func createAddVal(val int16, len byte) uint16 {
	if val < 0 {
		mask := (int16(1) << len) - 1
		val = int16(math.Abs(float64(val)))
		return uint16(val ^ mask)
	}
	return uint16(val)
}

// Кодирование дополнительного значения, которое идет после символа Хаффмана
func (jpeg *Encoder) encodeAddVal(val int16, ssss byte) error {
	return jpeg.writer.WriteBits(jpeg.writer.CreateBitsArray(createAddVal(val, ssss), ssss))
}

// Кодирование одного символа Хаффмана
func (jpeg *Encoder) encodeSymbol(symbol byte, table *huffman.HuffTable) error {
	code, len, err := table.GetCodeBySym(symbol)
	if err != nil {
		return err
	}

	if err := jpeg.writer.WriteBits(jpeg.writer.CreateBitsArray(code, len)); err != nil {
		return err
	}

	return nil
}

// Кодирование DC-элемента
func (jpeg *Encoder) encodeDC(val int16, table *huffman.HuffTable, ch byte) error {
	diff := val - jpeg.prev[ch]
	jpeg.prev[ch] = val

	ssss := jpeg.findCategory(diff)
	se := &stickyEncoder{encoder: jpeg}
	se.encodeSymbol(ssss, table)
	se.encodeAddVal(diff, ssss)
	if se.err != nil {
		return errors.New("Can't write a DC symbol\n" + se.err.Error())
	}

	return nil
}

// Кодирование AC-элемента
func (jpeg *Encoder) encodeAC(dataUnit []int16, table *huffman.HuffTable) error {
	var zeroCounter byte

	for k := 1; k <= baselineSE; k++ {
		val := dataUnit[k]
		if val == 0 {
			zeroCounter++
			continue
		}

		se := &stickyEncoder{encoder: jpeg}
		// ZRL для каждых 16 нулей
		for zeroCounter >= 16 {
			se.encodeSymbol(shared.ZRL, table)
			zeroCounter -= 16
		}

		// Кодировка ненулевого значения
		ssss := jpeg.findCategory(val)
		rs := (zeroCounter << 4) | ssss
		se.encodeSymbol(rs, table)
		se.encodeAddVal(val, ssss)

		if se.err != nil {
			return errors.New("Can't write an AC symbol\n" + se.err.Error())
		}

		zeroCounter = 0
	}

	if zeroCounter > 0 {
		return jpeg.encodeSymbol(shared.EndOfBlock, table)
	}
	return nil
}

// Кодирование одного data-unit
func (jpeg *Encoder) dataUnitEncode(dataUnit []int16, channel byte) error {
	var dcTable, acTable *huffman.HuffTable
	if mcu.Channel(channel) != mcu.Y { //Вариант для цветов
		dcTable = jpeg.cDCHuff
		acTable = jpeg.cACHuff
	} else { // Вариант для яркости
		dcTable = jpeg.yDCHuff
		acTable = jpeg.yACHuff
	}

	se := &stickyEncoder{encoder: jpeg}
	se.encodeDC(dataUnit[0], dcTable, channel)
	se.encodeAC(dataUnit, acTable)

	if se.err != nil {
		return se.err
	}
	return nil
}

// Кодирование блока baseline
func (jpeg *Encoder) baselineBlockEncode(block *mcu.CodingBlock) error {
	se := &stickyEncoder{encoder: jpeg}
	for _, y := range block.Y {
		se.dataUnitEncode(y, byte(mcu.Y))
	}
	se.dataUnitEncode(block.Cb, byte(mcu.Cb))
	se.dataUnitEncode(block.Cr, byte(mcu.Cr))
	se.restartIncrement()

	if se.err != nil {
		return errors.New("Error when encode block\n" + se.err.Error())
	}
	return nil
}
