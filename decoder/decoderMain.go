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

// Decoder - объект декодировщика.
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
	bandSkips          uint16                                 //Счетчик пропусков вычислений в progressive
	prev               []int16                                //Предыдущие значения DC для дельта кодирования
	img                shared.Image                           //Результирующее изображение
}

const readErrorMsg = "Can't read segment "
const minPrecision = 8
const maxPrecision = 16
const maxSubsample = 2 //По JFIF

// Чтение сегмента приложения
func (jpeg *Decoder) readApp() error {
	ln, err := jpeg.reader.GetWord()
	if err != nil {
		return errors.New(readErrorMsg + "APP\n" + err.Error())
	}
	if _, err = jpeg.reader.GetArray(ln - 2); err != nil {
		return errors.New(readErrorMsg + "APP\n" + err.Error())
	}
	return nil
}

// Чтение таблицы квантования
func (jpeg *Decoder) readQuantTable() error {
	sr := &binreader.StickyReader{Reader: jpeg.reader}
	sr.GetWord()
	tq := sr.GetByte()
	if sr.Err != nil {
		return errors.New(readErrorMsg + "DQT\n" + sr.Err.Error())
	}

	if tq > shared.NumOfTables-1 {
		return fmt.Errorf("DQT segment decode error: invalid table id %d", tq)
	}

	table, err := jpeg.reader.GetArray(shared.SizeOfTable)
	if err != nil {
		return errors.New(readErrorMsg + "DQT\n" + err.Error())
	}
	jpeg.quantTables[tq] = table
	return nil
}

// Чтение сегмента с перезапуском дельта-кодирования
func (jpeg *Decoder) readRestartInterval() error {
	sr := &binreader.StickyReader{Reader: jpeg.reader}
	sr.GetWord()
	jpeg.restartInterval = sr.GetWord()
	if sr.Err != nil {
		return errors.New(readErrorMsg + "DRI\n" + sr.Err.Error())
	}

	return nil
}

// Чтение сегмента таблиц, возвращает следующие за сегментами 2 байта
func (jpeg *Decoder) readTables() (uint16, error) {
	marker, _ := jpeg.reader.GetWord()
	isContinue := false
	if marker >= shared.APP0 && marker <= shared.APP15 {
		if err := jpeg.readApp(); err != nil {
			return 0, err
		}
		isContinue = true
	} else if marker == shared.DQT {
		if err := jpeg.readQuantTable(); err != nil {
			return 0, err
		}
		isContinue = true
	} else if marker == shared.DHT {
		const prefix = "DHT segment decode error: "
		tc, th, huff, err := huffman.ReadHuffTable(jpeg.reader)
		if err != nil {
			return 0, errors.New(prefix + "Huffman table recovery failed\n" + err.Error())
		}
		if th > shared.NumOfTables-1 {
			return 0, fmt.Errorf("%sinvalid table id %d", prefix, th)
		}
		switch tc {
		case 0:
			jpeg.dcTables[th] = huff
		case 1:
			jpeg.acTables[th] = huff
		default:
			return 0, fmt.Errorf("%sinvalid table class %d", prefix, tc)
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
	const prefix = "SOS segment decode error: invalid "
	sr := &binreader.StickyReader{Reader: jpeg.reader}
	sr.GetWord()
	ns := sr.GetByte()
	if sr.Err != nil {
		return errors.New(readErrorMsg + "SOS\n" + sr.Err.Error())
	}
	if ns > shared.MaxComps {
		return fmt.Errorf("%sheader param Num_Of_Channels %d", prefix, ns)
	}

	jpeg.updateFlags()
	//Для каждой компоненты
	for range ns {
		cs := sr.GetByte()
		td, ta := sr.Get4Bit()
		if sr.Err != nil {
			return errors.New(readErrorMsg + "SOS\n" + sr.Err.Error())
		}

		if cs > shared.MaxComps {
			return fmt.Errorf("%scomponent param Channel_selector %d", prefix, cs)
		}

		if td > shared.NumOfTables {
			return fmt.Errorf("%scomponent param DC_Hufftable_ID %d", prefix, td)
		}
		if ta > shared.NumOfTables {
			return fmt.Errorf("%scomponent param AC_Hufftable_ID %d", prefix, ta)
		}

		jpeg.comps[cs-1].dcTableID = td
		jpeg.comps[cs-1].acTableID = ta
		jpeg.comps[cs-1].used = true
	}

	jpeg.startSpectral = sr.GetByte()
	jpeg.endSpectral = sr.GetByte()
	jpeg.saHigh, jpeg.saLow = sr.Get4Bit()
	if sr.Err != nil {
		return errors.New(readErrorMsg + "SOS\n" + sr.Err.Error())
	}

	if jpeg.IsProgressive {
		if (jpeg.startSpectral == 0 && jpeg.endSpectral != 0) || (jpeg.startSpectral > jpeg.endSpectral) {
			return fmt.Errorf("%sheader param SS %d, SE %d ", prefix, jpeg.startSpectral, jpeg.endSpectral)
		}
	} else { //Baseline
		if jpeg.startSpectral != shared.BaselineSS || jpeg.endSpectral != shared.BaselineSE {
			return fmt.Errorf("%sheader param SS %d, SE %d ", prefix, jpeg.startSpectral, jpeg.endSpectral)
		}
		if jpeg.saHigh != shared.BaselineAh || jpeg.saLow != shared.BaselineAl {
			return fmt.Errorf("%sheader param AH %d, AL %d ", prefix, jpeg.saHigh, jpeg.saLow)
		}
	}

	return nil
}

// Чтение заголовка фрейма
func (jpeg *Decoder) readFrameHeader() error {
	const prefix = "SOF segment decode error: invalid "
	sr := &binreader.StickyReader{Reader: jpeg.reader}
	sr.GetWord()
	jpeg.samplePrecision = sr.GetByte()
	if sr.Err != nil {
		return errors.New(readErrorMsg + "SOF\n" + sr.Err.Error())
	}

	if jpeg.samplePrecision != minPrecision && jpeg.samplePrecision != maxPrecision {
		return fmt.Errorf("%sprecision %d", prefix, jpeg.samplePrecision)
	}

	jpeg.ImageHeight = sr.GetWord()
	jpeg.ImageWidth = sr.GetWord()
	jpeg.numOfComps = sr.GetByte()
	if sr.Err != nil {
		return errors.New(readErrorMsg + "SOF\n" + sr.Err.Error())
	}

	if jpeg.numOfComps > shared.MaxComps {
		return fmt.Errorf("%snum of channels %d", prefix, jpeg.numOfComps)
	}

	for range jpeg.numOfComps {
		c := sr.GetByte()
		h, v := sr.Get4Bit()
		tq := sr.GetByte()
		if sr.Err != nil {
			return errors.New(readErrorMsg + "SOF\n" + sr.Err.Error())
		}

		if h > maxSubsample || v > maxSubsample {
			return errors.New(prefix + "component param Subsample_factor")
		}
		if tq > shared.NumOfTables {
			return errors.New(prefix + "component param Quant_table_id")
		}

		if h > jpeg.maxH {
			jpeg.maxH = h
		}
		if v > jpeg.maxV {
			jpeg.maxV = v
		}

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
				return errors.New(readErrorMsg + "marker")
			}
			if err = jpeg.readScanHeader(); err != nil {
				return err
			}
			if err = jpeg.decodeProgressiveScan(); err != nil {
				return err
			}

			nextByte, err := jpeg.reader.GetNextByte()
			if err != nil {
				return errors.New(readErrorMsg + "Scan data\n" + err.Error())
			}

			if nextByte != 0xFF {
				if err := jpeg.reader.BitsAlign(); err != nil {
					return errors.New(readErrorMsg + "Scan data\n" + err.Error())
				}
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
				return errors.New(readErrorMsg + "marker")
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
		return fmt.Errorf("SOF segment decode error: invalid marker (not %d or %d)\n", shared.SOF0, shared.SOF2)
	}
	if err = jpeg.readFrameHeader(); err != nil {
		return err
	}
	return nil
}

// SetBuffer устанавливает буфер, в который записывается результат декодирования.
func (jpeg *Decoder) SetBuffer(res shared.Image) error {
	if len(res) != int(jpeg.ImageHeight) || len(res[0]) != int(jpeg.ImageWidth) {
		return errors.New("Invalid buffer size")
	}
	jpeg.img = res
	return nil
}

// ReadBaseJPEG проводит декодирование Baseline изображения.
// numOfRows задает количество строк, которые необходимо декодировать (0 - декодировать всё).
// При применении к Progressive изображению возвращает ошибку.
func (jpeg *Decoder) ReadBaseJPEG(numOfRows uint16) (bool, error) {
	if jpeg.CurStatus == 0 {
		jpeg.constInit()
	}

	if err := jpeg.readScans(numOfRows); err != nil {
		return jpeg.wasEOI, err
	}
	return jpeg.wasEOI, nil
}

// ReadProgJPEG проводит декодирование Progressive изображения.
// numOfScans задает количество сканов, которые необходимо декодировать (0 - декодировать всё).
// При применении к Baseline изображению возвращает ошибку.
func (jpeg *Decoder) ReadProgJPEG(numOfScans uint16) (bool, error) {
	if jpeg.CurStatus == 0 {
		jpeg.constInit()
	}

	if err := jpeg.readScans(numOfScans); err != nil {
		return jpeg.wasEOI, err
	}
	return jpeg.wasEOI, nil
}

// ReadJPEG создает объекта декодировщика.
// source задает источник чтения файла изображения.
func ReadJPEG(source *bufio.Reader) (*Decoder, error) {
	var res Decoder
	res.reader = binreader.BinReaderInit(source)

	marker, err := res.reader.GetWord()
	if err != nil {
		return nil, errors.New("Can't read a word in segment SOI\n" + err.Error())
	}
	if marker != shared.SOI {
		return nil, errors.New("Can't read SOI marker")
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
