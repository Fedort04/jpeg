package main

import (
	"bufio"
	"fmt"
	"jpeg/decoder"
	"jpeg/encoder"
	"jpeg/shared"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const encoderBaselinePath = "encoder/pics/Baseline/"
const encoderProgressivePath = "encoder/pics/Progressive/"

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

	res := shared.CreateMatrix[shared.Rgb](int(jpeg.ImageHeight), int(jpeg.ImageWidth))

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
		log.Print("This jpeg is not Progressive")
		return
	}

	res := shared.CreateMatrix[shared.Rgb](int(jpeg.ImageHeight), int(jpeg.ImageWidth))

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

	res := shared.CreateMatrix[shared.Rgb](int(jpeg.ImageHeight), int(jpeg.ImageWidth))

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
		log.Print("This jpeg is Progressive")
		return
	}

	res := shared.CreateMatrix[shared.Rgb](int(jpeg.ImageHeight), int(jpeg.ImageWidth))

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

// Чтение нескольких изображений с записью в .bmp
func CommonAll(files []string) {
	for i := 1; i < len(files); i++ {
		file, _ := os.Open(files[i])
		jpeg, err := decoder.ReadJPEG(bufio.NewReader(file))
		if err != nil {
			log.Fatal(err.Error())
			return
		}

		res := shared.CreateMatrix[shared.Rgb](int(jpeg.ImageHeight), int(jpeg.ImageWidth))

		if jpeg.IsProgressive {
			log.Print("Progressive " + files[i])
			_, err = jpeg.ReadProgJPEG(res, 0)
		} else {
			log.Print("Baseline " + files[i])
			_, err = jpeg.ReadBaseJPEG(res, 0)
		}

		if err != nil {
			log.Fatal(err.Error())
		}

		filename, _ := decoder.JpegNameToBmp(files[i], 0)
		decoder.EncodeBMP(res, filename)
	}
}

// Обычное чтение всего изображения сразу
func Common(files string) shared.Image {
	var err error

	file, _ := os.Open(files)
	jpeg, err := decoder.ReadJPEG(bufio.NewReader(file))
	if err != nil {
		log.Fatal(err.Error())
		return nil
	}

	res := shared.CreateMatrix[shared.Rgb](int(jpeg.ImageHeight), int(jpeg.ImageWidth))

	if jpeg.IsProgressive {
		log.Print("Progressive " + files)
		_, err = jpeg.ReadProgJPEG(res, 0)
	} else {
		log.Print("Baseline " + files)
		_, err = jpeg.ReadBaseJPEG(res, 0)
	}

	if err != nil {
		log.Fatal(err.Error())
	}

	filename, _ := decoder.JpegNameToBmp(files, 0)
	decoder.EncodeBMP(res, filename)
	return res
}

// Для декодера
// func main() {
// 	if len(os.Args) < 2 {
// 		log.Print("Введите путь к файлу в параметрах\n")
// 		return
// 	}

// 	CommonAll(os.Args)
// 	// Common(os.Args[1])
// 	// for i := 1; i < len(os.Args); i++ {
// 	// ProgressiveSequence(os.Args[i])
// 	// BaselineSequence(os.Args[i])
// 	// }
// }

// ==================================================================
// Для кодировщика (использует декодер при тестировании)

// Закодировать изображение
func encodeImage(filepath string, img shared.Image, isProgressive bool) {
	file, err := os.Create(filepath)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	writer := bufio.NewWriter(file)

	quantY := encoder.CreateOneTable()
	quantColor := encoder.CreateOneTable()
	jpgEncoder, err := encoder.CreateEncoder(writer, img, quantY, quantColor)
	jpgEncoder.Format = encoder.Both
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	if isProgressive {
		if _, err := jpgEncoder.StartProgressive(0); err != nil {
			log.Fatal(err.Error())
		}
	} else {
		if _, err := jpgEncoder.StartBaseline(0); err != nil {
			log.Fatal(err.Error())
		}
	}
	writer.Flush()

	Common(filepath)
}

func main() {
	if len(os.Args) < 2 {
		log.Print("Введите путь к файлу в параметрах\n")
		return
	}

	for i := 1; i < len(os.Args); i++ {
		filename := GetFileName(os.Args[i])
		// Тестирование Baseline кодировки
		// encodeImage(encoderBaselinePath+filename+".jpg", Common(os.Args[i]), false)
		encodeImage(encoderProgressivePath+filename+".jpeg", Common(os.Args[i]), true)
	}
}
