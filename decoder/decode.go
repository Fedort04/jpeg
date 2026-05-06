package decoder

import (
	"errors"
	binreader "jpeg/internal/binReader"
	"jpeg/internal/huffman"
	"jpeg/internal/mcu"
	"jpeg/shared"
)

var bandSkips uint16 //Счетчик пропусков вычислений в progressive
var prev []int16     //Предыдущие значения DC для дельта кодирования

// Переменные для AC refinement
var positiveBit int16
var negativeBit int16

// Вычисление тех переменных, которые нужны при сканах, но вычисляются единожды
func (jpeg *Decoder) constInit() {
	jpeg.numOfMCUHeightReal = (jpeg.ImageHeight + (mcu.UnitRowCount - 1)) / (mcu.UnitRowCount)
	jpeg.numOfMCUHeight = jpeg.numOfMCUHeightReal + jpeg.numOfMCUHeightReal%uint16(jpeg.maxV)

	jpeg.numOfMCUWidthReal = (jpeg.ImageWidth + (mcu.UnitColCount - 1)) / (mcu.UnitColCount)
	jpeg.numOfMCUWidth = jpeg.numOfMCUWidthReal + jpeg.numOfMCUWidthReal%uint16(jpeg.maxH)

	jpeg.numBlocksHeight = jpeg.numOfMCUHeight / uint16(jpeg.maxV)
	jpeg.numBlocksWidth = jpeg.numOfMCUWidth / uint16(jpeg.maxH)

	jpeg.blocks = mcu.CreateMCUMatrix(jpeg.numOfMCUHeight, jpeg.numOfMCUWidth)
}

// Инициализация дельта-декодирования, перезапуск bands, инициализация побитового чтения
func (jpeg *Decoder) decodeInit() {
	prev = make([]int16, jpeg.numOfComps)
	bandSkips = 0
	positiveBit = int16(1 << jpeg.saLow)
	temp := -1
	negativeBit = int16(uint(temp) << uint(jpeg.saLow))
	jpeg.reader.HuffStreamStart()
}

// Сброс дельта-кодирования
func (jpeg *Decoder) restart() {
	prev = make([]int16, jpeg.numOfComps)
	bandSkips = 0
}

// Декодирование символа EOB
func decodeEndOfBand(b *binreader.BinReader, count byte) uint16 {
	var ans uint16
	ans = 1 << count
	ans += b.GetBits(count)
	return ans
}

// Декодирование знака в потоке Хаффмана
func decodeSign(num int16, len byte) int16 {
	if num >= (1 << (len - 1)) {
		return int16(num)
	} else {
		return int16(num - (1 << len) + 1)
	}
}

// Декодирование DC элемента
func (jpeg *Decoder) decodeDC(id int, huff *huffman.HuffTable) (int16, error) {
	temp, err := huff.DecodeHuff(jpeg.reader)

	if err != nil {
		return 0, err
	}

	diff := decodeSign(int16(jpeg.reader.GetBits(byte(temp))), byte(temp))
	res := diff + prev[id]
	prev[id] = res
	return res, nil
}

// Декодирование AC элемента
func (jpeg *Decoder) decodeAC(unit []int16, huff *huffman.HuffTable) error {
	if bandSkips > 0 {
		bandSkips--
		return nil
	}

	unitLen := jpeg.endSpectral

	var k byte
	if jpeg.IsProgressive {
		k = jpeg.startSpectral
	} else {
		k = 1
	}

	for ; k <= unitLen; k++ {
		rs, err := huff.DecodeHuff(jpeg.reader)

		if err != nil {
			return err
		}

		big := byte(rs >> 4)
		small := byte(rs & 0x0f)

		if rs == shared.EndOfBlock { //Special symbol 00
			return nil
		}

		if small == 0 {
			if big != 15 {
				bandSkips = decodeEndOfBand(jpeg.reader, big)
				bandSkips--
				return nil
			} else {
				k += 15
				continue
			}
		} else {
			k += big
			if k > unitLen {
				return errors.New("Huffman bit-reading error: AC reading failed")
			}
			bits := jpeg.reader.GetBits(small)
			unit[k] = decodeSign(int16(bits), small) << int16(jpeg.saLow)
		}
	}
	return nil
}

// Декодирование data unit
func (jpeg *Decoder) decodeDataUnit(channel int) ([]int16, error) {
	var err error
	temp := make([]int16, mcu.UnitRowCount*mcu.UnitColCount)
	temp[0], err = jpeg.decodeDC(channel, jpeg.dcTables[jpeg.comps[channel].dcTableID])
	if err != nil {
		return nil, err
	}

	if err = jpeg.decodeAC(temp, jpeg.acTables[jpeg.comps[channel].acTableID]); err != nil {
		return nil, err
	}

	return temp, nil
}

// Выполнение рестарта дельта кодирвоания
func (jpeg *Decoder) makeRestart() bool {
	marker := jpeg.reader.GetWord()
	if marker == shared.EOI {
		return true
	} else if marker >= shared.RST0 && marker <= shared.RST7 {
		jpeg.reader.BitsAlign()
		jpeg.restart()
		return true
	}
	return false
}

// Декодирование блока MCU Baseline
// x y координаты левого верхнего MCU в блоке
func (jpeg *Decoder) decodeBaselineBlock(x uint16, y uint16) error {
	for i, comp := range jpeg.comps {
		if !comp.used {
			continue
		}

		var err error
		for curV := range uint16(comp.v) {
			for curH := range uint16(comp.h) {
				switch i {
				case int(mcu.Y):
					if jpeg.blocks[x+curV][y+curH].Y, err = jpeg.decodeDataUnit(i); err != nil {
						return err
					}
				case int(mcu.Cb):
					if jpeg.blocks[x+curV][y+curH].Cb, err = jpeg.decodeDataUnit(i); err != nil {
						return err
					}
				case int(mcu.Cr):
					if jpeg.blocks[x+curV][y+curH].Cr, err = jpeg.decodeDataUnit(i); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// Baseline
// Декодирование скана, blocks - ссылка на прочитанное к моменту вызова функции изображение
// Возвращает номер строки блоков и номер строки в пикселях, на которых остановилось вычисление
func (jpeg *Decoder) decodeBaselineScan(increment uint16) (uint16, error) {
	var row uint16 //Счетчик строк блоков MCU
	var col uint16 //Счетчик столбцов блоков MCU

	//Для построчного чтения
	row = (jpeg.CurStatus + (mcu.UnitRowCount - 1)) / (mcu.UnitRowCount)
	row += row % uint16(jpeg.maxV)
	row /= uint16(jpeg.maxV)
	if increment == 0 {
		increment--
	} else {
		increment = (increment + (mcu.UnitRowCount - 1)) / (mcu.UnitRowCount)
		increment += increment % uint16(jpeg.maxV)
		increment /= uint16(jpeg.maxV)
		increment += row
	}

	//Блоки в изображении с учетом subsample
	for ; row < jpeg.numBlocksHeight && row < increment; row++ {
		for col = range jpeg.numBlocksWidth {
			if err := jpeg.decodeBaselineBlock(row*uint16(jpeg.maxV), col*uint16(jpeg.maxH)); err != nil {
				return 0, err
			}

			jpeg.blockCount++
			if jpeg.restartInterval != 0 && jpeg.blockCount%uint(jpeg.restartInterval) == 0 && !jpeg.makeRestart() {
				return 0, errors.New("Huffman bit-reading error: make restart error")
			}
		}
	}
	jpeg.CurStatus = row * mcu.UnitColCount * uint16(jpeg.maxV)
	if jpeg.CurStatus >= jpeg.ImageHeight {
		jpeg.wasEOI = true
		jpeg.reader.HuffStreamEnd()
	}
	return row, nil
}

// Пропуск нулей при refinement
// Возвращает индекс следующего за промежутком нуля или endIndex
func (jpeg *Decoder) RefinementZeroSkip(data []int16, zeros byte, startIndex byte, endIndex byte) byte {
	for k := startIndex; k <= endIndex; k++ {
		if data[k] == 0 {
			if zeros == 0 {
				return k
			} else {
				zeros--
			}
		} else if data[k] > 0 {
			if jpeg.reader.GetBit() == 1 {
				data[k] |= positiveBit
			}
		} else {
			if jpeg.reader.GetBit() == 1 {
				data[k] += negativeBit
			}
		}
	}

	return endIndex
}

// Декодирование блока MCU Progressive (используется только для DC)
// x y координаты левого верхнего MCU в блоке
func (jpeg *Decoder) decodeProgressiveDC(x uint16, y uint16) error {
	for i, comp := range jpeg.comps {
		if !comp.used {
			continue
		}

		var err error
		for curV := range uint16(comp.v) {
			for curH := range uint16(comp.h) {
				if jpeg.saHigh == 0 { // Первое чтение DC
					switch i {
					case int(mcu.Y):
						if jpeg.blocks[x+curV][y+curH].Y[0], err = jpeg.decodeDC(i, jpeg.dcTables[comp.dcTableID]); err != nil {
							return err
						}
						jpeg.blocks[x+curV][y+curH].Y[0] <<= int16(jpeg.saLow)
					case int(mcu.Cb):
						if jpeg.blocks[x+curV][y+curH].Cb[0], err = jpeg.decodeDC(i, jpeg.dcTables[comp.dcTableID]); err != nil {
							return err
						}
						jpeg.blocks[x+curV][y+curH].Cb[0] <<= int16(jpeg.saLow)
					case int(mcu.Cr):
						if jpeg.blocks[x+curV][y+curH].Cr[0], err = jpeg.decodeDC(i, jpeg.dcTables[comp.dcTableID]); err != nil {
							return err
						}
						jpeg.blocks[x+curV][y+curH].Cr[0] <<= int16(jpeg.saLow)
					}
				} else { // Повторное чтение DC
					bit := jpeg.reader.GetBit()
					switch i {
					case int(mcu.Y):
						jpeg.blocks[x+curV][y+curH].Y[0] |= int16(bit << jpeg.saLow)
					case int(mcu.Cb):
						jpeg.blocks[x+curV][y+curH].Cb[0] |= int16(bit << jpeg.saLow)
					case int(mcu.Cr):
						jpeg.blocks[x+curV][y+curH].Cr[0] |= int16(bit << jpeg.saLow)
					}
				}
			}
		}
	}
	return nil
}

// Декодирование сканов AC
func (jpeg *Decoder) decodeProgressiveAC() error {
	for i, comp := range jpeg.comps {
		if !comp.used {
			continue
		}

		rowStep := jpeg.maxV / comp.v
		colStep := jpeg.maxH / comp.h
		for row := 0; row < int(jpeg.numOfMCUHeightReal); row += int(rowStep) {
			for col := 0; col < int(jpeg.numOfMCUWidthReal); col += int(colStep) {
				if jpeg.saHigh == 0 { // Первое чтение AC
					switch i {
					case int(mcu.Y):
						if err := jpeg.decodeAC(jpeg.blocks[row][col].Y, jpeg.acTables[comp.acTableID]); err != nil {
							return err
						}
					case int(mcu.Cb):
						if err := jpeg.decodeAC(jpeg.blocks[row][col].Cb, jpeg.acTables[comp.acTableID]); err != nil {
							return err
						}
					case int(mcu.Cr):
						if err := jpeg.decodeAC(jpeg.blocks[row][col].Cr, jpeg.acTables[comp.acTableID]); err != nil {
							return err
						}
					}
				} else { // Повторное чтение AC
					var arr []int16 // Указатель на текущий массив цвета
					switch i {
					case int(mcu.Y):
						arr = jpeg.blocks[row][col].Y
					case int(mcu.Cb):
						arr = jpeg.blocks[row][col].Cb
					case int(mcu.Cr):
						arr = jpeg.blocks[row][col].Cr
					}

					if bandSkips > 0 {
						jpeg.RefinementZeroSkip(arr, mcu.UnitRowCount*mcu.UnitColCount, jpeg.startSpectral, jpeg.endSpectral)
						bandSkips--
						continue
					}

					for k := jpeg.startSpectral; k <= jpeg.endSpectral; k++ {

						sym, err := jpeg.acTables[comp.acTableID].DecodeHuff(jpeg.reader)
						if err != nil {
							return err
						}

						high := byte(sym >> 4)
						low := byte(sym & 0x0F)
						coeff := int16(0)

						switch low {
						case 0:
							if high != 15 {
								bandSkips = decodeEndOfBand(jpeg.reader, high)
								k = jpeg.RefinementZeroSkip(arr, mcu.UnitRowCount*mcu.UnitColCount, k, jpeg.endSpectral)
								bandSkips--
							} else {
								k = jpeg.RefinementZeroSkip(arr, high, k, jpeg.endSpectral)
							}
						case 1:
							if jpeg.reader.GetBit() == 1 {
								coeff = positiveBit
							} else {
								coeff = negativeBit
							}
							k = jpeg.RefinementZeroSkip(arr, high, k, jpeg.endSpectral)
							arr[k] = coeff
						}
					}
				}
			}
		}
	}
	return nil
}

// Progressive
// Декодирование одного скана, blocks - ссылка на прочитанное к моменту вызова функции изображение
func (jpeg *Decoder) decodeProgressiveScan() error {
	jpeg.decodeInit()
	defer jpeg.reader.HuffStreamEnd()

	var blockCount uint //Общее количество прочитанных блоков mcu
	var row uint16      //Счетчик строк блоков MCU
	var col uint16      //Счетчик столбцов блоков MCU

	if jpeg.startSpectral == 0 && jpeg.endSpectral == 0 { // Только для DC сканов
		for row = range jpeg.numBlocksHeight {
			for col = range jpeg.numBlocksWidth {
				if err := jpeg.decodeProgressiveDC(row*uint16(jpeg.maxV), col*uint16(jpeg.maxH)); err != nil {
					return err
				}

				blockCount++
				if jpeg.restartInterval != 0 && blockCount%uint(jpeg.restartInterval) == 0 && !jpeg.makeRestart() {
					return errors.New("Huffman bit-reading error: make restart error")
				}
			}
		}
	} else {
		return jpeg.decodeProgressiveAC()
	}

	return nil
}

// Вычисление YCbCr для канала ch
// x y - координаты левого верхнего MCU в блоке
func (jpeg *Decoder) componentCalc(x uint, y uint, res [][]shared.YCbCrMatrix, ch mcu.Channel, readAll bool) {
	// Перевод в YCbCr
	for curV := range uint16(jpeg.comps[ch].v) {
		for curH := range uint16(jpeg.comps[ch].h) {
			var curMCU mcu.MCU
			if !readAll && jpeg.IsProgressive {
				curMCU = mcu.MakeMCU()
				jpeg.blocks[x+uint(curV)][y+uint(curH)].Copy(&curMCU)
			} else {
				curMCU = jpeg.blocks[x+uint(curV)][y+uint(curH)]
			}
			scalingX := jpeg.maxV / jpeg.comps[ch].v
			scalingY := jpeg.maxH / jpeg.comps[ch].h

			curMCU.Dequant(jpeg.quantTables[jpeg.comps[ch].quantTableID], ch)
			unit := curMCU.InverseCosin(ch)

			//chroma subsample
			var vPadding uint16 //Отступ в текущем MCU по x
			var hPadding uint16 //Отступ в текущем MCU по y
			for x := range mcu.UnitRowCount * scalingX {
				vPadding = uint16(x / mcu.UnitRowCount)

				for y := range mcu.UnitColCount * scalingY {
					hPadding = uint16(y / mcu.UnitColCount)

					switch ch {
					case mcu.Y:
						res[curV+vPadding][curH+hPadding][x%mcu.UnitRowCount][y%mcu.UnitColCount].Y = unit[x/scalingX][y/scalingY]
					case mcu.Cb:
						res[curV+vPadding][curH+hPadding][x%mcu.UnitRowCount][y%mcu.UnitColCount].Cb = unit[x/scalingX][y/scalingY]
					case mcu.Cr:
						res[curV+vPadding][curH+hPadding][x%mcu.UnitRowCount][y%mcu.UnitColCount].Cr = unit[x/scalingX][y/scalingY]
					}
				}
			}
		}
	}
}

// Копирование в результат информации из блока YCbCrMatrix
// x y - координаты левого верхнего угла блока в результате
func (jpeg *Decoder) copyToRes(curMatrix shared.YCbCrMatrix, res [][]shared.Rgb, x int, y int) {
	for i := 0; i < len(curMatrix) && x+i < int(jpeg.ImageHeight); i++ {
		for j := 0; j < len(curMatrix[0]) && y+j < int(jpeg.ImageWidth); j++ {
			curMatrix[i][j].ToRGB(&res[x+i][y+j])
		}
	}
}

// Вычисления над прочитанными данными, readAll - флаг чтения всего изображения сразу для отпимизации
func (jpeg *Decoder) rgbCalc(readAll bool, startRow int, endRow int) {
	var rowMax int
	var row int
	if jpeg.IsProgressive {
		rowMax = int(jpeg.numBlocksHeight)
		row = 0
	} else {
		rowMax = endRow
		row = int(startRow / mcu.UnitRowCount / int(jpeg.maxV))
	}

	for ; row < rowMax; row++ {
		for col := range int(jpeg.numBlocksWidth) {
			mcuRow := row * int(jpeg.maxV) // Номер текущего MCU
			mcuCol := col * int(jpeg.maxH) // Номер текущего MCU

			curBlock := shared.CreateYCbCrBlock(jpeg.maxV, jpeg.maxH, mcu.UnitRowCount, mcu.UnitColCount)

			for c := range jpeg.numOfComps {
				jpeg.componentCalc(uint(mcuRow), uint(mcuCol), curBlock, mcu.Channel(c), readAll)
			}

			for i := range int(jpeg.maxV) {
				for j := range int(jpeg.maxH) {
					jpeg.copyToRes(curBlock[i][j], jpeg.img, mcuRow*mcu.UnitRowCount+i*mcu.UnitRowCount, mcuCol*mcu.UnitColCount+j*mcu.UnitColCount)
				}
			}
		}
	}
}
