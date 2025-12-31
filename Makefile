.PHONY: build test test-unit test-integration check clean install help

# Default target
all: check

# Build the binary
build:
	go build -o bin/nac-service-media .

# Install the binary to $GOPATH/bin
install:
	go install .

# Run all checks (build + tests)
check: build test

# Run all tests
test: test-unit test-integration

# Run unit tests only
test-unit:
	go test ./...

# Run BDD integration tests only
test-integration:
	go test -tags=integration ./features/...

# Run tests with verbose output
test-verbose:
	go test -v ./...
	go test -v -tags=integration ./features/...

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Format code
fmt:
	go fmt ./...

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Show help
help:
	@echo "Available targets:"
	@echo "  all              - Run check (default)"
	@echo "  build            - Build the binary"
	@echo "  install          - Install to GOPATH/bin"
	@echo "  check            - Build and run all tests"
	@echo "  test             - Run all tests (unit + integration)"
	@echo "  test-unit        - Run unit tests only"
	@echo "  test-integration - Run BDD integration tests only"
	@echo "  test-verbose     - Run all tests with verbose output"
	@echo "  clean            - Remove build artifacts"
	@echo "  fmt              - Format code"
	@echo "  lint             - Run linter"
	@echo "  help             - Show this help"
