# Define the name for the output binary
BINARY_NAME=civitai-downloader

# Define the path to the main package
MAIN_PKG=./cmd/civitai-downloader

# Define the Go command
GO=go

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build -o $(BINARY_NAME) $(MAIN_PKG)
	@echo "$(BINARY_NAME) built successfully."

# Run the application (passes arguments after --)
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME) $(ARGS)

# Run tests
test:
	@echo "Running tests..."
	$(GO) test ./... -v

# Run integration tests (including database tests)
test-integration:
	@echo "Running integration tests..."
	$(GO) test ./... -v -timeout 30s

# Run short tests only (skips integration tests)
test-short:
	@echo "Running short tests..."
	$(GO) test ./... -v -short

# Run linters with golangci-lint (install if needed)
lint:
	@which golangci-lint > /dev/null || { echo "Installing golangci-lint..."; $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; }
	@echo "Running golangci-lint..."
	golangci-lint run

# Run security scanner
security:
	@which gosec > /dev/null || { echo "Installing gosec..."; $(GO) install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest; }
	@echo "Running security scan..."
	gosec ./...

# Run Go fmt
fmt:
	@echo "Running go fmt..."
	$(GO) fmt ./...

# Run Go vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

# Run Go mod tidy
tidy:
	@echo "Running go mod tidy..."
	$(GO) mod tidy

# Run all quality checks (fmt, vet, lint, security, test)
check: fmt vet lint security test-short
	@echo "All quality checks completed."

# Run full CI pipeline (includes integration tests)
ci: fmt vet lint security test-integration
	@echo "Full CI pipeline completed."

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -rf ./release
	@echo "Clean complete."

# Build release binaries for multiple platforms
release: clean
	@echo "Building release binaries..."
	@mkdir -p release
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-linux-amd64 $(MAIN_PKG)
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-linux-arm64 $(MAIN_PKG)
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PKG)
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-darwin-amd64 $(MAIN_PKG)
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-darwin-arm64 $(MAIN_PKG)
	@echo "Creating compressed archives..."
	cd release && tar -czf $(BINARY_NAME)-linux-amd64.tar.gz $(BINARY_NAME)-linux-amd64 && rm $(BINARY_NAME)-linux-amd64
	cd release && tar -czf $(BINARY_NAME)-linux-arm64.tar.gz $(BINARY_NAME)-linux-arm64 && rm $(BINARY_NAME)-linux-arm64
	cd release && zip $(BINARY_NAME)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe && rm $(BINARY_NAME)-windows-amd64.exe
	cd release && zip $(BINARY_NAME)-darwin-amd64.zip $(BINARY_NAME)-darwin-amd64 && rm $(BINARY_NAME)-darwin-amd64
	cd release && zip $(BINARY_NAME)-darwin-arm64.zip $(BINARY_NAME)-darwin-arm64 && rm $(BINARY_NAME)-darwin-arm64
	@echo "Release archives created successfully in ./release directory:"
	@ls -la release/

# Show help message
help:
	@echo "Available targets:"
	@echo "  build           - Build the application"
	@echo "  run             - Build and run the application (use ARGS=\"...\" to pass arguments)"
	@echo "  test            - Run all tests"
	@echo "  test-short      - Run short tests only (skip integration tests)"
	@echo "  test-integration- Run all tests including integration tests"
	@echo "  fmt             - Format Go code"
	@echo "  vet             - Run go vet"
	@echo "  lint            - Run golangci-lint (installs if needed)"
	@echo "  security        - Run gosec security scanner (installs if needed)"
	@echo "  tidy            - Run go mod tidy"
	@echo "  check           - Run fmt, vet, lint, security, and short tests"
	@echo "  ci              - Run full CI pipeline (fmt, vet, lint, security, integration tests)"
	@echo "  clean           - Clean build artifacts"
	@echo "  release         - Build and compress release binaries (.tar.gz for Linux, .zip for Windows/macOS)"
	@echo "  help            - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make run ARGS=\"download --help\""
	@echo "  make check        # Quick quality checks"
	@echo "  make ci           # Full CI pipeline"

# Default target
all: build

# Phony targets (targets that don't represent files)
.PHONY: all build run test test-integration test-short lint security fmt vet tidy check ci clean release help 