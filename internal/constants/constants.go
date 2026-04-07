package constants

// Структура для хранения данных в RGB формате
type Rgb struct {
	R byte
	G byte
	B byte
}

type Image = [][]Rgb

// Маркеры всех используемых заголовков
const (
	SOI   uint16 = 0xFFD8
	EOI   uint16 = 0xFFD9
	SOF0  uint16 = 0xFFC0
	SOF2  uint16 = 0xFFC2
	APP0  uint16 = 0xFFE0
	APP15 uint16 = 0xFFEF
	DQT   uint16 = 0xFFDB
	DHT   uint16 = 0xFFC4
	SOS   uint16 = 0xFFDA
	DRI   uint16 = 0xFFDD
	RST0  uint16 = 0xFFD0
	RST7  uint16 = 0xFFD7
)

const NumOfTables = 4   //Максимальное количество таблиц
const NumOfChannels = 3 //Максимальное количество цветовых компонент
const MaxComps = 3      //Максимальное количество компонент
const ColCount = 8      //Количество столбцов в таблице квантования (для вывода в лог)
const SizeOfTable = 64  //Количество элементов в одной таблице квантования
