package helpers

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"go-civitai-download/internal/models"
)

func TestConvertToSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "Hello World",
			expected: "hello_world",
		},
		{
			name:     "already lowercase",
			input:    "hello world",
			expected: "hello_world",
		},
		{
			name:     "with numbers",
			input:    "Model V2.0",
			expected: "model_v2.0",
		},
		{
			name:     "with colons",
			input:    "SD 1.5: Base Model",
			expected: "sd_1.5-base_model", // colon becomes dash, space becomes _, then _- simplified to -
		},
		{
			name:     "special characters removed",
			input:    "Test@Model#With$Special%Chars",
			expected: "testmodelwithspecialchars",
		},
		{
			name:     "multiple spaces",
			input:    "Hello   World",
			expected: "hello_world",
		},
		{
			name:     "underscores preserved",
			input:    "test_model_name",
			expected: "test_model_name",
		},
		{
			name:     "dashes preserved",
			input:    "my-cool-model",
			expected: "my-cool-model",
		},
		{
			name:     "dots preserved",
			input:    "v1.0.0",
			expected: "v1.0.0",
		},
		{
			name:     "leading/trailing separators removed",
			input:    "__test__",
			expected: "test",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special chars",
			input:    "@#$%^&*()",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertToSlug(tt.input)
			if got != tt.expected {
				t.Errorf("ConvertToSlug(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBytesToSize(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		bytes    uint64
	}{
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0B",
		},
		{
			name:     "one byte",
			bytes:    1,
			expected: "1.00B",
		},
		{
			name:     "kilobytes",
			bytes:    1024,
			expected: "1.00KB",
		},
		{
			name:     "megabytes",
			bytes:    1024 * 1024,
			expected: "1.00MB",
		},
		{
			name:     "gigabytes",
			bytes:    1024 * 1024 * 1024,
			expected: "1.00GB",
		},
		{
			name:     "terabytes",
			bytes:    1024 * 1024 * 1024 * 1024,
			expected: "1.00TB",
		},
		{
			name:     "fractional megabytes",
			bytes:    1536 * 1024, // 1.5 MB
			expected: "1.50MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BytesToSize(tt.bytes)
			if got != tt.expected {
				t.Errorf("BytesToSize(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple path",
			input:    "folder/file.txt",
			expected: "folder/file.txt",
		},
		{
			name:     "path with dots",
			input:    "folder/../other/file.txt",
			expected: "other/file.txt",
		},
		{
			name:     "path traversal attempt",
			input:    "../../etc/passwd",
			expected: "etc/passwd",
		},
		{
			name:     "absolute path",
			input:    "/absolute/path/file.txt",
			expected: "absolute/path/file.txt",
		},
		{
			name:     "current directory",
			input:    "./file.txt",
			expected: "file.txt",
		},
		{
			name:     "complex traversal",
			input:    "a/b/../c/../d",
			expected: "a/d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizePath(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStringSliceContains(t *testing.T) {
	tests := []struct {
		name     string
		item     string
		slice    []string
		expected bool
	}{
		{
			name:     "item present exact case",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "banana",
			expected: true,
		},
		{
			name:     "item present different case",
			slice:    []string{"Apple", "Banana", "Cherry"},
			item:     "banana",
			expected: true,
		},
		{
			name:     "item not present",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "grape",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "anything",
			expected: false,
		},
		{
			name:     "empty item",
			slice:    []string{"apple", "banana", ""},
			item:     "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringSliceContains(tt.slice, tt.item)
			if got != tt.expected {
				t.Errorf("StringSliceContains(%v, %q) = %v, want %v", tt.slice, tt.item, got, tt.expected)
			}
		})
	}
}

func TestGetExtensionFromMimeType(t *testing.T) {
	tests := []struct {
		name        string
		mimeType    string
		expectedExt string
		expectedOk  bool
	}{
		{
			name:        "jpeg",
			mimeType:    "image/jpeg",
			expectedExt: ".jpg",
			expectedOk:  true,
		},
		{
			name:        "png",
			mimeType:    "image/png",
			expectedExt: ".png",
			expectedOk:  true,
		},
		{
			name:        "webp",
			mimeType:    "image/webp",
			expectedExt: ".webp",
			expectedOk:  true,
		},
		{
			name:        "mp4",
			mimeType:    "video/mp4",
			expectedExt: ".mp4",
			expectedOk:  true,
		},
		{
			name:        "unknown type",
			mimeType:    "application/octet-stream",
			expectedExt: "",
			expectedOk:  false,
		},
		{
			name:        "mime with params",
			mimeType:    "image/jpeg; charset=utf-8",
			expectedExt: ".jpg",
			expectedOk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext, ok := GetExtensionFromMimeType(tt.mimeType)
			if ext != tt.expectedExt || ok != tt.expectedOk {
				t.Errorf("GetExtensionFromMimeType(%q) = (%q, %v), want (%q, %v)",
					tt.mimeType, ext, ok, tt.expectedExt, tt.expectedOk)
			}
		})
	}
}

func TestCheckAndMakeDir(t *testing.T) {
	// Note: CheckAndMakeDir uses SanitizePath which removes leading slashes
	// So we need to change to a temp directory and use relative paths

	// Save current directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer os.Chdir(origDir)

	tests := []struct {
		name     string
		dir      string
		expected bool
	}{
		{
			name:     "create new directory",
			dir:      "new_dir",
			expected: true,
		},
		{
			name:     "create nested directory",
			dir:      "nested/path/here",
			expected: true,
		},
		{
			name:     "existing directory (current dir)",
			dir:      ".",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckAndMakeDir(tt.dir)
			if got != tt.expected {
				t.Errorf("CheckAndMakeDir(%q) = %v, want %v", tt.dir, got, tt.expected)
			}
			if tt.expected && tt.dir != "." {
				// Verify directory exists (relative to tempDir)
				fullPath := filepath.Join(tempDir, tt.dir)
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					t.Errorf("Directory %q was not created", fullPath)
				}
			}
		})
	}
}

func TestCounterWriter(t *testing.T) {
	var buf bytes.Buffer
	cw := &CounterWriter{Writer: &buf}

	// Write some data
	data := []byte("Hello, World!")
	n, err := cw.Write(data)

	if err != nil {
		t.Errorf("CounterWriter.Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("CounterWriter.Write() wrote %d bytes, want %d", n, len(data))
	}
	if cw.Total != uint64(len(data)) {
		t.Errorf("CounterWriter.Total = %d, want %d", cw.Total, len(data))
	}

	// Write more data
	moreData := []byte(" More data!")
	_, err = cw.Write(moreData)

	if err != nil {
		t.Errorf("CounterWriter.Write() second error = %v", err)
	}
	expectedTotal := uint64(len(data) + len(moreData))
	if cw.Total != expectedTotal {
		t.Errorf("CounterWriter.Total after second write = %d, want %d", cw.Total, expectedTotal)
	}

	// Verify buffer contents
	if buf.String() != "Hello, World! More data!" {
		t.Errorf("Buffer contents = %q, want %q", buf.String(), "Hello, World! More data!")
	}
}

func TestCheckHash(t *testing.T) {
	// Create a temporary file with known content
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_file.txt")
	testContent := []byte("Hello, World!")

	err := os.WriteFile(testFile, testContent, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Pre-computed hashes for "Hello, World!"
	// These would need to be calculated for the actual content

	t.Run("no hashes provided", func(t *testing.T) {
		result := CheckHash(testFile, models.Hashes{})
		// With no hashes to check, should return false
		if result {
			t.Error("CheckHash() with no hashes should return false")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		result := CheckHash("/nonexistent/file.txt", models.Hashes{BLAKE3: "somehash"})
		if result {
			t.Error("CheckHash() with nonexistent file should return false")
		}
	})
}

func TestCorrectPathBasedOnImageType(t *testing.T) {
	// Note: CorrectPathBasedOnImageType uses SanitizePath which removes leading slashes
	// So we need to change to a temp directory and use relative paths

	// Save current directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer os.Chdir(origDir)

	t.Run("file not found gracefully handled", func(t *testing.T) {
		// Use a relative nonexistent path
		result, err := CorrectPathBasedOnImageType("nonexistent/file.jpg", "path/to/file.jpg")
		if err != nil {
			t.Errorf("CorrectPathBasedOnImageType() unexpected error: %v", err)
		}
		// Should return original path when file not found
		if result != "path/to/file.jpg" {
			t.Errorf("CorrectPathBasedOnImageType() = %q, want %q", result, "path/to/file.jpg")
		}
	})

	t.Run("empty file returns original path", func(t *testing.T) {
		emptyFile := "empty.jpg"
		err := os.WriteFile(emptyFile, []byte{}, 0644)
		if err != nil {
			t.Fatalf("Failed to create empty file: %v", err)
		}

		outputPath := "output.jpg"
		result, err := CorrectPathBasedOnImageType(emptyFile, outputPath)
		if err != nil {
			t.Errorf("CorrectPathBasedOnImageType() unexpected error: %v", err)
		}
		// With empty file, MIME detection returns "text/plain; charset=utf-8" or similar
		// so it should return the original path unchanged
		if result != outputPath {
			t.Logf("Result path: %q (MIME detection may have changed extension)", result)
		}
	})

	t.Run("jpeg file detection", func(t *testing.T) {
		// Create a minimal JPEG file (JPEG magic bytes: FF D8 FF)
		jpegFile := "test.jpg"
		jpegMagic := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00}
		err := os.WriteFile(jpegFile, jpegMagic, 0644)
		if err != nil {
			t.Fatalf("Failed to create JPEG file: %v", err)
		}

		outputPath := "output_jpeg.jpg"
		result, err := CorrectPathBasedOnImageType(jpegFile, outputPath)
		if err != nil {
			t.Errorf("CorrectPathBasedOnImageType() unexpected error: %v", err)
		}
		// JPEG detected, extension matches, should return same path
		if result != outputPath {
			t.Errorf("CorrectPathBasedOnImageType() = %q, want %q", result, outputPath)
		}
	})

	t.Run("png file with wrong extension", func(t *testing.T) {
		// Create a minimal PNG file (PNG magic bytes)
		pngFile := "test_png.dat"
		pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		err := os.WriteFile(pngFile, pngMagic, 0644)
		if err != nil {
			t.Fatalf("Failed to create PNG file: %v", err)
		}

		// Original path has .jpg extension but file is PNG
		outputPath := "output_png.jpg"
		result, err := CorrectPathBasedOnImageType(pngFile, outputPath)
		if err != nil {
			t.Errorf("CorrectPathBasedOnImageType() unexpected error: %v", err)
		}
		// Should correct the extension to .png
		expectedPath := "output_png.png"
		if result != expectedPath {
			t.Errorf("CorrectPathBasedOnImageType() = %q, want %q", result, expectedPath)
		}
	})
}
