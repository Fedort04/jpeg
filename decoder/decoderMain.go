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

type JPEG struct {
	ImageHeight   uint16 //Высота изображения
	ImageWidth    uint16 //Ширина изображения
	IsProgressive bool   //Флаг для прогрессивного декодирования
	CurStatus     uint16 //Текущее состояние чтения

	reader          *binreader.BinReader            //Объект для чтения файла
	quantTables     [numOfTables][]byte             //Массив с таблицами квантования
	acTables        [numOfTables]*huffman.HuffTable //Массив с AC таблицами Хаффмана
	dcTables        [numOfTables]*huffman.HuffTable //Массив с DC таблицами Хаффмана
	samplePrecision byte                            //Глубина цвета
	maxH            byte                            //Максимальный Н фактор
	maxV            byte                            //Максимальный V фактор
	numOfComps      byte                            //Количество цветовых компонет в изображении
	comps           [maxComps]component             //Массив с данными о компонентах
	restartInterval uint16                          //Интервал перезапуска дельта кодирования
	startSpectral   byte                            //Начало spectral selection для текущего скана
	endSpectral     byte                            //Конец spectral selection для текущего скана
	saHigh          byte                            //Предыдущий бит для аппроксимации компоненты для текущего скана
	saLow           byte                            //Текущий бит для аппроксимации компоненты для текущего скана
	numOfMCUHeight  uint16                          //Количество MCU в изображении по высоте
	numOfMCUWidth   uint16                          //Количество MCU в изображении по ширине
	img             Image                           //Результирующее изображение
}

// Чтение маркера marker
func (jpeg *JPEG) readMarker(marker uint16) bool {
	if temp := jpeg.reader.GetWord(); temp != marker {
		return false
	}
	return true
}

// Чтение сегмента приложения
func (jpeg *JPEG) readApp() {
	ln := jpeg.reader.GetWord()
	jpeg.reader.GetArray(ln - 2)
}

// Чтение таблицы квантования
func (jpeg *JPEG) readQuantTable() {
	jpeg.reader.GetWord()
	//До тех пор, пока следующий байт не будет маркером
	tq := jpeg.reader.GetByte()
	if tq > numOfTables-1 {
		log.Fatal("readQuantTable -> invalid table destination", tq)
	}
	table := jpeg.reader.GetArray(sizeOfTable)
	jpeg.quantTables[tq] = table
}

// Чтение сегмента с перезапуском дельта-кодирования
func (jpeg *JPEG) readRestartInterval() {
	jpeg.reader.GetWord()
	jpeg.restartInterval = jpeg.reader.GetWord()
}

// Чтение сегмента таблиц, возвращает следующие за сегментами 2 байта
func (jpeg *JPEG) readTables() uint16 {
	marker := jpeg.reader.GetWord()
	isContinue := false
	if marker >= APP0 && marker <= APP15 {
		jpeg.readApp()
		isContinue = true
	} else if marker == DQT {
		jpeg.readQuantTable()
		isContinue = true
	} else if marker == DHT {
		tc, th, huff, err := huffman.ReadHuffTable(jpeg.reader)

		if err != nil {
			log.Println("MakeHuffTable -> error")
			log.Fatal(err.Error())
		}
		if th > numOfTables-1 {
			log.Fatal("readHuffTable -> invalid table destination", th)
		}
		switch tc {
		case 0:
			jpeg.dcTables[th] = huff
		case 1:
			jpeg.acTables[th] = huff
		default:
			log.Fatal("readHuffTable -> invalid table ID")
		}

		isContinue = true
	} else if marker == DRI {
		jpeg.readRestartInterval()
		isContinue = true
	}
	if isContinue {
		marker = jpeg.readTables()
	}
	return marker
}

// @todo сделать чистой функцией вне объекта jpeg
// Обновление флагов использования в скане для каждой компоненты
func (jpeg *JPEG) updateFlags() {
	for i := range jpeg.comps {
		jpeg.comps[i].used = false
	}
}

// Чтение заголовка кадра
func (jpeg *JPEG) readScanHeader() {
	jpeg.reader.GetWord()
	ns := jpeg.reader.GetByte()

	jpeg.updateFlags()

	//Для каждой компоненты
	for range ns {
		cs := jpeg.reader.GetByte()
		td, ta := jpeg.reader.Get4Bit()
		jpeg.comps[cs-1].dcTableID = td
		jpeg.comps[cs-1].acTableID = ta
		jpeg.comps[cs-1].used = true
	}
	jpeg.startSpectral = jpeg.reader.GetByte()
	jpeg.endSpectral = jpeg.reader.GetByte()
	if jpeg.startSpectral > jpeg.endSpectral || jpeg.endSpectral > 63 {
		log.Printf("spectralSelection params error: start: %d\tend: %d", jpeg.startSpectral, jpeg.endSpectral)
		return
	}
	jpeg.saHigh, jpeg.saLow = jpeg.reader.Get4Bit()
}

// Чтение заголовка фрейма
func (jpeg *JPEG) readFrameHeader() {
	jpeg.reader.GetWord()
	jpeg.samplePrecision = jpeg.reader.GetByte()
	jpeg.ImageHeight = jpeg.reader.GetWord()
	jpeg.ImageWidth = jpeg.reader.GetWord()
	jpeg.numOfComps = jpeg.reader.GetByte()
	//Для каждой компоненты
	for range jpeg.numOfComps {
		c := jpeg.reader.GetByte()
		h, v := jpeg.reader.Get4Bit()
		if h > jpeg.maxH {
			jpeg.maxH = h
		}
		if v > jpeg.maxV {
			jpeg.maxV = v
		}
		tq := jpeg.reader.GetByte()
		jpeg.comps[c-1] = component{h: h, v: v, quantTableID: tq}
	}
}

// Чтение скана
func (jpeg *JPEG) readScans() {
	blocks := CreateMCUMatrix(jpeg.numOfMCUHeight, jpeg.numOfMCUWidth)
	if jpeg.IsProgressive { //Считает в цикле все сканы, а в конце проводит вычисления по функции и возвращает уже ргб
		for {
			nextMarker := jpeg.readTables()
			if nextMarker == EOI {
				break
			} else if nextMarker != SOS {
				log.Fatalf("readFrame can't read SOS\nMarker: %x", nextMarker)
			}
			jpeg.readScanHeader()
			jpeg.decodeProgressiveScan(blocks)
			if jpeg.reader.GetNextByte() != 0xFF {
				jpeg.reader.BitsAlign()
			}
		}
	} else { //Для Baseline
		nextMarker := jpeg.readTables()
		if nextMarker != SOS {
			log.Fatalf("readFrame can't read SOS\nMarker: %x", nextMarker)
		}
		jpeg.readScanHeader()
		jpeg.decodeBaselineScan(blocks)
	}
	jpeg.rgbCalc(blocks)
}

// Чтение кадра
func (jpeg *JPEG) readFrame() {
	nextMarker := jpeg.readTables()
	switch nextMarker {
	case SOF0:
		jpeg.IsProgressive = false
	case SOF2:
		jpeg.IsProgressive = true
	default:
		log.Fatalf("readFrame can't read SOF0 or SOF2\nMarker: %x", nextMarker)
	}
	jpeg.readFrameHeader()

	jpeg.unitsInit()

	jpeg.img = createRGBMatrix(jpeg.ImageHeight, jpeg.ImageWidth)

	jpeg.readScans()
}

// Кодирование в BMP для наглядности
func encodeBMP(img [][]Rgb, fileName string, ImageHeight uint16, ImageWidth uint16) {
	err := binwriter.BinwriterInit(fileName)
	if err != nil {
		log.Panic(err.Error())
	}
	height := ImageHeight
	width := ImageWidth
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
			binwriter.PutChar(img[i][j].B)
			binwriter.PutChar(img[i][j].G)
			binwriter.PutChar(img[i][j].R)
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
	var jpeg JPEG
	res, err := jpegNameToBmp(path)
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	img := jpeg.ReadJPEG(path, false)
	log.Print("READ SUCCESS")
	encodeBMP(img, res, jpeg.ImageHeight, jpeg.ImageWidth)
	log.Print("BMP SUCCESS")
}

func ReadProgressive(path string) {
	var jpeg JPEG
	res, err := jpegNameToBmp(path)
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	img := jpeg.ReadJPEG(path, false)
	log.Print("READ SUCCESS")
	encodeBMP(img, res, jpeg.ImageHeight, jpeg.ImageWidth)
	log.Print("BMP SUCCESS")
}

func (jpeg *JPEG) ReadJPEG(source string, dump bool) [][]Rgb {
	jpeg.reader, _ = binreader.BinReaderInit(source, binreader.BIG)
	defer func() {
		err := jpeg.reader.Close()
		if err != nil {
			log.Fatal(err.Error())
		}
	}()

	if !jpeg.readMarker(SOI) {
		log.Fatal("Can't read SOI marker")
	}

	jpeg.readFrame()

	return jpeg.img
}

// Чтение JPEG файла по пути source
// Здесь читается хедер до первого скана и исходя из этого заполняется структура
// func ReadJPEG(source *bufio.Reader) (*JPEG, error) {
// 	var res JPEG
// 	res.reader = binreader.BinReaderInit(source)
// 	if !res.readMarker(SOI) {
// 		return nil, errors.New("Image is not JPEG: can't read SOI marker")
// 	}
// 	return &res, nil
// }
