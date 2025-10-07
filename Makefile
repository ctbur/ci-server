.PHONY: build dev

test:
	go test ./...

build:
	go build -o ./build/ci-server ./cmd/server/

install: build
	./scripts/install.sh

dev:
	CI_SERVER_DEV=1 go run ./cmd/server
