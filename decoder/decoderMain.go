package decoder

import (
	"bufio"
	"errors"
	"fmt"
	binreader "jpeg/decoder/binReader"
	binwriter "jpeg/decoder/binWriter"
	"jpeg/decoder/huffman"
	"log"
	"path/filepath"
	"strconv"
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

const numOfTables = 4   //Максимальное количество таблиц
const numOfChannels = 3 //Максимальное количество цветовых компонент
const maxComps = 3      //Максимальное количество компонент
const colCount = 8      //Количество столбцов в таблице квантования (для вывода в лог)
const sizeOfTable = 64  //Количество элементов в одной таблице квантования

type JPEG struct {
	ImageHeight   uint16 //Высота изображения
	ImageWidth    uint16 //Ширина изображения
	IsProgressive bool   //Флаг для прогрессивного декодирования
	CurStatus     uint16 //Текущее состояние чтения

	reader          *binreader.BinReader            //Объект для чтения файла
	blocks          [][]MCU                         // Текущие матрицы с коэф из ДКП
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
	numBlocksHeight uint16                          //Количество блоков subsample по высоте
	numBlocksWidth  uint16                          //Количество блоков subsample по ширине
	blockCount      uint                            //Общее количество прочитанных блоков mcu
	wasEOI          bool                            //Флаг завершения чтения
	readError       error                           //Ошибка при декодировании
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
		jpeg.readError = errors.New("Segment reading error: Quant table invalid table destination")
		return
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
		jpeg.readError = err
		if th > numOfTables-1 {
			jpeg.readError = errors.New("Segment reading error: Huffman table invalid table destination")
			return 0
		}
		switch tc {
		case 0:
			jpeg.dcTables[th] = huff
		case 1:
			jpeg.acTables[th] = huff
		default:
			jpeg.readError = errors.New("Segment reading error: Huffman table invalid table ID")
			return 0
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
		if cs > numOfChannels {
			jpeg.readError = errors.New("Segment reading error: too much color channels")
			return
		}

		td, ta := jpeg.reader.Get4Bit()

		if td > numOfTables || ta > numOfTables {
			jpeg.readError = errors.New("Segment reading error: invalid huff-table channel ID")
			return
		}

		jpeg.comps[cs-1].dcTableID = td
		jpeg.comps[cs-1].acTableID = ta
		jpeg.comps[cs-1].used = true
	}
	jpeg.startSpectral = jpeg.reader.GetByte()
	jpeg.endSpectral = jpeg.reader.GetByte()
	if jpeg.startSpectral > jpeg.endSpectral || jpeg.endSpectral > 63 {
		jpeg.readError = fmt.Errorf("Segment reading error: spectralSelection params error: start: %d\tend: %d", jpeg.startSpectral, jpeg.endSpectral)
		return
	}
	jpeg.saHigh, jpeg.saLow = jpeg.reader.Get4Bit()
}

// Чтение заголовка фрейма
func (jpeg *JPEG) readFrameHeader() {
	jpeg.reader.GetWord()
	jpeg.samplePrecision = jpeg.reader.GetByte()

	if jpeg.samplePrecision != 8 && jpeg.samplePrecision != 16 {
		jpeg.readError = errors.New("Segment reading error: invalid segment precision")
	}

	jpeg.ImageHeight = jpeg.reader.GetWord()
	jpeg.ImageWidth = jpeg.reader.GetWord()
	jpeg.numOfComps = jpeg.reader.GetByte()

	if jpeg.numOfComps > numOfChannels {
		jpeg.readError = errors.New("Segment reading error: too much color channels")
		return
	}

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

// Чтение скана, iterCount - кол-во строк/сканов для текущего вычисления
func (jpeg *JPEG) readScans(iterCount uint16) bool {
	var curRow uint16
	var flag bool
	readAll := iterCount == 0
	startStatus := int(jpeg.CurStatus)

	if jpeg.IsProgressive {
		temp := jpeg.CurStatus
		for jpeg.CurStatus < temp+iterCount || readAll {
			nextMarker := jpeg.readTables()
			if nextMarker == EOI {
				jpeg.wasEOI = true
				break
			} else if nextMarker != SOS {
				jpeg.readError = errors.New("Scan reading error")
				return false
			}
			jpeg.readScanHeader()
			if !jpeg.decodeProgressiveScan(jpeg.blocks) {
				return false
			}

			if jpeg.reader.GetNextByte() != 0xFF {
				jpeg.reader.BitsAlign()
			}
			jpeg.CurStatus++
		}
	} else if !jpeg.wasEOI { //Для Baseline
		if jpeg.CurStatus == 0 {
			nextMarker := jpeg.readTables()
			if nextMarker != SOS {
				jpeg.readError = errors.New("Scan reading error")
				return false
			}
			jpeg.readScanHeader()
			jpeg.decodeInit()
		}
		jpeg.CurStatus, curRow, flag = jpeg.decodeBaselineScan(jpeg.blocks, iterCount)
		if !flag {
			return false
		}
	}
	jpeg.rgbCalc(jpeg.blocks, readAll, startStatus, int(curRow))
	return true
}

// Чтение заголовка файла до заголовка фрейма включительно
func (jpeg *JPEG) readFileHeader() {
	nextMarker := jpeg.readTables()
	switch nextMarker {
	case SOF0:
		jpeg.IsProgressive = false
	case SOF2:
		jpeg.IsProgressive = true
	default:
		jpeg.readError = errors.New("Decoder works only with Baseline and Progressive DCT-based JPEG")
		return
	}
	jpeg.readFrameHeader()
}

// Чтение изображения на кол-во строк numOfRows
// Возвращает true, если прочитано до конца
func (jpeg *JPEG) ReadBaseJPEG(result Image, numOfRows uint16) (bool, error) {
	if jpeg.CurStatus == 0 {
		jpeg.constInit()
	}

	if len(result) != int(jpeg.ImageHeight) || len(result[0]) != int(jpeg.ImageWidth) {
		return false, errors.New("Buffer size error")
	}
	jpeg.img = result

	if !jpeg.readScans(numOfRows) {
		return jpeg.wasEOI, jpeg.readError
	}
	return jpeg.wasEOI, nil
}

// Чтение изображения на кол-во сканов numOfScans
// Возвращает true, если прочитано до конца
func (jpeg *JPEG) ReadProgJPEG(result Image, numOfScans uint16) (bool, error) {
	if jpeg.CurStatus == 0 {
		jpeg.constInit()
	}

	if len(result) != int(jpeg.ImageHeight) || len(result[0]) != int(jpeg.ImageWidth) {
		return false, errors.New("Buffer size error")
	}
	jpeg.img = result

	if !jpeg.readScans(numOfScans) {
		return jpeg.wasEOI, jpeg.readError
	}
	return jpeg.wasEOI, nil
}

// Чтение JPEG файла по пути source
func ReadJPEG(source *bufio.Reader) (*JPEG, error) {
	var res JPEG
	res.reader = binreader.BinReaderInit(source)

	if !res.readMarker(SOI) {
		return nil, errors.New("Image is not JPEG: can't read SOI marker")
	}

	res.readFileHeader()

	if res.readError != nil {
		return nil, res.readError
	}

	return &res, nil
}

// =======================================
// Кодирование в BMP для наглядности
func EncodeBMP(img Image, fileName string) {
	err := binwriter.BinwriterInit(fileName)
	if err != nil {
		log.Panic(err.Error())
	}
	height := len(img)
	width := len(img[0])
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
func JpegNameToBmp(name string, counter int) (string, error) {
	ext := filepath.Ext(name)
	lowerExt := strings.ToLower(ext)
	if lowerExt == ".jpg" || lowerExt == ".jpeg" {
		base := name[:len(name)-len(ext)]
		if counter != 0 {
			num := strconv.Itoa(counter)
			return base + num + ".bmp", nil
		}
		return base + ".bmp", nil
	}
	return "", fmt.Errorf("File is not jpeg")
}
