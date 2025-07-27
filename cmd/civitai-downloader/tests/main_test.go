package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// --- Test Setup ---

var (
	binaryName            = "civitai-downloader"
	binaryPath            string
	projectRoot           string
	originalConfigContent []byte
)

// TestMain runs setup before all tests in the package
func TestMain(m *testing.M) {
	// Find project root (assuming tests run from within the cmd directory or project root)
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Println("Could not get caller information")
		os.Exit(1)
	}
	// Navigate up from cmd/civitai-downloader/tests
	projectRoot = filepath.Join(filepath.Dir(filename), "..", "..", "..") // Adjusted path

	// Build the binary
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath = filepath.Join(projectRoot, binaryName)
	fmt.Println("Building binary for integration tests...")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	// Assumes the main package to build is in cmd/civitai-downloader relative to projectRoot
	buildCmd.Dir = filepath.Join(projectRoot, "cmd", "civitai-downloader")
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to build binary: %v\nOutput:\n%s\n", err, string(buildOutput))
		os.Exit(1)
	}
	fmt.Println("Binary built successfully:", binaryPath)

	// Backup original config.toml (though we prefer temp files now)
	configPath := filepath.Join(projectRoot, "config.toml")
	originalConfigContent, err = os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Warning: Could not read original config.toml: %v\n", err)
		originalConfigContent = nil // Ensure it's nil if read fails
	}

	// Run tests
	exitCode := m.Run()

	// Cleanup: Restore original config.toml if backed up
	if originalConfigContent != nil {
		err = os.WriteFile(configPath, originalConfigContent, 0644)
		if err != nil {
			fmt.Printf("Warning: Failed to restore original config.toml: %v\n", err)
		}
	}
	// Optional: remove built binary
	// os.Remove(binaryPath)

	os.Exit(exitCode)
}
