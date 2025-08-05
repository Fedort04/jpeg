package main

import (
	"decoder/huffman"
	"log"
	"math"
)

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

// Декодирование знака в потоке Хаффмана
func decodeSign(num byte, len byte) int16 {
	if num >= (1<<len - 1) {
		return int16(num)
	} else {
		return int16(num - (1 << len) + 1)
	}
}

// Декодирование DC элемента
func decodeDC(id byte, huff *huffman.HuffTable) int16 {
	temp := huff.DecodeHuff(reader)
	diff := decodeSign(reader.GetBits(byte(temp)), byte(temp))
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
		unit[k] = decodeSign(bits, small)
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

}

// Создание пустого изображения YCbCr
// Переписать как чистую
func createEmptyImage() [][]rgb {
	img := make([][]rgb, imageHeight)
	for i := range imageHeight {
		img[i] = make([]rgb, imageWidth)
	}
	return img
}

// Создание пустого MCU
// Переписать как чистую
func createEmptyMCU() [][]yCbCr {
	img := make([][]yCbCr, mcuHeight)
	for i := range imageWidth {
		img[i] = make([]yCbCr, mcuWidth)
	}
	return img
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
	matrix := zigZag(temp)
	inverseCosin()
}

// Перевод изображения в RGB
func toRGB(img [][]yCbCr) [][]rgb {
	res := make([][]rgb, len(img))
	for i := range len(res) {
		res[i] = make([]rgb, len(img[0]))
	}
	for i := range mcuHeight {
		for j := range mcuWidth {
			img[i][j].y += 128
			img[i][j].cb += 128
			img[i][j].cr += 128
			res[i][j].r = Clamp255(int(math.Round(float64(img[i][j].y) + 1.402*float64((img[i][j].cr-128)))))
			res[i][j].g = Clamp255(int(math.Round(float64(img[i][j].y) - 0.34414*float64((img[i][j].cb-128)) - 0.71414*float64((img[i][j].cr-128)))))
			res[i][j].b = Clamp255(int(math.Round(float64(img[i][j].y) + 1.772*float64((img[i][j].cb-128)))))
		}
	}
	return res
}

// Декодирование одного MCU
func decodeMCU() [][]yCbCr {
	img := createEmptyMCU()
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
					if i == 0 {
						img[x][y].y = unit[mcuI/scalingX][mcuJ/scalingY]
					} else if i == 1 {
						img[x][y].cb = unit[mcuI/scalingX][mcuJ/scalingY]
					} else if i == 2 {
						img[x][y].cr = unit[mcuI/scalingX][mcuJ/scalingY]
					}
				}
			}
			if comps[i].h > 1 && yPadding == 0 && k != 3 {
				yPadding += dataUnitRowCount
			} else if comps[i].v > 1 && xPadding == 0 && k != 3 {
				yPadding = 0
				xPadding += dataUnitColCount
			} else if comps[i].h == 2 && comps[i].v == 2 {
				yPadding += dataUnitRowCount
			} else if comps[i].h != 1 && comps[i].v != 1 {
				log.Fatalf("decodeMCU -> incorrect (h.v) values(%d.%d)", comps[i].h, comps[i].v)
			}
		}
	}
	return img
}

// Декодирование скана
func decodeScan() {
	decodeInit()
	img := createEmptyImage()
	var mcuCount uint //Общее количество прочитанных mcu
	var row uint16    //Счетчик строк MCU
	var col uint16    //Счетчик столбцов MCU
	var i uint16      //Счетчик пикселей в изображении по ширине
	var j uint16      //Счетчик пикселей в изображении по высоте
	for row = 0; row < (imageHeight+(mcuHeight-1))/mcuHeight; row++ {
		for col = 0; col < (imageWidth+(mcuWidth-1))/mcuWidth; col++ {
			mcu := toRGB(decodeMCU())
			for i = row * mcuHeight; i < mcuHeight*(row+1) && i < imageHeight; i++ {
				for j = col * mcuWidth; j < mcuWidth*(col+1) && j < imageWidth; j++ {
					mcuI := i % mcuWidth        //Счетчик пикселей в MCU по ширине
					mcuJ := j % mcuHeight       //Счетчик пикселей в MCU по высоте
					img[i][j] = mcu[mcuI][mcuJ] //Копирование в результирующее изображение
				}
				mcuCount++
			}
		}
	}
}
