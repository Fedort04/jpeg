package decoder

import (
	"jpeg/decoder/huffman"
	"log"
	"math"
)

const rgbDelta = 128 //Константа, которая прибавляется при переводе в RGB

type yCbCrMatrix = [][]yCbCr

// Структура для хранения данных в YCbCr формате
type yCbCr struct {
	y  float32
	cb float32
	cr float32
}

// Перевод в RGB пространство по указателю
func (cur *yCbCr) toRGB(res *rgb) {
	cur.y += rgbDelta
	cur.cb += rgbDelta
	cur.cr += rgbDelta
	res.r = Clamp255(int(math.Round(float64(cur.y) + 1.402*float64((float64(cur.cr)-rgbDelta)))))
	res.g = Clamp255(int(math.Round(float64(cur.y) - 0.34414*float64((float64(cur.cb)-rgbDelta)) - 0.71414*float64((float64(cur.cr)-rgbDelta)))))
	res.b = Clamp255(int(math.Round(float64(cur.y) + 1.772*float64((float64(cur.cb)-rgbDelta)))))
}

// Структура для хранения данных в RGB формате
type rgb struct {
	r byte
	g byte
	b byte
}

var bandSkips uint16 //Счетчик пропусков вычислений в progressive
var prev []int16     //Предыдущие значения DC для дельта кодирования

// Переменные для AC refinement
var positiveBit int16
var negativeBit int16

// Создание пустого изображения RGB
func createRGBMatrix(height uint16, width uint16) [][]rgb {
	res := make([][]rgb, height)
	for i := range height {
		res[i] = make([]rgb, width)
	}
	return res
}

// Создание yCbCrMatrix
func createYCbCrMatrix(height byte, width byte) yCbCrMatrix {
	res := make([][]yCbCr, height)
	for i := range height {
		res[i] = make([]yCbCr, width)
	}
	return res
}

// Создание пустого блока размерами [height][width] из MCU(8х8) в YCbCr
func createYCbCrBlock(height byte, width byte) [][]yCbCrMatrix {
	res := make([][]yCbCrMatrix, height)
	for i := range height {
		res[i] = make([]yCbCrMatrix, width)
		for j := range width {
			res[i][j] = createYCbCrMatrix(UnitRowCount, UnitColCount)
		}
	}
	return res
}

// Вычисление тех переменных, которые нужны при сканах, но вычисляются единожды
func unitsInit() {
	NumOfMCUHeight = (imageHeight + (UnitRowCount - 1)) / (UnitRowCount)
	NumOfMCUHeight += NumOfMCUHeight % uint16(maxV)

	NumOfMCUWidth = (imageWidth + (UnitColCount - 1)) / (UnitColCount)
	NumOfMCUWidth += NumOfMCUWidth % uint16(maxH)
}

// Инициализация дельта-декодирования, перезапуск bands, инициализация побитового чтения
func decodeInit() {
	prev = make([]int16, numOfComps)
	bandSkips = 0
	positiveBit = int16(1 << saLow)
	temp := -1
	negativeBit = int16(uint(temp) << uint(saLow))
	reader.HuffStreamStart()
}

// Сброс дельта-кодирования
func restart() {
	prev = make([]int16, numOfComps)
	bandSkips = 0
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
func decodeDC(id int, huff *huffman.HuffTable) int16 {
	temp := huff.DecodeHuff(reader)
	diff := decodeSign(int16(reader.GetBits(byte(temp))), byte(temp))
	res := diff + prev[id]
	prev[id] = res
	return res
}

// Декодирование символа EOB
func decodeEndOfBand(count byte) uint16 {
	var ans uint16
	ans = 1 << count
	ans += reader.GetBits(count)
	return ans
}

// Декодирование AC элемента
func decodeAC(unit []int16, huff *huffman.HuffTable) {
	if bandSkips > 0 {
		bandSkips--
		return
	}

	unitLen := endSpectral

	var k byte
	if isProgressive {
		k = startSpectral
	} else {
		k = 1
	}

	for ; k <= unitLen; k++ {
		rs := huff.DecodeHuff(reader)
		big := byte(rs >> 4)
		small := byte(rs & 0x0f)

		if rs == 0x00 { //Special symbol 00
			return
		}

		if small == 0 {
			if big != 15 {
				bandSkips = decodeEndOfBand(big)
				bandSkips--
				break
			} else {
				k += 15
				continue
			}
		} else {
			k += big
			if k > unitLen {
				log.Fatalf("decodeAC -> error: k(%d) bigger than unit length(%d)", k, unitLen)
			}
			bits := reader.GetBits(small)
			unit[k] = decodeSign(int16(bits), small) << int16(saLow)
		}
	}
}

// Проверка в диапазоне 0-255
func Clamp255(val int) byte {
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

// Декодирование data unit
func decodeDataUnit(channel int) []int16 {
	temp := make([]int16, UnitRowCount*UnitColCount)
	temp[0] = decodeDC(channel, dcTables[comps[channel].dcTableID])
	decodeAC(temp, acTables[comps[channel].acTableID])
	return temp
}

// Выполнение рестарта дельта кодирвоания
func makeRestart() bool {
	marker := reader.GetWord()
	if marker == EOI {
		return true
	} else if marker >= RST0 && marker <= RST7 {
		reader.BitsAlign()
		restart()
		return true
	}
	log.Printf("marker: %x, nextByte: %d", marker, reader.GetNextByte())
	return false
}

// Декодирование блока MCU Baseline
// x y координаты левого верхнего MCU в блоке
func decodeBaselineBlock(mcus [][]MCU, x uint16, y uint16) {
	for i, comp := range comps {
		if !comp.used {
			continue
		}

		for curV := range uint16(comp.v) {
			for curH := range uint16(comp.h) {
				switch i {
				case int(Y):
					mcus[x+curV][y+curH].Y = decodeDataUnit(i)
				case int(Cb):
					mcus[x+curV][y+curH].Cb = decodeDataUnit(i)
				case int(Cr):
					mcus[x+curV][y+curH].Cr = decodeDataUnit(i)
				}
			}
		}
	}
}

// Baseline
// Декодирование скана, blocks - ссылка на прочитанное к моменту вызова функции изображение
func decodeBaselineScan(mcus [][]MCU) {
	decodeInit()
	defer reader.HuffStreamEnd()

	var blockCount uint //Общее количество прочитанных блоков mcu
	var row uint16      //Счетчик строк блоков MCU
	var col uint16      //Счетчик столбцов блоков MCU
	numBlocksHeight := NumOfMCUHeight / uint16(maxV)
	numBlocksWidth := NumOfMCUWidth / uint16(maxH)

	for row = range numBlocksHeight {
		for col = range numBlocksWidth {
			decodeBaselineBlock(mcus, row*uint16(maxV), col*uint16(maxH))
			blockCount++
			if restartInterval != 0 && blockCount%uint(restartInterval) == 0 && !makeRestart() {
				log.Fatal("makeRestart wrong marker")
			}
		}
	}
}

// Пропуск нулей при refinement
// Возвращает индекс следующего за промежутком нуля или endIndex
func refinementZeroSkip(data []int16, zeros byte, startIndex byte, endIndex byte) byte {
	for k := startIndex; k <= endIndex; k++ {
		if data[k] == 0 {
			if zeros == 0 {
				return k
			} else {
				zeros--
			}
		} else if data[k] > 0 {
			if reader.GetBit() == 1 {
				data[k] |= positiveBit
			}
		} else {
			if reader.GetBit() == 1 {
				data[k] += negativeBit
			}
		}
	}

	return endIndex
}

// Декодирование блока MCU Progressive (используется только для DC)
// x y координаты левого верхнего MCU в блоке
func decodeProgressiveDC(mcus [][]MCU, x uint16, y uint16) {
	for i, comp := range comps {
		if !comp.used {
			continue
		}

		for curV := range uint16(comp.v) {
			for curH := range uint16(comp.h) {
				if saHigh == 0 { // Первое чтение DC
					switch i {
					case int(Y):
						mcus[x+curV][y+curH].Y[0] = decodeDC(i, dcTables[comp.dcTableID]) << int16(saLow)
					case int(Cb):
						mcus[x+curV][y+curH].Cb[0] = decodeDC(i, dcTables[comp.dcTableID]) << int16(saLow)
					case int(Cr):
						mcus[x+curV][y+curH].Cr[0] = decodeDC(i, dcTables[comp.dcTableID]) << int16(saLow)
					}
				} else { // Повторное чтение DC
					bit := reader.GetBit()
					switch i {
					case int(Y):
						mcus[x+curV][y+curH].Y[0] |= int16(bit << saLow)
					case int(Cb):
						mcus[x+curV][y+curH].Cb[0] |= int16(bit << saLow)
					case int(Cr):
						mcus[x+curV][y+curH].Cr[0] |= int16(bit << saLow)
					}
				}
			}
		}
	}
}

// Декодирование сканов AC
func decodeProgressiveAC(mcus [][]MCU) {
	for i, comp := range comps {
		if !comp.used {
			continue
		}

		rowCount := int(NumOfMCUHeight)
		colCount := int(NumOfMCUWidth)

		if imageHeight%UnitRowCount == 0 {
			rowCount = int(imageHeight / UnitRowCount)
		}

		if imageWidth%UnitColCount == 0 {
			colCount = int(imageWidth / UnitColCount)
		}

		rowStep := maxV / comp.v
		colStep := maxH / comp.h
		for row := 0; row < rowCount; row += int(rowStep) {
			for col := 0; col < colCount; col += int(colStep) {
				if saHigh == 0 { // Первое чтение AC
					switch i {
					case int(Y):
						decodeAC(mcus[row][col].Y, acTables[comp.acTableID])
					case int(Cb):
						decodeAC(mcus[row][col].Cb, acTables[comp.acTableID])
					case int(Cr):
						decodeAC(mcus[row][col].Cr, acTables[comp.acTableID])
					}
				} else { // Повторное чтение AC
					var arr []int16 // Указатель на текущий массив цвета
					switch i {
					case int(Y):
						arr = mcus[row][col].Y
					case int(Cb):
						arr = mcus[row][col].Cb
					case int(Cr):
						arr = mcus[row][col].Cr
					}

					if bandSkips > 0 {
						refinementZeroSkip(arr, UnitRowCount*UnitColCount, startSpectral, endSpectral)
						bandSkips--
						continue
					}

					for k := startSpectral; k <= endSpectral; k++ {

						sym := acTables[comp.acTableID].DecodeHuff(reader)
						high := byte(sym >> 4)
						low := byte(sym & 0x0F)
						coeff := int16(0)

						switch low {
						case 0:
							if high != 15 {
								bandSkips = decodeEndOfBand(high)
								k = refinementZeroSkip(arr, UnitRowCount*UnitColCount, k, endSpectral)
								bandSkips--
							} else {
								k = refinementZeroSkip(arr, high, k, endSpectral)
							}
						case 1:
							if reader.GetBit() == 1 {
								coeff = positiveBit
							} else {
								coeff = negativeBit
							}
							k = refinementZeroSkip(arr, high, k, endSpectral)
							arr[k] = coeff
						}
					}
				}
			}
		}
	}
}

// Progressive
// Декодирование одного скана, blocks - ссылка на прочитанное к моменту вызова функции изображение
func decodeProgressiveScan(mcus [][]MCU) {
	decodeInit()
	defer reader.HuffStreamEnd()

	var blockCount uint //Общее количество прочитанных блоков mcu
	var row uint16      //Счетчик строк блоков MCU
	var col uint16      //Счетчик столбцов блоков MCU
	numBlocksHeight := NumOfMCUHeight / uint16(maxV)
	numBlocksWidth := NumOfMCUWidth / uint16(maxH)

	if startSpectral == 0 && endSpectral == 0 { // Только для DC сканов
		for row = range numBlocksHeight {
			for col = range numBlocksWidth {
				decodeProgressiveDC(mcus, row*uint16(maxV), col*uint16(maxH))
				blockCount++
				if restartInterval != 0 && blockCount%uint(restartInterval) == 0 && !makeRestart() {
					log.Fatal("makeRestart wrong marker")
				}
			}
		}
	} else {
		decodeProgressiveAC(mcus)
	}
}

// Вычисление YCbCr для канала ch
// x y - координаты левого верхнего MCU в блоке
func componentCalc(blocks [][]MCU, x uint, y uint, res [][]yCbCrMatrix, ch Channel) {
	// Перевод в YCbCr
	for curV := range uint16(comps[ch].v) {
		for curH := range uint16(comps[ch].h) {
			curMCU := blocks[x+uint(curV)][y+uint(curH)]
			scalingX := maxV / comps[ch].v
			scalingY := maxH / comps[ch].h

			curMCU.Dequant(quantTables[comps[ch].quantTableID], ch)
			unit := curMCU.InverseCosin(ch)

			//chroma subsample
			var vPadding uint16 //Отступ в текущем MCU по x
			var hPadding uint16 //Отступ в текущем MCU по y
			for x := range UnitRowCount * scalingX {
				vPadding = uint16(x / UnitRowCount)

				for y := range UnitColCount * scalingY {
					hPadding = uint16(y / UnitColCount)

					switch ch {
					case Y:
						res[curV+vPadding][curH+hPadding][x%UnitRowCount][y%UnitColCount].y = unit[x/scalingX][y/scalingY]
					case Cb:
						res[curV+vPadding][curH+hPadding][x%UnitRowCount][y%UnitColCount].cb = unit[x/scalingX][y/scalingY]
					case Cr:
						res[curV+vPadding][curH+hPadding][x%UnitRowCount][y%UnitColCount].cr = unit[x/scalingX][y/scalingY]
					}
				}
			}
		}
	}
}

// Копирование в результат информации из блока YCbCrMatrix
// x y - координаты левого верхнего угла блока в результате
func copyToRes(curMatrix yCbCrMatrix, res [][]rgb, x int, y int) {
	for i := 0; i < len(curMatrix) && x+i < int(imageHeight); i++ {
		for j := 0; j < len(curMatrix[0]) && y+j < int(imageWidth); j++ {
			curMatrix[i][j].toRGB(&res[x+i][y+j])
		}
	}
}

// Вычисления над прочитанными данными
func rgbCalc(blocks [][]MCU) {
	numBlocksHeight := int(NumOfMCUHeight) / int(maxV)
	numBlocksWidth := int(NumOfMCUWidth) / int(maxH)

	for row := range numBlocksHeight {
		for col := range numBlocksWidth {
			mcuRow := row * int(maxV) // Номер текущего MCU
			mcuCol := col * int(maxH) // Номер текущего MCU

			curBlock := createYCbCrBlock(maxV, maxH)

			for c := range numOfComps {
				componentCalc(blocks, uint(mcuRow), uint(mcuCol), curBlock, Channel(c))
			}

			for i := range int(maxV) {
				for j := range int(maxH) {
					copyToRes(curBlock[i][j], img, mcuRow*UnitRowCount+i*UnitRowCount, mcuCol*UnitColCount+j*UnitColCount)
				}
			}
		}
	}
}
