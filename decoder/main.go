package main

import (
	binreader "decoder/binReader"
	binwriter "decoder/binWriter"
	"decoder/huffman"
	"fmt"
	"log"
)

// Структура цветовой компоненты
type component struct {
	h            byte
	v            byte
	quantTableID byte //ID таблицы квантования для этого цвета
	dcTableID    byte //DC таблица для этого цвета
	acTableID    byte //AC таблица для этого цвета
}

// Маркеры всех используемых заголовков
const (
	SOI   uint16 = 0xFFD8
	EOI   uint16 = 0xFFD9
	SOF0  uint16 = 0xFFC0
	APP0  uint16 = 0xFFE0
	APP15 uint16 = 0xFFEF
	DQT   uint16 = 0xFFDB
	DHT   uint16 = 0xFFC4
	SOS   uint16 = 0xFFDA
	DRI   uint16 = 0xFFDD
	RST0  uint16 = 0xFFD0
	RST7  uint16 = 0xFFD7
)

const numOfTables = 4  //Максимальное количество таблиц
const maxComps = 3     //Максимальное количество компонент
const colCount = 8     //Количество столбцов
const sizeOfTable = 64 //Количество элементов в одной таблице

var withDump bool = false                    //Флаг вывода информации заголовков в лог
var reader *binreader.BinReader              //Объект для чтения файла
var quantTables [numOfTables][]byte          //Массив с таблицами квантования
var acTables [numOfTables]*huffman.HuffTable //Массив с AC таблицами Хаффмана
var dcTables [numOfTables]*huffman.HuffTable //Массив с DC таблицами Хаффмана
var samplePrecision byte                     //Глубина цвета
var imageWidth uint16                        //Ширина изображения
var imageHeight uint16                       //Высота изображения
var maxH byte                                //Максимальный Н фактор
var maxV byte                                //Максимальный V фактор
var numOfComps byte                          //Количество цветовых компонет в изображении
var comps [maxComps]component                //Массив с данными о компонентах
var restartInterval uint16                   //Интервал перезапуска дельта кодирования
var startSpectral byte
var endSpectral byte
var ah byte
var al byte

// Вывод компоненты в лог
func printComponent(c component) {
	res := fmt.Sprintf("h: %d, v: %d, quant table id: %d\n", c.h, c.v, c.quantTableID)
	log.Println(res)
}

// Вывод таблицы в лог
func printTable(table []byte) {
	res := "\n"
	for i := range sizeOfTable {
		if i%colCount == 0 && i != 0 {
			res += "\n"
		}
		res += fmt.Sprintf("%d\t", table[i])
	}
	log.Println(res)
}

// Чтение маркера marker
func readMarker(marker uint16) bool {
	if temp := reader.GetWord(); temp != marker {
		return false
	}
	return true
}

// Чтение сегмента приложения
func readApp() {
	ln := reader.GetWord()
	temp := reader.GetArray(ln - 2)

	if withDump {
		log.Println("APP", string(temp))
	}
}

// Чтение таблицы квантования
func readQuantTable() {
	reader.GetWord()
	for reader.GetNextByte() != 0xFF {
		tq := reader.GetByte()
		if tq > numOfTables-1 {
			log.Fatal("readQuantTable -> invalid table destination", tq)
		}
		table := reader.GetArray(sizeOfTable)
		quantTables[tq] = table

		if withDump {
			log.Printf("Quant table destination: %d\n", tq)
			printTable(quantTables[tq])
		}
	}
}

// Чтение и конструирование таблиц Хаффмана
func readHuffTable() {
	reader.GetWord()
	tc, th := reader.Get4Bit()
	if th > numOfTables-1 {
		log.Fatal("readHuffTable -> invalid table destination", th)
	}
	offset := make([]byte, huffman.NumHuffCodesLen+1)
	var sumElem byte //Количество символов
	for i := 1; i < huffman.NumHuffCodesLen+1; i++ {
		sumElem += reader.GetByte()
		offset[i] = sumElem
	}
	symbols := make([]byte, sumElem)
	for i := range sumElem {
		symbols[i] = reader.GetByte()
	}
	huff, err := huffman.MakeHuffTable(offset, symbols)
	if err != nil {
		log.Println("MakeHuffTable -> error")
		log.Fatal(err.Error())
	}
	if tc == 0 {
		dcTables[th] = huff
	} else if tc == 1 {
		acTables[th] = huff
	} else {
		log.Fatal("readHuffTable -> invalid table ID")
	}

	if withDump {
		temp := "DHT "
		if tc == 0 {
			temp += fmt.Sprintf("DC-table %d\n", th)
		} else {
			temp += fmt.Sprintf("AC-table %d\n", th)
		}
		log.Print(temp)
	}

}

// Чтение сегмента с перезапуском дельта-кодирования
func readRestartInterval() {
	reader.GetWord()
	restartInterval = reader.GetWord()

	if withDump {
		log.Print("DRI restart interval: ", restartInterval)
	}
}

// Чтение сегмента таблиц, возвращает следующие за сегментами 2 байта
func readTables() uint16 {
	marker := reader.GetWord()
	isContinue := false
	if marker >= APP0 && marker <= APP15 {
		readApp()
		isContinue = true
	} else if marker == DQT {
		readQuantTable()
		isContinue = true
	} else if marker == DHT {
		readHuffTable()
		isContinue = true
	} else if marker == DRI {
		readRestartInterval()
		isContinue = true
	}
	if isContinue {
		marker = readTables()
	}
	return marker
}

// Чтение заголовка кадра
func readScanHeader() {
	reader.GetWord()
	ns := reader.GetByte()
	for range ns {
		cs := reader.GetByte()
		td, ta := reader.Get4Bit()
		comps[cs-1].dcTableID = td
		comps[cs-1].acTableID = ta
	}
	startSpectral = reader.GetByte()
	endSpectral = reader.GetByte()
	ah, al = reader.Get4Bit()

	if withDump {
		log.Print("SOS")
		for i, temp := range comps {
			log.Printf("component %d:\tDC: %d\tAC: %d\n", i+1, temp.dcTableID, temp.acTableID)
		}
		log.Print("start Spectral Selection: ", startSpectral)
		log.Print("end Spectral Selection: ", endSpectral)
		log.Print("approximation high: ", ah, "; approximation low: ", al)
	}
}

// Чтение заголовка фрейма
func readFrameHeader() {
	reader.GetWord()
	samplePrecision = reader.GetByte()
	imageHeight = reader.GetWord()
	imageWidth = reader.GetWord()
	numOfComps = reader.GetByte()
	for range numOfComps {
		c := reader.GetByte()
		h, v := reader.Get4Bit()
		if h > maxH {
			maxH = h
		}
		if v > maxV {
			maxV = v
		}
		tq := reader.GetByte()
		comps[c-1] = component{h: h, v: v, quantTableID: tq}
	}

	if withDump {
		log.Printf("sample Precision: %d\n", samplePrecision)
		log.Printf("image Width: %d\n", imageWidth)
		log.Printf("image Height: %d\n", imageHeight)
		log.Printf("num Of Comps: %d\n", numOfComps)
		for i := range numOfComps {
			log.Printf("component %d\n", i+1)
			printComponent(comps[i])
		}
	}
}

// Чтение скана
func readScan() [][]rgb {
	nextMarker := readTables()
	if nextMarker != SOS {
		log.Fatalf("readFrame can't read SOS\nMarker: %x", nextMarker)
	}
	readScanHeader()
	return decodeScan()
}

// Чтение кадра
func readFrame() [][]rgb {
	nextMarker := readTables()
	if nextMarker != SOF0 {
		log.Fatalf("readFrame can't read SOF0\nMarker: %x", nextMarker)
	}
	readFrameHeader()
	res := readScan()

	if withDump {
		log.Print("Scan was readed")
	}
	return res
}

// Чтение JPEG файла по пути source
func ReadJPEG(source string, dump bool) [][]rgb {
	withDump = dump
	var err error
	reader, err = binreader.BinReaderInit(source, binreader.BIG)
	if err != nil {
		log.Println("BinReaderInit -> Error")
		log.Fatal(err.Error())
	}

	if !readMarker(SOI) {
		log.Fatal("Can't read SOI marker")
	}

	if withDump {
		log.Println("SOI")
	}

	res := readFrame()

	if !readMarker(EOI) {
		log.Fatal("Can't read EOI marker")
	}

	if withDump {
		log.Print("EOI")
	}

	return res
}

// Кодирование в BMP для наглядности
func encodeBMP(img [][]rgb, fileName string) {
	err := binwriter.BinwriterInit(fileName)
	if err != nil {
		log.Panic(err.Error())
	}
	height := imageHeight
	width := imageWidth
	paddingSize := width % 4
	size := 14 + 12 + height*width*3 + paddingSize*height
	binwriter.PutChar('B')
	binwriter.PutChar('M')
	binwriter.PutInt(uint(size))
	binwriter.PutInt(0)
	binwriter.PutInt(0x1A)
	binwriter.PutInt(12)
	binwriter.PutShort(uint(width))
	binwriter.PutShort(uint(height))
	binwriter.PutShort(1)
	binwriter.PutShort(24)

	for i := int(height - 1); i >= 0; i-- {
		for j := 0; j < int(width); j++ {
			binwriter.PutChar(img[i][j].b)
			binwriter.PutChar(img[i][j].g)
			binwriter.PutChar(img[i][j].r)
		}
		for range paddingSize {
			binwriter.PutChar(0)
		}
	}
	err = binwriter.Close()
	if err != nil {
		log.Panic(err.Error())
	}
}

func main() {
	img := ReadJPEG("pics/Aqours.jpg", true)
	encodeBMP(img, "pics/Aqours.bmp")
}
