.PHONY: libs
libs:
	go mod tidy

.PHONY: update
update:
	go get -u ./...
	make libs

.PHONY: build
build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o shome .
	upx shome

.PHONY: run
run:
	go run . serve
