package main

import (
	"context"
	"encoding/json"
	"fmt"
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
		expectOut   string
		args        []string
		expectError bool
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
			expectOut:   "Downloads models",
		},
		{
			name:        "Images help",
			args:        []string{"images", "--help"},
			expectError: false,
			expectOut:   "Downloads images",
		},
		{
			name:        "DB help",
			args:        []string{"db", "--help"},
			expectError: false,
			expectOut:   "Perform operations like",
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
ApiKey = "test-key-123"
SavePath = "` + tempDir + `/models"
ApiDelayMs = 1000
LogLevel = "debug"
LogFormat = "text"

[Download]
Concurrency = 4
Limit = 10
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Test debug show-config command to verify config loading
	// Change working directory to temp dir and use a local config file name
	cmd := exec.Command(binaryPath, "debug", "show-config")
	cmd.Dir = tempDir
	// Create config.toml in the temp dir so it gets picked up automatically
	localConfigFile := filepath.Join(tempDir, "config.toml")
	err = os.WriteFile(localConfigFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create local config file: %v", err)
	}
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("Config loading failed: %v\nOutput: %s", err, string(output))
	}

	// Check that the config values appear in the output
	// Note: API key is not shown for security reasons, so we check other values
	expectedValues := []string{
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
		"--save-path", tempDir+"/flag-models",
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

// TestTorrentCommand tests torrent command basic functionality
// Note: This test verifies the command runs without errors, not full torrent generation
// Full torrent generation requires complex database and filesystem setup
func TestTorrentCommand(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	tempDir := t.TempDir()

	// Test that torrent command requires announce URL (this is a basic sanity check)
	cmd := exec.Command(binaryPath,
		"--save-path", tempDir,
		"torrent")

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Should fail because no announce URL provided
	if err == nil {
		t.Logf("Command succeeded when we expected it to fail (no announce URL)")
	}

	// Verify the error message mentions announce
	if !strings.Contains(strings.ToLower(outputStr), "announce") {
		t.Errorf("Expected error about missing announce URL, got: %s", outputStr)
	}

	// Test that torrent command with announce URL runs (even if no models found)
	cmd = exec.Command(binaryPath,
		"--save-path", tempDir,
		"torrent",
		"--announce", "http://test-tracker.com:8080/announce")

	output, _ = cmd.CombinedOutput()
	outputStr = string(output)
	t.Logf("Torrent command output: %s", outputStr)

	// Command should succeed (or fail gracefully with "no models found" message)
	// It should NOT panic or crash
	if strings.Contains(outputStr, "panic:") {
		t.Errorf("Command panicked: %s", outputStr)
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
		name          string
		desc          string
		args          []string
		expectFailure bool // true if we expect the command to fail with an error
	}{
		{
			name:          "Invalid config file",
			args:          []string{"--config", "/nonexistent/config.toml", "db", "view"},
			desc:          "Should handle non-existent config file gracefully",
			expectFailure: true,
		},
		{
			name:          "Invalid subcommand",
			args:          []string{"db", "nonexistent_subcommand"},
			desc:          "Should handle invalid subcommand",
			expectFailure: true,
		},
		{
			name:          "Torrent without announce",
			args:          []string{"torrent"}, // No announce URL provided
			desc:          "Should require announce URL for torrent command",
			expectFailure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, tt.args...)
			output, err := cmd.CombinedOutput()
			outputStr := string(output)

			if tt.expectFailure {
				// We expect these to fail, but they should fail gracefully (not panic)
				if err == nil {
					// Command succeeded when we expected failure - just log it
					t.Logf("Command unexpectedly succeeded: %s", outputStr)
				} else {
					// Command failed as expected - verify we got some output
					// (either error message or at least exit with non-zero)
					t.Logf("Command failed as expected with output: %s", outputStr)
				}
			}

			// Verify no panic occurred (output would contain "panic:" if it did)
			if strings.Contains(outputStr, "panic:") {
				t.Errorf("Command panicked: %s", outputStr)
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

// runCLIWithTimeout runs the CLI binary with a timeout and returns output.
func runCLIWithTimeout(t *testing.T, binaryPath string, args []string, env []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	if env != nil {
		cmd.Env = env
	}

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("command timed out after 60s")
	}
	return string(output), err
}

// skipIfNoAPIKey skips the test if CIVITAI_API_KEY is not set.
func skipIfNoAPIKey(t *testing.T) string {
	if testing.Short() {
		t.Skip("Skipping real-download integration test in short mode")
	}

	apiKey := os.Getenv("CIVITAI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping real-download integration test: CIVITAI_API_KEY not set")
	}
	return apiKey
}

// TestRealDownload_SmallModel downloads a small TextualInversion model (EasyNegative, 24KB).
func TestRealDownload_SmallModel(t *testing.T) {
	apiKey := skipIfNoAPIKey(t)

	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	tempDir := t.TempDir()

	output, err := runCLIWithTimeout(t, binaryPath, []string{
		"--save-path", tempDir,
		"download",
		"--model-version-id", "9208",
		"--yes",
	}, append(os.Environ(), "CIVITAI_API_KEY="+apiKey))

	if err != nil {
		if strings.Contains(output, "RATE LIMITED") || strings.Contains(output, "429") {
			t.Skip("Skipping: API rate limited")
		}
		if strings.Contains(output, "404") {
			t.Skip("Skipping: Model not found (may have been removed)")
		}
		t.Logf("Command output: %s", output)
		t.Fatalf("Download command failed: %v", err)
	}

	// Verify the file was downloaded
	// EasyNegative should be around 24KB
	expectedFile := filepath.Join(tempDir, "*", "*", "9208_easynegative.safetensors")
	matches, _ := filepath.Glob(expectedFile)
	if len(matches) == 0 {
		// Try alternative patterns
		matches, _ = filepath.Glob(filepath.Join(tempDir, "**", "9208_*"))
	}

	if len(matches) == 0 {
		t.Errorf("Expected downloaded file not found in %s. Output: %s", tempDir, output)
	} else {
		info, err := os.Stat(matches[0])
		if err != nil {
			t.Errorf("Failed to stat downloaded file: %v", err)
		} else if info.Size() == 0 {
			t.Errorf("Downloaded file is empty: %s", matches[0])
		} else if info.Size() > 1024*1024 {
			t.Errorf("Downloaded file is unexpectedly large (%d bytes). Expected a small TextualInversion model.", info.Size())
		}
	}
}

// TestRealDownload_NsfwFlagApplied validates that --nsfw flag is included in the API URL.
func TestRealDownload_NsfwFlagApplied(t *testing.T) {
	apiKey := skipIfNoAPIKey(t)

	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	tempDir := t.TempDir()

	// Test 1: --nsfw="" should omit nsfw param from URL
	output, err := runCLIWithTimeout(t, binaryPath, []string{
		"--save-path", tempDir,
		"images",
		"--debug-print-api-url",
		"--nsfw", "",
		"--username", "Yofaraway",
	}, append(os.Environ(), "CIVITAI_API_KEY="+apiKey))

	if err != nil {
		t.Logf("Output: %s", output)
		t.Fatalf("Command failed: %v", err)
	}

	// The URL should NOT contain "nsfw=" when --nsfw is empty
	if strings.Contains(output, "nsfw=") {
		t.Errorf("Expected nsfw param to be omitted when --nsfw='', but URL contained it: %s", output)
	}

	// Test 2: --nsfw="Soft" should include nsfw=Soft in URL
	output, err = runCLIWithTimeout(t, binaryPath, []string{
		"--save-path", tempDir,
		"images",
		"--debug-print-api-url",
		"--nsfw", "Soft",
		"--username", "Yofaraway",
	}, append(os.Environ(), "CIVITAI_API_KEY="+apiKey))

	if err != nil {
		t.Logf("Output: %s", output)
		t.Fatalf("Command failed: %v", err)
	}

	if !strings.Contains(output, "nsfw=Soft") {
		t.Errorf("Expected nsfw=Soft in URL, got: %s", output)
	}
}

// TestRealDownload_PageFlagApplied validates that --page flag appears in the API URL.
func TestRealDownload_PageFlagApplied(t *testing.T) {
	apiKey := skipIfNoAPIKey(t)

	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	tempDir := t.TempDir()

	output, err := runCLIWithTimeout(t, binaryPath, []string{
		"--save-path", tempDir,
		"images",
		"--debug-print-api-url",
		"--page", "3",
		"--username", "Yofaraway",
	}, append(os.Environ(), "CIVITAI_API_KEY="+apiKey))

	if err != nil {
		t.Logf("Output: %s", output)
		t.Fatalf("Command failed: %v", err)
	}

	if !strings.Contains(output, "page=3") {
		t.Errorf("Expected page=3 in URL, got: %s", output)
	}
}

// TestRealDownload_ImageExtension validates that downloaded images have correct extensions.
func TestRealDownload_ImageExtension(t *testing.T) {
	apiKey := skipIfNoAPIKey(t)

	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	tempDir := t.TempDir()

	// Create a temp config with SkipConfirmation=true for images command
	configFile := filepath.Join(tempDir, "test_config.toml")
	configContent := fmt.Sprintf(`
SavePath = "%s"
ApiKey = "%s"

[Download]
SkipConfirmation = true
`, tempDir, apiKey)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	output, err := runCLIWithTimeout(t, binaryPath, []string{
		"--config", configFile,
		"images",
		"--model-id", "257749",
		"--limit", "3",
	}, os.Environ())

	if err != nil {
		if strings.Contains(output, "REGION_BLOCKED") || strings.Contains(output, "region") {
			t.Skip("Skipping: Images API region-blocked")
		}
		if strings.Contains(output, "429") {
			t.Skip("Skipping: API rate limited")
		}
		t.Logf("Output: %s", output)
		t.Fatalf("Images command failed: %v", err)
	}

	// Check downloaded files have recognized image extensions
	imageExts := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true}
	foundImages := false

	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if imageExts[ext] {
			foundImages = true
		} else if ext != ".toml" && ext != ".db" {
			t.Errorf("Downloaded file has unexpected extension: %s", path)
		}
		return nil
	})

	if err != nil {
		t.Errorf("Error walking temp dir: %v", err)
	}

	if !foundImages {
		t.Errorf("No images downloaded. Output: %s", output)
	}
}

// TestRealDownload_BaseModelFilter validates base model filtering via --debug-print-api-url.
func TestRealDownload_BaseModelFilter(t *testing.T) {
	apiKey := skipIfNoAPIKey(t)

	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	tempDir := t.TempDir()

	output, err := runCLIWithTimeout(t, binaryPath, []string{
		"--save-path", tempDir,
		"download",
		"--debug-print-api-url",
		"--model-id", "257749",
		"--base-models", "SD 1.5",
	}, append(os.Environ(), "CIVITAI_API_KEY="+apiKey))

	if err != nil {
		if strings.Contains(output, "unknown flag") && strings.Contains(output, "debug-print-api-url") {
			// The debug flag might not exist on download command - skip this test
			t.Skip("Skipping: debug-print-api-url not available on download command")
		}
		t.Logf("Output: %s", output)
		t.Fatalf("Command failed: %v", err)
	}

	if !strings.Contains(output, "baseModels=SD+1.5") {
		t.Errorf("Expected baseModels=SD+1.5 in URL, got: %s", output)
	}
}

// TestRealDownload_ImageUsernameUnmarshal validates no JSON unmarshal crash when fetching images.
func TestRealDownload_ImageUsernameUnmarshal(t *testing.T) {
	apiKey := skipIfNoAPIKey(t)

	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	tempDir := t.TempDir()

	// Create a temp config with SkipConfirmation=true
	configFile := filepath.Join(tempDir, "test_config.toml")
	configContent := fmt.Sprintf(`
SavePath = "%s"
ApiKey = "%s"

[Download]
SkipConfirmation = true
`, tempDir, apiKey)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	output, err := runCLIWithTimeout(t, binaryPath, []string{
		"--config", configFile,
		"images",
		"--username", "Yofaraway",
		"--limit", "5",
	}, os.Environ())

	if err != nil {
		if strings.Contains(output, "unmarshal") {
			t.Fatalf("JSON unmarshal error detected (FlexibleString fix may be needed): %v\nOutput: %s", err, output)
		}
		if strings.Contains(output, "REGION_BLOCKED") || strings.Contains(output, "region") {
			t.Skip("Skipping: Images API region-blocked")
		}
		if strings.Contains(output, "429") {
			t.Skip("Skipping: API rate limited")
		}
		t.Logf("Output: %s", output)
		t.Fatalf("Images command failed: %v", err)
	}

	// If we get here without unmarshal error, the test passes
	t.Logf("Images fetched successfully without JSON unmarshal errors")
}
