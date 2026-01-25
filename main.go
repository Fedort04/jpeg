package main

import (
	"jpeg/decoder"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		log.Print("Введите путь к файлу в параметрах\n")
		return
	}

	for i := 1; i < len(os.Args); i++ {
		decoder.ReadProgressive(os.Args[i])
	}
	// decoder.ReadProgressive("decoder/pics/Progressive/EikyuuHours.jpeg", "decoder/pics/Progressive/EikyuuHours.bmp")
}
