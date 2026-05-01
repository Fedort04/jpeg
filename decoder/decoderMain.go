package decoder

import (
	"bufio"
	"errors"
	"fmt"
	bmpwriter "jpeg/decoder/bmpWriter"
	binreader "jpeg/internal/binReader"
	"jpeg/internal/huffman"
	"jpeg/internal/mcu"
	"jpeg/shared"
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

type Decoder struct {
	ImageHeight   uint16 //Высота изображения
	ImageWidth    uint16 //Ширина изображения
	IsProgressive bool   //Флаг для прогрессивного декодирования
	CurStatus     uint16 //Текущее состояние чтения

	reader             *binreader.BinReader                   //Объект для чтения файла
	blocks             [][]mcu.MCU                            //Текущие матрицы с коэф из ДКП
	quantTables        [shared.NumOfTables][]byte             //Массив с таблицами квантования
	acTables           [shared.NumOfTables]*huffman.HuffTable //Массив с AC таблицами Хаффмана
	dcTables           [shared.NumOfTables]*huffman.HuffTable //Массив с DC таблицами Хаффмана
	samplePrecision    byte                                   //Глубина цвета
	maxH               byte                                   //Максимальный Н фактор
	maxV               byte                                   //Максимальный V фактор
	numOfComps         byte                                   //Количество цветовых компонет в изображении
	comps              [shared.MaxComps]component             //Массив с данными о компонентах
	restartInterval    uint16                                 //Интервал перезапуска дельта кодирования
	startSpectral      byte                                   //Начало spectral selection для текущего скана
	endSpectral        byte                                   //Конец spectral selection для текущего скана
	saHigh             byte                                   //Предыдущий бит для аппроксимации компоненты для текущего скана
	saLow              byte                                   //Текущий бит для аппроксимации компоненты для текущего скана
	numOfMCUHeight     uint16                                 //Количество MCU в изображении по высоте
	numOfMCUWidth      uint16                                 //Количество MCU в изображении по ширине
	numOfMCUHeightReal uint16                                 //Реальное количество MCU в изображении по высоте
	numOfMCUWidthReal  uint16                                 //Реальное количество MCU в изображении по ширине
	numBlocksHeight    uint16                                 //Количество блоков subsample по высоте
	numBlocksWidth     uint16                                 //Количество блоков subsample по ширине
	blockCount         uint                                   //Общее количество прочитанных блоков mcu
	wasEOI             bool                                   //Флаг завершения чтения
	img                shared.Image                           //Результирующее изображение
}

// Чтение маркера marker
func (jpeg *Decoder) readMarker(marker uint16) bool {
	if temp := jpeg.reader.GetWord(); temp != marker {
		return false
	}
	return true
}

// Чтение сегмента приложения
func (jpeg *Decoder) readApp() {
	ln := jpeg.reader.GetWord()
	jpeg.reader.GetArray(ln - 2)
}

// Чтение таблицы квантования
func (jpeg *Decoder) readQuantTable() error {
	jpeg.reader.GetWord()
	//До тех пор, пока следующий байт не будет маркером
	tq := jpeg.reader.GetByte()

	if tq > shared.NumOfTables-1 {
		return errors.New("Segment reading error: Quant table invalid table destination")
	}

	table := jpeg.reader.GetArray(shared.SizeOfTable)
	jpeg.quantTables[tq] = table
	return nil
}

// Чтение сегмента с перезапуском дельта-кодирования
func (jpeg *Decoder) readRestartInterval() {
	jpeg.reader.GetWord()
	jpeg.restartInterval = jpeg.reader.GetWord()
}

// Чтение сегмента таблиц, возвращает следующие за сегментами 2 байта
func (jpeg *Decoder) readTables() (uint16, error) {
	marker := jpeg.reader.GetWord()
	isContinue := false
	if marker >= shared.APP0 && marker <= shared.APP15 {
		jpeg.readApp()
		isContinue = true
	} else if marker == shared.DQT {
		if err := jpeg.readQuantTable(); err != nil {
			return 0, err
		}
		isContinue = true
	} else if marker == shared.DHT {
		tc, th, huff, err := huffman.ReadHuffTable(jpeg.reader)
		if err != nil {
			return 0, err
		}
		if th > shared.NumOfTables-1 {
			return 0, errors.New("Segment reading error: Huffman table invalid table destination")
		}
		switch tc {
		case 0:
			jpeg.dcTables[th] = huff
		case 1:
			jpeg.acTables[th] = huff
		default:
			return 0, errors.New("Segment reading error: Huffman table invalid table ID")
		}

		isContinue = true
	} else if marker == shared.DRI {
		jpeg.readRestartInterval()
		isContinue = true
	}
	if isContinue {
		var err error
		marker, err = jpeg.readTables()
		if err != nil {
			return marker, nil
		}
	}
	return marker, nil
}

// Обновление флагов использования в скане для каждой компоненты
func (jpeg *Decoder) updateFlags() {
	for i := range jpeg.comps {
		jpeg.comps[i].used = false
	}
}

// Чтение заголовка кадра
func (jpeg *Decoder) readScanHeader() error {
	jpeg.reader.GetWord()
	ns := jpeg.reader.GetByte()

	jpeg.updateFlags()

	//Для каждой компоненты
	for range ns {
		cs := jpeg.reader.GetByte()
		if cs > shared.NumOfChannels {
			return errors.New("Segment reading error: too much color channels")
		}

		td, ta := jpeg.reader.Get4Bit()

		if td > shared.NumOfTables || ta > shared.NumOfTables {
			return errors.New("Segment reading error: invalid huff-table channel ID")
		}

		jpeg.comps[cs-1].dcTableID = td
		jpeg.comps[cs-1].acTableID = ta
		jpeg.comps[cs-1].used = true
	}
	jpeg.startSpectral = jpeg.reader.GetByte()
	jpeg.endSpectral = jpeg.reader.GetByte()
	if jpeg.startSpectral > jpeg.endSpectral || jpeg.endSpectral > 63 {
		return fmt.Errorf("Segment reading error: spectralSelection params error: start: %d\tend: %d", jpeg.startSpectral, jpeg.endSpectral)
	}
	jpeg.saHigh, jpeg.saLow = jpeg.reader.Get4Bit()
	return nil
}

// Чтение заголовка фрейма
func (jpeg *Decoder) readFrameHeader() error {
	jpeg.reader.GetWord()
	jpeg.samplePrecision = jpeg.reader.GetByte()

	if jpeg.samplePrecision != 8 && jpeg.samplePrecision != 16 {
		return errors.New("Segment reading error: invalid segment precision")
	}

	jpeg.ImageHeight = jpeg.reader.GetWord()
	jpeg.ImageWidth = jpeg.reader.GetWord()
	jpeg.numOfComps = jpeg.reader.GetByte()

	if jpeg.numOfComps > shared.NumOfChannels {
		return errors.New("Segment reading error: too much color channels")
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
	return nil
}

// Чтение скана, iterCount - кол-во строк/сканов для текущего вычисления
func (jpeg *Decoder) readScans(iterCount uint16) error {
	var curRow uint16
	readAll := iterCount == 0
	startStatus := int(jpeg.CurStatus)

	if jpeg.IsProgressive {
		temp := jpeg.CurStatus
		for jpeg.CurStatus < temp+iterCount || readAll {
			nextMarker, err := jpeg.readTables()
			if err != nil {
				return err
			}
			if nextMarker == shared.EOI {
				jpeg.wasEOI = true
				break
			} else if nextMarker != shared.SOS {
				return errors.New("Scan reading error")
			}
			if err = jpeg.readScanHeader(); err != nil {
				return err
			}
			if err = jpeg.decodeProgressiveScan(); err != nil {
				return err
			}

			if jpeg.reader.GetNextByte() != 0xFF {
				jpeg.reader.BitsAlign()
			}
			jpeg.CurStatus++
		}
	} else if !jpeg.wasEOI { //Для Baseline
		if jpeg.CurStatus == 0 {
			nextMarker, err := jpeg.readTables()
			if err != nil {
				return err
			}
			if nextMarker != shared.SOS {
				return errors.New("Scan reading error")
			}
			if err = jpeg.readScanHeader(); err != nil {
				return err
			}
			jpeg.decodeInit()
		}
		var err error
		if curRow, err = jpeg.decodeBaselineScan(iterCount); err != nil {
			return err
		}
	}
	jpeg.rgbCalc(readAll, startStatus, int(curRow))
	return nil
}

// Чтение заголовка файла до заголовка фрейма включительно
func (jpeg *Decoder) readFileHeader() error {
	nextMarker, err := jpeg.readTables()
	if err != nil {
		return err
	}
	switch nextMarker {
	case shared.SOF0:
		jpeg.IsProgressive = false
	case shared.SOF2:
		jpeg.IsProgressive = true
	default:
		return errors.New("Decoder works only with Baseline and Progressive DCT-based JPEG")
	}
	if err = jpeg.readFrameHeader(); err != nil {
		return err
	}
	return nil
}

// Чтение изображения на кол-во строк numOfRows
// Возвращает true, если прочитано до конца
func (jpeg *Decoder) ReadBaseJPEG(result shared.Image, numOfRows uint16) (bool, error) {
	if jpeg.CurStatus == 0 {
		jpeg.constInit()
	}

	if len(result) != int(jpeg.ImageHeight) || len(result[0]) != int(jpeg.ImageWidth) {
		return false, errors.New("Buffer size error")
	}
	jpeg.img = result

	if err := jpeg.readScans(numOfRows); err != nil {
		return jpeg.wasEOI, err
	}
	return jpeg.wasEOI, nil
}

// Чтение изображения на кол-во сканов numOfScans
// Возвращает true, если прочитано до конца
func (jpeg *Decoder) ReadProgJPEG(result shared.Image, numOfScans uint16) (bool, error) {
	if jpeg.CurStatus == 0 {
		jpeg.constInit()
	}

	if len(result) != int(jpeg.ImageHeight) || len(result[0]) != int(jpeg.ImageWidth) {
		return false, errors.New("Buffer size error")
	}
	jpeg.img = result

	if err := jpeg.readScans(numOfScans); err != nil {
		return jpeg.wasEOI, err
	}
	return jpeg.wasEOI, nil
}

// Чтение JPEG файла по пути source
func ReadJPEG(source *bufio.Reader) (*Decoder, error) {
	var res Decoder
	res.reader = binreader.BinReaderInit(source)

	if !res.readMarker(shared.SOI) {
		return nil, errors.New("Image is not JPEG: can't read SOI marker")
	}

	if err := res.readFileHeader(); err != nil {
		return nil, err
	}

	return &res, nil
}

// =======================================
// Кодирование в BMP для наглядности
func EncodeBMP(img shared.Image, fileName string) {
	err := bmpwriter.BinwriterInit(fileName)
	if err != nil {
		log.Panic(err.Error())
	}
	height := len(img)
	width := len(img[0])
	paddingSize := width % 4
	size := 14 + 12 + height*width*3 + paddingSize*height
	bmpwriter.PutChar('B')
	bmpwriter.PutChar('M')
	bmpwriter.PutInt(uint(size))
	bmpwriter.PutInt(0)
	bmpwriter.PutInt(0x1A)
	bmpwriter.PutInt(12)
	bmpwriter.PutShort(uint(width))
	bmpwriter.PutShort(uint(height))
	bmpwriter.PutShort(1)
	bmpwriter.PutShort(24)

	for i := int(height - 1); i >= 0; i-- {
		for j := 0; j < int(width); j++ {
			bmpwriter.PutChar(img[i][j].B)
			bmpwriter.PutChar(img[i][j].G)
			bmpwriter.PutChar(img[i][j].R)
		}
		for range paddingSize {
			bmpwriter.PutChar(0)
		}
	}
	err = bmpwriter.Close()
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
