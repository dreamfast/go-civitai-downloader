package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	// Correct the import path based on the new location of models
	// Assuming models is in internal/models relative to projectRoot
	"go-civitai-download/internal/models"

	"github.com/stretchr/testify/require"
)

// runCommand executes the downloader binary with given arguments
// binaryPath and projectRoot are global variables defined in main_test.go
func runCommand(t *testing.T, args ...string) (string, string, error) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = projectRoot // Run command from project root

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err != nil {
		t.Logf("Command failed with error: %v\nStderr:\n%s", err, stderr.String())
	}

	return stdout.String(), stderr.String(), err
}

// createTempConfig creates a temporary TOML config file
func createTempConfig(t *testing.T, content string) string {
	t.Helper()
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "temp_config.toml")
	err := os.WriteFile(tempFile, []byte(content), 0644)
	require.NoError(t, err, "Failed to write temporary config file")
	return tempFile
}

// parseShowConfigOutput parses the JSON output of 'debug show-config'
func parseShowConfigOutput(t *testing.T, output string) models.Config {
	t.Helper()
	var cfg models.Config
	err := json.Unmarshal([]byte(output), &cfg)
	if err != nil {
		t.Logf("Failed to unmarshal JSON output:\n%s", output)
	}
	require.NoError(t, err, "Failed to parse JSON output from debug show-config")
	return cfg
}
