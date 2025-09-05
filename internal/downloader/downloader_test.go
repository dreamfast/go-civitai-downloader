package downloader

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-civitai-download/internal/models"

	"lukechampine.com/blake3"
)

// TestNewDownloader tests downloader creation
func TestNewDownloader(t *testing.T) {
	apiKey := "test-key"
	httpClient := &http.Client{Timeout: 30 * time.Second}

	downloader := NewDownloader(httpClient, apiKey)

	if downloader == nil {
		t.Fatal("Expected downloader to be created")
	}

	if downloader.client != httpClient {
		t.Error("Expected downloader to store HTTP client reference")
	}

	if downloader.apiKey != apiKey {
		t.Error("Expected downloader to store API key")
	}
}

// TestNewDownloader_NilClient tests that a default client is created when nil is passed
func TestNewDownloader_NilClient(t *testing.T) {
	downloader := NewDownloader(nil, "test-key")

	if downloader == nil {
		t.Fatal("Expected downloader to be created")
	}

	if downloader.client == nil {
		t.Error("Expected default HTTP client to be created")
	}

	if downloader.client.Timeout != 15*time.Minute {
		t.Errorf("Expected default timeout to be 15 minutes, got %v", downloader.client.Timeout)
	}
}

// TestDownloadFile_Success tests successful file download
func TestDownloadFile_Success(t *testing.T) {
	// Create test data
	testData := []byte("test file content for download")
	hash := blake3.Sum256(testData)
	hashHex := hex.EncodeToString(hash[:])

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))
		w.Header().Set("Content-Disposition", "attachment; filename=test-file.bin")
		w.Write(testData)
	}))
	defer server.Close()

	// Setup test directory
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "test-file.bin")

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key")

	// Test hashes
	hashes := models.Hashes{
		BLAKE3: hashHex,
	}

	// Download the file
	finalPath, err := downloader.DownloadFile(targetPath, server.URL, hashes, 12345)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		t.Errorf("Downloaded file does not exist at %s", finalPath)
	}

	// Verify file content
	downloadedContent, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(downloadedContent) != string(testData) {
		t.Errorf("Downloaded content doesn't match. Expected %s, got %s",
			string(testData), string(downloadedContent))
	}
}

// TestDownloadFile_HashMismatch tests hash validation failure
func TestDownloadFile_HashMismatch(t *testing.T) {
	// Create test data
	testData := []byte("test file content")
	wrongHash := "0123456789abcdef" // Intentionally wrong hash

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=test-file.bin")
		w.Write(testData)
	}))
	defer server.Close()

	// Setup test directory
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "test-file.bin")

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key")

	// Test hashes with wrong hash
	hashes := models.Hashes{
		BLAKE3: wrongHash,
	}

	// Download should fail due to hash mismatch
	_, err := downloader.DownloadFile(targetPath, server.URL, hashes, 12345)
	if err == nil {
		t.Error("Expected DownloadFile to fail with hash mismatch")
	}

	if !strings.Contains(err.Error(), "hash") && !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("Expected error to mention hash mismatch, got: %v", err)
	}
}

// TestDownloadFile_NetworkError tests network error handling
func TestDownloadFile_NetworkError(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Server error"))
	}))
	defer server.Close()

	// Setup test directory
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "test-file.bin")

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key")

	// Test hashes
	hashes := models.Hashes{
		BLAKE3: "somehash",
	}

	// Download should fail
	_, err := downloader.DownloadFile(targetPath, server.URL, hashes, 12345)
	if err == nil {
		t.Error("Expected DownloadFile to fail with network error")
	}
}

// TestDownloadFile_Timeout tests download timeout handling
func TestDownloadFile_Timeout(t *testing.T) {
	// Create mock server that hangs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hang for longer than the client timeout
		time.Sleep(2 * time.Second)
		w.Write([]byte("too late"))
	}))
	defer server.Close()

	// Setup test directory
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "test-file.bin")

	// Use very short timeout
	downloader := NewDownloader(&http.Client{Timeout: 100 * time.Millisecond}, "test-key")

	// Test hashes
	hashes := models.Hashes{
		BLAKE3: "somehash",
	}

	// Download should fail with timeout
	_, err := downloader.DownloadFile(targetPath, server.URL, hashes, 12345)
	if err == nil {
		t.Error("Expected DownloadFile to fail with timeout")
	}

	// Error should be timeout-related
	errorStr := strings.ToLower(err.Error())
	if !strings.Contains(errorStr, "timeout") &&
		!strings.Contains(errorStr, "deadline") &&
		!strings.Contains(errorStr, "context") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

// TestDownloadFile_Progress tests download progress functionality
func TestDownloadFile_Progress(t *testing.T) {
	// Create larger test data to see progress
	testData := make([]byte, 10*1024) // 10KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	hash := blake3.Sum256(testData)
	hashHex := hex.EncodeToString(hash[:])

	// Create mock server with slow response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))
		w.Header().Set("Content-Disposition", "attachment; filename=test-large-file.bin")

		// Send data in chunks to simulate progress
		chunkSize := 1024
		for i := 0; i < len(testData); i += chunkSize {
			end := i + chunkSize
			if end > len(testData) {
				end = len(testData)
			}
			w.Write(testData[i:end])

			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

			// Small delay to make progress visible
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()

	// Setup test directory
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "test-large-file.bin")

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key")

	// Test hashes
	hashes := models.Hashes{
		BLAKE3: hashHex,
	}

	// Download the file (progress testing is mostly about not crashing)
	finalPath, err := downloader.DownloadFile(targetPath, server.URL, hashes, 12345)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// Verify file was created correctly
	downloadedContent, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if len(downloadedContent) != len(testData) {
		t.Errorf("Downloaded file size mismatch. Expected %d bytes, got %d bytes",
			len(testData), len(downloadedContent))
	}
}

// TestDownloadFile_Authentication tests that API key is used in requests
func TestDownloadFile_Authentication(t *testing.T) {
	expectedAPIKey := "test-api-key-123"
	var receivedAuth string

	// Create mock server that checks authentication
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Disposition", "attachment; filename=test-file.bin")
		w.Write([]byte("test content"))
	}))
	defer server.Close()

	// Setup test directory
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "test-file.bin")

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, expectedAPIKey)

	// Create test hash
	testData := []byte("test content")
	hash := blake3.Sum256(testData)
	hashHex := hex.EncodeToString(hash[:])

	hashes := models.Hashes{
		BLAKE3: hashHex,
	}

	// Download the file
	_, err := downloader.DownloadFile(targetPath, server.URL, hashes, 12345)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// Verify API key was sent
	expectedAuth := "Bearer " + expectedAPIKey
	if receivedAuth != expectedAuth {
		t.Errorf("Expected Authorization header '%s', got '%s'", expectedAuth, receivedAuth)
	}
}

// TestDownloadFile_FileNaming tests file naming and path construction
func TestDownloadFile_FileNaming(t *testing.T) {
	testData := []byte("test file content")
	hash := blake3.Sum256(testData)
	hashHex := hex.EncodeToString(hash[:])

	// Create mock server with specific filename
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=server-provided-name.txt")
		w.Write(testData)
	}))
	defer server.Close()

	// Setup test directory
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "original-name.bin")

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key")

	hashes := models.Hashes{
		BLAKE3: hashHex,
	}

	// Download the file
	finalPath, err := downloader.DownloadFile(targetPath, server.URL, hashes, 12345)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// Verify file exists at the returned path
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		t.Errorf("File does not exist at returned path: %s", finalPath)
	}

	// The final path should contain the model version ID for uniqueness
	if !strings.Contains(finalPath, "12345") {
		t.Errorf("Expected final path to contain version ID '12345', got: %s", finalPath)
	}
}

// Benchmark test for download performance
func BenchmarkDownloadFile(b *testing.B) {
	// Create test data
	testData := make([]byte, 1024*1024) // 1MB
	_, err := rand.Read(testData)
	if err != nil {
		b.Fatalf("Failed to generate test data: %v", err)
	}

	hash := blake3.Sum256(testData)
	hashHex := hex.EncodeToString(hash[:])

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))
		w.Header().Set("Content-Disposition", "attachment; filename=benchmark-file.bin")
		w.Write(testData)
	}))
	defer server.Close()

	// Setup test directory
	tempDir := b.TempDir()

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key")
	hashes := models.Hashes{
		BLAKE3: hashHex,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		targetPath := filepath.Join(tempDir, fmt.Sprintf("benchmark-file-%d.bin", i))
		_, err := downloader.DownloadFile(targetPath, server.URL, hashes, i)
		if err != nil {
			b.Fatalf("DownloadFile failed: %v", err)
		}

		// Clean up for next iteration
		os.Remove(targetPath)
	}
}
