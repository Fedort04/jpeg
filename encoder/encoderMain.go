package encoder

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
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

// Типы форматов аппроксимации
type ApproxFormat byte

const (
	ZeroBit ApproxFormat = iota //Без аппроксимации
	OneBit                      //Один бит аппроксимирован
	TwoBits                     //Два бита аппроксимированы
)

type Encoder struct {
	Subsampling EncodeFormat //Формат прореживания (по умолчанию 4:2:0)

	// Не используется при Progressive кодировании
	RestartInterval byte //Интервал перезапуска дельта кодирования (по умолчанию 5)

	// Не используется при Baseline кодировании
	Yspectral []byte       //SpectralSelection яркости (по умолчанию [1, 5, 63])
	Cspectral []byte       //SpectralSelection цвета (по умолчанию [1, 63])
	DCApprox  ApproxFormat //Аппроксимация DC коэффициентов
	Yapprox   ApproxFormat //Аппроксимация яркости (по умолчанию 2)
	Capprox   ApproxFormat //Аппроксимация цвета (по умолчанию 1)

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
	curDCApp        byte                 //Текущее значение аппроксимации для DC
	curYApp         byte                 //Текущее значение аппроксимации для Y
	curCApp         byte                 //Текущее значение аппроксимации для цвета
	numBlocksHeight uint16               //Количество блоков mcu в изображении по высоте
	numBlocksWidth  uint16               //Количество блоков mcu в изображении по ширине
	blockVSize      byte                 //Размер блока по вертикали
	blockHSize      byte                 //Размер блока по горизонтали
	yDCHuff         *huffman.HuffTable   //Таблица Хаффманна DC яркости
	yACHuff         *huffman.HuffTable   //Таблица Хаффманна AC яркости
	cDCHuff         *huffman.HuffTable   //Таблица Хаффманна DC цвета
	cACHuff         *huffman.HuffTable   //Таблица Хаффманна AC цвета
	prev            []int16              //Предыдущие значения для дельта кодирования
	mcuBlockCounter byte                 //Счетчик для restartInterval
	restartCounter  byte                 //Счетчик кол-ва рестартов в потоке
	eobCounter      int                  //Счетчик EOB
	eobBuffer       *binwriter.BinWriter //Буфер для записи refinement EOB
	codingBlocks    [][]mcu.CodingBlock  //Подготовленные для кодирования данные

	curStatus    uint16 //Текущее состояние кодирования
	targetStatus byte   //Сколько должно быть сканов после текущего прохода
	numOfScans   byte   //Количество сканов в изображении
	forSkip      byte   //Для продолжения сканирования со старого места
}

// Конструктор объекта кодирования
func CreateEncoder(dst *bufio.Writer, data shared.Image, quantTableY [][]byte, quantTableColor [][]byte) (*Encoder, error) {
	var encoder Encoder
	encoder.data = &data
	shared.CopyToMatrix(quantTableY, &encoder.quantTableY)
	shared.CopyToMatrix(quantTableColor, &encoder.quantTableColor)
	encoder.Subsampling = Both
	encoder.RestartInterval = defaultRestartInterval
	// Для прогрессива
	encoder.Yspectral = defaultYSpectral
	encoder.Cspectral = defaultCSpectral
	encoder.DCApprox = defaultDCApprox
	encoder.Yapprox = defaultYapprox
	encoder.Capprox = defaultCapprox

	encoder.writer = binwriter.BinWriterInit(dst)
	encoder.prev = make([]int16, shared.NumOfChannels)
	return &encoder, nil
}

// Запись маркера начала изображения
func (jpeg *Encoder) writeStartImg() error {
	if err := jpeg.writer.WriteWord(shared.SOI); err != nil {
		return errors.New("Can't write a segment SOI\n" + err.Error())
	}
	return nil
}

func (jpeg *Encoder) writeEndImg() error {
	if err := jpeg.writer.WriteWord(shared.EOI); err != nil {
		return errors.New("Can't write a segment EOI\n" + err.Error())
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
		return errors.New("Can't write a segment APP0\n" + sw.Err.Error())
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
		return errors.New("Can't write a segment DQT\n" + sw.Err.Error())
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
		return errors.New("Can't write a segment SOF\n" + sw.Err.Error())
	}
	return nil
}

// Запись таблицы Хаффмана и ее сохранение
func (jpeg *Encoder) writeHuffTable(class byte, id byte, bits []byte, symbols []byte) (*huffman.HuffTable, error) {
	offset, _, err := huffman.OffsetCreate(bits)
	if err != nil {
		return nil, errors.New("Can't write a segment DHT\n" + err.Error())
	}

	huff, err := huffman.RecoverHuffTable(offset, symbols)
	if err != nil {
		return nil, errors.New("Can't write a segment DHT\n" + err.Error())
	}

	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	length := uint16(2 + 1 + len(bits) + len(symbols))
	sw.WriteWord(shared.DHT)
	sw.WriteWord(length)
	sw.Write4Bit(class, id)
	sw.WriteArray(bits)
	sw.WriteArray(symbols)
	if sw.Err != nil {
		return nil, errors.New("Can't write a segment DHT\n" + sw.Err.Error())
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
		return errors.New("Can't write a segment DRI\n" + sw.Err.Error())
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

// Общая функция для записи заголовка скана
func (jpeg *Encoder) writeSos(config *scanHeader) error {
	sw := binwriter.StickyWriter{Writer: jpeg.writer}
	sw.WriteWord(config.marker)
	sw.WriteWord(config.length)
	sw.WriteByte(byte(len(config.comps)))
	if sw.Err != nil {
		return errors.New("Can't write a segment SOS\n" + sw.Err.Error())
	}

	for _, elm := range config.comps {
		if err := jpeg.writeComponent(&elm); err != nil {
			return err
		}
	}

	sw.WriteByte(config.ss)
	sw.WriteByte(config.se)
	sw.Write4Bit(config.ah, config.al)
	if sw.Err != nil {
		return errors.New("Can't write a segment SOS\n" + sw.Err.Error())
	}

	return nil
}

// Запись заголовка скана (предварительно записываются таблицы Хаффмана и DRI)
// Реализация для Baseline
func (jpeg *Encoder) writeBaselineScanHeader(blocks [][]mcu.CodingBlock) error {
	se := &stickyEncoder{encoder: jpeg}
	//Записать таблицы Хаффмана
	jpeg.yDCHuff = se.writeHuffTable(0, 0, yDCBits[:], yDCSymbols[:])
	jpeg.cDCHuff = se.writeHuffTable(0, 1, cDCBits[:], cDCSymbols[:])
	bits, huffval := huffman.MakeHuffTable(jpeg.histFound(blocks, 0, shared.BaselineSS+1, shared.BaselineSE, 0, false))
	jpeg.yACHuff = se.writeHuffTable(1, 0, bits, huffval)
	bits, huffval = huffman.MakeHuffTable(jpeg.histFound(blocks, 1, shared.BaselineSS+1, shared.BaselineSE, 0, false))
	jpeg.cACHuff = se.writeHuffTable(1, 1, bits, huffval)
	//Записать DRI
	se.writeDri()
	if se.err != nil {
		return se.err
	}

	compArray := make([]component, shared.NumOfChannels)
	for i := range byte(shared.NumOfChannels) {
		curSelector := i + 1
		compArray[i] = component{selector: curSelector, dcTable: tableIds[curSelector], acTable: tableIds[curSelector]}
	}

	header := scanHeader{
		marker: shared.SOS,
		length: baselineSOSLength,
		ss:     shared.BaselineSS,
		se:     shared.BaselineSE,
		ah:     shared.BaselineAh,
		al:     shared.BaselineAl,
	}
	header.setComps(compArray)
	//Записать сам заголовок
	if err := jpeg.writeSos(&header); err != nil {
		return err
	}

	return nil
}

// Создает строку с описанием текущего прогрессивного скана
func (jpeg *Encoder) createProgressiveScanDescript(head *scanHeader) string {
	var refine string //refine or first visit
	if head.ah == 0 {
		refine = "first visit"
	} else {
		refine = fmt.Sprintf("refine (ah: %d)", head.ah)
	}

	if len(head.comps) == shared.NumOfChannels { //DC
		return fmt.Sprintf("DC %s scan", refine)
	} else { //AC
		var str = map[mcu.Channel]string{
			mcu.Y:  "Y",
			mcu.Cb: "Cb",
			mcu.Cr: "Cr",
		}
		specSelect := fmt.Sprintf("%s [%d, %d]", str[mcu.Channel(head.comps[0].selector-1)], head.ss, head.se) //Channel [spectral selection]
		return fmt.Sprintf("AC %s %s scan", refine, specSelect)
	}
}

// Запись заголовка скана (предварительно записываются таблицы Хаффмана и DRI)
// Реализация для Progressive
func (jpeg *Encoder) writeProgressiveScan(blocks [][]mcu.CodingBlock, head *scanHeader) (bool, error) {
	if jpeg.forSkip < byte(jpeg.curStatus) {
		jpeg.forSkip++
		return false, nil
	}

	se := stickyEncoder{encoder: jpeg}

	if len(head.comps) == shared.NumOfChannels { //DC скан
		jpeg.yDCHuff = se.writeHuffTable(0, 0, yDCBits[:], yDCSymbols[:])
		jpeg.cDCHuff = se.writeHuffTable(0, 1, cDCBits[:], cDCSymbols[:])
		se.writeSos(head)

		if se.err != nil {
			return false, fmt.Errorf("Can't write header in %s\n%s", jpeg.createProgressiveScanDescript(head), se.err.Error())
		}

		if err := shared.MatrixMapError(blocks, func(elm *mcu.CodingBlock) error {
			if err := jpeg.encodeProgressiveDC(elm); err != nil {
				return err
			}
			return nil
			//Конец лямбды
		}); err != nil {
			return false, fmt.Errorf("Can't write scan data in %s\n%s", jpeg.createProgressiveScanDescript(head), err.Error())
		}

	} else { //AC скан
		jpeg.eobCounter = 0
		bits, huffval := huffman.MakeHuffTable(jpeg.histFound(blocks, head.comps[0].selector-1, head.ss, head.se, head.al, true))
		huff := se.writeHuffTable(1, head.comps[0].acTable, bits, huffval)
		se.writeSos(head)
		if se.err != nil {
			return false, fmt.Errorf("Can't write header in %s\n%s", jpeg.createProgressiveScanDescript(head), se.err.Error())
		}

		jpeg.eobCounter = 0
		if err := jpeg.encodeProgressiveAC(blocks, head.comps[0].selector-1, huff, head.ss, head.se); err != nil {
			return false, fmt.Errorf("Can't write scan data in %s\n%s", jpeg.createProgressiveScanDescript(head), err.Error())
		}
	}
	allDone := jpeg.curStatusScanIncrement()

	return allDone, nil
}

// Запись refinement скана
func (jpeg *Encoder) writeRefinementScan(blocks [][]mcu.CodingBlock, head *scanHeader) (bool, error) {
	if jpeg.forSkip < byte(jpeg.curStatus) {
		jpeg.forSkip++
		return false, nil
	}

	se := stickyEncoder{encoder: jpeg}

	if len(head.comps) == shared.NumOfChannels { //DC скан
		if err := jpeg.writeSos(head); err != nil {
			return false, fmt.Errorf("Can't write header in %s\n%s", jpeg.createProgressiveScanDescript(head), se.err.Error())
		}
		if err := shared.MatrixMapError(blocks, func(elm *mcu.CodingBlock) error {
			if err := jpeg.encodeRefineDC(elm); err != nil {
				return err
			}
			return nil
			//Конец лямбды
		}); err != nil {
			return false, fmt.Errorf("Can't write scan data in %s\n%s", jpeg.createProgressiveScanDescript(head), err.Error())
		}

	} else { //AC скан
		jpeg.eobCounter = 0
		bits, huffval := huffman.MakeHuffTable(jpeg.histRefinement(blocks, head.comps[0].selector-1, head.al))
		huff := se.writeHuffTable(1, head.comps[0].acTable, bits, huffval)
		se.writeSos(head)
		if se.err != nil {
			return false, fmt.Errorf("Can't write header in %s\n%s", jpeg.createProgressiveScanDescript(head), se.err.Error())
		}

		//Кодим скан
		jpeg.eobCounter = 0
		jpeg.eobBuffer = binwriter.LocalBinWriterInit(&bytes.Buffer{})
		if err := jpeg.encodeRefinementAC(blocks, head.comps[0].selector-1, huff); err != nil {
			return false, fmt.Errorf("Can't write scan data in %s\n%s", jpeg.createProgressiveScanDescript(head), err.Error())
		}
	}
	allDone := jpeg.curStatusScanIncrement()

	return allDone, nil
}

// Запись заголовка файла
func (jpeg *Encoder) writeHeader(isProgressive bool) error {
	se := &stickyEncoder{encoder: jpeg}
	se.writeStartImg()
	se.writeApp()
	se.writeQuantTable(mcu.ZigZagRow[byte](jpeg.quantTableY), lumId)
	se.writeQuantTable(mcu.ZigZagRow[byte](jpeg.quantTableColor), colorId)
	se.writeFrameHeader(isProgressive)

	if se.err != nil {
		return se.err
	}
	return nil
}

// Вычисления над данными перед кодированием
func (jpeg *Encoder) prepare() [][]mcu.CodingBlock {
	jpeg.factorUpdate()
	img := jpeg.convertToYCbCr()
	blocks := jpeg.blockSubsample(img)
	shared.MatrixMap(blocks, func(elm *mcu.BlockRaw) {
		elm.DCT()
		elm.Quantization(shared.MultMatrixOnNumber(jpeg.quantTableY, mcu.DCTQuantCoeff), shared.MultMatrixOnNumber(jpeg.quantTableColor, mcu.DCTQuantCoeff))
	})
	return jpeg.zigZag(blocks)
}

// Кодирование первых сканов progressive
func (jpeg *Encoder) commonScans(codingBlocks [][]mcu.CodingBlock) error {
	//DC (все компоненты)
	se := &stickyEncoder{encoder: jpeg}
	compArray := make([]component, shared.NumOfChannels)
	for i := range byte(shared.NumOfChannels) {
		curSelector := i + 1
		compArray[i] = component{selector: curSelector, dcTable: tableIds[curSelector]}
	}
	head := scanHeader{
		marker: shared.SOS,
		ss:     dcSpectral,
		se:     dcSpectral,
		ah:     0,
		al:     byte(jpeg.DCApprox),
	}
	head.setComps(compArray)

	if se.writeProgressiveScan(codingBlocks, &head) {
		jpeg.curDCApp--
		return se.err
	}
	jpeg.curDCApp--

	//AC
	stop := false
	for i := 1; !stop; i++ {
		notlum, notcolor := true, true
		if i < len(jpeg.Yspectral) {
			head.ss = jpeg.Yspectral[i-1] + 1
			head.se = jpeg.Yspectral[i]
			head.al = byte(jpeg.Yapprox)
			compArray = make([]component, 1)
			compArray[0].selector = 1
			compArray[0].acTable = 0
			compArray[0].dcTable = 0
			head.setComps(compArray)
			if se.writeProgressiveScan(codingBlocks, &head) {
				return se.err
			}
			if se.err != nil {
				return se.err
			}
			notlum = false
		}

		if i < len(jpeg.Cspectral) {
			//cr
			head.ss = jpeg.Cspectral[i-1] + 1
			head.se = jpeg.Cspectral[i]
			head.al = byte(jpeg.Capprox)
			compArray = make([]component, 1)
			compArray[0].selector = 3
			compArray[0].acTable = 1
			compArray[0].dcTable = 0
			head.setComps(compArray)
			if se.writeProgressiveScan(codingBlocks, &head) {
				return se.err
			}
			//cb
			head.comps[0].selector = 2
			if se.writeProgressiveScan(codingBlocks, &head) {
				return se.err
			}
			if se.err != nil {
				return se.err
			}
			notcolor = false
		}
		stop = notcolor && notlum
	}

	if jpeg.Yapprox != ZeroBit {
		jpeg.curYApp--
	}
	if jpeg.Capprox != ZeroBit {
		jpeg.curCApp--
	}
	return nil
}

// Кодирование сканов с аппроксимацией
func (jpeg *Encoder) approxScans(codingBlocks [][]mcu.CodingBlock) error {
	se := stickyEncoder{encoder: jpeg}
	for i := int(TwoBits); i >= 0; i-- {
		if jpeg.curDCApp == byte(i) && jpeg.DCApprox != ZeroBit {
			head := scanHeader{
				marker: shared.SOS,
				ss:     0,
				se:     0,
				ah:     0,
				al:     jpeg.curDCApp,
			}
			if jpeg.curDCApp+1 <= byte(TwoBits) {
				head.ah = jpeg.curDCApp + 1
			}
			compArray := make([]component, shared.NumOfChannels)
			for i := range byte(shared.NumOfChannels) {
				curSelector := i + 1
				compArray[i] = component{selector: curSelector, dcTable: 0}
			}
			head.setComps(compArray)
			if se.writeRefinementScan(codingBlocks, &head) {
				jpeg.curDCApp--
				return se.err
			}
			jpeg.curDCApp--
		}

		if jpeg.curCApp == byte(i) && jpeg.Capprox != ZeroBit {
			head := scanHeader{
				marker: shared.SOS,
				ss:     approxSS,
				se:     approxSE,
				ah:     0,
				al:     jpeg.curCApp,
			}
			if jpeg.curCApp+1 <= byte(TwoBits) {
				head.ah = jpeg.curCApp + 1
			}

			//cr
			compArray := make([]component, 1)
			compArray[0].selector = 3
			compArray[0].acTable = 1
			compArray[0].dcTable = 0
			head.setComps(compArray)
			if se.writeRefinementScan(codingBlocks, &head) {
				return se.err
			}

			//cb
			compArray[0].selector = 2
			head.setComps(compArray)
			if se.writeRefinementScan(codingBlocks, &head) {
				jpeg.curCApp--
				return se.err
			}
			if se.err != nil {
				return se.err
			}
			jpeg.curCApp--
		}

		//Y
		if jpeg.curYApp == byte(i) && jpeg.Yapprox != ZeroBit {
			head := scanHeader{
				marker: shared.SOS,
				ss:     approxSS,
				se:     approxSE,
				ah:     0,
				al:     jpeg.curYApp,
			}
			if jpeg.curYApp+1 <= byte(TwoBits) {
				head.ah = jpeg.curYApp + 1
			}
			compArray := make([]component, 1)
			compArray[0].selector = 1
			compArray[0].acTable = 0
			compArray[0].dcTable = 0
			head.setComps(compArray)
			if se.writeRefinementScan(codingBlocks, &head) {
				jpeg.curYApp--
				return se.err
			}
			if se.err != nil {
				return se.err
			}
			jpeg.curYApp--
		}
	}
	return nil
}

// Вычисление количества сканов в изображении
func (jpeg *Encoder) numOfScansCalc() {
	jpeg.numOfScans = byte(len(jpeg.Yspectral)-1+(len(jpeg.Cspectral)-1)*2+int(jpeg.DCApprox)+int(jpeg.Yapprox)+int(jpeg.Capprox)*2) + 1
}

// Изменение текущего статуса сканов
// Возвращает прочитано ли необходимая часть
func (jpeg *Encoder) curStatusScanIncrement() bool {
	jpeg.curStatus++
	jpeg.forSkip++
	return jpeg.curStatus >= uint16(jpeg.targetStatus)
}

// По вызову функции выполняется Baseline кодирование
func (jpeg *Encoder) StartBaseline(numOfRows uint16) (bool, error) {
	if jpeg.RestartInterval > 10 {
		return false, fmt.Errorf("Invalid RestartInterval: %d", jpeg.RestartInterval)
	}

	jpeg.Capprox = ZeroBit
	jpeg.DCApprox = ZeroBit
	jpeg.Yapprox = ZeroBit
	if jpeg.curStatus == 0 {
		jpeg.codingBlocks = jpeg.prepare()
		se := &stickyEncoder{encoder: jpeg}
		se.writeHeader(false)
		se.writeBaselineScanHeader(jpeg.codingBlocks)
		if se.err != nil {
			return false, se.err
		}
		jpeg.restartCounter = 0
	}

	//Начало кодирование потока Хаффмана
	//Запись построчно
	startRow := jpeg.curStatus / uint16(jpeg.blockVSize)
	var deltaRows uint16
	if numOfRows == 0 {
		deltaRows = jpeg.realImgHeight
	} else {
		deltaRows = (numOfRows + uint16(jpeg.blockVSize) - 1) / uint16(jpeg.blockVSize)
	}
	if err := shared.MatrixMapRows(jpeg.codingBlocks, startRow, deltaRows, func(elm *mcu.CodingBlock) error {
		if err := jpeg.baselineBlockEncode(elm); err != nil {
			return err
		}
		return nil
		//Конец лямбды
	}); err != nil {
		return false, err
	}

	jpeg.curStatus += deltaRows * uint16(jpeg.blockVSize)

	if jpeg.curStatus >= jpeg.realImgHeight {
		if err := jpeg.writeEndImg(); err != nil {
			return false, err
		}
	} else {
		return false, nil
	}

	return true, nil
}

// Проверка слайса спектральной подборки
func checkSpectralRange(slice []byte) bool {
	if len(slice) == 0 {
		return false
	}

	prev := slice[0]
	if prev > shared.SizeOfTable-1 {
		return false
	}

	for i := 1; i < len(slice); i++ {
		curr := slice[i]

		if curr > shared.SizeOfTable-1 {
			return false
		}

		if curr <= prev {
			return false
		}

		prev = curr
	}

	return true
}

// По вызову функции выполняется Progressive кодирование
func (jpeg *Encoder) StartProgressive(numOfScans byte) (bool, error) {
	if !checkSpectralRange(jpeg.Cspectral) {
		return false, fmt.Errorf("Invalid Cspectral: %v", jpeg.Cspectral)
	}

	if !checkSpectralRange(jpeg.Yspectral) {
		return false, fmt.Errorf("Invalid Yspectral: %v", jpeg.Yspectral)
	}

	jpeg.numOfScansCalc()
	jpeg.forSkip = 0
	jpeg.curDCApp = byte(jpeg.DCApprox)
	jpeg.curYApp = byte(jpeg.Yapprox)
	jpeg.curCApp = byte(jpeg.Capprox)
	if numOfScans == 0 {
		jpeg.targetStatus = jpeg.numOfScans
	} else {
		jpeg.targetStatus = byte(jpeg.curStatus) + numOfScans
	}

	if jpeg.curStatus == 0 {
		jpeg.codingBlocks = jpeg.prepare()
		jpeg.RestartInterval = 0
		jpeg.restartCounter = 0
		se := &stickyEncoder{encoder: jpeg}
		se.writeHeader(true)
	}

	//Первый проход (без approx)
	if jpeg.curStatus < uint16(jpeg.targetStatus) {
		if err := jpeg.commonScans(jpeg.codingBlocks); err != nil {
			return false, err
		}
	}

	//Проходы с аппроксимацией
	if jpeg.curStatus < uint16(jpeg.targetStatus) {
		if err := jpeg.approxScans(jpeg.codingBlocks); err != nil {
			return false, err
		}
	}

	if jpeg.curStatus == uint16(jpeg.numOfScans) {
		if jpeg.writeEndImg() != nil {
			return false, nil
		}
	} else {
		return false, nil
	}

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
