.PHONY: all server agent

all: server agent

server:
	CGO_ENABLED=0 GOOS=linux go build -o ./build/server ./cmd/server/

agent:
	CGO_ENABLED=0 GOOS=linux go build -o ./build/agent ./cmd/agent/

compose: server agent
	sudo docker build -f ./images/Dockerfile.server -t ci-server ./build
	sudo docker build -f ./images/Dockerfile.agent -t ci-agent ./build
	sudo docker compose up
