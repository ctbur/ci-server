.PHONY: build dev

build:
	CGO_ENABLED=0 GOOS=linux go build -o ./build/server ./cmd/server/
	tar -czvf ./build/ci-server.tar.gz ./build/server ./migrations ./ui

dev:
	CI_SERVER_DEV=1 go run ./cmd/server
