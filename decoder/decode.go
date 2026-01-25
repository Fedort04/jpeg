package decoder

import (
	"fmt"
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

var bandSkips uint16      //Счетчик пропусков вычислений в progressive
var prev []int16          //Предыдущие значения DC для дельта кодирования
var dataUnitByComp []byte //Количество блоков для каждой компоненты (позже вероятно)

var wasEOI = false //Флаг встречался ли маркер EOI при выполнении restart

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
	//Количество data unit для каждой компоненты
	dataUnitByComp = make([]byte, numOfComps)
	for i := range numOfComps {
		dataUnitByComp[i] = comps[i].h * comps[i].v
	}

	NumOfMCUHeight = (imageHeight + (UnitRowCount - 1)) / (UnitRowCount)
	NumOfMCUHeight += NumOfMCUHeight % uint16(maxV)

	NumOfMCUWidth = (imageWidth + (UnitColCount - 1)) / (UnitColCount)
	NumOfMCUWidth += NumOfMCUWidth % uint16(maxH)
}

// Инициализация дельта-декодирования, перезапуск bands, инициализация побитового чтения
func decodeInit() {
	prev = make([]int16, numOfComps)
	bandSkips = 0
	reader.BitReadInit()
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
func decodeDC(id byte, huff *huffman.HuffTable) int16 {
	temp := huff.DecodeHuff(reader)
	diff := decodeSign(int16(reader.GetBits(byte(temp))), byte(temp))
	res := diff + prev[id]
	prev[id] = res
	return res
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

		if small == 0 {
			if big != 15 {
				bandSkips = ((1 << big) - 1)
				bandSkips += reader.GetBits(big)
				// log.Printf("small: %d, big : %d", small, big)
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
			unit[k] = decodeSign(int16(bits), small) << int16(approxL)
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
func decodeDataUnit(channel byte) []int16 {
	temp := make([]int16, UnitRowCount*UnitColCount)
	temp[0] = decodeDC(channel, dcTables[comps[channel].dcTableID])
	decodeAC(temp, acTables[comps[channel].acTableID])
	return temp
}

// Перевод изображения в RGB (позже удалить)
func toRGB(img yCbCrMatrix) [][]rgb {
	res := createRGBMatrix(uint16(len(img)), uint16(len(img[0])))

	var width byte
	var height byte
	if isProgressive {
		width = UnitColCount
		height = UnitRowCount
	}

	for i := range height {
		for j := range width {
			img[i][j].y += rgbDelta
			img[i][j].cb += rgbDelta
			img[i][j].cr += rgbDelta
			res[i][j].r = Clamp255(int(math.Round(float64(img[i][j].y) + 1.402*float64((float64(img[i][j].cr)-rgbDelta)))))
			res[i][j].g = Clamp255(int(math.Round(float64(img[i][j].y) - 0.34414*float64((float64(img[i][j].cb)-rgbDelta)) - 0.71414*float64((float64(img[i][j].cr)-rgbDelta)))))
			res[i][j].b = Clamp255(int(math.Round(float64(img[i][j].y) + 1.772*float64((float64(img[i][j].cb)-rgbDelta)))))

			//Вывод в 16 виде преобразованных данных для отладки
			// fmt.Printf("x%x%x%x ", res[i][j].r, res[i][j].g, res[i][j].b)
			// if j == 15 {
			// 	fmt.Printf("\n")
			// }
		}
	}
	return res
}

// Декодирование блока MCU
// x y координаты левого верхнего MCU в блоке
func decodeBlock(blocks [][]MCU, x uint16, y uint16) {
	for i := range numOfComps {
		if !comps[i].used {
			continue
		}

		for curV := range uint16(comps[i].v) {
			for curH := range uint16(comps[i].h) {
				switch i {
				case byte(Y):
					blocks[x+curV][y+curH].Y = decodeDataUnit(i)
				case byte(Cb):
					blocks[x+curV][y+curH].Cb = decodeDataUnit(i)
				case byte(Cr):
					blocks[x+curV][y+curH].Cr = decodeDataUnit(i)
				}
			}
		}
	}
}

// Выполнение рестарта дельта кодирвоания
func makeRestart() bool {
	marker := reader.GetWord()
	if marker == EOI {
		wasEOI = true
		return true
	} else if marker >= RST0 && marker <= RST7 {
		reader.BitsAlign()
		restart()
		return true
	}
	log.Printf("marker: %x, nextByte: %d", marker, reader.GetNextByte())
	return false
}

// Baseline
// Декодирование скана, blocks - ссылка на прочитанное к моменту вызова функции изображение
func decodeBaselineScan(blocks [][]MCU) {
	decodeInit()
	var blockCount uint //Общее количество прочитанных блоков mcu
	var row uint16      //Счетчик строк блоков MCU
	var col uint16      //Счетчик столбцов блоков MCU
	numBlocksHeight := NumOfMCUHeight / uint16(maxV)
	numBlocksWidth := NumOfMCUWidth / uint16(maxH)

	for row = range numBlocksHeight {
		for col = range numBlocksWidth {
			decodeBlock(blocks, row*uint16(maxV), col*uint16(maxH))
			blockCount++
			if restartInterval != 0 && blockCount%uint(restartInterval) == 0 && !makeRestart() {
				log.Fatal("makeRestart wrong marker")
			}
		}
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

// Вычисление преобразований после чтения всех частей прогрессива
// Проводятся деквантование, зиг-заг и ОДКП преобразования
func progressiveCalc(img1 [][]MCU) {
	res := createRGBMatrix(imageHeight, imageWidth)

	for row := range NumOfMCUHeight {
		for col := range NumOfMCUWidth {
			// mcu := img1[row][col].toRGB(quantTables[comps[0].quantTableID], quantTables[comps[1].quantTableID], quantTables[comps[2].quantTableID])

			//Копирование в результирующее изображение
			for i := row * UnitRowCount; i < UnitRowCount*(row+1) && i < imageHeight; i++ {
				for j := col * UnitColCount; j < UnitColCount*(col+1) && j < imageWidth; j++ {
					// mcuI := i % UnitColCount //Счетчик пикселей в MCU по ширине
					// mcuJ := j % UnitRowCount //Счетчик пикселей в MCU по высоте
					// res[i][j] = mcu[mcuI][mcuJ]
				}
			}
		}
	}
	img = res
}

// Декодирование скана, blocks - изображение разбитое на блоки(передается по ссылке от скана к скану)
func decodeProgScan(blocks [][]MCU) {
	decodeInit()

	var blockCount uint16
	fmt.Printf("Dataunits: y:%d cb:%d cr:%d\n", dataUnitByComp[0], dataUnitByComp[1], dataUnitByComp[2])

	//Chroma subsampling для DC
	if true && endSpectral != 5 && startSpectral != 6 {
		//Поблочное чтение со сдвигом на subsampling
		//Например, maxH:2<->maxV:2 будут читаться блоками 2х2
		for row := uint16(0); row < NumOfMCUHeight; row += uint16(maxV) {
			for col := uint16(0); col < NumOfMCUWidth; col += uint16(maxH) {
				for i := range comps {
					if !comps[i].used {
						continue
					}

					var readed byte
					var value int16
					var bit byte
					temp := make([]int16, 64)

					for xPadding := uint16(0); xPadding < uint16(maxV) && (row+xPadding) < NumOfMCUHeight; xPadding++ {
						for yPadding := uint16(0); yPadding < uint16(maxH) && (col+yPadding) < NumOfMCUWidth; yPadding++ {
							if startSpectral == 0 && approxH == 0 { //Первое чтение DC=======================
								if readed < dataUnitByComp[i] {
									value = decodeDC(byte(i), dcTables[comps[i].dcTableID]) << int16(approxL)
									readed++
								}
								switch i {
								case 0:
									blocks[row+xPadding][col+yPadding].Y[0] = value
								case 1:
									blocks[row+xPadding][col+yPadding].Cb[0] = value
								case 2:
									blocks[row+xPadding][col+yPadding].Cr[0] = value
								}

							} else if startSpectral == 0 && approxH != 0 { //Повторное чтение DC=======================
								if readed < dataUnitByComp[i] {
									bit = reader.GetBit()
									readed++
								}
								switch i {
								case 0:
									blocks[row+xPadding][col+yPadding].Y[0] |= int16(bit << approxL)
								case 1:
									blocks[row+xPadding][col+yPadding].Cb[0] |= int16(bit << approxL)
								case 2:
									blocks[row+xPadding][col+yPadding].Cr[0] |= int16(bit << approxL)
								}

							} else if startSpectral != 0 && approxH == 0 { //ошибка, так как должно построчно читаться only luminance...
								var arr []int16
								switch i {
								case 1:
									arr = blocks[row+xPadding][col+yPadding].Cb
								case 2:
									arr = blocks[row+xPadding][col+yPadding].Cr
								}
								if readed < dataUnitByComp[i] {
									temp = make([]int16, 64)
									copy(temp, arr)
									decodeAC(temp, acTables[comps[i].acTableID])
									readed++
								}
								copy(arr, temp)
							}
						}
					}
				}
			}
		}

		return
	}

	//Для каждого блока (для AC)
	for row := range NumOfMCUHeight {
		for col := range NumOfMCUWidth {
			blockCount++

			//Для каждой цветовой компоненты
			for i := range comps {
				if !comps[i].used {
					continue
				}

				if startSpectral != 0 && approxH == 0 { //Первое чтение AC=======================
					switch i {
					case 0:
						decodeAC(blocks[row][col].Y, acTables[comps[i].acTableID])
					case 1:
						decodeAC(blocks[row][col].Cb, acTables[comps[i].acTableID])
					case 2:
						decodeAC(blocks[row][col].Cr, acTables[comps[i].acTableID])
					}

				} else { //Повторное чтение AC=======================
					positive := int16(1 << approxL)
					temp := -1 //Нужно для перевода в uint отрицательного числа с переполнением
					negative := int16((uint(temp)) << approxL)

					k := startSpectral

					//Сохранение указателя на текущий цветовой блок
					var arr []int16
					switch i {
					case 0:
						arr = blocks[row][col].Y
					case 1:
						arr = blocks[row][col].Cb
					case 2:
						arr = blocks[row][col].Cr
					}

					// Если не нужно пропускать диапазоны
					if bandSkips == 0 {
						for ; k <= endSpectral; k++ {
							sym := acTables[comps[i].acTableID].DecodeHuff(reader)

							high := byte(sym >> 4)
							low := byte(sym & 0x0F)
							coeff := int16(0)

							if low == 1 {
								switch reader.GetBit() {
								case 1:
									coeff = positive
								case 0:
									coeff = negative
								}
							} else { //low == 0
								if high != 15 {
									bandSkips = 1 << high
									bandSkips += reader.GetBits(high)
									break
								}
							}

							//While loop для пропуска необходимого количества нулей
							//с дополнительной проверкой на конец спектральной подборки

							for k <= endSpectral {
								if arr[k] != 0 {
									if reader.GetBit() == 1 {
										if arr[k]&positive == 0 { //Проверка, не был ли записан бит ранее
											if arr[k] >= 0 {
												arr[k] += positive
											} else {
												arr[k] += negative
											}
										}
									}

								} else { //Пропуск нулей
									if high == 0 {
										break
									}

									high -= 1
								}
								k++
							}
							if coeff != 0 && k <= endSpectral {
								arr[k] = coeff
							}
						}
					} else { //skips > 0
						//Считываем для каждого ненулевого значения новый бит
						for ; k <= endSpectral; k++ {
							if arr[k] != 0 {
								if reader.GetBit() == 1 {
									if arr[k]&positive == 0 { //Проверка, не был ли записан бит ранее
										if arr[k] >= 0 {
											arr[k] += positive
										} else {
											arr[k] += negative
										}
									}
								}
							}
						}
						bandSkips -= 1
					}
				}
			}

			if restartInterval != 0 && blockCount%uint16(restartInterval) == 0 && !makeRestart() {
				log.Fatal("makeRestart wrong marker")
			}

		}
	}
}
