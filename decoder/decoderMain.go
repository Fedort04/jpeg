package decoder

import (
	"fmt"
	binreader "jpeg/decoder/binReader"
	binwriter "jpeg/decoder/binWriter"
	"jpeg/decoder/huffman"
	"log"
	"path/filepath"
	"strings"
)

// Структура цветовой компоненты, данные для текущего скана
type component struct {
	h            byte
	v            byte
	quantTableID byte //ID таблицы квантования для этого цвета
	dcTableID    byte //DC таблица для этого цвета
	acTableID    byte //AC таблица для этого цвета
	used         bool //Флаг использования компоненты в текущем скане
}

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

const numOfTables = 4  //Максимальное количество таблиц
const maxComps = 3     //Максимальное количество компонент
const colCount = 8     //Количество столбцов в таблице квантования (для вывода в лог)
const sizeOfTable = 64 //Количество элементов в одной таблице квантования

var withDump bool = false                    //Флаг вывода информации заголовков в лог
var isProgressive bool                       //Флаг для прогрессивного декодирования
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
var startSpectral byte                       //Начало spectral selection для текущего скана
var endSpectral byte                         //Конец spectral selection для текущего скана
var approxH byte                             //Предыдущий бит для аппроксимации компоненты для текущего скана
var approxL byte                             //Текущий бит для аппроксимации компоненты для текущего скана

var img [][]rgb //Результирующее изображение

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
	reader.GetArray(ln - 2)

	if withDump {
		log.Print("APP")
		// log.Println("APP", string(temp))
	}
}

// Чтение таблицы квантования
func readQuantTable() {
	reader.GetWord()
	//До тех пор, пока следующий байт не будет маркером
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
	//Запись offset
	for i := 1; i < huffman.NumHuffCodesLen+1; i++ {
		sumElem += reader.GetByte()
		offset[i] = sumElem
	}
	symbols := make([]byte, sumElem)
	//Чтение символов
	for i := range sumElem {
		symbols[i] = reader.GetByte()
	}
	huff, err := huffman.MakeHuffTable(offset, symbols)
	if err != nil {
		log.Println("MakeHuffTable -> error")
		log.Fatal(err.Error())
	}
	switch tc {
	case 0:
		dcTables[th] = huff
	case 1:
		acTables[th] = huff
	default:
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
		huffman.PrintHuffTable(huff)
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

// Обновление флагов использования в скане для каждой компоненты
func updateFlags() {
	for i := range comps {
		comps[i].used = false
	}
}

// Чтение заголовка кадра
func readScanHeader() {
	reader.GetWord()
	ns := reader.GetByte()

	updateFlags()

	//Для каждой компоненты
	for range ns {
		cs := reader.GetByte()
		td, ta := reader.Get4Bit()
		comps[cs-1].dcTableID = td
		comps[cs-1].acTableID = ta
		comps[cs-1].used = true
	}
	startSpectral = reader.GetByte()
	endSpectral = reader.GetByte()
	approxH, approxL = reader.Get4Bit()

	if withDump {
		log.Print("SOS")
		for i, temp := range comps {
			if temp.used {
				log.Printf("component %d:\tDC: %d\tAC: %d\n", i+1, temp.dcTableID, temp.acTableID)
			}
		}
		log.Print("start Spectral Selection: ", startSpectral)
		log.Print("end Spectral Selection: ", endSpectral)
		log.Print("approximation high: ", approxH, "; approximation low: ", approxL)
	}
}

// Чтение заголовка фрейма
func readFrameHeader() {
	reader.GetWord()
	samplePrecision = reader.GetByte()
	imageHeight = reader.GetWord()
	imageWidth = reader.GetWord()
	numOfComps = reader.GetByte()
	//Для каждой компоненты
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
func readScans() {
	blocks := CreateMCUMatrix(NumOfMCUHeight, NumOfMCUWidth)
	if isProgressive { //Считает в цикле все сканы, а в конце проводит вычисления по функции и возвращает уже ргб
		for range 4 {
			nextMarker := readTables()
			if nextMarker == EOI {
				wasEOI = true
				break
			} else if nextMarker != SOS {
				log.Fatalf("readFrame can't read SOS\nMarker: %x", nextMarker)
			}

			// log.Printf("Scan %d!!!!!!!!!!!!!", i+1)
			readScanHeader()
			decodeProgScan(blocks)
			if reader.GetNextByte() != 0xFF {
				reader.BitsAlign()
			}
		}
		progressiveCalc(blocks)

	} else { //Для Baseline
		nextMarker := readTables()
		if nextMarker != SOS {
			log.Fatalf("readFrame can't read SOS\nMarker: %x", nextMarker)
		}
		readScanHeader()
		decodeBaselineScan(blocks)
		rgbCalc(blocks)
	}
}

// Чтение кадра
func readFrame() {
	nextMarker := readTables()
	switch nextMarker {
	case SOF0:
		isProgressive = false
	case SOF2:
		isProgressive = true
	default:
		log.Fatalf("readFrame can't read SOF0 or SOF2\nMarker: %x", nextMarker)
	}
	readFrameHeader()

	unitsInit()

	img = createRGBMatrix(imageHeight, imageWidth)

	readScans()

	if withDump {
		log.Print("Frame was readed")
	}
}

// Чтение JPEG файла по пути source
func ReadJPEG(source string, dump bool) [][]rgb {
	withDump = dump
	var err error
	reader, err = binreader.BinReaderInit(source, binreader.BIG)
	defer func() {
		err := reader.Close()
		if err != nil {
			log.Fatal(err.Error())
		}
	}()

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

	// if !wasEOI && !readMarker(EOI) {
	// 	log.Fatal("Can't read EOI marker")
	// }

	if withDump {
		log.Print("EOI")
	}

	return img
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

// Изменение строки названия расширения на .bmp
func jpegNameToBmp(name string) (string, error) {
	ext := filepath.Ext(name)
	lowerExt := strings.ToLower(ext)
	if lowerExt == ".jpg" || lowerExt == ".jpeg" {
		base := name[:len(name)-len(ext)]
		return base + ".bmp", nil
	}
	return "", fmt.Errorf("File is not jpeg")
}

func ReadBaseline(path string) {
	res, err := jpegNameToBmp(path)
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	img := ReadJPEG(path, true)
	log.Print("READ SUCCESS")
	encodeBMP(img, res)
	log.Print("BMP SUCCESS")
}

func ReadProgressive(path string) {
	res, err := jpegNameToBmp(path)
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	img := ReadJPEG(path, true)
	log.Print("READ SUCCESS")
	encodeBMP(img, res)
	log.Print("BMP SUCCESS")
}

// func main() {
// 	// img := ReadJPEG("pics/Baseline/Aqours.jpg", true)
// 	// img := ReadJPEG("pics/Progressive/AqoursProgressive.jpeg", true)
// 	img := ReadJPEG("pics/Progressive/EikyuuHours.jpeg", true)
// 	// img := ReadJPEG("pics/Progressive/EikyuuStage.jpeg", true)

// 	log.Print("READ SUCCESS")
// 	// encodeBMP(img, "pics/Baseline/Aqours.bmp")
// 	// encodeBMP(img, "pics/Progressive/AqoursProgressive.bmp")
// 	encodeBMP(img, "pics/Progressive/EikyuuHours.bmp")
// 	// encodeBMP(img, "pics/Progressive/EikyuuStage.bmp")
// 	log.Print("BMP SUCCESS")
// }
