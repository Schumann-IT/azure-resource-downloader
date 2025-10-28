.PHONY: build install clean test test-coverage run help lint fmt deps check all ci

# Binary name
BINARY_NAME=azure-rd

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOFMT=$(GOCMD) fmt
GOMOD=$(GOCMD) mod

# Build the application
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@$(GOBUILD) -o $(BINARY_NAME) .
	@echo "✅ Build complete: ./$(BINARY_NAME)"

# Install the application globally
install:
	@echo "📦 Installing $(BINARY_NAME)..."
	@$(GOCMD) install .
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
	@$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "🧪 Running tests with coverage..."
	@$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report generated: coverage.html"

# Download dependencies
deps:
	@echo "📥 Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy
	@echo "✅ Dependencies updated"

# Run the application with example args
run:
	@echo "🚀 Running $(BINARY_NAME)..."
	@$(GOCMD) run . --help

# Format code
fmt:
	@echo "🎨 Formatting code..."
	@$(GOFMT) ./...
	@echo "✅ Code formatted"

# Lint code (requires golangci-lint)
lint:
	@echo "🔍 Linting code..."
	@golangci-lint run ./...
	@echo "✅ Linting complete"

# Check code quality (fmt + lint + test)
check: fmt lint test
	@echo "✅ All checks passed"

# Run all quality checks and build (useful for CI/CD)
ci: deps check build
	@echo "✅ CI pipeline complete"

# Run everything (format, lint, test, build)
all: fmt lint test build
	@echo "✅ All tasks complete"

# Display help
help:
	@echo "Azure Resource Downloader - Makefile commands:"
	@echo ""
	@echo "Primary targets:"
	@echo "  make build          - Build the binary"
	@echo "  make install        - Install globally to GOPATH/bin"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make deps           - Download and tidy dependencies"
	@echo "  make run            - Run the application"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Lint code (requires golangci-lint)"
	@echo ""
	@echo "Composite targets:"
	@echo "  make check          - Run fmt + lint + test"
	@echo "  make all            - Run fmt + lint + test + build"
	@echo "  make ci             - Run deps + check + build (for CI/CD)"
	@echo ""
	@echo "  make help           - Display this help message"
	@echo ""
	@echo "⚠️  Always use 'make' targets instead of running commands directly!"
	@echo "    ✅ make lint    (not: golangci-lint run)"
	@echo "    ✅ make test    (not: go test)"
	@echo "    ✅ make build   (not: go build)"

# Default target
.DEFAULT_GOAL := build

