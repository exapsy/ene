# Makefile for ene project

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build information
BINARY_NAME = ene
BUILD_DIR = bin
LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
GOMOD = $(GOCMD) mod

.PHONY: all build clean test install version help

# Default target
all: build

# Build the binary
build:
	@echo "Building $(BINARY_NAME) version $(VERSION)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Build and install to GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME) version $(VERSION)"
	$(GOCMD) install $(LDFLAGS) .

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts"
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)

# Run tests
test:
	@echo "Running tests"
	$(GOTEST) -v ./...

# Tidy go modules
tidy:
	@echo "Tidying go modules"
	$(GOMOD) tidy

# Download dependencies
deps:
	@echo "Downloading dependencies"
	$(GOMOD) download

# Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	@echo "Building for Linux"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .

build-darwin:
	@echo "Building for macOS"
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .

build-windows:
	@echo "Building for Windows"
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

# Show version information
version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Date: $(DATE)"

# Development build (same as build but more verbose)
dev: clean build
	@echo "Development build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Quick development install
dev-install: clean install
	@echo "Development install complete"

# Help target
help:
	@echo "Available targets:"
	@echo "  build       - Build the binary"
	@echo "  install     - Build and install to GOPATH/bin"
	@echo "  clean       - Clean build artifacts"
	@echo "  test        - Run tests"
	@echo "  tidy        - Tidy go modules"
	@echo "  deps        - Download dependencies"
	@echo "  build-all   - Build for all platforms"
	@echo "  build-linux - Build for Linux"
	@echo "  build-darwin- Build for macOS"
	@echo "  build-windows- Build for Windows"
	@echo "  version     - Show version information"
	@echo "  dev         - Clean and build for development"
	@echo "  dev-install - Clean and install for development"
	@echo "  help        - Show this help message"