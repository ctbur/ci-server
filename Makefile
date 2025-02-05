.PHONY: server

server:
	CGO_ENABLED=0 GOOS=linux go build -o ./build/server ./cmd/server/

compose: server
	mkdir -p data
	docker build --build-arg USER=$(shell id -u):$(shell id -g) -f ./Dockerfile -t ci-server ./build
	docker compose up
