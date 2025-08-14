.PHONY: server

build:
	CGO_ENABLED=0 GOOS=linux go build -o ./build/server ./cmd/server/

dev:
	go run ./cmd/server
