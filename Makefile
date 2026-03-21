# NanoClaw Makefile
.PHONY: build build-setup build-auth test run clean container-build fmt lint

BINARY_NAME=nanoclaw
BUILD_DIR=bin
GO_FILES=$(shell find . -name "*.go")

build: $(BUILD_DIR)/$(BINARY_NAME) build-setup build-auth

$(BUILD_DIR)/$(BINARY_NAME): $(GO_FILES)
	@echo "Building Go binary: $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/nanoclaw

build-setup:
	@echo "Building Go binary: setup..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -o $(BUILD_DIR)/setup ./cmd/setup

build-auth:
	@echo "Building Go binary: whatsapp-auth..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -o $(BUILD_DIR)/whatsapp-auth ./cmd/whatsapp-auth

test:
	@echo "Running Go tests..."
	go test ./pkg/...

run: build
	@echo "Starting NanoClaw..."
	./$(BUILD_DIR)/$(BINARY_NAME)

auth: build-auth
	@echo "Starting WhatsApp authentication..."
	./$(BUILD_DIR)/whatsapp-auth

fmt:
	@echo "Formatting Go code..."
	go fmt ./...

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)

container-build:
	@echo "Building agent container..."
	./container/build.sh
