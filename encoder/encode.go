package encoder

import (
	"bytes"
	"errors"
	binwriter "jpeg/internal/binWriter"
	"jpeg/internal/huffman"
	"jpeg/internal/mcu"
	"jpeg/shared"
	"math"
)

// Перевод изображения в YCbCr с расширением изображения до кратного subsample размера
func (jpeg *Encoder) convertToYCbCr() shared.YCbCrMatrix {
	data := *jpeg.data

	jpeg.realImgHeight = uint16(len(data))
	jpeg.realImgWidth = uint16(len(data[0]))
	jpeg.imgWidth = ((jpeg.realImgWidth + uint16(jpeg.blockHSize) - 1) / uint16(jpeg.blockHSize)) * uint16(jpeg.blockHSize)
	jpeg.imgHeight = ((jpeg.realImgHeight + uint16(jpeg.blockVSize) - 1) / uint16(jpeg.blockVSize)) * uint16(jpeg.blockVSize)
	img := shared.CreateMatrix[shared.YCbCr](int(jpeg.imgHeight), int(jpeg.imgWidth))

	var realI, realJ uint16
	for i := range jpeg.imgHeight {
		if i >= jpeg.realImgHeight {
			realI = jpeg.realImgHeight - 1
		} else {
			realI = i
		}

		for j := range jpeg.imgWidth {
			if j >= jpeg.realImgWidth {
				realJ = jpeg.realImgWidth - 1
			} else {
				realJ = j
			}
			data[realI][realJ].ToYCbCr(&img[i][j])
		}
	}
	return img
}

// Вычисление значений факторов subsample по выбранному формату
func (jpeg *Encoder) factorUpdate() {
	jpeg.ch = 1
	jpeg.cv = 1

	switch jpeg.Subsampling {
	case Without:
		jpeg.yh = 1
		jpeg.yv = 1
		jpeg.maxH = 1
		jpeg.maxV = 1
	case Horizontal:
		jpeg.yh = 2
		jpeg.yv = 1
		jpeg.maxH = 2
		jpeg.maxV = 1
	case Vertical:
		jpeg.yv = 2
		jpeg.yh = 1
		jpeg.maxV = 2
		jpeg.maxH = 1
	case Both:
		jpeg.yh = 2
		jpeg.maxH = 2
		jpeg.yv = 2
		jpeg.maxV = 2
	}
	jpeg.blockVSize = mcu.UnitRowCount * jpeg.maxV
	jpeg.blockHSize = mcu.UnitColCount * jpeg.maxH
}

// Характеризует часть изображения
type part struct {
	// Глобальный левый угол
	globalVPos uint16
	globalHPos uint16

	// Левый угол текущего MCU в блоке (относительно самого блока)
	// Пример: 4:2:0 блок 2х2 -> позиции от 0 до 2
	vPos byte
	hPos byte
}

// Копирует данные части img в dst (вместе с использованием subsample)
// dst матрица слайсов уже создана, просто заполняет значениями
func (jpeg *Encoder) copyImgPartToMatrix(img shared.YCbCrMatrix, dst *mcu.RawMCU, curPart part, channel mcu.Channel) {
	for i := range uint16(mcu.UnitRowCount) {
		for j := range uint16(mcu.UnitColCount) {
			// По глобальному изображению
			curV := curPart.globalVPos + i
			curH := curPart.globalHPos + j
			switch channel {
			case mcu.Y:
				dst.Data[i][j] = img[curV][curH].Y
			case mcu.Cb, mcu.Cr:
				// По текущему фрагменту
				subJ := (j + uint16(curPart.hPos)*mcu.UnitColCount) / uint16(jpeg.maxH)
				subI := (i + uint16(curPart.vPos)*mcu.UnitRowCount) / uint16(jpeg.maxV)
				remainI := curH % uint16(jpeg.maxH)
				remainJ := curV % uint16(jpeg.maxV)
				if remainI == 0 && remainJ == 0 {
					var value float32
					if channel == mcu.Cb {
						value = img[curV][curH].Cb
					} else {
						value = img[curV][curH].Cr
					}
					dst.Data[subI][subJ] = value
				}
			}
		}
	}
}

// Chroma blockSubsample
// Возвращает матрицу прореженных и структурирвованных под Baseline кодирование блоков
func (jpeg *Encoder) blockSubsample(img shared.YCbCrMatrix) [][]mcu.BlockRaw {
	jpeg.numBlocksHeight = jpeg.imgHeight / uint16(jpeg.blockVSize)
	jpeg.numBlocksWidth = jpeg.imgWidth / uint16(jpeg.blockHSize)

	res := shared.CreateMatrix[mcu.BlockRaw](int(jpeg.numBlocksHeight), int(jpeg.numBlocksWidth))

	// Для каждого блока
	for blockI := range jpeg.numBlocksHeight {
		for blockJ := range jpeg.numBlocksWidth {
			var curBlock mcu.BlockRaw

			globalVPos := blockI * uint16(jpeg.blockVSize)
			globalHPos := blockJ * uint16(jpeg.blockHSize)

			curBlock.Y = shared.CreateMatrix[mcu.RawMCU](int(jpeg.maxV), int(jpeg.maxH))
			curBlock.Cb.Data = shared.CreateMatrix[float32](mcu.UnitRowCount, mcu.UnitColCount)
			curBlock.Cr.Data = shared.CreateMatrix[float32](mcu.UnitRowCount, mcu.UnitColCount)

			for i := range uint16(jpeg.maxV) {
				for j := range uint16(jpeg.maxH) {
					// Текущие позиции в изображении
					curHpos := globalHPos + j*mcu.UnitColCount
					curVPos := globalVPos + i*mcu.UnitRowCount
					curPart := part{globalVPos: curVPos, globalHPos: curHpos, vPos: byte(i), hPos: byte(j)}

					// Обработка Y
					curBlock.Y[i][j].Data = shared.CreateMatrix[float32](mcu.UnitRowCount, mcu.UnitColCount)
					jpeg.copyImgPartToMatrix(img, &curBlock.Y[i][j], curPart, mcu.Channel(mcu.Y))

					// Обработка Cb и Cr
					jpeg.copyImgPartToMatrix(img, &curBlock.Cb, curPart, mcu.Channel(mcu.Cb))
					jpeg.copyImgPartToMatrix(img, &curBlock.Cr, curPart, mcu.Channel(mcu.Cr))
				}
			}

			res[blockI][blockJ] = curBlock
		}
	}
	return res
}

// Преобразование в вид, пригодный для кодирования
func (jpeg *Encoder) zigZag(blocks [][]mcu.BlockRaw) [][]mcu.CodingBlock {
	res := shared.CreateMatrix[mcu.CodingBlock](len(blocks), len(blocks[0]))
	for i, row := range blocks {
		for j, elm := range row {
			res[i][j] = elm.ZigZag(jpeg.maxH, jpeg.maxV)
		}
	}
	return res
}

// Запись маркера сброса дельты
func (jpeg *Encoder) writeRestart() error {
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	sw.FlushBits()

	if jpeg.restartCounter >= shared.NumOfRstMarkers {
		jpeg.restartCounter = 0
	}
	curMarker := shared.RST0 + uint16(jpeg.restartCounter)
	sw.WriteWord(curMarker)
	jpeg.restartCounter++

	if sw.Err != nil {
		return errors.New("Can't write an RST marker\n" + sw.Err.Error())
	}
	return nil
}

// Умный счетчик для перезапуска дельта-кодирования
// При необходимости сам записывает сегмент сброса дельты
func (jpeg *Encoder) restartIncrement() error {
	jpeg.mcuBlockCounter++

	var err error
	if jpeg.RestartInterval != 0 && jpeg.mcuBlockCounter >= jpeg.RestartInterval {
		jpeg.prev = make([]int16, shared.MaxComps)
		jpeg.mcuBlockCounter = 0
		err = jpeg.writeRestart()
	}

	return err
}

// Создание дополнительного кода для кодирования значения
func createAddVal(val int16, len byte) uint16 {
	if val < 0 {
		mask := (int16(1) << len) - 1
		val = int16(math.Abs(float64(val)))
		return uint16(val ^ mask)
	}
	return uint16(val)
}

// Кодирование дополнительного значения, которое идет после символа Хаффмана
func (jpeg *Encoder) encodeAddVal(val int16, ssss byte) error {
	return jpeg.writer.WriteBits(jpeg.writer.CreateBitsArray(createAddVal(val, ssss), ssss))
}

// Кодирование одного символа Хаффмана
func (jpeg *Encoder) encodeSymbol(symbol byte, table *huffman.HuffTable) error {
	code, len, err := table.GetCodeBySym(symbol)
	if err != nil {
		return err
	}

	if err := jpeg.writer.WriteBits(jpeg.writer.CreateBitsArray(code, len)); err != nil {
		return err
	}

	return nil
}

// Кодирование DC-элемента
func (jpeg *Encoder) encodeDC(val int16, table *huffman.HuffTable, ch byte) error {
	val = val >> int16(jpeg.curDCApp)
	diff := val - jpeg.prev[ch]
	jpeg.prev[ch] = val

	ssss := shared.FindCategory(diff)
	se := &stickyEncoder{encoder: jpeg}
	se.encodeSymbol(ssss, table)
	se.encodeAddVal(diff, ssss)
	if se.err != nil {
		return errors.New("Error when encode a DC symbol\n" + se.err.Error())
	}

	return nil
}

// Кодирование AC-элемента
func (jpeg *Encoder) encodeAC(dataUnit []int16, table *huffman.HuffTable) error {
	var zeroCounter byte

	for k := shared.BaselineSS + 1; k <= shared.BaselineSE; k++ {
		val := shared.Truncate(dataUnit[k], 0)
		if val == 0 {
			zeroCounter++
			continue
		}

		seenc := &stickyEncoder{encoder: jpeg}
		// ZRL для каждых 16 нулей
		for zeroCounter >= shared.MaxZeros {
			seenc.encodeSymbol(shared.ZRL, table)
			zeroCounter -= shared.MaxZeros
		}

		// Кодировка ненулевого значения
		ssss := shared.FindCategory(val)
		rs := (zeroCounter << 4) | ssss
		seenc.encodeSymbol(rs, table)
		seenc.encodeAddVal(val, ssss)

		if seenc.err != nil {
			return errors.New("Error when encode an AC symbol\n" + seenc.err.Error())
		}

		zeroCounter = 0
	}

	if zeroCounter > 0 {
		return jpeg.encodeSymbol(shared.EndOfBlock, table)
	}
	return nil
}

// Кодирование одного data-unit
func (jpeg *Encoder) dataUnitEncode(dataUnit []int16, channel byte) error {
	var dcTable, acTable *huffman.HuffTable
	if mcu.Channel(channel) != mcu.Y { //Вариант для цветов
		dcTable = jpeg.cDCHuff
		acTable = jpeg.cACHuff
	} else { // Вариант для яркости
		dcTable = jpeg.yDCHuff
		acTable = jpeg.yACHuff
	}

	se := &stickyEncoder{encoder: jpeg}
	se.encodeDC(dataUnit[0], dcTable, channel)
	se.encodeAC(dataUnit, acTable)

	if se.err != nil {
		return se.err
	}
	return nil
}

// Кодирование блока baseline
func (jpeg *Encoder) baselineBlockEncode(block *mcu.CodingBlock) error {
	se := &stickyEncoder{encoder: jpeg}
	for _, y := range block.Y {
		se.dataUnitEncode(y, byte(mcu.Y))
	}
	se.dataUnitEncode(block.Cb, byte(mcu.Cb))
	se.dataUnitEncode(block.Cr, byte(mcu.Cr))
	se.restartIncrement()

	if se.err != nil {
		return errors.New("Error when encode block\n" + se.err.Error())
	}
	return nil
}

// Кодирование прогрессивного DC скана
func (jpeg *Encoder) encodeProgressiveDC(block *mcu.CodingBlock) error {
	se := &stickyEncoder{encoder: jpeg}
	for _, y := range block.Y {
		se.encodeDC(y[0], jpeg.yDCHuff, byte(mcu.Y))
	}
	se.encodeDC(block.Cb[0], jpeg.cDCHuff, byte(mcu.Cb))
	se.encodeDC(block.Cr[0], jpeg.cDCHuff, byte(mcu.Cr))

	return nil
}

// Формирование дополнительного значения для EOB
func (jpeg *Encoder) addValEob(ssss byte) int {
	return jpeg.eobCounter & int((1<<ssss)-1)
}

// Кодирование символа EOB
func (jpeg *Encoder) encodeEOB(table *huffman.HuffTable) error {
	if jpeg.eobCounter == 0 {
		return nil
	}

	ssss := shared.FindCategory(int16(jpeg.eobCounter)) - 1
	se := &stickyEncoder{encoder: jpeg}
	se.encodeSymbol(ssss<<4, table)
	se.encodeAddVal(int16(jpeg.addValEob(ssss)), ssss)
	if se.err != nil {
		return errors.New("Error when encode an End of Band symbol\n" + se.err.Error())
	}

	jpeg.eobCounter = 0
	return nil
}

// Кодирование AC с учетом EOB
func (jpeg *Encoder) encodeEOBAC(cfg *mcu.ProgressiveConfig, table *huffman.HuffTable) error {
	allZero := true
	var zeroCounter, count byte

	for count = cfg.SS; count <= cfg.SE; count++ {
		if shared.Truncate(cfg.Row[count], cfg.App) != 0 {
			allZero = false
			break
		} else {
			zeroCounter++
		}
	}

	if allZero {
		jpeg.eobCounter++
		return nil
	}

	if jpeg.eobCounter != 0 {
		if err := jpeg.encodeEOB(table); err != nil {
			return err
		}
	}

	for ; count <= cfg.SE; count++ {
		val := shared.Truncate(cfg.Row[count], cfg.App)
		if val == 0 {
			zeroCounter++
			continue
		}

		seenc := &stickyEncoder{encoder: jpeg}
		for zeroCounter >= shared.MaxZeros {
			seenc.encodeSymbol(shared.ZRL, table)
			zeroCounter -= shared.MaxZeros
		}

		// Кодировка ненулевого значения
		ssss := shared.FindCategory(val)
		rs := (zeroCounter << 4) | ssss
		seenc.encodeSymbol(rs, table)
		seenc.encodeAddVal(val, ssss)

		if seenc.err != nil {
			return errors.New("Error when encode an AC symbol\n" + seenc.err.Error())
		}

		zeroCounter = 0
	}

	if zeroCounter > 0 {
		jpeg.eobCounter++
	}
	return nil
}

// Кодирование обычного прогрессивного AC скана
func (jpeg *Encoder) encodeProgressiveAC(blocks [][]mcu.CodingBlock, ch byte, table *huffman.HuffTable, ss, se byte) error {
	var err error

	switch ch {
	case byte(mcu.Y):
		jpeg.foreachY(blocks, func(data []int16) {
			err = jpeg.encodeEOBAC(&mcu.ProgressiveConfig{Row: data, SS: ss, SE: se, App: jpeg.curYApp}, table)
			if err != nil {
				return
			}
		})
	case byte(mcu.Cb):
		for _, row := range blocks {
			for _, elm := range row {
				if err = jpeg.encodeEOBAC(&mcu.ProgressiveConfig{Row: elm.Cb, SS: ss, SE: se, App: jpeg.curCApp}, table); err != nil {
					return err
				}
			}
		}
	case byte(mcu.Cr):
		for _, row := range blocks {
			for _, elm := range row {
				if err = jpeg.encodeEOBAC(&mcu.ProgressiveConfig{Row: elm.Cr, SS: ss, SE: se, App: jpeg.curCApp}, table); err != nil {
					return err
				}
			}
		}
	}
	if jpeg.eobCounter > 0 {
		if err = jpeg.encodeEOB(table); err != nil {
			return err
		}
	}
	return err
}

// Кодирование одного mcu с refinement
func (jpeg *Encoder) encodeRefinementAC(row []int16, app byte, table *huffman.HuffTable) error {
	allZero := true
	tempBuffer := binwriter.LocalBinWriterInit(&bytes.Buffer{})

	for k := mcu.ApproxSS; k <= mcu.ApproxSE; k++ {
		val := shared.Truncate(row[k], app)
		if val != 0 {
			if shared.CheckHistory(row[k], app) {
				allZero = false
				tempBuffer = binwriter.LocalBinWriterInit(&bytes.Buffer{})
				break
			} else {
				if err := tempBuffer.WriteBit(val&1 == 1); err != nil {
					return errors.New("Error when encode an AC Refine value\n" + err.Error())
				}
			}
		}
	}
	if allZero {
		if err := jpeg.eobBuffer.MergeFrom(tempBuffer); err != nil {
			return errors.New("Error when merging AC Refine buffers\n" + err.Error())
		}
		jpeg.eobCounter++
		return nil
	}

	var afterLast bool
	var zeroCounter byte
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	se := &stickyEncoder{encoder: jpeg}

	for k := mcu.ApproxSS; k <= mcu.ApproxSE; k++ {
		val := shared.Truncate(row[k], app)

		if val == 0 {
			zeroCounter++
			continue
		}

		// ZRL для каждых 16 нулей
		for zeroCounter >= shared.MaxZeros {
			if jpeg.eobCounter != 0 {
				se.encodeEOB(table)
				sw.MergeFrom(jpeg.eobBuffer)
			}
			se.encodeSymbol(shared.ZRL, table)
			sw.MergeFrom(tempBuffer)
			zeroCounter -= shared.MaxZeros

			if sw.Err != nil {
				return errors.New("Error when encode an AC Refine value" + sw.Err.Error())
			}
			if se.err != nil {
				return errors.New("Error when encode an AC Refine symbol" + sw.Err.Error())
			}
		}

		if shared.CheckHistory(row[k], app) {
			if jpeg.eobCounter != 0 {
				se.encodeEOB(table)
				sw.MergeFrom(jpeg.eobBuffer)
			}
			rs := (zeroCounter << 4) + 1
			se.encodeSymbol(rs, table)

			bit := true
			if row[k] < 0 {
				bit = false
			}

			sw.WriteBit(bit)

			sw.MergeFrom(tempBuffer)
			zeroCounter = 0
			afterLast = false

			if sw.Err != nil {
				return errors.New("Error when encode an AC Refine value" + sw.Err.Error())
			}
			if se.err != nil {
				return errors.New("Error when encode an AC Refine symbol" + sw.Err.Error())
			}
		} else {
			afterLast = true
			if err := tempBuffer.WriteBit(val&1 == 1); err != nil {
				return errors.New("Error when encode an AC Refine value" + sw.Err.Error())
			}
		}
	}

	if zeroCounter > 0 || afterLast {
		jpeg.eobBuffer.MergeFrom(tempBuffer)
		jpeg.eobCounter++
	}
	return nil
}

// Кодирование refinement AC скана
func (jpeg *Encoder) encodeRefinementUnit(blocks [][]mcu.CodingBlock, ch byte, table *huffman.HuffTable) error {
	var err error

	switch ch {
	case byte(mcu.Y):
		jpeg.foreachY(blocks, func(data []int16) {
			err = jpeg.encodeRefinementAC(data, jpeg.curYApp, table)
			if err != nil {
				return
			}
		})
	case byte(mcu.Cb):
		for _, row := range blocks {
			for _, elm := range row {
				if err = jpeg.encodeRefinementAC(elm.Cb, jpeg.curCApp, table); err != nil {
					return err
				}
			}
		}
	case byte(mcu.Cr):
		for _, row := range blocks {
			for _, elm := range row {
				if err = jpeg.encodeRefinementAC(elm.Cr, jpeg.curCApp, table); err != nil {
					return err
				}
			}
		}
	}
	if jpeg.eobCounter > 0 {
		if err = jpeg.encodeEOB(table); err != nil {
			return err
		}
		jpeg.writer.MergeFrom(jpeg.eobBuffer)
	}
	return err
}

// Кодирование refine DC блока
func (jpeg *Encoder) encodeRefineDC(block *mcu.CodingBlock) error {
	sw := &binwriter.StickyWriter{Writer: jpeg.writer}
	for _, y := range block.Y {
		sw.WriteBit((y[0] >> int16(jpeg.curDCApp) & 1) == 1)
	}
	//cb
	sw.WriteBit((block.Cb[0] >> int16(jpeg.curDCApp) & 1) == 1)
	//cr
	sw.WriteBit((block.Cr[0] >> int16(jpeg.curDCApp) & 1) == 1)
	if sw.Err != nil {
		return errors.New("Error when encode a DC Refine value\n" + sw.Err.Error())
	}

	return nil
}

// Кодирование refinement DC скана
func (jpeg *Encoder) encodeRefinementDC(blocks [][]mcu.CodingBlock) error {
	for _, row := range blocks {
		for _, elm := range row {
			if err := jpeg.encodeRefineDC(&elm); err != nil {
				return err
			}
		}
	}
	return nil
}

// Обход данных Y для AC сканов
func (jpeg *Encoder) foreachY(blocks [][]mcu.CodingBlock, f func([]int16)) {
	realHeight := (jpeg.realImgHeight + (mcu.UnitRowCount - 1)) / mcu.UnitRowCount
	realWidth := (jpeg.realImgWidth + (mcu.UnitColCount - 1)) / mcu.UnitColCount

	for vert := range realHeight {
		blockV := vert / uint16(jpeg.maxV)
		blockVPad := vert % uint16(jpeg.maxV)
		for hor := range realWidth {
			blockH := hor / uint16(jpeg.maxH)
			blockHPad := hor % uint16(jpeg.maxH)
			f(blocks[blockV][blockH].Y[blockVPad*uint16(jpeg.maxH)+blockHPad])
		}
	}
}

// Обертка, которая возвращает функцию предварительных действий для вычисления гистограммы progressive
func (jpeg *Encoder) progressiveHistPrepare(res map[uint16]int) func(cfg *mcu.ProgressiveConfig, zeroCounter *byte, count *byte) bool {
	return func(cfg *mcu.ProgressiveConfig, zeroCounter *byte, count *byte) bool {
		allZero := true
		for *count = cfg.SS; *count <= cfg.SE; *count++ {
			if shared.Truncate(cfg.Row[*count], cfg.App) != 0 {
				allZero = false
				break
			} else {
				*zeroCounter++
			}
		}

		if allZero {
			jpeg.eobCounter++
			return false
		}

		if jpeg.eobCounter != 0 {
			ssss := shared.FindCategory(int16(jpeg.eobCounter)) - 1
			res[uint16(ssss<<4)]++
			jpeg.eobCounter = 0
		}
		return true
	}
}

// Нахождение гистограммы по каналу
func (jpeg *Encoder) histFound(blocks [][]mcu.CodingBlock, ch byte, cfg *mcu.ProgressiveConfig, isProgressive bool) map[uint16]int {
	res := make(map[uint16]int)
	prep := jpeg.progressiveHistPrepare(res)
	eobFunc := func() { jpeg.eobCounter++ }

	if isProgressive {
		switch ch {
		case byte(mcu.Y): //Для яркости
			jpeg.foreachY(blocks, func(data []int16) {
				cfg.Row = data
				mcu.ChannelHist(res, cfg, prep, eobFunc)
			})
		case byte(mcu.Cb):
			for _, row := range blocks {
				for _, elm := range row {
					cfg.Row = elm.Cb
					mcu.ChannelHist(res, cfg, prep, eobFunc)
				}
			}
		case byte(mcu.Cr):
			for _, row := range blocks {
				for _, elm := range row {
					cfg.Row = elm.Cr
					mcu.ChannelHist(res, cfg, prep, eobFunc)
				}
			}
		}
		if jpeg.eobCounter != 0 {
			ssss := shared.FindCategory(int16(jpeg.eobCounter)) - 1
			res[uint16(ssss<<4)]++
			jpeg.eobCounter = 0
		}
	} else {
		for _, row := range blocks {
			for _, elm := range row {
				elm.GetBlockHist(res, ch, cfg.SS, cfg.SE)
			}
		}
	}
	return res
}

// Нахождение гистограммы для refinement скана по каналу
func (jpeg *Encoder) histRefinement(blocks [][]mcu.CodingBlock, ch, app byte) map[uint16]int {
	res := make(map[uint16]int)
	switch ch {
	case byte(mcu.Y):
		jpeg.foreachY(blocks, func(row []int16) {
			shared.MergeInto(res, mcu.GetRefinementHist(row, app, &jpeg.eobCounter))
		})
	case byte(mcu.Cb):
		for _, row := range blocks {
			for _, elm := range row {
				shared.MergeInto(res, mcu.GetRefinementHist(elm.Cb, app, &jpeg.eobCounter))
			}
		}
	case byte(mcu.Cr):
		for _, row := range blocks {
			for _, elm := range row {
				shared.MergeInto(res, mcu.GetRefinementHist(elm.Cr, app, &jpeg.eobCounter))
			}
		}
	}
	if jpeg.eobCounter != 0 {
		ssss := shared.FindCategory(int16(jpeg.eobCounter)) - 1
		res[uint16(ssss)<<4]++
		jpeg.eobCounter = 0
	}

	return res
}
