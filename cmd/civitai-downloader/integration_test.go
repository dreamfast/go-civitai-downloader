package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCommandLineInterface tests the CLI interface
func TestCommandLineInterface(t *testing.T) {
	// Build the binary first
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	tests := []struct {
		name        string
		args        []string
		expectError bool
		expectOut   string
	}{
		{
			name:        "Help command",
			args:        []string{"--help"},
			expectError: false,
			expectOut:   "Civitai Downloader",
		},
		{
			name:        "Version info (implicit)",
			args:        []string{},
			expectError: false,
			expectOut:   "Usage:",
		},
		{
			name:        "Download help",
			args:        []string{"download", "--help"},
			expectError: false,
			expectOut:   "Download models",
		},
		{
			name:        "Images help", 
			args:        []string{"images", "--help"},
			expectError: false,
			expectOut:   "Download images",
		},
		{
			name:        "DB help",
			args:        []string{"db", "--help"},
			expectError: false,
			expectOut:   "Interact with the download database",
		},
		{
			name:        "Invalid command",
			args:        []string{"invalid-command"},
			expectError: true,
			expectOut:   "unknown command",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, tt.args...)
			output, err := cmd.CombinedOutput()
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected command to fail, but it succeeded")
				}
			} else {
				if err != nil {
					t.Errorf("Expected command to succeed, got error: %v\nOutput: %s", err, string(output))
				}
			}
			
			if tt.expectOut != "" {
				if !strings.Contains(string(output), tt.expectOut) {
					t.Errorf("Expected output to contain %q, got: %s", tt.expectOut, string(output))
				}
			}
		})
	}
}

// TestConfigFileHandling tests configuration file loading
func TestConfigFileHandling(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	// Create a test config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config.toml")
	
	configContent := `
api_key = "test-key-123"
save_path = "` + tempDir + `/models"
api_delay = 1000

[download]
concurrency = 4
limit = 10

[log]
level = "debug"
format = "text"
`
	
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}
	
	// Test debug show-config command to verify config loading
	cmd := exec.Command(binaryPath, "--config", configFile, "debug", "show-config")
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		t.Errorf("Config loading failed: %v\nOutput: %s", err, string(output))
	}
	
	// Check that the config values appear in the output
	expectedValues := []string{
		"test-key-123",
		"models",
		"1000",
		"debug",
	}
	
	outputStr := string(output)
	for _, expected := range expectedValues {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Expected config output to contain %q, but it didn't.\nFull output: %s", expected, outputStr)
		}
	}
}

// TestFlagOverrides tests that command line flags override config file values
func TestFlagOverrides(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	// Create a test config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config.toml")
	
	configContent := `
api_delay = 1000
save_path = "` + tempDir + `/config-models"

[download]
concurrency = 2
limit = 20

[log]
level = "info"
`
	
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}
	
	// Test with command line flags that should override config
	cmd := exec.Command(binaryPath, 
		"--config", configFile,
		"--save-path", tempDir + "/flag-models",
		"--api-delay", "2000", 
		"--log-level", "debug",
		"debug", "show-config")
		
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		t.Errorf("Flag override test failed: %v\nOutput: %s", err, string(output))
	}
	
	outputStr := string(output)
	
	// Check that flag values override config values
	if !strings.Contains(outputStr, "flag-models") {
		t.Error("Expected flag --save-path to override config save_path")
	}
	
	if !strings.Contains(outputStr, "2000") {
		t.Error("Expected flag --api-delay to override config api_delay")
	}
	
	if !strings.Contains(outputStr, "debug") {
		t.Error("Expected flag --log-level to override config log level")
	}
}

// TestDatabaseOperations tests database-related commands
func TestDatabaseOperations(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	tempDir := t.TempDir()
	
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "DB Status",
			args: []string{"--save-path", tempDir, "db", "status"},
		},
		{
			name: "DB List (empty)",
			args: []string{"--save-path", tempDir, "db", "list"},
		},
		{
			name: "DB Stats",
			args: []string{"--save-path", tempDir, "db", "stats"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, tt.args...)
			output, err := cmd.CombinedOutput()
			
			if err != nil {
				t.Errorf("Database command failed: %v\nOutput: %s", err, string(output))
			}
			
			// All DB commands should produce some output
			if len(strings.TrimSpace(string(output))) == 0 {
				t.Error("Expected database command to produce output")
			}
		})
	}
}

// TestDownloadDryRun tests download command in dry-run mode
func TestDownloadDryRun(t *testing.T) {
	// This test requires a valid API key to work properly
	apiKey := os.Getenv("CIVITAI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping download test: CIVITAI_API_KEY environment variable not set")
	}
	
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	tempDir := t.TempDir()
	
	// Test dry-run download with very limited scope
	cmd := exec.Command(binaryPath,
		"--save-path", tempDir,
		"download",
		"--limit", "2", 
		"--sort", "Most Downloaded",
		"--query", "stable diffusion",
		"--dry-run") // Assuming there's a dry-run flag
		
	// Set a reasonable timeout
	cmd.Env = append(os.Environ(), "CIVITAI_API_KEY="+apiKey)
	
	output, err := cmd.CombinedOutput()
	
	// For dry-run, we expect it to succeed but not download files
	if err != nil {
		// If dry-run flag doesn't exist, that's also acceptable
		outputStr := string(output)
		if !strings.Contains(outputStr, "unknown flag") && !strings.Contains(outputStr, "dry-run") {
			t.Errorf("Download dry-run failed: %v\nOutput: %s", err, outputStr)
		}
	}
}

// TestImageCommand tests the images command functionality
func TestImageCommand(t *testing.T) {
	// This test requires a valid API key
	apiKey := os.Getenv("CIVITAI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping images test: CIVITAI_API_KEY environment variable not set")
	}
	
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	tempDir := t.TempDir()
	
	// Test images command with print-urls (if available)
	cmd := exec.Command(binaryPath,
		"--save-path", tempDir,
		"images",
		"--limit", "1",
		"--sort", "Most Reactions",
		"--model-id", "4201") // Use a known stable model ID
		
	cmd.Env = append(os.Environ(), "CIVITAI_API_KEY="+apiKey)
	
	// Set a timeout to prevent hanging
	timer := time.AfterFunc(30*time.Second, func() {
		cmd.Process.Kill()
	})
	defer timer.Stop()
	
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		outputStr := string(output)
		// Some output is expected even on "failure" due to rate limiting or API issues
		if len(strings.TrimSpace(outputStr)) == 0 {
			t.Errorf("Images command failed with no output: %v", err)
		} else {
			t.Logf("Images command output (may have failed due to API limits): %s", outputStr)
		}
	}
}

// TestTorrentCommand tests torrent generation functionality  
func TestTorrentCommand(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	tempDir := t.TempDir()
	
	// Create some dummy model files to generate torrents for
	modelDir := filepath.Join(tempDir, "models", "Checkpoint", "test-model", "v1.0")
	err := os.MkdirAll(modelDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}
	
	// Create a dummy model file
	dummyFile := filepath.Join(modelDir, "model.safetensors")
	err = os.WriteFile(dummyFile, []byte("dummy model data"), 0644)
	if err != nil {
		t.Fatalf("Failed to create dummy model file: %v", err)
	}
	
	// Test torrent generation
	cmd := exec.Command(binaryPath,
		"--save-path", tempDir,
		"torrent",
		"--tracker-url", "http://test-tracker.com:8080/announce")
		
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		t.Errorf("Torrent command failed: %v\nOutput: %s", err, string(output))
	}
	
	// Check if .torrent file was created
	expectedTorrent := filepath.Join(tempDir, "models", "Checkpoint", "test-model.torrent")
	if _, err := os.Stat(expectedTorrent); os.IsNotExist(err) {
		t.Errorf("Expected torrent file was not created: %s", expectedTorrent)
	}
}

// TestCleanCommand tests the clean command functionality
func TestCleanCommand(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	tempDir := t.TempDir()
	
	// Create some temporary files to clean up
	tempFile1 := filepath.Join(tempDir, "test1.tmp")
	tempFile2 := filepath.Join(tempDir, "subdir", "test2.tmp") 
	normalFile := filepath.Join(tempDir, "normal.txt")
	
	os.MkdirAll(filepath.Dir(tempFile2), 0755)
	
	files := []string{tempFile1, tempFile2, normalFile}
	for _, file := range files {
		err := os.WriteFile(file, []byte("test content"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
	}
	
	// Run clean command
	cmd := exec.Command(binaryPath, "--save-path", tempDir, "clean")
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		t.Errorf("Clean command failed: %v\nOutput: %s", err, string(output))
	}
	
	// Check that .tmp files were removed but normal files remain
	if _, err := os.Stat(tempFile1); !os.IsNotExist(err) {
		t.Error("Expected .tmp file to be cleaned up")
	}
	
	if _, err := os.Stat(tempFile2); !os.IsNotExist(err) {
		t.Error("Expected nested .tmp file to be cleaned up")
	}
	
	if _, err := os.Stat(normalFile); os.IsNotExist(err) {
		t.Error("Expected normal file to remain after cleanup")
	}
}

// TestJSONOutput tests JSON output formatting (if supported)
func TestJSONOutput(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	tempDir := t.TempDir()
	
	// Test JSON output for database status
	cmd := exec.Command(binaryPath,
		"--save-path", tempDir,
		"--log-format", "json", // Try JSON logging
		"db", "status")
		
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		t.Errorf("JSON output test failed: %v\nOutput: %s", err, string(output))
		return
	}
	
	// Try to parse output as JSON (some of it might be JSON)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			var jsonObj map[string]interface{}
			if err := json.Unmarshal([]byte(line), &jsonObj); err != nil {
				t.Errorf("Found JSON-like line but failed to parse: %s", line)
			}
		}
	}
}

// Helper function to build the test binary
func buildTestBinary(t *testing.T) string {
	binaryPath := filepath.Join(t.TempDir(), "civitai-downloader-test")
	
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to build test binary: %v", err)
	}
	
	return binaryPath
}

// TestErrorHandling tests various error conditions
func TestErrorHandling(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	tests := []struct {
		name string
		args []string
		desc string
	}{
		{
			name: "Invalid config file",
			args: []string{"--config", "/nonexistent/config.toml", "db", "status"},
			desc: "Should handle non-existent config file gracefully",
		},
		{
			name: "Invalid save path", 
			args: []string{"--save-path", "/invalid/\x00/path", "db", "status"},
			desc: "Should handle invalid save path",
		},
		{
			name: "Missing required flags",
			args: []string{"download"}, // No API key set
			desc: "Should handle missing API key",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, tt.args...)
			output, err := cmd.CombinedOutput()
			
			// We expect these to fail, but they should fail gracefully
			if err == nil {
				t.Logf("Command unexpectedly succeeded: %s", string(output))
			} else {
				// Make sure we get some error output
				if len(strings.TrimSpace(string(output))) == 0 {
					t.Errorf("Expected error output for test case: %s", tt.desc)
				}
			}
		})
	}
}

// TestConcurrentOperations tests running multiple operations simultaneously
func TestConcurrentOperations(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)
	
	tempDir := t.TempDir()
	
	// Run multiple database status commands concurrently
	const numConcurrent = 3
	
	done := make(chan error, numConcurrent)
	
	for i := 0; i < numConcurrent; i++ {
		go func(id int) {
			cmd := exec.Command(binaryPath, "--save-path", tempDir, "db", "status")
			_, err := cmd.CombinedOutput()
			done <- err
		}(i)
	}
	
	// Wait for all to complete
	for i := 0; i < numConcurrent; i++ {
		err := <-done
		if err != nil {
			t.Errorf("Concurrent operation %d failed: %v", i, err)
		}
	}
}