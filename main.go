package main

import (
	"bufio"
	"jpeg/decoder"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Создает директорию по указанному пути и названию
// filePath - полный путь, включая имя файла (например: /home/user/newdir/file.txt)
func CreateDir(basePath string, dirName string) string {
	fullPath := filepath.Join(filepath.Dir(basePath), dirName)
	os.Mkdir(fullPath, 0755)
	return fullPath
}

// Получение названия файла по пути без его расширения
func GetFileName(filePath string) string {
	fileName := filepath.Base(filePath)
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

// Пример из ТЗ для прогрессива
func ProgressiveExample(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err.Error())
	}

	reader := bufio.NewReader(file)

	jpeg, err := decoder.ReadJPEG(reader)
	if err != nil {
		log.Fatal(err.Error())
	}

	if !jpeg.IsProgressive {
		return
	}

	res := decoder.CreateRGBMatrix(jpeg.ImageHeight, jpeg.ImageWidth)

	flag := false
	for !flag {
		flag, err = jpeg.ReadProgJPEG(res, 3)
		if err != nil {
			log.Fatal(err.Error())
		}
		// Действия пользователя после прочтения фрагмента
	}
}

// Чтение прогрессива посканно и запись в новую директорию
func ProgressiveSequence(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err.Error())
	}

	reader := bufio.NewReader(file)

	jpeg, err := decoder.ReadJPEG(reader)
	if err != nil {
		log.Fatal(err.Error())
	}

	if !jpeg.IsProgressive {
		return
	}

	res := decoder.CreateRGBMatrix(jpeg.ImageHeight, jpeg.ImageWidth)

	flag := false
	count := 1
	name := GetFileName(filename)
	path := CreateDir(filename, name+"Sequence")
	for !flag {
		flag, err = jpeg.ReadProgJPEG(res, 1)
		if err != nil {
			log.Fatal(err.Error())
		}
		// Действия пользователя после прочтения фрагмента
		decoder.EncodeBMP(res, path+"/"+name+strconv.Itoa(count)+".bmp")
		count++
	}
}

// Пример из ТЗ для baseline
func BaselineExample(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err.Error())
	}

	reader := bufio.NewReader(file)

	jpeg, err := decoder.ReadJPEG(reader)
	if err != nil {
		log.Fatal(err.Error())
	}

	if jpeg.IsProgressive {
		return
	}

	res := decoder.CreateRGBMatrix(jpeg.ImageHeight, jpeg.ImageWidth)

	flag := false
	for !flag {
		flag, err = jpeg.ReadProgJPEG(res, 3)
		if err != nil {
			log.Fatal(err.Error())
		}
		// Действия пользователя после прочтения фрагмента
	}
}

// Чтение baseline построчно и запись в новую директорию
func BaselineSequence(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err.Error())
	}

	reader := bufio.NewReader(file)

	jpeg, err := decoder.ReadJPEG(reader)
	if err != nil {
		log.Fatal(err.Error())
	}

	if jpeg.IsProgressive {
		return
	}

	res := decoder.CreateRGBMatrix(jpeg.ImageHeight, jpeg.ImageWidth)

	flag := false
	count := 1
	name := GetFileName(filename)
	path := CreateDir(filename, name+"Sequence")
	for !flag {
		flag, err = jpeg.ReadBaseJPEG(res, 100)
		if err != nil {
			log.Fatal(err.Error())
		}
		// Действия пользователя после прочтения фрагмента
		decoder.EncodeBMP(res, path+"/"+name+strconv.Itoa(count)+".bmp")
		count++
	}
}

// Обычное чтение всего изображения сразу
func Common(files []string) {
	for i := 1; i < len(files); i++ {
		file, _ := os.Open(files[i])
		jpeg, _ := decoder.ReadJPEG(bufio.NewReader(file))

		res := decoder.CreateRGBMatrix(jpeg.ImageHeight, jpeg.ImageWidth)

		if jpeg.IsProgressive {
			jpeg.ReadProgJPEG(res, 0)
		} else {
			jpeg.ReadBaseJPEG(res, 0)
		}

		filename, _ := decoder.JpegNameToBmp(files[i], 0)
		decoder.EncodeBMP(res, filename)
	}
}

func main() {
	if len(os.Args) < 2 {
		log.Print("Введите путь к файлу в параметрах\n")
		return
	}

	Common(os.Args)
	for i := 1; i < len(os.Args); i++ {
		// ProgressiveSequence(os.Args[i])
		// BaselineSequence(os.Args[i])
	}
}
