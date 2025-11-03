package main

import (
	"decoder/huffman"
	"fmt"
	"log"
	"math"
)

// Последовательность зиг-зага
var zigZagTable [8][8]byte = [8][8]byte{
	{0, 1, 5, 6, 14, 15, 27, 28},
	{2, 4, 7, 13, 16, 26, 29, 42},
	{3, 8, 12, 17, 25, 30, 41, 43},
	{9, 11, 18, 24, 31, 40, 44, 53},
	{10, 19, 23, 32, 39, 45, 52, 54},
	{20, 22, 33, 38, 46, 51, 55, 60},
	{21, 34, 37, 47, 50, 56, 59, 61},
	{35, 36, 48, 49, 57, 58, 62, 63},
}

// Таблица с коэффициентами в ОДКП
var idctTable [8][8]float64 = [8][8]float64{
	{0.707107, 0.707107, 0.707107, 0.707107, 0.707107, 0.707107, 0.707107, 0.707107},
	{0.980785, 0.831470, 0.555570, 0.195090, -0.195090, -0.555570, -0.831470, -0.980785},
	{0.923880, 0.382683, -0.382683, -0.923880, -0.923880, -0.382683, 0.382683, 0.923880},
	{0.831470, -0.195090, -0.980785, -0.555570, 0.555570, 0.980785, 0.195090, -0.831470},
	{0.707107, -0.707107, -0.707107, 0.707107, 0.707107, -0.707107, -0.707107, 0.707107},
	{0.555570, -0.980785, 0.195090, 0.831470, -0.831470, -0.195090, 0.980785, -0.555570},
	{0.382683, -0.923880, 0.923880, -0.382683, -0.382683, 0.923880, -0.923880, 0.382683},
	{0.195090, -0.555570, 0.831470, -0.980785, 0.980785, -0.831470, 0.555570, -0.195090},
}

// Структура для хранения данных в YCbCr формате
type yCbCr struct {
	y  float32
	cb float32
	cr float32
}

// Структура для хранения данных в RGB формате
type rgb struct {
	r byte
	g byte
	b byte
}

const rgbDelta = 128       //Константа, которая прибавляется при переводе в RGB
const dataUnitRowCount = 8 //Количество строк в data unit
const dataUnitColCount = 8 //Количество столбцов в data unit

var numOfMcuWidth uint16  //Количество MCU в скане при baseline по высоте
var numOfMcuHeight uint16 //Количество MCU в скане при baseline по ширине
var mcuHeight uint16      //Высота MCU
var mcuWidth uint16       //Ширина MCU
var skips uint16          //Счетчик пропусков вычислений в progressive
var prev []int16          //Предыдущие значения DC для дельта кодирования
var dataUnitByComp []byte //Количество блоков для каждой компоненты

var wasEOI = false //Флаг встречался ли маркер EOI при выполнении restart
// var sumUnits byte

// Использовалась при отладке для печати data unit
func printUnit(table []int16) {
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			fmt.Printf("%d\t", table[i*8+j])
		}
		fmt.Printf("\n")

	}
	fmt.Printf("\n\n")
	// log.Fatal()
}

// Использовалась при отладке для печати результата ОДКП
func printCos(table [][]byte) {
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			fmt.Printf("%d\t", table[i][j])
		}
		fmt.Printf("\n")

	}
	fmt.Printf("\n\n")
}

// Создание пустого изображения YCbCr
func createEmptyImage(height uint16, width uint16) [][]rgb {
	img := make([][]rgb, height)
	for i := range height {
		img[i] = make([]rgb, width)
	}
	return img
}

// Создание пустого MCU
func createEmptyMCU(height uint16, width uint16) [][]yCbCr {
	img := make([][]yCbCr, height)
	for i := range height {
		img[i] = make([]yCbCr, width)
	}
	return img
}

// Сoздание пустой матрицы блоков
func createBlockMatrix(blocksHeight uint16, blocksWidth uint16) [][]block {
	blocks := make([][]block, blocksHeight)
	for i := range blocksHeight {
		blocks[i] = make([]block, blocksWidth)
		for j := range blocksWidth {
			blocks[i][j] = makeBlock()
		}
	}
	return blocks
}

// Вычисление тех переменных, которые нужны при сканах, но вычисляются единожды
func preInit() {
	skips = 0
	mcuHeight = uint16(dataUnitRowCount * maxV)
	mcuWidth = uint16(dataUnitColCount * maxH)

	//Количество data unit для каждой компоненты
	dataUnitByComp = make([]byte, numOfComps)
	for i := range numOfComps {
		dataUnitByComp[i] = comps[i].h * comps[i].v
	}

	if isProgressive {
		numOfBlocksHeight = (imageHeight + (blockHeight - 1)) / (blockHeight)
		numOfBlocksWidth = (imageWidth + (blockWidth - 1)) / (blockWidth)
	} else {
		numOfMcuHeight = (imageHeight + (mcuHeight - 1)) / mcuHeight
		numOfMcuWidth = (imageWidth + (mcuWidth - 1)) / mcuWidth
	}
}

// Инициализация декодирования, вычисление вспомогательных переменных
func decodeInit() {
	prev = make([]int16, numOfComps)
	skips = 0
	reader.UpdateBitRead()
}

// Сброс дельта-кодирования
func restart() {
	prev = make([]int16, numOfComps)
	skips = 0
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
	if skips > 0 {
		skips--
		return
	}

	// unitLen := byte(dataUnitRowCount * dataUnitColCount)
	unitLen := endSpectral

	//При baseline отсчет ведется учитывая DC, что следует учесть
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
				skips = ((1 << big) - 1)
				skips += reader.GetBits(big)
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

// Деквантование
func dequant(unit []int16, table []byte) {
	for i := range unit {
		unit[i] = unit[i] * int16(table[i])
	}
}

// Зиг-заг преобразование
func zigZag(unit []int16) [][]int16 {
	//Создание матрицы
	res := make([][]int16, dataUnitRowCount)
	for i := range dataUnitRowCount {
		res[i] = make([]int16, dataUnitColCount)
		for j := range dataUnitColCount {
			res[i][j] = unit[zigZagTable[i][j]]
		}
	}
	return res
}

// Обратное дискретно-косинусное преобразование
func inverseCosin(unit [][]int16) [][]float32 {
	res := make([][]float32, dataUnitRowCount)
	for i := range dataUnitRowCount {
		res[i] = make([]float32, dataUnitColCount)
	}
	for x := range dataUnitRowCount {
		for y := range dataUnitColCount {
			sum := 0.0
			for u := range dataUnitRowCount {
				for v := range dataUnitColCount {
					sum += float64(unit[u][v]) * idctTable[u][x] * idctTable[v][y]
				}
			}
			res[x][y] = float32(0.25 * sum)
		}
	}
	return res
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
func decodeDataUnit(elemID byte) [][]float32 {
	temp := make([]int16, dataUnitRowCount*dataUnitColCount)
	temp[0] = decodeDC(elemID, dcTables[comps[elemID].dcTableID])
	decodeAC(temp, acTables[comps[elemID].acTableID])
	dequant(temp, quantTables[comps[elemID].quantTableID])
	// printUnit(temp)
	matrix := zigZag(temp)
	t := inverseCosin(matrix)
	// printCos(t)

	return t
}

// Перевод изображения в RGB
func toRGB(img [][]yCbCr) [][]rgb {
	res := createEmptyImage(uint16(len(img)), uint16(len(img[0])))

	var width byte
	var height byte
	if isProgressive {
		width = blockWidth
		height = blockHeight
	} else {
		width = byte(mcuWidth)
		height = byte(mcuHeight)
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
	// log.Fatal()

	return res
}

// Декодирование одного MCU
func decodeMCU() [][]yCbCr {
	img := createEmptyMCU(mcuHeight, mcuWidth)
	//Для каждой компоненты
	for i := range numOfComps {
		var xPadding byte //Отступ в текущем MCU по x
		var yPadding byte //Отступ в текущем MCU по y
		//Для каждого data unit в компоненте
		for k := range dataUnitByComp[i] {
			scalingX := maxV / comps[i].v //Растяжение по высоте
			scalingY := maxH / comps[i].h //Растяжение по ширине
			unit := decodeDataUnit(i)
			//Копирование data unit в MCU с растяжением и сдвигом
			for x := xPadding; x < xPadding+dataUnitRowCount*scalingX; x++ {
				for y := yPadding; y < yPadding+dataUnitColCount*scalingY; y++ {
					mcuI := x % (dataUnitRowCount * scalingX) //Координаты в текущем data unit высота
					mcuJ := y % (dataUnitColCount * scalingY) //Координаты в текущем data unit ширина
					//В зависимости от компоненты записываем результат в разные поля
					switch i {
					case 0:
						img[x][y].y = unit[mcuI/scalingX][mcuJ/scalingY]
					case 1:
						img[x][y].cb = unit[mcuI/scalingX][mcuJ/scalingY]
					case 2:
						img[x][y].cr = unit[mcuI/scalingX][mcuJ/scalingY]
					}
				}
			}

			//Вычисление сдвига текущего data unit при записи в MCU
			if (k+1)%(comps[i].h) != 0 {
				yPadding += dataUnitRowCount
			} else {
				yPadding = 0
				xPadding += dataUnitColCount
			}

			// 	log.Fatalf("decodeMCU -> incorrect (h.v) values(%d.%d)", comps[i].h, comps[i].v)
		}
	}
	return img
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
// Декодирование скана, img - прочитанное к моменту вызова функции изображение
func decodeScan(img [][]rgb) [][]rgb {
	decodeInit()
	var mcuCount uint //Общее количество прочитанных mcu
	var row uint16    //Счетчик строк MCU
	var col uint16    //Счетчик столбцов MCU
	var i uint16      //Счетчик пикселей в изображении по ширине
	var j uint16      //Счетчик пикселей в изображении по высоте
	// Для каждого MCU в изображении
	for row = 0; row < numOfMcuHeight; row++ {
		for col = 0; col < numOfMcuWidth; col++ {
			//Декодировать его и преобразовать в RGB
			mcu := toRGB(decodeMCU())
			for i = row * mcuHeight; i < mcuHeight*(row+1) && i < imageHeight; i++ {
				for j = col * mcuWidth; j < mcuWidth*(col+1) && j < imageWidth; j++ {
					mcuI := i % mcuWidth        //Счетчик пикселей в MCU по ширине
					mcuJ := j % mcuHeight       //Счетчик пикселей в MCU по высоте
					img[i][j] = mcu[mcuI][mcuJ] //Копирование в результирующее изображение
				}
			}
			mcuCount++
			if restartInterval != 0 && mcuCount%uint(restartInterval) == 0 && !makeRestart() {
				log.Fatal("makeRestart wrong marker")
			}
		}
	}
	return img
}

// Вычисление преобразований после чтения всех частей прогрессива
// Проводятся деквантование, зиг-заг и ОДКП преобразования
func progressiveCalc(img [][]block) [][]rgb {
	res := createEmptyImage(imageHeight, imageWidth)

	for row := range numOfBlocksHeight {
		for col := range numOfBlocksWidth {
			mcu := img[row][col].toRGB(quantTables[comps[0].quantTableID], quantTables[comps[1].quantTableID], quantTables[comps[2].quantTableID])

			//Копирование в результирующее изображение
			for i := row * blockHeight; i < blockHeight*(row+1) && i < imageHeight; i++ {
				for j := col * blockWidth; j < blockWidth*(col+1) && j < imageWidth; j++ {
					mcuI := i % blockWidth  //Счетчик пикселей в MCU по ширине
					mcuJ := j % blockHeight //Счетчик пикселей в MCU по высоте
					res[i][j] = mcu[mcuI][mcuJ]
				}
			}
		}
	}
	return res
}

// Декодирование скана, blocks - изображение разбитое на блоки(передается по ссылке от скана к скану)
func decodeProgScan(blocks [][]block) {
	decodeInit()

	var blockCount uint16
	fmt.Printf("Dataunits: y:%d cb:%d cr:%d\n", dataUnitByComp[0], dataUnitByComp[1], dataUnitByComp[2])

	//Chroma subsampling для DC
	if true && endSpectral != 5 && startSpectral != 6 {
		//Поблочное чтение со сдвигом на subsampling
		//Например, maxH:2<->maxV:2 будут читаться блоками 2х2
		for row := uint16(0); row < numOfBlocksHeight; row += uint16(maxV) {
			for col := uint16(0); col < numOfBlocksWidth; col += uint16(maxH) {
				for i := range comps {
					if !comps[i].used {
						continue
					}

					var readed byte
					var value int16
					var bit byte
					temp := make([]int16, 64)

					for xPadding := uint16(0); xPadding < uint16(maxV) && (row+xPadding) < numOfBlocksHeight; xPadding++ {
						for yPadding := uint16(0); yPadding < uint16(maxH) && (col+yPadding) < numOfBlocksWidth; yPadding++ {
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
	for row := range numOfBlocksHeight {
		for col := range numOfBlocksWidth {
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
					if skips == 0 {
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
									skips = 1 << high
									skips += reader.GetBits(high)
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
						skips -= 1
					}
				}
			}

			if restartInterval != 0 && blockCount%uint16(restartInterval) == 0 && !makeRestart() {
				log.Fatal("makeRestart wrong marker")
			}

		}
	}
}
