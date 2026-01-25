package decoder

import (
	"fmt"
	"log"
)

// Вывод компоненты в лог
func printComponent(c component) {
	res := fmt.Sprintf("h: %d, v: %d, quant table id: %d\n", c.h, c.v, c.quantTableID)
	log.Println(res)
}

// Вывод таблицы в лог
func printTable(table []byte) {
	res := "\n"
	for i := range sizeOfTable {
		if i%colCount == 0 && i != 0 {
			res += "\n"
		}
		res += fmt.Sprintf("%d\t", table[i])
	}
	log.Println(res)
}

// Использовалась при отладке для печати data unit
func printUnit(table []int16) {
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			fmt.Printf("%d\t", table[i*8+j])
		}
		fmt.Printf("\n")

	}
	fmt.Printf("\n\n")
	// log.Fatal()
}

// Использовалась при отладке для печати результата ОДКП
func printCos(table [][]byte) {
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			fmt.Printf("%d\t", table[i][j])
		}
		fmt.Printf("\n")

	}
	fmt.Printf("\n\n")
}
