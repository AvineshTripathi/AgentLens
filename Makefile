.PHONY: all build fmt test lint clean docker-build docker-push deploy deploy-down setup-certs setup-certs-force

# App name and binary paths
APP_NAME := agentlens
BIN_DIR := bin
CMD_DIR := ./cmd
DOCKER_IMAGE ?= $(APP_NAME):latest

all: build

# Build the binaries
build:
	@echo "==> Building $(APP_NAME)..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/agentlens $(CMD_DIR)/agentlens
	go build -o $(BIN_DIR)/agentlens-ca $(CMD_DIR)/agentlens-ca

# Generate CA certificates
setup-certs: build
	@echo "==> Generating CA certificates..."
	./$(BIN_DIR)/agentlens-ca

# Force regenerate CA certificates
setup-certs-force: build
	@echo "==> Force regenerating CA certificates..."
	./$(BIN_DIR)/agentlens-ca --force

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
	docker build -t $(DOCKER_IMAGE) .

# Push the Docker image
docker-push: docker-build
	@echo "==> Pushing docker image..."
	docker push $(DOCKER_IMAGE)

# Deploy using docker-compose
deploy: setup-certs
	@echo "==> Starting services with docker-compose..."
	docker-compose up -d --build

# Stop the docker-compose deployment
deploy-down:
	@echo "==> Stopping services..."
	docker-compose down
