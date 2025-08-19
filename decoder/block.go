package main

// Инициализация при decodeInit
var mcuWidth uint16  //Ширина MCU
var mcuHeight uint16 //Высота MCU

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
	res.Y = make([]int16, mcuHeight*mcuWidth)
	res.Cb = make([]int16, mcuHeight*mcuWidth)
	res.Cr = make([]int16, mcuHeight*mcuWidth)
	return res
}

// Перевод блока в RGB c проведением деквантования, ОДКП
func (b *block) toRGB(quantTableL []byte, quantTableCr []byte) [][]rgb {
	dequant(b.Y, quantTableL)
	dequant(b.Cb, quantTableCr)
	dequant(b.Cr, quantTableCr)

	y := inverseCosin(zigZag(b.Y))
	cb := inverseCosin(zigZag(b.Cb))
	cr := inverseCosin(zigZag(b.Cr))

	lum := createEmptyMCU(mcuHeight, mcuWidth)
	for i := range mcuHeight {
		for j := range mcuWidth {
			lum[i][j].y = y[i][j]
			lum[i][j].cb = cb[i][j]
			lum[i][j].cr = cr[i][j]
		}
	}

	return toRGB(lum)
}
