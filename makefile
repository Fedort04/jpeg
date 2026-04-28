JPG_PROGRESSIVE := $(wildcard decoder/pics/Progressive/*.jpg decoder/pics/Progressive/*.jpeg)
JPG_BASELINE := $(wildcard decoder/pics/Baseline/*.jpg decoder/pics/Baseline/*.jpeg)

all:
	go run main.go ${JPG_PROGRESSIVE} ${JPG_BASELINE}

ProgressiveAll:
	go run main.go ${JPG_PROGRESSIVE}

BaselineAll:
	go run main.go ${JPG_BASELINE}
