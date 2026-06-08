package shared

import (
	"errors"
	"testing"
)

func TestClamp255Int(t *testing.T) {
	tests := []struct {
		name string
		val  int
		want byte
	}{
		{"Нижняя граница", -100, 0},
		{"Минимальное допустимое", 0, 0},
		{"Середина", 128, 128},
		{"Максимальное допустимое", 255, 255},
		{"Верхняя граница", 300, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clamp255Int(tt.val); got != tt.want {
				t.Errorf("Clamp255Int(%d) = %d, ожидалось %d", tt.val, got, tt.want)
			}
		})
	}
}

func TestClamp255Float(t *testing.T) {
	tests := []struct {
		name string
		val  float64
		want float32
	}{
		{"Нижняя граница", -10.5, 0},
		{"Минимальное допустимое", 0.0, 0},
		{"Середина", 128.3, 128.3},
		{"Максимальное допустимое", 255.0, 255},
		{"Верхняя граница", 300.7, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clamp255Float(tt.val); got != tt.want {
				t.Errorf("Clamp255Float(%f) = %f, ожидалось %f", tt.val, got, tt.want)
			}
		})
	}
}

func TestFindCategory(t *testing.T) {
	tests := []struct {
		val  int16
		want byte
	}{
		{0, 0},
		{1, 1},
		{-1, 1},
		{2, 2},
		{-2, 2},
		{3, 2},
		{4, 3},
		{7, 3},
		{8, 4},
		{15, 4},
		{30000, 15},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := FindCategory(tt.val); got != tt.want {
				t.Errorf("FindCategory(%d) = %d, ожидалось %d", tt.val, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		v    int16
		app  byte
		want int16
	}{
		{100, 0, 100},
		{100, 1, 50},
		{100, 2, 25},
		{99, 1, 49},
		{-100, 1, -50},
		{0, 5, 0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := Truncate(tt.v, tt.app); got != tt.want {
				t.Errorf("Truncate(%d, %d) = %d, ожидалось %d", tt.v, tt.app, got, tt.want)
			}
		})
	}
}

func TestCheckHistory(t *testing.T) {
	tests := []struct {
		val  int16
		app  byte
		want bool
	}{
		{8, 3, true},   // 8 >> 3 = 1
		{9, 3, true},   // 9 >> 3 = 1
		{15, 3, true},  // 15 >> 3 = 1
		{7, 3, false},  // 7 >> 3 = 0
		{-8, 3, true},  // abs = 8 >> 3 = 1
		{-7, 3, false}, // abs = 7 >> 3 = 0
		{0, 5, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := CheckHistory(tt.val, tt.app); got != tt.want {
				t.Errorf("CheckHistory(%d, %d) = %v, ожидалось %v", tt.val, tt.app, got, tt.want)
			}
		})
	}
}

func TestMatrixMap(t *testing.T) {
	matrix := [][]int{
		{1, 2},
		{3, 4},
	}

	MatrixMap(matrix, func(elm *int) {
		*elm = *elm * 2
	})

	expected := [][]int{
		{2, 4},
		{6, 8},
	}
	for i := range expected {
		for j := range expected[i] {
			if matrix[i][j] != expected[i][j] {
				t.Errorf("matrix[%d][%d] = %d, ожидалось %d", i, j, matrix[i][j], expected[i][j])
			}
		}
	}
}

func TestMatrixMapError(t *testing.T) {
	t.Run("Без ошибки", func(t *testing.T) {
		matrix := [][]int{{1, 2}, {3, 4}}
		err := MatrixMapError(matrix, func(elm *int) error {
			*elm += 1
			return nil
		})
		if err != nil {
			t.Errorf("Ошибка не ожидалась, получена %v", err)
		}
		expected := [][]int{{2, 3}, {4, 5}}
		for i := range expected {
			for j := range expected[i] {
				if matrix[i][j] != expected[i][j] {
					t.Errorf("matrix[%d][%d] = %d, ожидалось %d", i, j, matrix[i][j], expected[i][j])
				}
			}
		}
	})

	t.Run("С ошибкой", func(t *testing.T) {
		matrix := [][]int{{1, 2}, {3, 4}}
		curErr := errors.New("test error")
		err := MatrixMapError(matrix, func(elm *int) error {
			if *elm == 3 {
				return curErr
			}
			return nil
		})
		if err != curErr {
			t.Errorf("Ожидалась ошибка %v, получена %v", curErr, err)
		}
	})
}

func TestMatrixMapRows(t *testing.T) {
	t.Run("Обработка первых двух строк", func(t *testing.T) {
		matrix := [][]int{
			{1, 2},
			{3, 4},
			{5, 6},
		}
		err := MatrixMapRows(matrix, 0, 2, func(elm *int) error {
			*elm *= 10
			return nil
		})
		if err != nil {
			t.Fatalf("Не ожидалась ошибка: %v", err)
		}
		expected := [][]int{
			{10, 20},
			{30, 40},
			{5, 6}, // третья строка не изменена
		}
		for i := range expected {
			for j := range expected[i] {
				if matrix[i][j] != expected[i][j] {
					t.Errorf("matrix[%d][%d] = %d, ожидалось %d", i, j, matrix[i][j], expected[i][j])
				}
			}
		}
	})
}
