package main

// Инициализация при decodeInit
const blockWidth = 8         //Ширина блока
const blockHeight = 8        //Высота блока
var numOfBlocksHeight uint16 //Количество блоков(MCU) в изображении по высоте
var numOfBlocksWidth uint16  //Количество блоков(MCU) в изображении по ширине

// Структура для MCU
type block struct {
	//Пиксели текущего блока (коэффициенты из потока)
	Y  []int16 //Коэффициент из потока
	Cb []int16 //Коэффициент из потока
	Cr []int16 //Коэффициент из потока
}

// Конструткор блока
func makeBlock() block {
	var res block
	res.Y = make([]int16, blockHeight*blockWidth)
	res.Cb = make([]int16, blockHeight*blockWidth)
	res.Cr = make([]int16, blockHeight*blockWidth)
	return res
}

// Перевод блока в RGB c проведением деквантования, ОДКП
func (b *block) toRGB(quantTableL []byte, quantTableCb []byte, quantTableCr []byte) [][]rgb {
	dequant(b.Y, quantTableL)
	dequant(b.Cb, quantTableCb)
	dequant(b.Cr, quantTableCr)

	y := inverseCosin(zigZag(b.Y))
	cb := inverseCosin(zigZag(b.Cb))
	cr := inverseCosin(zigZag(b.Cr))

	lum := createEmptyMCU(uint16(blockHeight), uint16(blockWidth))
	//Копирование в структуру YCbCr
	for i := range blockHeight {
		for j := range blockWidth {
			lum[i][j].y = y[i][j]
			lum[i][j].cb = cb[i][j]
			lum[i][j].cr = cr[i][j]
		}
	}

	return toRGB(lum)
}
