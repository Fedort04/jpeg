package decoder

import (
	"jpeg/decoder/huffman"
	"log"
	"math"
)

const rgbDelta = 128 //Константа, которая прибавляется при переводе в RGB

type Image = [][]Rgb
type yCbCrMatrix = [][]yCbCr

// Структура для хранения данных в YCbCr формате
type yCbCr struct {
	y  float32
	cb float32
	cr float32
}

// Перевод в RGB пространство по указателю
func (cur *yCbCr) toRGB(res *Rgb) {
	cur.y += rgbDelta
	cur.cb += rgbDelta
	cur.cr += rgbDelta
	res.R = Clamp255(int(math.Round(float64(cur.y) + 1.402*float64((float64(cur.cr)-rgbDelta)))))
	res.G = Clamp255(int(math.Round(float64(cur.y) - 0.34414*float64((float64(cur.cb)-rgbDelta)) - 0.71414*float64((float64(cur.cr)-rgbDelta)))))
	res.B = Clamp255(int(math.Round(float64(cur.y) + 1.772*float64((float64(cur.cb)-rgbDelta)))))
}

// Структура для хранения данных в RGB формате
type Rgb struct {
	R byte
	G byte
	B byte
}

var bandSkips uint16 //Счетчик пропусков вычислений в progressive
var prev []int16     //Предыдущие значения DC для дельта кодирования

// Переменные для AC refinement
var positiveBit int16
var negativeBit int16

// Создание пустого изображения RGB
func CreateRGBMatrix(height uint16, width uint16) Image {
	res := make([][]Rgb, height)
	for i := range height {
		res[i] = make([]Rgb, width)
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
			res[i][j] = createYCbCrMatrix(unitRowCount, unitColCount)
		}
	}
	return res
}

// Вычисление тех переменных, которые нужны при сканах, но вычисляются единожды
func (jpeg *JPEG) constInit() {
	jpeg.numOfMCUHeight = (jpeg.ImageHeight + (unitRowCount - 1)) / (unitRowCount)
	jpeg.numOfMCUHeight += jpeg.numOfMCUHeight % uint16(jpeg.maxV)

	jpeg.numOfMCUWidth = (jpeg.ImageWidth + (unitColCount - 1)) / (unitColCount)
	jpeg.numOfMCUWidth += jpeg.numOfMCUWidth % uint16(jpeg.maxH)

	jpeg.numBlocksHeight = jpeg.numOfMCUHeight / uint16(jpeg.maxV)
	jpeg.numBlocksWidth = jpeg.numOfMCUWidth / uint16(jpeg.maxH)

	jpeg.blocks = CreateMCUMatrix(jpeg.numOfMCUHeight, jpeg.numOfMCUWidth)
}

// Инициализация дельта-декодирования, перезапуск bands, инициализация побитового чтения
func (jpeg *JPEG) decodeInit() {
	prev = make([]int16, jpeg.numOfComps)
	bandSkips = 0
	positiveBit = int16(1 << jpeg.saLow)
	temp := -1
	negativeBit = int16(uint(temp) << uint(jpeg.saLow))
	jpeg.reader.HuffStreamStart()
}

// Сброс дельта-кодирования
func (jpeg *JPEG) restart() {
	prev = make([]int16, jpeg.numOfComps)
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
func (jpeg *JPEG) decodeDC(id int, huff *huffman.HuffTable) int16 {
	temp := huff.DecodeHuff(jpeg.reader)
	diff := decodeSign(int16(jpeg.reader.GetBits(byte(temp))), byte(temp))
	res := diff + prev[id]
	prev[id] = res
	return res
}

// Декодирование AC элемента
func (jpeg *JPEG) decodeAC(unit []int16, huff *huffman.HuffTable) {
	if bandSkips > 0 {
		bandSkips--
		return
	}

	unitLen := jpeg.endSpectral

	var k byte
	if jpeg.IsProgressive {
		k = jpeg.startSpectral
	} else {
		k = 1
	}

	for ; k <= unitLen; k++ {
		rs := huff.DecodeHuff(jpeg.reader)
		big := byte(rs >> 4)
		small := byte(rs & 0x0f)

		if rs == 0x00 { //Special symbol 00
			return
		}

		if small == 0 {
			if big != 15 {
				bandSkips = jpeg.reader.DecodeEndOfBand(big)
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
			bits := jpeg.reader.GetBits(small)
			unit[k] = decodeSign(int16(bits), small) << int16(jpeg.saLow)
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
func (jpeg *JPEG) decodeDataUnit(channel int) []int16 {
	temp := make([]int16, unitRowCount*unitColCount)
	temp[0] = jpeg.decodeDC(channel, jpeg.dcTables[jpeg.comps[channel].dcTableID])
	jpeg.decodeAC(temp, jpeg.acTables[jpeg.comps[channel].acTableID])
	return temp
}

// Выполнение рестарта дельта кодирвоания
func (jpeg *JPEG) makeRestart() bool {
	marker := jpeg.reader.GetWord()
	if marker == EOI {
		return true
	} else if marker >= RST0 && marker <= RST7 {
		jpeg.reader.BitsAlign()
		jpeg.restart()
		return true
	}
	log.Printf("marker: %x, nextByte: %d", marker, jpeg.reader.GetNextByte())
	return false
}

// Декодирование блока MCU Baseline
// x y координаты левого верхнего MCU в блоке
func (jpeg *JPEG) decodeBaselineBlock(mcus [][]MCU, x uint16, y uint16) {
	for i, comp := range jpeg.comps {
		if !comp.used {
			continue
		}

		for curV := range uint16(comp.v) {
			for curH := range uint16(comp.h) {
				switch i {
				case int(Y):
					mcus[x+curV][y+curH].Y = jpeg.decodeDataUnit(i)
				case int(Cb):
					mcus[x+curV][y+curH].Cb = jpeg.decodeDataUnit(i)
				case int(Cr):
					mcus[x+curV][y+curH].Cr = jpeg.decodeDataUnit(i)
				}
			}
		}
	}
}

// Baseline
// Декодирование скана, blocks - ссылка на прочитанное к моменту вызова функции изображение
// Возвращает номер строки блоков и номер строки в пикселях, на которых остановилось вычисление
func (jpeg *JPEG) decodeBaselineScan(mcus [][]MCU, increment uint16) (uint16, uint16) {
	var row uint16 //Счетчик строк блоков MCU
	var col uint16 //Счетчик столбцов блоков MCU

	//Для построчного чтения
	row = (jpeg.CurStatus + (unitRowCount - 1)) / (unitRowCount)
	row += row % uint16(jpeg.maxV)
	row /= uint16(jpeg.maxV)
	if increment == 0 {
		increment--
	} else {
		increment = (increment + (unitRowCount - 1)) / (unitRowCount)
		increment += increment % uint16(jpeg.maxV)
		increment /= uint16(jpeg.maxV)
		increment += row
	}

	//Блоки в изображении с учетом subsample
	for ; row < jpeg.numBlocksHeight && row < increment; row++ {
		for col = range jpeg.numBlocksWidth {
			jpeg.decodeBaselineBlock(mcus, row*uint16(jpeg.maxV), col*uint16(jpeg.maxH))
			jpeg.blockCount++
			if jpeg.restartInterval != 0 && jpeg.blockCount%uint(jpeg.restartInterval) == 0 && !jpeg.makeRestart() {
				log.Fatal("makeRestart wrong marker")
			}
		}
	}
	res := row * unitColCount * uint16(jpeg.maxV)
	if res >= jpeg.ImageHeight {
		jpeg.wasEOI = true
		jpeg.reader.HuffStreamEnd()
	}
	return res, row
}

// Пропуск нулей при refinement
// Возвращает индекс следующего за промежутком нуля или endIndex
func (jpeg *JPEG) RefinementZeroSkip(data []int16, zeros byte, startIndex byte, endIndex byte) byte {
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
func (jpeg *JPEG) decodeProgressiveDC(mcus [][]MCU, x uint16, y uint16) {
	for i, comp := range jpeg.comps {
		if !comp.used {
			continue
		}

		for curV := range uint16(comp.v) {
			for curH := range uint16(comp.h) {
				if jpeg.saHigh == 0 { // Первое чтение DC
					switch i {
					case int(Y):
						mcus[x+curV][y+curH].Y[0] = jpeg.decodeDC(i, jpeg.dcTables[comp.dcTableID]) << int16(jpeg.saLow)
					case int(Cb):
						mcus[x+curV][y+curH].Cb[0] = jpeg.decodeDC(i, jpeg.dcTables[comp.dcTableID]) << int16(jpeg.saLow)
					case int(Cr):
						mcus[x+curV][y+curH].Cr[0] = jpeg.decodeDC(i, jpeg.dcTables[comp.dcTableID]) << int16(jpeg.saLow)
					}
				} else { // Повторное чтение DC
					bit := jpeg.reader.GetBit()
					switch i {
					case int(Y):
						mcus[x+curV][y+curH].Y[0] |= int16(bit << jpeg.saLow)
					case int(Cb):
						mcus[x+curV][y+curH].Cb[0] |= int16(bit << jpeg.saLow)
					case int(Cr):
						mcus[x+curV][y+curH].Cr[0] |= int16(bit << jpeg.saLow)
					}
				}
			}
		}
	}
}

// Декодирование сканов AC
func (jpeg *JPEG) decodeProgressiveAC(mcus [][]MCU) {
	for i, comp := range jpeg.comps {
		if !comp.used {
			continue
		}

		rowCount := int(jpeg.numOfMCUHeight)
		colCount := int(jpeg.numOfMCUWidth)

		if jpeg.ImageHeight%unitRowCount == 0 {
			rowCount = int(jpeg.ImageHeight / unitRowCount)
		}

		if jpeg.ImageWidth%unitColCount == 0 {
			colCount = int(jpeg.ImageWidth / unitColCount)
		}

		rowStep := jpeg.maxV / comp.v
		colStep := jpeg.maxH / comp.h
		for row := 0; row < rowCount; row += int(rowStep) {
			for col := 0; col < colCount; col += int(colStep) {
				if jpeg.saHigh == 0 { // Первое чтение AC
					switch i {
					case int(Y):
						jpeg.decodeAC(mcus[row][col].Y, jpeg.acTables[comp.acTableID])
					case int(Cb):
						jpeg.decodeAC(mcus[row][col].Cb, jpeg.acTables[comp.acTableID])
					case int(Cr):
						jpeg.decodeAC(mcus[row][col].Cr, jpeg.acTables[comp.acTableID])
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
						jpeg.RefinementZeroSkip(arr, unitRowCount*unitColCount, jpeg.startSpectral, jpeg.endSpectral)
						bandSkips--
						continue
					}

					for k := jpeg.startSpectral; k <= jpeg.endSpectral; k++ {

						sym := jpeg.acTables[comp.acTableID].DecodeHuff(jpeg.reader)
						high := byte(sym >> 4)
						low := byte(sym & 0x0F)
						coeff := int16(0)

						switch low {
						case 0:
							if high != 15 {
								bandSkips = jpeg.reader.DecodeEndOfBand(high)
								k = jpeg.RefinementZeroSkip(arr, unitRowCount*unitColCount, k, jpeg.endSpectral)
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
}

// Progressive
// Декодирование одного скана, blocks - ссылка на прочитанное к моменту вызова функции изображение
func (jpeg *JPEG) decodeProgressiveScan(mcus [][]MCU) {
	jpeg.decodeInit()
	defer jpeg.reader.HuffStreamEnd()

	var blockCount uint //Общее количество прочитанных блоков mcu
	var row uint16      //Счетчик строк блоков MCU
	var col uint16      //Счетчик столбцов блоков MCU

	if jpeg.startSpectral == 0 && jpeg.endSpectral == 0 { // Только для DC сканов
		for row = range jpeg.numBlocksHeight {
			for col = range jpeg.numBlocksWidth {
				jpeg.decodeProgressiveDC(mcus, row*uint16(jpeg.maxV), col*uint16(jpeg.maxH))
				blockCount++
				if jpeg.restartInterval != 0 && blockCount%uint(jpeg.restartInterval) == 0 && !jpeg.makeRestart() {
					log.Fatal("makeRestart wrong marker")
				}
			}
		}
	} else {
		jpeg.decodeProgressiveAC(mcus)
	}
}

// Вычисление YCbCr для канала ch
// x y - координаты левого верхнего MCU в блоке
func (jpeg *JPEG) componentCalc(blocks [][]MCU, x uint, y uint, res [][]yCbCrMatrix, ch Channel, readAll bool) {
	// Перевод в YCbCr
	for curV := range uint16(jpeg.comps[ch].v) {
		for curH := range uint16(jpeg.comps[ch].h) {
			var curMCU MCU
			if !readAll && jpeg.IsProgressive {
				curMCU = MakeMCU()
				blocks[x+uint(curV)][y+uint(curH)].Copy(&curMCU)
			} else {
				curMCU = blocks[x+uint(curV)][y+uint(curH)]
			}
			scalingX := jpeg.maxV / jpeg.comps[ch].v
			scalingY := jpeg.maxH / jpeg.comps[ch].h

			curMCU.Dequant(jpeg.quantTables[jpeg.comps[ch].quantTableID], ch)
			unit := curMCU.InverseCosin(ch)

			//chroma subsample
			var vPadding uint16 //Отступ в текущем MCU по x
			var hPadding uint16 //Отступ в текущем MCU по y
			for x := range unitRowCount * scalingX {
				vPadding = uint16(x / unitRowCount)

				for y := range unitColCount * scalingY {
					hPadding = uint16(y / unitColCount)

					switch ch {
					case Y:
						res[curV+vPadding][curH+hPadding][x%unitRowCount][y%unitColCount].y = unit[x/scalingX][y/scalingY]
					case Cb:
						res[curV+vPadding][curH+hPadding][x%unitRowCount][y%unitColCount].cb = unit[x/scalingX][y/scalingY]
					case Cr:
						res[curV+vPadding][curH+hPadding][x%unitRowCount][y%unitColCount].cr = unit[x/scalingX][y/scalingY]
					}
				}
			}
		}
	}
}

// Копирование в результат информации из блока YCbCrMatrix
// x y - координаты левого верхнего угла блока в результате
func (jpeg *JPEG) copyToRes(curMatrix yCbCrMatrix, res [][]Rgb, x int, y int) {
	for i := 0; i < len(curMatrix) && x+i < int(jpeg.ImageHeight); i++ {
		for j := 0; j < len(curMatrix[0]) && y+j < int(jpeg.ImageWidth); j++ {
			curMatrix[i][j].toRGB(&res[x+i][y+j])
		}
	}
}

// Вычисления над прочитанными данными, readAll - флаг чтения всего изображения сразу для отпимизации
func (jpeg *JPEG) rgbCalc(blocks [][]MCU, readAll bool, startRow int, endRow int) {
	var rowMax int
	var row int
	if jpeg.IsProgressive {
		rowMax = int(jpeg.numBlocksHeight)
		row = 0
	} else {
		rowMax = endRow
		row = int(startRow / unitRowCount / int(jpeg.maxV))
	}

	for ; row < rowMax; row++ {
		for col := range int(jpeg.numBlocksWidth) {
			mcuRow := row * int(jpeg.maxV) // Номер текущего MCU
			mcuCol := col * int(jpeg.maxH) // Номер текущего MCU

			curBlock := createYCbCrBlock(jpeg.maxV, jpeg.maxH)

			for c := range jpeg.numOfComps {
				jpeg.componentCalc(blocks, uint(mcuRow), uint(mcuCol), curBlock, Channel(c), readAll)
			}

			for i := range int(jpeg.maxV) {
				for j := range int(jpeg.maxH) {
					jpeg.copyToRes(curBlock[i][j], jpeg.img, mcuRow*unitRowCount+i*unitRowCount, mcuCol*unitColCount+j*unitColCount)
				}
			}
		}
	}
}
