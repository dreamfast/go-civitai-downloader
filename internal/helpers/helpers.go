package helpers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go-civitai-download/internal/models" // Import the models package

	log "github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// Common file extensions for image MIME types
const (
	ExtJPG  = ".jpg"
	ExtPNG  = ".png"
	ExtGIF  = ".gif"
	ExtWebP = ".webp"
	ExtMP4  = ".mp4"

	MimeJPG  = "image/jpeg"
	MimePNG  = "image/png"
	MimeGIF  = "image/gif"
	MimeWebP = "image/webp"
)

// imageMimeToExt maps known image MIME types to their file extensions.
var imageMimeToExt = map[string]string{
	MimeJPG:  ExtJPG,
	MimePNG:  ExtPNG,
	MimeGIF:  ExtGIF,
	MimeWebP: ExtWebP,
}

// CheckHash verifies the hash of a file against expected values.
// Returns true if ANY of the provided hashes match the calculated ones.
// Checks in the order: BLAKE3, SHA256, CRC32, AutoV2.
func CheckHash(filePath string, hashes models.Hashes) bool {
	// Check BLAKE3 (Prioritized for speed)
	if hashes.BLAKE3 != "" {
		hasher := blake3.New()
		calculatedHash, err := calculateHash(filePath, hasher)
		if err != nil {
			log.WithError(err).Errorf("Failed to calculate BLAKE3 for %s, skipping check.", filePath)
		} else {
			if strings.EqualFold(calculatedHash, hashes.BLAKE3) {
				log.Debugf("BLAKE3 match for %s", filePath)
				return true // Match found!
			} else {
				log.Warnf("BLAKE3 mismatch for %s: Expected %s, Got %s", filePath, hashes.BLAKE3, calculatedHash)
			}
		}
	}

	// Check SHA256
	if hashes.SHA256 != "" {
		hasher := sha256.New()
		calculatedHash, err := calculateHash(filePath, hasher)
		if err != nil {
			log.WithError(err).Errorf("Failed to calculate SHA256 for %s, skipping check.", filePath)
		} else {
			if strings.EqualFold(calculatedHash, hashes.SHA256) {
				log.Debugf("SHA256 match for %s", filePath)
				return true // Match found!
			} else {
				log.Warnf("SHA256 mismatch for %s: Expected %s, Got %s", filePath, hashes.SHA256, calculatedHash)
			}
		}
	}

	// Check CRC32 (using Castagnoli polynomial)
	if hashes.CRC32 != "" {
		table := crc32.MakeTable(crc32.Castagnoli)
		hasher := crc32.New(table)
		calculatedHash, err := calculateHash(filePath, hasher)
		if err != nil {
			log.WithError(err).Errorf("Failed to calculate CRC32 for %s, skipping check.", filePath)
		} else {
			if strings.EqualFold(calculatedHash, hashes.CRC32) {
				log.Debugf("CRC32 match for %s", filePath)
				return true // Match found!
			} else {
				log.Warnf("CRC32 mismatch for %s: Expected %s, Got %s", filePath, hashes.CRC32, calculatedHash)
			}
		}
	}

	// Check AutoV2 (derived from SHA256)
	if hashes.AutoV2 != "" {
		hasher := sha256.New() // Still need SHA256 calculation for AutoV2
		calculatedSha256Hash, err := calculateHash(filePath, hasher)
		if err != nil {
			log.WithError(err).Errorf("Failed to calculate hash (for AutoV2 check) for %s, skipping check.", filePath)
		} else {
			// Civitai AutoV2 hashes seem to be the first 10 chars of SHA256
			if len(calculatedSha256Hash) >= 10 && strings.EqualFold(calculatedSha256Hash[:10], hashes.AutoV2) {
				log.Debugf("AutoV2 match for %s", filePath)
				return true // Match found!
			} else {
				log.Warnf("AutoV2 mismatch for %s: Expected %s, Got %s (derived from SHA256: %s)", filePath, hashes.AutoV2, calculatedSha256Hash[:min(10, len(calculatedSha256Hash))], calculatedSha256Hash)
			}
		}
	}

	// If we reached here, none of the provided hashes matched.
	log.Warnf("No matching hash found for %s after checking all provided types.", filePath)
	return false
}

// CounterWriter tracks the number of bytes written to the underlying writer.
// It's used to display download progress.
// Note: Consider moving this to the 'downloader' package later.
type CounterWriter struct {
	Writer io.Writer // Put interface first (8 bytes on 64-bit)
	Total  uint64    // Then 8-byte integer
}

// Write implements the io.Writer interface for CounterWriter.
func (cw *CounterWriter) Write(p []byte) (n int, err error) {
	n, err = cw.Writer.Write(p)
	// Only add to total if write was successful and n is positive
	if err == nil && n > 0 {
		cw.Total += uint64(n)
	}
	// Progress reporting might be handled differently in CLI context
	// fmt.Printf("\rDownloaded %s", BytesToSize(cw.Total))
	return n, err
}

// BytesToSize converts a byte count into a human-readable string (KB, MB, GB, etc.).
func BytesToSize(bytes uint64) string {
	sizes := []string{"B", "KB", "MB", "GB", "TB"}
	if bytes == 0 {
		return "0B"
	}
	i := int(math.Floor(math.Log(float64(bytes)) / math.Log(1024)))
	if i >= len(sizes) {
		i = len(sizes) - 1 // Handle very large sizes
	}
	return fmt.Sprintf("%.2f%s", float64(bytes)/math.Pow(1024, float64(i)), sizes[i])
}

// ConvertToSlug converts a string into a filesystem-friendly slug.
func ConvertToSlug(str string) string {
	str = strings.ReplaceAll(str, " ", "_")
	str = strings.ReplaceAll(str, ":", "-")
	str = strings.ToLower(str)

	allowedChars := "0123456789abcdefghijklmnopqrstuvwxyz._-"

	var filteredDescription strings.Builder
	for _, ch := range str {
		if strings.ContainsRune(allowedChars, ch) {
			filteredDescription.WriteRune(ch)
		}
	}
	str = filteredDescription.String()

	// Simplify repeated separators
	for strings.Contains(str, "--") {
		str = strings.ReplaceAll(str, "--", "-")
	}
	for strings.Contains(str, "__") {
		str = strings.ReplaceAll(str, "__", "_")
	}
	str = strings.ReplaceAll(str, "-_", "-")
	str = strings.ReplaceAll(str, "_-", "-")

	// Remove leading/trailing separators
	str = strings.Trim(str, "_-")

	return str
}

// CheckAndMakeDir ensures a directory exists, creating it if necessary.
// Uses standard directory permissions (0700).
func CheckAndMakeDir(dir string) bool {
	// Use MkdirAll to create parent directories if they don't exist
	err := os.MkdirAll(SanitizePath(dir), 0700)
	if err != nil {
		log.WithError(err).Errorf("Error creating directory %s", dir) // Use logrus
		return false
	}
	return true
}

// imageMagicSignatures defines magic byte signatures for common image formats.
// Each entry contains the minimum data length, the signature check function, and the MIME type.
var imageMagicSignatures = []struct {
	Check    func(data []byte) bool
	MIMEType string
	MinLen   int
}{
	{func(d []byte) bool {
		return d[0] == 0x89 && d[1] == 0x50 && d[2] == 0x4E && d[3] == 0x47 &&
			d[4] == 0x0D && d[5] == 0x0A && d[6] == 0x1A && d[7] == 0x0A
	}, MimePNG, 8},
	{func(d []byte) bool {
		return d[0] == 0xFF && d[1] == 0xD8 && d[2] == 0xFF
	}, MimeJPG, 3},
	{func(d []byte) bool {
		return d[0] == 'G' && d[1] == 'I' && d[2] == 'F' &&
			d[3] == '8' && (d[4] == '7' || d[4] == '9') && d[5] == 'a'
	}, MimeGIF, 6},
	{func(d []byte) bool {
		return d[0] == 'R' && d[1] == 'I' && d[2] == 'F' && d[3] == 'F' &&
			d[8] == 'W' && d[9] == 'E' && d[10] == 'B' && d[11] == 'P'
	}, MimeWebP, 12},
}

// DetectImageTypeFromMagicBytes detects the MIME type of image data by checking
// magic byte signatures first (more reliable for short reads), then falling back
// to http.DetectContentType. Returns the MIME type string (e.g., "image/png").
func DetectImageTypeFromMagicBytes(data []byte) string {
	// Check magic byte signatures first — these are more reliable than
	// http.DetectContentType for image files and work with short reads.
	for _, sig := range imageMagicSignatures {
		if len(data) >= sig.MinLen && sig.Check(data) {
			return sig.MIMEType
		}
	}

	// Fall back to Go's built-in MIME detection (WHATWG MIME sniffing)
	mimeType := http.DetectContentType(data)
	// Strip parameters (e.g., "text/plain; charset=utf-8" → "text/plain")
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	return mimeType
}

// -- Hashing Helper --
func calculateHash(filePath string, hashAlgo hash.Hash) (string, error) {
	// #nosec G304 -- filePath is internal, not user input
	file, err := os.Open(filePath) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("opening file %s for hashing: %w", filePath, err)
	}
	defer func() { _ = file.Close() }()
	if _, err := io.Copy(hashAlgo, file); err != nil {
		return "", fmt.Errorf("hashing file %s: %w", filePath, err)
	}

	return hex.EncodeToString(hashAlgo.Sum(nil)), nil
}

// GetExtensionFromMimeType returns the standard file extension for a given MIME type.
// It returns the extension (including the dot) and true if found, otherwise empty string and false.
func GetExtensionFromMimeType(mimeType string) (string, bool) {
	// Handle potential parameters in the MIME type (e.g., "text/plain; charset=utf-8")
	baseMimeType, _, _ := mime.ParseMediaType(mimeType)

	// Build extended MIME extension map from image map plus additional types
	mimeExtensionMap := map[string]string{
		"video/mp4": ExtMP4,
	}
	for k, v := range imageMimeToExt {
		mimeExtensionMap[k] = v
	}

	ext, ok := mimeExtensionMap[baseMimeType]
	return ext, ok
}

// StringSliceContains checks if a string slice contains a specific item (case-insensitive).
func StringSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

// SanitizePath cleans a file path to prevent directory traversal.
// It removes ".." and ensures the path is relative.
func SanitizePath(path string) string {
	// Clean the path to resolve any ".." elements.
	cleaned := filepath.Clean(path)

	// Remove any leading slashes to ensure it's a relative path.
	trimmed := strings.TrimLeft(cleaned, `\/`)

	// As a final safety measure, split the path and remove any remaining ".." parts.
	// This is belt-and-suspenders after filepath.Clean, but adds extra security.
	parts := strings.Split(trimmed, string(filepath.Separator))
	safeParts := []string{}
	for _, part := range parts {
		if part != ".." {
			safeParts = append(safeParts, part)
		}
	}

	// Join the safe parts back together.
	safePath := filepath.Join(safeParts...)

	if safePath != trimmed {
		log.Warnf("Path sanitization changed '%s' to '%s'", path, safePath)
	}

	return safePath
}
