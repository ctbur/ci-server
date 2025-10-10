.PHONY: dev lint test build install

dev:
	CI_SERVER_DEV=1 go run ./cmd/server

lint:
	@echo "Checking go.mod..."
	@go mod tidy -diff

	@echo "Checking code format..."
	@output=$$( $(gofmt -d -s) ); \
	if [ -n "$$output" ]; then \
	    echo "--- UNFORMATTED FILES FOUND ---"; \
		echo "$$output"; \
		exit 1; \
	fi

	@echo "Checking gosec..."
	@gosec --quiet ./...

test:
	go test ./...

build:
	go build -o ./build/ci-server ./cmd/server/

install: build
	./scripts/install.sh
