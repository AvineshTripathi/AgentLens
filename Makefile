.PHONY: all build fmt test lint clean docker-build deploy deploy-down

# App name and binary paths
APP_NAME := agentlens
BIN_DIR := bin
CMD_DIR := ./cmd

all: build

# Build the binaries
build:
	@echo "==> Building $(APP_NAME)..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/agentlens $(CMD_DIR)/agentlens
	go build -o $(BIN_DIR)/agentlens-ca $(CMD_DIR)/agentlens-ca

# Format Go source code
fmt:
	@echo "==> Formatting code..."
	go fmt ./...

# Run tests
test:
	@echo "==> Running tests..."
	go test -v ./...

# Run golangci-lint or go vet
lint:
	@echo "==> Linting code..."
	go vet ./...

# Clean build artifacts
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BIN_DIR)

# Build the Docker image
docker-build:
	@echo "==> Building docker image..."
	docker build -t $(APP_NAME):latest .

# Deploy using docker-compose
deploy:
	@echo "==> Starting services with docker-compose..."
	docker-compose up -d --build

# Stop the docker-compose deployment
deploy-down:
	@echo "==> Stopping services..."
	docker-compose down
