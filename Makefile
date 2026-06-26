APP_NAME=modelmux

.PHONY: run build test tidy

run:
	go run ./cmd/modelmux

build:
	go build -o bin/$(APP_NAME) ./cmd/modelmux

test:
	go test ./...

tidy:
	go mod tidy
