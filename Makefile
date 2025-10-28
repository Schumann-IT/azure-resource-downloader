.PHONY: build install clean test run help

# Binary name
BINARY_NAME=azure-rd

# Build the application
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) .
	@echo "✅ Build complete: ./$(BINARY_NAME)"

# Install the application globally
install:
	@echo "📦 Installing $(BINARY_NAME)..."
	@go install .
	@echo "✅ Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -rf output/
	@echo "✅ Clean complete"

# Run tests
test:
	@echo "🧪 Running tests..."
	@go test -v ./...

# Download dependencies
deps:
	@echo "📥 Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "✅ Dependencies updated"

# Run the application with example args
run:
	@echo "🚀 Running $(BINARY_NAME)..."
	@go run . --help

# Format code
fmt:
	@echo "🎨 Formatting code..."
	@go fmt ./...
	@echo "✅ Code formatted"

# Lint code (requires golangci-lint)
lint:
	@echo "🔍 Linting code..."
	@golangci-lint run ./...
	@echo "✅ Linting complete"

# Display help
help:
	@echo "Azure Resource Downloader - Makefile commands:"
	@echo ""
	@echo "  make build    - Build the binary"
	@echo "  make install  - Install globally to GOPATH/bin"
	@echo "  make clean    - Remove build artifacts"
	@echo "  make test     - Run tests"
	@echo "  make deps     - Download and tidy dependencies"
	@echo "  make run      - Run the application"
	@echo "  make fmt      - Format code"
	@echo "  make lint     - Lint code (requires golangci-lint)"
	@echo "  make help     - Display this help message"

# Default target
.DEFAULT_GOAL := build

