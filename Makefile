.PHONY: server

server:
	CGO_ENABLED=0 GOOS=linux go build -o ./build/server ./cmd/server/

compose: server
	sudo docker build -f ./Dockerfile -t ci-server ./build
	sudo docker compose up
