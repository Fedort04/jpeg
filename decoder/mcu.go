package decoder

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

const UnitRowCount = 8 //Количество строк в data unit
const UnitColCount = 8 //Количество столбцов в data unit

type Channel byte

const (
	Y  Channel = 0
	Cb Channel = 1
	Cr Channel = 2
)

var NumOfMCUHeight uint16 //Количество MCU в изображении по высоте
var NumOfMCUWidth uint16  //Количество MCU в изображении по ширине

// Структура для MCU
type MCU struct {
	//Пиксели текущего блока (коэффициенты из потока)
	Y  []int16 //Коэффициент из потока
	Cb []int16 //Коэффициент из потока
	Cr []int16 //Коэффициент из потока
}

// Конструткор MCU
func MakeMCU() MCU {
	var res MCU
	res.Y = make([]int16, UnitRowCount*UnitColCount)
	res.Cb = make([]int16, UnitRowCount*UnitColCount)
	res.Cr = make([]int16, UnitRowCount*UnitColCount)
	return res
}

// Сoздание пустой матрицы MCU
func CreateMCUMatrix(MCUsHeight uint16, MCUsWidth uint16) [][]MCU {
	blocks := make([][]MCU, MCUsHeight)
	for i := range MCUsHeight {
		blocks[i] = make([]MCU, MCUsWidth)
		for j := range MCUsWidth {
			blocks[i][j] = MakeMCU()
		}
	}
	return blocks
}

// Деквантование
// Передается номер канала ch и таблица квантования для него
func (unit *MCU) Dequant(quantTable []byte, ch Channel) {
	switch ch {
	case Y:
		for i := range unit.Y {
			unit.Y[i] = unit.Y[i] * int16(quantTable[i])
		}
	case Cb:
		for i := range unit.Cb {
			unit.Cb[i] = unit.Cb[i] * int16(quantTable[i])
		}
	case Cr:
		for i := range unit.Cr {
			unit.Cr[i] = unit.Cr[i] * int16(quantTable[i])
		}
	}
}

// Зиг-заг преобразование
func zigZag(unit []int16) [][]int16 {
	//Создание матрицы
	res := make([][]int16, UnitRowCount)
	for i := range UnitRowCount {
		res[i] = make([]int16, UnitColCount)
		for j := range UnitColCount {
			res[i][j] = unit[zigZagTable[i][j]]
		}
	}
	return res
}

// Обратное дискретно-косинусное преобразование
func idctCalc(unit [][]int16) [][]float32 {
	res := make([][]float32, UnitRowCount)
	for i := range UnitRowCount {
		res[i] = make([]float32, UnitColCount)
	}
	for x := range UnitRowCount {
		for y := range UnitColCount {
			sum := 0.0
			for u := range UnitRowCount {
				for v := range UnitColCount {
					sum += float64(unit[u][v]) * idctTable[u][x] * idctTable[v][y]
				}
			}
			res[x][y] = float32(0.25 * sum)
		}
	}
	return res
}

// Обратное дискретно-косинусное преобразование канала ch
// Используя ее создается блок MCU, который обрабатывается до ргб и записывается в результат
func (unit *MCU) InverseCosin(ch Channel) [][]float32 {
	switch ch {
	case Y:
		return idctCalc(zigZag(unit.Y))
	case Cb:
		return idctCalc(zigZag(unit.Cb))
	case Cr:
		return idctCalc(zigZag(unit.Cr))
	default:
		return nil
	}
}

// Перевод блока в RGB c проведением деквантования, ОДКП
// func (b *MCU) toRGB(quantTableL []byte, quantTableCb []byte, quantTableCr []byte) [][]rgb {
// 	// dequant(b.Y, quantTableL)
// 	// dequant(b.Cb, quantTableCb)
// 	// dequant(b.Cr, quantTableCr)

// 	y := idctCalc(zigZag(b.Y))
// 	cb := idctCalc(zigZag(b.Cb))
// 	cr := idctCalc(zigZag(b.Cr))

// 	lum := createYCbCrBlock(byte(UnitRowCount), byte(UnitColCount))
// 	//Копирование в структуру YCbCr
// 	for i := range UnitRowCount {
// 		for j := range UnitColCount {
// 			lum[i][j].y = y[i][j]
// 			lum[i][j].cb = cb[i][j]
// 			lum[i][j].cr = cr[i][j]
// 		}
// 	}

// 	return toRGB(lum)
// }
