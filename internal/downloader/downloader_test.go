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
	"unicode/utf8"

	"go-civitai-download/internal/models"

	"lukechampine.com/blake3"
)

// TestNewDownloader tests downloader creation
func TestNewDownloader(t *testing.T) {
	apiKey := "test-key"
	httpClient := &http.Client{Timeout: 30 * time.Second}

	downloader := NewDownloader(httpClient, apiKey, "")

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
	downloader := NewDownloader(nil, "test-key", "")

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

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key", "")

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

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key", "")

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

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key", "")

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
	downloader := NewDownloader(&http.Client{Timeout: 100 * time.Millisecond}, "test-key", "")

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

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key", "")

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

// TestDownloadFile_Authentication tests that API key is used in requests via token query parameter
func TestDownloadFile_Authentication(t *testing.T) {
	expectedAPIKey := "test-api-key-123"
	var receivedToken string

	// Create mock server that checks authentication via query parameter
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.URL.Query().Get("token")
		w.Header().Set("Content-Disposition", "attachment; filename=test-file.bin")
		w.Write([]byte("test content"))
	}))
	defer server.Close()

	// Setup test directory
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "test-file.bin")

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, expectedAPIKey, "")

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

	// Verify API key was sent via token query parameter
	if receivedToken != expectedAPIKey {
		t.Errorf("Expected token query param '%s', got '%s'", expectedAPIKey, receivedToken)
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

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key", "")

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

// TestDownloadImage_MimeDetection tests that DownloadImage detects the actual
// MIME type and renames the file with the correct extension.
func TestDownloadImage_MimeDetection(t *testing.T) {
	// PNG magic bytes (first 8 bytes of a PNG file)
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	// Add some padding to make it look like a real file
	pngData := append(pngMagic, []byte("PNG DATA HERE")...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngData)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "", "")

	// Pass a URL that ends in .jpeg (like Civitai does) but serves PNG content
	imageURL := server.URL + "/test_image.jpeg"
	filename, err := downloader.DownloadImage(tempDir, imageURL)
	if err != nil {
		t.Fatalf("DownloadImage failed: %v", err)
	}

	// The filename should be corrected to .png based on MIME detection
	if !strings.HasSuffix(filename, ".png") {
		t.Errorf("Expected filename to end with .png (MIME-detected), got: %s", filename)
	}

	// Verify the file exists
	finalPath := filepath.Join(tempDir, filename)
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		t.Errorf("File does not exist at expected path: %s", finalPath)
	}
}

// TestDownloadImage_JpegKeepsExtension tests that .jpeg files are not renamed to .jpg
// when the MIME type matches (JPEG and JPG are the same format).
func TestDownloadImage_JpegKeepsExtension(t *testing.T) {
	// JPEG magic bytes (SOI marker + JFIF APP0 marker)
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(jpegData)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "", "")

	// URL ends in .jpeg, content is JPEG
	imageURL := server.URL + "/test_image.jpeg"
	filename, err := downloader.DownloadImage(tempDir, imageURL)
	if err != nil {
		t.Fatalf("DownloadImage failed: %v", err)
	}

	// Should keep .jpeg extension, NOT rename to .jpg
	if !strings.HasSuffix(filename, ".jpeg") {
		t.Errorf("Expected .jpeg extension to be preserved, got: %s", filename)
	}
}

// TestDownloadImage_HtmlResponse tests that DownloadImage rejects HTML responses.
func TestDownloadImage_HtmlResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>Error</body></html>"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "", "")

	imageURL := server.URL + "/test_image.jpeg"
	_, err := downloader.DownloadImage(tempDir, imageURL)
	if err == nil {
		t.Error("Expected error for HTML response, got nil")
	}
}

// TestDownloadImage_FallbackOnMimeFailure tests fallback when MIME detection fails.
func TestDownloadImage_FallbackOnMimeFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("some binary data"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "", "")

	imageURL := server.URL + "/test_image.jpeg"
	filename, err := downloader.DownloadImage(tempDir, imageURL)
	if err != nil {
		t.Fatalf("DownloadImage failed: %v", err)
	}

	// Should fall back to original extension when MIME is unknown
	if !strings.HasSuffix(filename, ".jpeg") {
		t.Errorf("Expected fallback to .jpeg extension, got: %s", filename)
	}
}

// TestDownloadImage_MimeDetectionDisabled tests that DownloadImage keeps the original
// extension when MIME detection is disabled.
func TestDownloadImage_MimeDetectionDisabled(t *testing.T) {
	// PNG magic bytes
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	pngData := append(pngMagic, []byte("PNG DATA HERE")...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngData)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "", "")
	downloader.SetDetectImageMimeType(false) // Disable MIME detection

	// Pass a URL that ends in .jpeg but serves PNG content
	imageURL := server.URL + "/test_image.jpeg"
	filename, err := downloader.DownloadImage(tempDir, imageURL)
	if err != nil {
		t.Fatalf("DownloadImage failed: %v", err)
	}

	// When disabled, should keep original .jpeg extension despite PNG content
	if !strings.HasSuffix(filename, ".jpeg") {
		t.Errorf("Expected filename to keep .jpeg extension when MIME detection disabled, got: %s", filename)
	}

	// Verify the file exists
	finalPath := filepath.Join(tempDir, filename)
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		t.Errorf("File does not exist at expected path: %s", finalPath)
	}
}

// TestDownloadImage_PngDetectedWithoutContentType tests that PNG content is correctly
// detected via magic bytes even when the server sends no Content-Type header.
// This matches real Civitai CDN behavior where images may lack proper Content-Type.
func TestDownloadImage_PngDetectedWithoutContentType(t *testing.T) {
	// PNG magic bytes with IHDR chunk header for a more realistic PNG file
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1 image
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, // 8-bit RGB
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately do NOT set Content-Type header
		w.Write(pngData)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "", "")

	// URL ends in .jpeg (like Civitai URLs) but content is PNG
	imageURL := server.URL + "/test_image.jpeg"
	filename, err := downloader.DownloadImage(tempDir, imageURL)
	if err != nil {
		t.Fatalf("DownloadImage failed: %v", err)
	}

	// The filename should be corrected to .png based on magic byte detection
	if !strings.HasSuffix(filename, ".png") {
		t.Errorf("Expected filename to end with .png (magic-byte detected), got: %s", filename)
	}

	// Verify the file exists
	finalPath := filepath.Join(tempDir, filename)
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		t.Errorf("File does not exist at expected path: %s", finalPath)
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

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "test-key", "")
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

// responseWithContentDisposition builds a synthetic *http.Response with the
// given Content-Disposition header value (or no header when empty). Using a
// synthetic response (rather than an httptest server) lets tests inject raw
// non-UTF-8 bytes into the header reliably, without a wire round-trip.
func responseWithContentDisposition(contentDisposition string) *http.Response {
	header := http.Header{}
	if contentDisposition != "" {
		header.Set("Content-Disposition", contentDisposition)
	}
	return &http.Response{Header: header}
}

// TestExtractFilenameFromResponse tests filename extraction and sanitization
// from the Content-Disposition header, including the Latin-1 encoding repair
// (the root cause of the corrupted-filename bug) and path-traversal safety.
func TestExtractFilenameFromResponse(t *testing.T) {
	// Raw Latin-1 byte 0xE2 (a-circumflex 'â') — invalid UTF-8 on its own.
	latin1Byte := string([]byte{0xE2})

	tests := []struct {
		name     string
		header   string // Content-Disposition value; "" means no header
		expected string
	}{
		{
			name:     "latin-1 encoded filename re-encoded to UTF-8 (repro)",
			header:   "attachment; filename=\"MidnightFl" + latin1Byte + "neurDressILL.safetensors\"",
			expected: "MidnightFlâneurDressILL.safetensors",
		},
		{
			name:     "valid UTF-8 filename unchanged",
			header:   "attachment; filename=\"normal-model.safetensors\"",
			expected: "normal-model.safetensors",
		},
		{
			name:     "no Content-Disposition header",
			header:   "",
			expected: "",
		},
		{
			name:     "path traversal filename sanitized",
			header:   "attachment; filename=\"../../etc/passwd\"",
			expected: "etcpasswd",
		},
		{
			name:     "RFC 5987 filename* parameter decoded by mime.ParseMediaType",
			header:   "attachment; filename*=UTF-8''MidnightFl%C3%A2neurDressILL.safetensors",
			expected: "MidnightFlâneurDressILL.safetensors",
		},
		{
			name:     "filename sanitizes to empty (fallback to constructed name)",
			header:   "attachment; filename=\"...\"",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFilenameFromResponse(responseWithContentDisposition(tt.header))
			if got != tt.expected {
				t.Errorf("extractFilenameFromResponse() = %q, want %q", got, tt.expected)
			}
			if !utf8.ValidString(got) {
				t.Errorf("extractFilenameFromResponse() = %q is not valid UTF-8", got)
			}
		})
	}
}

// TestDownloadFile_Latin1ContentDisposition is an end-to-end regression test
// for the corrupted-filename bug: a server sending a raw Latin-1 byte (0xE2)
// in the Content-Disposition filename must result in a valid UTF-8 filename
// on disk (with 'â'), not a corrupted one.
// Note: the test content is plain text, which MIME detection classifies as
// "text/plain" — a type not present in the extension map — so the original
// .safetensors extension is preserved by detectMimeAndRename.
func TestDownloadFile_Latin1ContentDisposition(t *testing.T) {
	testData := []byte("latin1 filename test content")
	hash := blake3.Sum256(testData)
	hashHex := hex.EncodeToString(hash[:])

	// Build a filename containing a raw Latin-1 byte 0xE2 (invalid UTF-8).
	latin1Filename := "MidnightFl" + string([]byte{0xE2}) + "neurDressILL.safetensors"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+latin1Filename+"\"")
		w.Write(testData)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "12345_constructed-name.safetensors")

	downloader := NewDownloader(&http.Client{Timeout: 30 * time.Second}, "", "")
	hashes := models.Hashes{
		BLAKE3: hashHex,
	}

	finalPath, err := downloader.DownloadFile(targetPath, server.URL, hashes, 12345)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	baseName := filepath.Base(finalPath)
	if !utf8.ValidString(baseName) {
		t.Errorf("Final filename %q is not valid UTF-8", baseName)
	}

	expected := "12345_MidnightFlâneurDressILL.safetensors"
	if baseName != expected {
		t.Errorf("Final filename = %q, want %q", baseName, expected)
	}

	// Verify the file exists on disk at the expected (repaired) path.
	if _, err := os.Stat(filepath.Join(tempDir, expected)); os.IsNotExist(err) {
		t.Errorf("File does not exist at expected path: %s", filepath.Join(tempDir, expected))
	}
}
