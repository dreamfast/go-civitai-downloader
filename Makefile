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

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -rf ./release
	@echo "Clean complete."

# Build release binaries for multiple platforms
release: clean
	@echo "Building release binaries..."
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-linux-amd64 $(MAIN_PKG)
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-linux-arm64 $(MAIN_PKG)
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PKG)
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-darwin-amd64 $(MAIN_PKG)
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o release/$(BINARY_NAME)-darwin-arm64 $(MAIN_PKG)
	@echo "Release binaries built successfully in ./release directory."

# Default target
all: build

# Phony targets (targets that don't represent files)
.PHONY: all build run test clean release 