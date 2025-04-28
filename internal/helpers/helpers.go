package helpers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go-civitai-download/internal/models" // Import the models package

	log "github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

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
	Total  uint64
	Writer io.Writer
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
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		log.WithError(err).Errorf("Error creating directory %s", dir) // Use logrus
		return false
	}
	return true
}

// CorrectPathBasedOnImageType checks the MIME type of a file and corrects the extension
// in the final path if it doesn't match the detected image type.
// It only corrects for common image types (jpg, png, gif, webp).
// Returns the corrected final path and an error if reading fails.
func CorrectPathBasedOnImageType(tempFilePath, finalFilePath string) (string, error) {
	originalExt := strings.ToLower(filepath.Ext(finalFilePath))
	// Map of known image MIME types to extensions
	mimeToExt := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
		"image/webp": ".webp",
	}

	fRead, errRead := os.Open(tempFilePath)
	if errRead != nil {
		log.WithError(errRead).Warnf("Could not open file %s for MIME type detection, using original extension.", tempFilePath)
		// Return original path, don't treat this as a fatal error for the caller
		return finalFilePath, nil
	}
	defer fRead.Close()

	// Read the first 512 bytes for MIME detection
	buff := make([]byte, 512)
	n, errReadBytes := fRead.Read(buff)
	if errReadBytes != nil && errReadBytes != io.EOF {
		log.WithError(errReadBytes).Warnf("Could not read from file %s for MIME type detection, using original extension.", tempFilePath)
		// Return original path
		return finalFilePath, nil
	}

	// Only use the bytes actually read
	mimeType := http.DetectContentType(buff[:n])
	// Extract the main type (e.g., "image/jpeg")
	mainMimeType := strings.Split(mimeType, ";")[0]

	correctedFinalPath := finalFilePath // Default to original

	if detectedExt, ok := mimeToExt[mainMimeType]; ok {
		log.Debugf("Detected MIME type: %s -> Extension: %s for %s", mimeType, detectedExt, tempFilePath)

		// Check for mismatch, BUT allow .jpeg for image/jpeg
		mismatch := originalExt != detectedExt
		if mismatch && mainMimeType == "image/jpeg" && originalExt == ".jpeg" {
			mismatch = false // Allow .jpeg extension for jpeg content
			log.Debugf("Original extension '.jpeg' is valid for detected type 'image/jpeg'. No correction needed.")
		}

		if mismatch { // Correct only if it's a real mismatch
			correctedFinalPath = strings.TrimSuffix(finalFilePath, originalExt) + detectedExt
			log.Warnf("Original extension '%s' differs from detected image type '%s'. Correcting final path to: %s", originalExt, detectedExt, correctedFinalPath)
		} else if originalExt == detectedExt { // Log if it matched exactly
			log.Debugf("Original extension '%s' matches detected image type '%s'. No path correction needed.", originalExt, detectedExt)
		}
	} else {
		log.Debugf("Detected MIME type '%s' for %s is not in the recognized image map. Using original extension '%s'.", mimeType, tempFilePath, originalExt)
	}

	return correctedFinalPath, nil
}

// TODO: Move loadConfig function to internal/config/config.go

// -- Hashing Helper --
func calculateHash(filePath string, hashAlgo hash.Hash) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file %s for hashing: %w", filePath, err)
	}
	defer file.Close()

	if _, err := io.Copy(hashAlgo, file); err != nil {
		return "", fmt.Errorf("hashing file %s: %w", filePath, err)
	}

	return hex.EncodeToString(hashAlgo.Sum(nil)), nil
}

// -- MIME Type to Extension Mapping --

// Define a map for common MIME types to their preferred extensions
var mimeTypeToExtensionMap = map[string]string{
	"image/jpeg": ".jpg", // Covers jpg and jpeg
	"video/mp4":  ".mp4",
	"image/webp": ".webp", // Common for animated webp, though technically video/webp exists
	// Add more mappings as needed
}

// GetExtensionFromMimeType attempts to return a standard file extension for a given MIME type.
// It considers the primary type (e.g., "image/png") and ignores parameters (like charset).
// Returns the extension (including the dot) and true if found, otherwise empty string and false.
func GetExtensionFromMimeType(mimeType string) (string, bool) {
	// MIME types can have parameters like "text/plain; charset=utf-8"
	// We only care about the main type.
	mainMimeType := strings.Split(mimeType, ";")[0]
	ext, ok := mimeTypeToExtensionMap[strings.ToLower(mainMimeType)]
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
