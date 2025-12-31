.PHONY: build build-detection test test-unit test-integration check clean install install-deps install-python-deps help

# Default target
all: check

# Build the binary (without detection)
build:
	go build -o bin/nac-service-media .

# Build with auto-detection enabled (requires OpenCV + Python)
build-detection:
	go build -tags=detection -o bin/nac-service-media .

# Install the binary to $GOPATH/bin
install:
	go install .

# Install the binary with detection to $GOPATH/bin
install-detection:
	go install -tags=detection .

# Install system dependencies (Ubuntu/Debian)
install-deps:
	sudo apt-get update
	sudo apt-get install -y ffmpeg libopencv-dev libopencv-contrib-dev build-essential python3 python3-pip

# Install Python dependencies for end detection
install-python-deps:
	pip3 install librosa numpy scipy

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
	@echo "  all                 - Run check (default)"
	@echo "  build               - Build the binary (no detection)"
	@echo "  build-detection     - Build with auto-detection (requires OpenCV + Python)"
	@echo "  install             - Install to GOPATH/bin"
	@echo "  install-detection   - Install with detection to GOPATH/bin"
	@echo "  install-deps        - Install system dependencies (Ubuntu/Debian)"
	@echo "  install-python-deps - Install Python packages for end detection"
	@echo "  check               - Build and run all tests"
	@echo "  test                - Run all tests (unit + integration)"
	@echo "  test-unit           - Run unit tests only"
	@echo "  test-integration    - Run BDD integration tests only"
	@echo "  test-verbose        - Run all tests with verbose output"
	@echo "  clean               - Remove build artifacts"
	@echo "  fmt                 - Format code"
	@echo "  lint                - Run linter"
	@echo "  help                - Show this help"
