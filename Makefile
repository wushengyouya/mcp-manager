APP_NAME := mcp-manager
BIN_DIR := bin
MAIN := ./cmd/server

.PHONY: build run test test-e2e test-race test-matrix test-pg clean vet cover swagger docker omx-doctor omx-report

MATRIX_PACKAGES := ./internal/database ./internal/repository ./internal/bootstrap ./tests/integration

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP_NAME) $(MAIN)

run:
	go run $(MAIN)

test:
	go test -cover ./...
	go test ./tests/integration ./tests/e2e
	go test -race ./...

test-e2e:
	go test ./tests/integration ./tests/e2e

test-race:
	go test -race ./...

test-matrix:
	go test $(MATRIX_PACKAGES)

test-pg:
	@if [ -z "$$MCP_TEST_POSTGRES_DSN" ]; then \
		echo "skip: MCP_TEST_POSTGRES_DSN 未设置，跳过 PostgreSQL matrix"; \
	else \
		go test $(MATRIX_PACKAGES); \
	fi

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
