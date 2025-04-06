.PHONY: build clean run run-debug test lint vet fmt help install check all tidy

BINARY_NAME=codezilla
BUILD_DIR=build
LOG_DIR=logs
GO=go
GOFLAGS=-trimpath
CMD_DIR=cmd/codezilla
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.versionInfo=$(VERSION)"

all: check build

help:
	@echo "Available commands:"
	@echo "  make build      - Build the application"
	@echo "  make install    - Install the application to GOPATH/bin"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make run        - Run the application"
	@echo "  make run-debug  - Run with debug logging"
	@echo "  make test       - Run tests"
	@echo "  make lint       - Run linter"
	@echo "  make fmt        - Format code"
	@echo "  make vet        - Run go vet"
	@echo "  make check      - Run fmt, vet and lint"
	@echo "  make all        - Run check and build"
	@echo "  make tidy       - Tidy and verify dependencies"

build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

install:
	$(GO) install $(GOFLAGS) $(LDFLAGS) ./$(CMD_DIR)

clean:
	@echo "Cleaning up..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)

run: build
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

run-debug: build
	@echo "Running $(BINARY_NAME) in debug mode..."
	@mkdir -p $(LOG_DIR)
	@./$(BUILD_DIR)/$(BINARY_NAME) -log $(LOG_DIR)/$(BINARY_NAME).log -log-level debug -log-silent

test:
	$(GO) test -v ./...

test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running golangci-lint..."; \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		golangci-lint run; \
	fi

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

idy:
	$(GO) mod tidy
	$(GO) mod verify

check: tidy fmt vet lint