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

.PHONY: all build clean test install version help install-completion uninstall-completion completion-bash completion-zsh completion-fish completion-powershell verify-completion

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

# Shell completion targets
install-completion: install
	@echo "Installing shell completion..."
	@if [ "$$SHELL" = "/bin/zsh" ] || [ "$$SHELL" = "/usr/bin/zsh" ]; then \
		echo "Installing zsh completion..."; \
		make completion-zsh; \
	elif [ "$$SHELL" = "/bin/bash" ] || [ "$$SHELL" = "/usr/bin/bash" ]; then \
		echo "Installing bash completion..."; \
		make completion-bash; \
	elif [ "$$SHELL" = "/usr/bin/fish" ] || [ "$$SHELL" = "/bin/fish" ]; then \
		echo "Installing fish completion..."; \
		make completion-fish; \
	else \
		echo "Unsupported shell: $$SHELL"; \
		echo "Please manually install completion using one of:"; \
		echo "  make completion-bash"; \
		echo "  make completion-zsh"; \
		echo "  make completion-fish"; \
		echo "  make completion-powershell"; \
	fi

completion-bash:
	@echo "Generating bash completion..."
	@which $(BINARY_NAME) >/dev/null 2>&1 || { \
		echo "Error: $(BINARY_NAME) binary not found in PATH. Run 'make install' first."; \
		exit 1; \
	}
	@mkdir -p ~/.local/share/bash-completion/completions
	@$(BINARY_NAME) completion bash > ~/.local/share/bash-completion/completions/$(BINARY_NAME)
	@echo "Bash completion installed to ~/.local/share/bash-completion/completions/$(BINARY_NAME)"
	@echo "You may need to restart your shell or run: source ~/.bashrc"

completion-zsh:
	@echo "Generating zsh completion..."
	@which $(BINARY_NAME) >/dev/null 2>&1 || { \
		echo "Error: $(BINARY_NAME) binary not found in PATH. Run 'make install' first."; \
		exit 1; \
	}
	@mkdir -p ~/.local/share/zsh/completions
	@$(BINARY_NAME) completion zsh > ~/.local/share/zsh/completions/_$(BINARY_NAME)
	@echo "Zsh completion installed to ~/.local/share/zsh/completions/_$(BINARY_NAME)"
	@echo ""
	@if ! grep -q "fpath=.*\.local/share/zsh/completions" ~/.zshrc 2>/dev/null; then \
		echo "Adding completion configuration to ~/.zshrc..."; \
		if grep -q "source.*oh-my-zsh" ~/.zshrc 2>/dev/null; then \
			echo "Detected Oh My Zsh - adding fpath before Oh My Zsh loads..."; \
			sed -i '/source.*oh-my-zsh/i# ene completion\nfpath=(~/.local/share/zsh/completions $$fpath)\n' ~/.zshrc; \
		else \
			echo "# ene completion" >> ~/.zshrc; \
			echo "fpath=(~/.local/share/zsh/completions \$$fpath)" >> ~/.zshrc; \
			echo "autoload -U compinit && compinit" >> ~/.zshrc; \
		fi; \
		echo "Configuration added to ~/.zshrc"; \
	else \
		echo "Completion configuration already exists in ~/.zshrc"; \
	fi
	@echo ""
	@echo "Restart your shell or run: exec zsh"

completion-fish:
	@echo "Generating fish completion..."
	@which $(BINARY_NAME) >/dev/null 2>&1 || { \
		echo "Error: $(BINARY_NAME) binary not found in PATH. Run 'make install' first."; \
		exit 1; \
	}
	@mkdir -p ~/.config/fish/completions
	@$(BINARY_NAME) completion fish > ~/.config/fish/completions/$(BINARY_NAME).fish
	@echo "Fish completion installed to ~/.config/fish/completions/$(BINARY_NAME).fish"
	@echo "Completion should be available immediately in new fish sessions"

completion-powershell:
	@echo "Generating PowerShell completion..."
	@which $(BINARY_NAME) >/dev/null 2>&1 || { \
		echo "Error: $(BINARY_NAME) binary not found in PATH. Run 'make install' first."; \
		exit 1; \
	}
	@$(BINARY_NAME) completion powershell > $(BINARY_NAME).ps1
	@echo "PowerShell completion generated as $(BINARY_NAME).ps1"
	@echo "To install, run in PowerShell: Import-Module ./$(BINARY_NAME).ps1"

uninstall-completion:
	@echo "Removing completion files..."
	@rm -f ~/.local/share/bash-completion/completions/$(BINARY_NAME)
	@rm -f ~/.local/share/zsh/completions/_$(BINARY_NAME)
	@rm -f ~/.config/fish/completions/$(BINARY_NAME).fish
	@rm -f $(BINARY_NAME).ps1
	@echo "Removing completion configuration from ~/.zshrc..."
	@if [ -f ~/.zshrc ]; then \
		sed -i '/# ene completion/,+1d' ~/.zshrc; \
		echo "Configuration removed from ~/.zshrc"; \
	fi
	@echo "Completion files removed"

# Verify completion functionality
verify-completion:
	@echo "Verifying ENE completion functionality..."
	@./verify_completion.sh

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
	@echo "  install-completion - Auto-detect shell and install completion (requires 'make install' first)"
	@echo "  completion-bash - Generate and install bash completion"
	@echo "  completion-zsh - Generate and install zsh completion (auto-configures ~/.zshrc)"
	@echo "  completion-fish - Generate and install fish completion"
	@echo "  completion-powershell - Generate PowerShell completion"
	@echo "  uninstall-completion - Remove all completion files"
	@echo "  verify-completion - Test completion functionality"
	@echo "  help        - Show this help message"