package encoder

import "jpeg/shared"

// Константы для кодирования

const samplePrecision = 8        //Глубина цвета
const defaultRestartInterval = 5 //restartInterval по умолчанию
const maxRst = 10                //Максимальное значение параметра RST
const defaultDCApprox = OneBit

var defaultYSpectral = []byte{0, 5, 63} //Для яркости по умолчанию
const defaultYapprox = TwoBits

var defaultCSpectral = []byte{0, 63} //Для цвета по умолчанию
const defaultCapprox = OneBit

// ID таблиц для компонент
var tableIds = map[byte]byte{
	1: 0,
	2: 1,
	3: 1,
}

// =========== Константы для сегмента APP0 ===========
const (
	app0Length  uint16 = 16     //Длина
	jfifVersion uint16 = 0x0102 //Версия
	densityUnit byte   = 0      //Единица измерений
	xDensity    uint16 = 1      //Плотность по Х
	yDensity    uint16 = 1      //Плотность по Y
	xThumb      byte   = 0      //Ширина превью
	yThumb      byte   = 0      //Высота превью
)

var jfif = [5]byte{'J', 'F', 'I', 'F', 0}

// =========== Константы для сегмента DQT ===========
const (
	lumId     byte = 0                          //ID таблицы квантования для яркости
	colorId   byte = 1                          //ID таблицы квантования для цвета
	dqtLength byte = 2 + 1 + shared.SizeOfTable // Длина сегмента DQT
)

// =========== Константы для сегмента SOF ===========
const sofLength byte = 17

// =========== Константы для сегмента DRI ===========
const driLength = 4

// =========== Константы для сегмента SOS ===========
type component struct {
	selector byte
	dcTable  byte
	acTable  byte
}

// Структура с параметрами заголовка скана
type scanHeader struct {
	marker uint16      //Маркер текущего скана
	length uint16      //Длина заголовка
	comps  []component //Данные компонент
	ss     byte        //Spectral selection start
	se     byte        //Spectral selection end
	ah     byte        //Successive approx start
	al     byte        //Successive approx end
}

// После копирования обязательно установить компоненты
func (sh *scanHeader) copy() *scanHeader {
	return &scanHeader{
		marker: sh.marker,
		length: 0,
		ss:     sh.ss,
		se:     sh.se,
		ah:     sh.ah,
		al:     sh.al,
		comps:  nil,
	}
}

// Вычисление длины заголовка по готовой структуре
func (sh *scanHeader) evalLength() {
	sh.length = 2 + 1 + 1 + 1 + 1 + 2*uint16(len(sh.comps))
}

// Установить массив компонент (обновляет длину маркера)
func (sh *scanHeader) setComps(data []component) {
	sh.comps = data
	sh.evalLength()
}

// Значения, которые используются только в Progressive
// Их значение по умолчанию для Baseline
const baselineSOSLength = 12
const baselineColors = 3
const dcSpectral = 0 //Значение DC скана в Spec selection

// =========== Константы для сегмента DHT ===========
// Таблицы квантования из спецификации
// DC яркость
var yDCBits = [16]byte{
	0x00, 0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01,
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}
var yDCSymbols = [12]byte{
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0A, 0x0B,
}

// DC цвет
var cDCBits = [16]byte{
	0x00, 0x03, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
	0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
}
var cDCSymbols = [12]byte{
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0A, 0x0B,
}
