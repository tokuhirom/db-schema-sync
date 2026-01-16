# Makefile for db-schema-sync

# Variables
BINARY_NAME=db-schema-sync
MAIN_FILE=./cmd/db-schema-sync

# Default target
all: build

# Build the application
build:
	go build -o $(BINARY_NAME) $(MAIN_FILE)

# Run tests
test:
	go test ./...

# Run integration tests (requires Docker)
test-integration:
	go test -tags=integration ./...

# Run linter
lint:
	golangci-lint run

# Run the application (requires environment variables to be set)
run:
	go run $(MAIN_FILE)

# Install dependencies
deps:
	go mod tidy

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)

# Help target
help:
	@echo "Available targets:"
	@echo "  all              - Build the application (default)"
	@echo "  build            - Build the application"
	@echo "  test             - Run tests"
	@echo "  test-integration - Run integration tests (requires Docker)"
	@echo "  lint             - Run linter"
	@echo "  run              - Run the application locally"
	@echo "  deps             - Install/update dependencies"
	@echo "  clean            - Clean build artifacts"
	@echo "  help             - Show this help message"

.PHONY: all build test test-integration lint run deps clean help
