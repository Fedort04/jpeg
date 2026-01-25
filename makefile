all:
	go run main.go
	eog decoder/pics/Baseline/Aqours.bmp&
	eog decoder/pics/Progressive/EikyuuHours.bmp&

Baseline:
	go run main.go decoder/pics/Baseline/Aika.jpg decoder/pics/Baseline/Aqours.jpg decoder/pics/Baseline/Suwa.jpg
	eog decoder/pics/Baseline/Aika.bmp&
	eog decoder/pics/Baseline/Aqours.bmp&
	eog decoder/pics/Baseline/Suwa.bmp&

Progressive:
	go run main.go decoder/pics/Progressive/AqoursProgressive.jpeg
	eog decoder/pics/Progressive/AqoursProgressive.bmp&

# 	eog decoder/pics/Progressive/EikyuuHours.bmp&
