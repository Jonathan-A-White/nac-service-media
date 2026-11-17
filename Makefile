.PHONY: install-watch install-watch-timer uninstall-watch-timer build build-no-detection test test-unit test-integration check clean install install-no-detection install-deps install-python-deps install-scheduled-task uninstall-scheduled-task update-and-install test-production help

# Default target
all: check

# Build the binary with auto-detection enabled (default, requires OpenCV + Python)
build:
	go build -tags=detection -o bin/nac-service-media .

# Build without detection
build-no-detection:
	go build -o bin/nac-service-media .

# Install the binary with detection to $GOPATH/bin (default), and the Sunday
# watch timers (see install-watch-timer; it skips itself on the church PC).
install: install-watch-timer
	go install -tags=detection .

# Install the binary without detection to $GOPATH/bin
install-no-detection:
	go install .

# The Sunday and Wednesday watch (Laptop): two systemd user timers, Sundays 13:00
# and Wednesdays 22:30 (provisional) America/New_York, running
# `nac-service-media watch --sunday` and `watch --wednesday` from $GOBIN (or $GOPATH/bin).
# It skips itself where the recordings are (the church PC has /mnt/d/Videos) or
# where systemctl --user does not answer; WATCH_TIMER=1 forces it, WATCH_TIMER=0
# skips it (both timers). The way back is `make uninstall-watch-timer`.
UNIT_DIR ?= $(HOME)/.config/systemd/user
install-watch-timer:
	@if [ "$(WATCH_TIMER)" = "0" ]; then echo "WATCH_TIMER=0: not installing the Sunday and Wednesday watch timers"; exit 0; fi; \
	if [ "$(WATCH_TIMER)" != "1" ] && [ -d /mnt/d/Videos ]; then echo "Church PC (/mnt/d/Videos exists): not installing the Sunday and Wednesday watch timers"; exit 0; fi; \
	if ! systemctl --user show-environment >/dev/null 2>&1; then echo "systemctl --user does not answer: not installing the Sunday and Wednesday watch timers"; exit 0; fi; \
	bin="$${GOBIN:-$$(go env GOPATH)/bin}"; \
	mkdir -p "$(UNIT_DIR)"; \
	sed "s|@BIN@|$$bin|" scripts/systemd/nac-service-media-watch.service > "$(UNIT_DIR)/nac-service-media-watch.service"; \
	cp scripts/systemd/nac-service-media-watch.timer "$(UNIT_DIR)/nac-service-media-watch.timer"; \
	sed "s|@BIN@|$$bin|" scripts/systemd/nac-service-media-watch-wednesday.service > "$(UNIT_DIR)/nac-service-media-watch-wednesday.service"; \
	cp scripts/systemd/nac-service-media-watch-wednesday.timer "$(UNIT_DIR)/nac-service-media-watch-wednesday.timer"; \
	systemctl --user daemon-reload; \
	systemctl --user enable --now nac-service-media-watch.timer nac-service-media-watch-wednesday.timer; \
	echo "Sunday and Wednesday watch timers installed:"; systemctl --user list-timers 'nac-service-media-watch*.timer' --no-pager

# Laptop: the binary without detection (OpenCV is not on the Laptop) and the timer.
# Never run this on the church PC: it would replace the detection binary.
install-watch: install-no-detection install-watch-timer

# Turn the Sunday and Wednesday watch off: stop both timers and delete their four unit files.
uninstall-watch-timer:
	-systemctl --user disable --now nac-service-media-watch.timer nac-service-media-watch-wednesday.timer
	rm -f $(UNIT_DIR)/nac-service-media-watch.timer $(UNIT_DIR)/nac-service-media-watch.service $(UNIT_DIR)/nac-service-media-watch-wednesday.timer $(UNIT_DIR)/nac-service-media-watch-wednesday.service
	-systemctl --user daemon-reload

# Default recipient for test-production
RECIPIENT ?= Jonathan

# Pull latest changes and install the binary with detection
update-and-install:
	git pull
	$(MAKE) install

# Update, install, and run the binary in production mode
# Usage: make test-production [RECIPIENT=Name]
test-production: update-and-install
	nac-service-media process --recipient $(RECIPIENT)

# Install system dependencies (Ubuntu/Debian)
install-deps:
	sudo apt-get update
	sudo apt-get install -y ffmpeg libopencv-dev libopencv-contrib-dev build-essential python3 python3-pip
	go install github.com/mikefarah/yq/v4@latest

# Install Python dependencies for end detection
install-python-deps:
	pip3 install librosa numpy scipy

# Install Windows Scheduled Tasks (WSL only - calls PowerShell)
install-scheduled-task: install
	@echo "Installing Windows Scheduled Tasks..."
	@# Ensure PATH includes ~/go/bin in .profile (not .bashrc, which has non-interactive guard)
	@if ! grep -q 'export PATH=.*\$$HOME/go/bin' ~/.profile 2>/dev/null; then \
		echo '' >> ~/.profile; \
		echo '# Go binaries' >> ~/.profile; \
		echo 'export PATH="$$HOME/go/bin:$$PATH"' >> ~/.profile; \
		echo "Added ~/go/bin to PATH in .profile"; \
	else \
		echo "PATH already includes ~/go/bin"; \
	fi
	powershell.exe -ExecutionPolicy Bypass -File "$(CURDIR)/scripts/install-both-tasks.ps1" -SkipBinaryInstall

# Uninstall Windows Scheduled Tasks (WSL only - calls PowerShell)
uninstall-scheduled-task:
	powershell.exe -ExecutionPolicy Bypass -File "$(CURDIR)/scripts/install-both-tasks.ps1" -Uninstall

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
	@echo "  all                      - Run check (default)"
	@echo "  build                    - Build with auto-detection (default, requires OpenCV + Python)"
	@echo "  build-no-detection       - Build without detection"
	@echo "  install                  - Install with detection to GOPATH/bin (default)"
	@echo "  install-no-detection     - Install without detection to GOPATH/bin"
	@echo "  install-watch            - Laptop: install without detection, and the Sunday and Wednesday watch timers"
	@echo "  install-watch-timer      - Install the Sunday and Wednesday watch's systemd user timers (Sundays 13:00, Wednesdays 22:30 America/New_York)"
	@echo "  uninstall-watch-timer    - Remove the Sunday and Wednesday watch's timers"
	@echo "  update-and-install      - Git pull and install with detection"
	@echo "  test-production          - Update, install, and run process (RECIPIENT=Jonathan)"
	@echo "  install-deps             - Install system dependencies (Ubuntu/Debian)"
	@echo "  install-python-deps      - Install Python packages for end detection"
	@echo "  install-scheduled-task   - Install Windows Scheduled Tasks (Sunday & Wednesday) (WSL only)"
	@echo "  uninstall-scheduled-task - Remove Windows Scheduled Tasks (WSL only)"
	@echo "  check                    - Build and run all tests"
	@echo "  test                     - Run all tests (unit + integration)"
	@echo "  test-unit                - Run unit tests only"
	@echo "  test-integration         - Run BDD integration tests only"
	@echo "  test-verbose             - Run all tests with verbose output"
	@echo "  clean                    - Remove build artifacts"
	@echo "  fmt                      - Format code"
	@echo "  lint                     - Run linter"
	@echo "  help                     - Show this help"
