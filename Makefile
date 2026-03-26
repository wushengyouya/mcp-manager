APP_NAME := mcp-manager
BIN_DIR := bin
MAIN := ./cmd/server

.PHONY: build run test test-e2e test-race clean vet cover swagger docker

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP_NAME) $(MAIN)

run:
	go run $(MAIN)

test:
	go test ./...

test-e2e:
	go test ./tests/integration ./tests/e2e

test-race:
	go test -race ./...

vet:
	go vet ./...
cover:
	go test -cover ./...

swagger:
	swag init -g cmd/server/main.go -o api/docs

docker:
	docker build -t $(APP_NAME):latest -f deployments/docker/Dockerfile .

clean:
	rm -rf $(BIN_DIR)
