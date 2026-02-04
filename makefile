all:
	go run main.go decoder/pics/Baseline/Aida.jpg decoder/pics/Progressive/EikyuuHours.jpeg
	eog decoder/pics/Baseline/Aida.bmp&
	eog decoder/pics/Progressive/EikyuuHours.bmp&

Baseline:
	go run main.go decoder/pics/Baseline/Aika.jpg decoder/pics/Baseline/Aqours.jpg decoder/pics/Baseline/Suwa.jpg
	eog decoder/pics/Baseline/Aika.bmp&
	eog decoder/pics/Baseline/Aqours.bmp&
	eog decoder/pics/Baseline/Suwa.bmp&

Progressive:
	go run main.go decoder/pics/Progressive/AqoursProgressive.jpeg decoder/pics/Progressive/EikyuuHours.jpeg decoder/pics/Progressive/EikyuuStage.jpeg
	eog decoder/pics/Progressive/AqoursProgressive.bmp&
	eog decoder/pics/Progressive/EikyuuHours.bmp&
	eog decoder/pics/Progressive/EikyuuStage.bmp&

ProgressiveChroma:
	go run main.go decoder/pics/Progressive/EikyuuHours.jpeg
	eog decoder/pics/Progressive/EikyuuHours.bmp&