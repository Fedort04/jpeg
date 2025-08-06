package binwriter

import (
	"bufio"
	"os"
)

var writer *bufio.Writer

func BinwriterInit(fileName string) error {
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	writer = bufio.NewWriter(file)
	return nil
}

func Close() error {
	err := writer.Flush()
	return err
}

func PutInt(num uint) {
	writer.WriteByte(byte(num >> 0))
	writer.WriteByte(byte(num >> 8))
	writer.WriteByte(byte(num >> 16))
	writer.WriteByte(byte(num >> 24))
}

func PutShort(num uint) {
	writer.WriteByte(byte(num >> 0))
	writer.WriteByte(byte(num >> 8))
}

func PutChar(num byte) {
	writer.WriteByte(num)
}
