package main

import (
	binreader "decoder/binReader"
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
)

const numOfTables = 4  //Максимальное количество таблиц
const maxComps = 3     //Максимальное количество компонент
const colCount = 8     //Количество столбцов
const sizeOfTable = 64 //Количество элементов в одной таблице

var withDump bool = false           //Флаг вывода информации заголовков в лог
var reader *binreader.BinReader     //Объект для чтения файла
var quantTables [numOfTables][]byte //Массив с таблицами квантования
var samplePrecision byte            //Глубина цвета
var imageWidth uint16               //Ширина изображения
var imageHeight uint16              //Высота изображения
var maxH byte                       //Максимальный Н фактор
var maxV byte                       //Максимальный V фактор
var numOfComps byte                 //Количество цветовых компонет в изображении
var comps [maxComps]component       //Массив с данными о компонентах

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
		if tq > 3 {
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

// Чтение сегмента таблиц
func readTables() uint16 {
	marker := reader.GetWord()
	isContinue := false
	if marker >= APP0 && marker <= APP15 {
		readApp()
		isContinue = true
	} else if marker == DQT {
		readQuantTable()
		isContinue = true
	}
	if isContinue {
		marker = readTables()
	}
	return marker
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

// Чтение кадра
func readFrame() {
	nextMarker := readTables()
	if nextMarker != SOF0 {
		log.Fatalf("readFrame can't read SOF0\nMarker: %x", nextMarker)
	}
	readFrameHeader()
	// img := readScan()
}

// Чтение JPEG файла по пути source
func ReadJPEG(source string, dump bool) {
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

	readFrame()

	if !readMarker(EOI) {
		log.Fatal("Can't read EOI marker")
	}
}

func main() {
	ReadJPEG("pics/Aqours.jpg", true)
}
