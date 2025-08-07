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
	y  byte
	cb byte
	cr byte
}

// Структура для хранения данных в RGB формате
type rgb struct {
	r byte
	g byte
	b byte
}

const dataUnitRowCount = 8 //Количество строк в data unit
const dataUnitColCount = 8 //Количество столбцов в data unit

var prev []int16          //Предыдущие значения DC для дельта кодирования
var mcuWidth uint16       //Ширина MCU
var mcuHeight uint16      //Высота MCU
var dataUnitByComp []byte //Количество блоков для каждой компоненты
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

// Инициализация декодирования, вычисление вспомогательных переменных
func decodeInit() {
	prev = make([]int16, numOfComps)
	dataUnitByComp = make([]byte, numOfComps)
	for i := range numOfComps {
		dataUnitByComp[i] = comps[i].h * comps[i].v
	}
	mcuHeight = uint16(dataUnitRowCount * maxV)
	mcuWidth = uint16(dataUnitColCount * maxH)
}

// Сброс дельта-кодирования
func restart() {
	prev = make([]int16, numOfComps)
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
	unitLen := byte(dataUnitRowCount * dataUnitColCount)
	var k byte
	k = 1
	for k < unitLen {
		rs := huff.DecodeHuff(reader)
		big := byte(rs >> 4)
		small := byte(rs & 0x0f)
		if small == 0 {
			if big != 15 {
				return
			} else {
				k += 16
				continue
			}
		}
		k += big
		if k >= unitLen {
			log.Fatalf("decodeAC -> error: k(%d) bigger than unit length", k)
		}
		bits := reader.GetBits(small)
		unit[k] = decodeSign(int16(bits), small)
		k++
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
func inverseCosin(unit [][]int16) [][]byte {
	res := make([][]byte, dataUnitRowCount)
	for i := range dataUnitRowCount {
		res[i] = make([]byte, dataUnitColCount)
	}
	for x := range dataUnitRowCount {
		for y := range dataUnitColCount {
			sum := 0.0
			for u := range dataUnitRowCount {
				for v := range dataUnitColCount {
					sum += float64(unit[u][v]) * idctTable[u][x] * idctTable[v][y]
				}
			}
			res[x][y] = byte(0.25 * sum)
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
func decodeDataUnit(elemID byte) [][]byte {
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

	for i := range mcuHeight {
		for j := range mcuWidth {
			img[i][j].y += 128
			img[i][j].cb += 128
			img[i][j].cr += 128
			res[i][j].r = Clamp255(int(math.Round(float64(img[i][j].y) + 1.402*float64((float64(img[i][j].cr)-128)))))
			res[i][j].g = Clamp255(int(math.Round(float64(img[i][j].y) - 0.34414*float64((float64(img[i][j].cb)-128)) - 0.71414*float64((float64(img[i][j].cr)-128)))))
			res[i][j].b = Clamp255(int(math.Round(float64(img[i][j].y) + 1.772*float64((float64(img[i][j].cb)-128)))))

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
	reader.BitsAlign()
	if marker == EOI {
		return true
	} else if marker >= RST0 && marker <= RST7 {
		restart()
		return true
	}
	return false
}

// Декодирование скана
func decodeScan() [][]rgb {
	decodeInit()
	img := createEmptyImage(imageHeight, imageWidth)
	var mcuCount uint //Общее количество прочитанных mcu
	var row uint16    //Счетчик строк MCU
	var col uint16    //Счетчик столбцов MCU
	var i uint16      //Счетчик пикселей в изображении по ширине
	var j uint16      //Счетчик пикселей в изображении по высоте
	// Для каждого MCU в изображении
	for row = 0; row < (imageHeight+(mcuHeight-1))/mcuHeight; row++ {
		for col = 0; col < (imageWidth+(mcuWidth-1))/mcuWidth; col++ {
			//Декодировать его и преобразовать в RGB
			mcu := toRGB(decodeMCU())
			for i = row * mcuHeight; i < mcuHeight*(row+1) && i < imageHeight; i++ {
				for j = col * mcuWidth; j < mcuWidth*(col+1) && j < imageWidth; j++ {
					mcuI := i % mcuWidth        //Счетчик пикселей в MCU по ширине
					mcuJ := j % mcuHeight       //Счетчик пикселей в MCU по высоте
					img[i][j] = mcu[mcuI][mcuJ] //Копирование в результирующее изображение
				}
				mcuCount++
			}
			if mcuCount%uint(restartInterval) == 0 && !makeRestart() {
				log.Fatal("makeRestart wrong marker")
			}
		}
	}
	return img
}
