package encoder

import (
	"bufio"
	"errors"
	binwriter "jpeg/internal/binWriter"
	"jpeg/internal/huffman"
	"jpeg/internal/mcu"
	"jpeg/shared"
)

// Типы форматов прореживания
type EncodeFormat byte

const (
	Without    EncodeFormat = iota //4:4:4
	Horizontal                     //4:2:2 вертикальный
	Vertical                       //4:2:2 горизонтальный
	Both                           //4:2:0
)

type Encoder struct {
	RestartInterval byte         //Интервал перезапуска дельта кодирования (по умолчанию 5)
	Format          EncodeFormat //Формат прореживания (по умолчанию 4:2:0)

	// Не используется при Baseline кодировании
	Yspectral []byte //SpectralSelection яркости (по умолчанию [0, 5, 63])
	Cspectral []byte //SpectralSelection цвета (по умолчанию [0, 63])
	Yapprox   byte   //Аппроксимация яркости (по умолчанию 2)
	Capprox   byte   //Аппроксимация цвета (по умолчанию 1)

	//private:
	writer          *binwriter.BinWriter //Объект для записи файла
	data            *shared.Image        //Данные изображения
	realImgHeight   uint16               //Высота изображения (реальная)
	realImgWidth    uint16               //Ширина изображения (реальная)
	imgHeight       uint16               //Высота изображения (для кодирования)
	imgWidth        uint16               //Ширина изображения (для кодирования)
	quantTableY     [][]byte             //Таблица квантования для яркости
	quantTableColor [][]byte             //Таблица квантования для цвета
	yh              byte                 //Горизонтальный фактор яркости
	yv              byte                 //Вертикальный фактор яркости
	ch              byte                 //Горизонтальный фактор цвета
	cv              byte                 //Вертикальный фактор цвета
	maxH            byte                 //Максимальный H фактор
	maxV            byte                 //Максимальный V фактор
	numBlocksHeight uint16               //Количество блоков mcu в изображении по высоте
	numBlocksWidth  uint16               //Количество блоков mcu в изображении по ширине
	blockVSize      byte                 //Размер блока по вертикали
	blockHSize      byte                 //Размер блока по горизонтали
	yDCHuff         *huffman.HuffTable   //Таблица Хаффманна DC яркости
	yACHuff         *huffman.HuffTable   //Таблица Хаффманна AC яркости
	cDCHuff         *huffman.HuffTable   //Таблица Хаффманна DC цвета
	cACHuff         *huffman.HuffTable   //Таблица Хаффманна AC цвета
	prev            []int16              //Предыдущие значения для дельта кодирования
	mcuCounter      byte                 //Счетчик для restartInterval
	restartCounter  byte                 //Счетчик кол-ва рестартов в потоке
}

// Конструктор объекта кодирования
func CreateEncoder(dst *bufio.Writer, data shared.Image, quantTableY [][]byte, quantTableColor [][]byte) (*Encoder, error) {
	var encoder Encoder
	encoder.data = &data
	shared.CopyToMatrix(quantTableY, &encoder.quantTableY)
	shared.CopyToMatrix(quantTableColor, &encoder.quantTableColor)
	encoder.Format = Both
	encoder.RestartInterval = 5
	// Для прогрессива
	encoder.Yspectral = []byte{0, 5, 63}
	encoder.Cspectral = []byte{0, 63}
	encoder.Yapprox = 2
	encoder.Capprox = 1

	encoder.writer = binwriter.BinWriterInit(dst)
	encoder.prev = make([]int16, shared.NumOfChannels)
	return &encoder, nil
}

// Запись маркера начала изображения
func (jpeg *Encoder) writeStartImg() error {
	if err := jpeg.writer.WriteWord(shared.SOI); err != nil {
		return errors.New("Can't write an SOI marker\n" + err.Error())
	}
	return nil
}

func (jpeg *Encoder) writeEndImg() error {
	if err := jpeg.writer.WriteWord(shared.EOI); err != nil {
		return errors.New("Can't write an EOI marker\n" + err.Error())
	}
	return nil
}

// Запись сегмента APP0
func (jpeg *Encoder) writeApp() error {
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	sw.WriteWord(shared.APP0)
	sw.WriteWord(app0Length)
	sw.WriteArray(jfif[:])
	sw.WriteWord(jfifVersion)
	sw.WriteByte(densityUnit)
	sw.WriteWord(xDensity)
	sw.WriteWord(yDensity)
	sw.WriteByte(xThumb)
	sw.WriteByte(yThumb)

	if sw.Err != nil {
		return errors.New("Can't write an APP0 segment\n" + sw.Err.Error())
	}
	return nil
}

// Запись таблиц квантования
func (jpeg *Encoder) writeQuantTable(quantTable []byte, compId byte) error {
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	sw.WriteWord(shared.DQT)
	sw.WriteWord(uint16(dqtLength))
	sw.WriteByte(compId)
	sw.WriteArray(quantTable)

	if sw.Err != nil {
		return errors.New("Can't write an DQT segment\n" + sw.Err.Error())
	}
	return nil
}

// Запись заголовка фрейма
func (jpeg *Encoder) writeFrameHeader(isProgressive bool) error {
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	if isProgressive {
		sw.WriteWord(shared.SOF2)
	} else {
		sw.WriteWord(shared.SOF0)
	}
	sw.WriteWord(uint16(sofLength))
	sw.WriteByte(samplePrecision)
	sw.WriteWord(jpeg.realImgHeight)
	sw.WriteWord(jpeg.realImgWidth)
	sw.WriteByte(shared.NumOfChannels)

	for i := range byte(shared.NumOfChannels) {
		sw.WriteByte(i + 1)
		if i == 0 {
			sw.Write4Bit(jpeg.yh, jpeg.yv)
		} else {
			sw.Write4Bit(jpeg.ch, jpeg.cv)
		}
		sw.WriteByte(tableIds[i+1])
	}

	if sw.Err != nil {
		return errors.New("Can't write an SOF segment\n" + sw.Err.Error())
	}
	return nil
}

// Запись таблицы Хаффмана и ее сохранение
func (jpeg *Encoder) writeHuffTable(class byte, id byte, bits []byte, symbols []byte) (*huffman.HuffTable, error) {
	offset, _, err := huffman.OffsetCreate(bits)
	if err != nil {
		return nil, err
	}

	huff, err := huffman.MakeHuffTable(offset, symbols)
	if err != nil {
		return nil, err
	}

	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	length := uint16(2 + 1 + len(bits) + len(symbols))
	sw.WriteWord(shared.DHT)
	sw.WriteWord(length)
	sw.Write4Bit(class, id)
	sw.WriteArray(bits)
	sw.WriteArray(symbols)
	if sw.Err != nil {
		return nil, errors.New("Can't write an DHT segment\n" + sw.Err.Error())
	}

	return huff, nil
}

// Запись сегмента DRI
func (jpeg *Encoder) writeDri() error {
	if jpeg.RestartInterval == 0 {
		return nil
	}
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	sw.WriteWord(shared.DRI)
	sw.WriteWord(driLength)
	sw.WriteWord(uint16(jpeg.RestartInterval))
	if sw.Err != nil {
		return errors.New("Can't write an DRI segment\n" + sw.Err.Error())
	}

	return nil
}

// Запись компоненты в файл
func (jpeg *Encoder) writeComponent(c *component) error {
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	sw.WriteByte(c.selector)
	sw.Write4Bit(c.dcTable, c.acTable)
	if sw.Err != nil {
		return errors.New("Can't write color component data in SOS segment\n" + sw.Err.Error())
	}
	return nil
}

// Запись заголовка скана (предварительно записываются таблицы Хаффмана и DRI)
// Реализация для Baseline
func (jpeg *Encoder) writeBaselineScanHeader() error {
	se := &stickyEncoder{encoder: jpeg}
	//Записать таблицы Хаффмана
	jpeg.yDCHuff = se.writeHuffTable(0, 0, yDCBits[:], yDCSymbols[:])
	jpeg.cDCHuff = se.writeHuffTable(0, 1, cDCBits[:], cDCSymbols[:])
	jpeg.yACHuff = se.writeHuffTable(1, 0, yACBits[:], yACSymbols[:])
	jpeg.cACHuff = se.writeHuffTable(1, 1, cACBits[:], cACSymbols[:])
	//Записать DRI
	se.writeDri()
	if se.err != nil {
		return se.err
	}

	//Записать сам заголовок
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	sw.WriteWord(shared.SOS)
	sw.WriteWord(baselineSOSLength)
	sw.WriteByte(baselineColors)
	if sw.Err != nil {
		return errors.New("Can't write an SOS segment\n" + sw.Err.Error())
	}

	for i := range byte(shared.NumOfChannels) {
		curSelector := i + 1
		se.writeComponent(&component{selector: curSelector, dcTable: tableIds[curSelector], acTable: tableIds[curSelector]})
	}
	if se.err != nil {
		return se.err
	}

	sw.WriteByte(baselineSS)
	sw.WriteByte(baselineSE)
	sw.Write4Bit(baselineAh, baselineAl)
	if sw.Err != nil {
		return errors.New("Can't write an SOS segment\n" + sw.Err.Error())
	}

	return nil
}

// Запись заголовка файла
func (jpeg *Encoder) writeHeader() error {
	se := &stickyEncoder{encoder: jpeg}
	se.writeStartImg()
	se.writeApp()
	se.writeQuantTable(mcu.ZigZagRow[byte](jpeg.quantTableY), lumId)
	se.writeQuantTable(mcu.ZigZagRow[byte](jpeg.quantTableColor), colorId)
	se.writeFrameHeader(false)

	if se.err != nil {
		return se.err
	}
	return nil
}

// По вызову функции выполняется Baseline кодирование
func (jpeg *Encoder) StartBaseline(numOfRows uint16) (bool, error) {
	jpeg.factorUpdate()
	img := jpeg.convertToYCbCr()
	blocks := jpeg.blockSubsample(img)
	shared.MatrixMap(blocks, func(elm *mcu.BlockRaw) {
		elm.DCT()
		elm.Quantization(shared.MultMatrixOnNumber(jpeg.quantTableY, mcu.DCTQuantCoeff), shared.MultMatrixOnNumber(jpeg.quantTableColor, mcu.DCTQuantCoeff))
	})
	codingBlocks := jpeg.zigZag(blocks)

	if err := jpeg.writeHeader(); err != nil {
		return false, err
	}

	if err := jpeg.writeBaselineScanHeader(); err != nil {
		return false, err
	}

	//Начало кодирование потока Хаффмана (не забыть сбросить счетчик рестартов)
	jpeg.restartCounter = 0
	if err := shared.MatrixMapError(codingBlocks, func(elm *mcu.CodingBlock) error {
		if err := jpeg.baselineBlockEncode(elm); err != nil {
			return err
		}
		return nil
		//Конец лямбды
	}); err != nil {
		return false, err
	}

	if err := jpeg.writeEndImg(); err != nil {
		return false, err
	}
	return true, nil
}

// По вызову функции выполняется Progressive кодирование
func (encoder *Encoder) StartProgressive(numOfScans byte) (bool, error) {
	return true, nil
}

// Создание единичной таблицы квантования
func CreateOneTable() [][]byte {
	table := make([][]byte, mcu.UnitRowCount)

	for i := range mcu.UnitRowCount {
		row := make([]byte, mcu.UnitColCount)
		for j := range mcu.UnitColCount {
			row[j] = 1
		}
		table[i] = row
	}

	return table
}
