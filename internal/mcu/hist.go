package mcu

import "jpeg/shared"

// Структура конфигурации для вычисления гистограммы
type ProgressiveConfig struct {
	Row []int16
	SS  byte
	SE  byte
	App byte
}

// Получение гистограммы по конфигурации с выполнением действий до операции и после операции, если остались нули
// before - опциональная функция
func ChannelHist(res map[uint16]int, config *ProgressiveConfig, before func(cfg *ProgressiveConfig, zeroCounter, count *byte) bool, after func()) {
	var zeroCounter, count byte
	if before != nil {
		if !before(config, &zeroCounter, &count) {
			return
		}
	}

	for ; count <= config.SE; count++ {
		val := shared.Truncate(config.Row[count], config.App)
		if val == 0 {
			zeroCounter++
			continue
		}

		for zeroCounter >= shared.MaxZeros {
			res[shared.ZRL]++
			zeroCounter -= shared.MaxZeros
		}

		ssss := shared.FindCategory(val)
		rs := uint16((zeroCounter << 4) | ssss)
		res[rs]++
		zeroCounter = 0
	}

	if zeroCounter > 0 {
		after()
	}
}

// Получение гистограммы частоты встречаемых символов канала ch в отрезке ss-se
func (block *CodingBlock) GetBlockHist(res map[uint16]int, ch, ss, se byte) {
	channel := Channel(ch)
	eobFunc := func() {
		res[shared.EndOfBlock]++
	}
	cfg := ProgressiveConfig{SS: ss, SE: se, App: 0}

	switch channel {
	case Y:
		for _, row := range block.Y {
			cfg.Row = row
			ChannelHist(res, &cfg, nil, eobFunc)
		}
	default:
		cfg.Row = block.Cb
		ChannelHist(res, &cfg, nil, eobFunc)
		cfg.Row = block.Cr
		ChannelHist(res, &cfg, nil, eobFunc)
	}
}

// Получение гистограммы частоты встречаемых символов канала ch для refinement скана
func GetRefinementHist(row []int16, app byte, eobCounter *int) map[uint16]int {
	res := make(map[uint16]int)

	allZero := true
	for k := ApproxSS; k <= ApproxSE; k++ {
		val := shared.Truncate(row[k], app)
		if val != 0 && shared.CheckHistory(row[k], app) {
			allZero = false
			break
		}
	}
	if allZero {
		*eobCounter++
		return res
	}

	var zeroCounter byte
	var afterLast bool

	for k := ApproxSS; k <= ApproxSE; k++ {
		val := shared.Truncate(row[k], app)
		if val == 0 {
			zeroCounter++
			continue
		}

		// ZRL для каждых 16 нулей
		for zeroCounter >= shared.MaxZeros {
			if *eobCounter != 0 {
				ssss := shared.FindCategory(int16(*eobCounter)) - 1
				res[uint16(ssss<<4)]++
				*eobCounter = 0
			}
			res[shared.ZRL]++
			zeroCounter -= shared.MaxZeros
		}

		if shared.CheckHistory(row[k], app) {
			if *eobCounter != 0 {
				ssss := shared.FindCategory(int16(*eobCounter)) - 1
				res[uint16(ssss)<<4]++
				*eobCounter = 0
			}
			res[uint16(zeroCounter<<4)+1]++
			zeroCounter = 0
			afterLast = false
		} else {
			afterLast = true
		}
	}

	if zeroCounter > 0 || afterLast {
		*eobCounter++
	}
	return res
}
