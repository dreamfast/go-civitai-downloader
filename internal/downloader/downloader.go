package downloader

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go-civitai-download/internal/helpers"
	"go-civitai-download/internal/models"

	log "github.com/sirupsen/logrus"
)

// Custom Downloader Errors
var (
	ErrHashMismatch = errors.New("downloaded file hash mismatch")
	ErrHttpStatus   = errors.New("unexpected HTTP status code")
	ErrFileSystem   = errors.New("filesystem error") // Covers create, remove, rename
	ErrHttpRequest  = errors.New("HTTP request creation/execution error")
)

// Downloader handles downloading files with progress and hash checks.
type Downloader struct {
	client *http.Client
	apiKey string // Add field to store API key
}

// NewDownloader creates a new Downloader instance.
func NewDownloader(client *http.Client, apiKey string) *Downloader {
	if client == nil {
		// Provide a default client if none is passed
		client = &http.Client{
			Timeout: 15 * time.Minute,
		}
	}
	return &Downloader{
		client: client,
		apiKey: apiKey, // Store the API key
	}
}

// Helper function to check for existing file by base name and hash.
// Now requires the expected file extension to avoid checking hashes on mismatched file types (e.g., .json vs .safetensors).
func findExistingFileWithMatchingBaseAndHash(dirPath string, baseNameWithoutExt string, expectedExt string, hashes models.Hashes) (foundPath string, exists bool, err error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil // Directory doesn't exist, so file doesn't exist
		}
		return "", false, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	log.Debugf("Scanning directory %s for base name '%s' with expected extension '%s' and matching hash...", dirPath, baseNameWithoutExt, expectedExt)
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories
		}
		entryName := entry.Name()
		ext := filepath.Ext(entryName)
		entryBaseName := strings.TrimSuffix(entryName, ext)

		if strings.EqualFold(entryBaseName, baseNameWithoutExt) {
			// Base names match
			fullPath := filepath.Join(dirPath, entryName)

			hashesProvided := hashes.SHA256 != "" || hashes.BLAKE3 != "" || hashes.CRC32 != "" || hashes.AutoV2 != ""

			if !hashesProvided {
				// No standard hashes provided (likely an image), base name match is enough
				log.Debugf("Base name match found and no standard hashes provided. Assuming valid existing file: %s", fullPath)
				return fullPath, true, nil
			} else {
				// Hashes ARE provided. Check if the extension ALSO matches before checking hash.
				if strings.EqualFold(ext, expectedExt) {
					log.Debugf("Base name and extension match found: %s. Checking hash...", fullPath)
					if helpers.CheckHash(fullPath, hashes) {
						log.Debugf("Hash match successful for existing file: %s", fullPath)
						return fullPath, true, nil // Found a valid match!
					} else {
						log.Debugf("Hash mismatch for existing file with matching base name and extension: %s", fullPath)
						// Continue checking other files in case of duplicates with different content but same name/ext
					}
				} else {
					log.Debugf("Base name match found (%s), but extension '%s' does not match expected '%s'. Skipping hash check.", fullPath, ext, expectedExt)
				}
			}
		}
	}

	log.Debugf("No valid existing file found matching base name '%s' and extension '%s' in %s", baseNameWithoutExt, expectedExt, dirPath)
	return "", false, nil // No matching file found
}

// checkExistingFile checks if a file already exists with the correct hash
func (d *Downloader) checkExistingFile(targetFilepath string, hashes models.Hashes) (string, bool, error) {
	targetDir := filepath.Dir(targetFilepath)
	baseName := filepath.Base(targetFilepath)
	ext := filepath.Ext(baseName)
	baseNameWithoutExt := strings.TrimSuffix(baseName, ext)

	log.Debugf("Checking for existing file based on path: Dir=%s, BaseName=%s, Ext=%s", targetDir, baseNameWithoutExt, ext)
	foundPath, exists, err := findExistingFileWithMatchingBaseAndHash(targetDir, baseNameWithoutExt, ext, hashes)
	if err != nil {
		log.WithError(err).Errorf("Error during check for existing file matching %s%s in %s", baseNameWithoutExt, ext, targetDir)
		return "", false, fmt.Errorf("%w: check for existing file: %v", ErrFileSystem, err)
	}
	if exists {
		log.Infof("Found valid existing file matching base name '%s' and extension '%s': %s. Skipping download.", baseNameWithoutExt, ext, foundPath)
		return foundPath, true, nil
	}
	log.Infof("No valid file matching base name '%s' and extension '%s' found. Proceeding with download process.", baseNameWithoutExt, ext)
	return "", false, nil
}

// createHTTPRequest creates and configures an HTTP request for downloading
func (d *Downloader) createHTTPRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: creating download request for %s: %w", ErrHttpRequest, url, err)
	}

	if d.apiKey != "" {
		log.Debug("Adding Authorization header to download request.")
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	} else {
		log.Debug("No API Key found, skipping Authorization header for download.")
	}

	return req, nil
}

// extractFilenameFromResponse extracts filename from Content-Disposition header
func extractFilenameFromResponse(resp *http.Response) string {
	contentDisposition := resp.Header.Get("Content-Disposition")
	if contentDisposition == "" {
		log.Warn("Warning: No Content-Disposition header found, will use constructed filename.")
		return ""
	}

	_, params, err := mime.ParseMediaType(contentDisposition)
	if err == nil && params["filename"] != "" {
		log.Infof("Received filename from Content-Disposition: %s", params["filename"])
		return params["filename"]
	}

	if strings.HasPrefix(contentDisposition, "inline") && params["filename"] == "" {
		log.Debugf("Content-Disposition is '%s' (no filename), using constructed filename.", contentDisposition)
	} else {
		log.WithError(err).Warnf("Could not parse Content-Disposition header: %s", contentDisposition)
	}
	return ""
}

// constructFinalPath creates the final file path with version ID and API filename
func constructFinalPath(originalPath, apiFilename string, modelVersionID int) string {
	var baseFilenameToUse string
	if apiFilename != "" {
		baseFilenameToUse = apiFilename
	} else {
		baseFilenameToUse = filepath.Base(originalPath)
	}

	pathBeforeId := filepath.Join(filepath.Dir(originalPath), baseFilenameToUse)

	if modelVersionID > 0 {
		finalPath := filepath.Join(filepath.Dir(pathBeforeId), fmt.Sprintf("%d_%s", modelVersionID, baseFilenameToUse))
		log.Debugf("Prepended model version ID, final target path: %s", finalPath)
		return finalPath
	}

	log.Debugf("Model version ID is 0, final target path: %s", pathBeforeId)
	return pathBeforeId
}

// downloadToTemp downloads the response body to a temporary file
func downloadToTemp(resp *http.Response, tempFile *os.File, targetPath string) error {
	size, _ := strconv.ParseUint(resp.Header.Get("Content-Length"), 10, 64)

	counter := &helpers.CounterWriter{
		Writer: tempFile,
		Total:  0,
	}

	log.Infof("Downloading to %s (Target: %s, Size: %s)...",
		tempFile.Name(),
		targetPath,
		helpers.BytesToSize(size),
	)

	_, err := io.Copy(counter, resp.Body)
	if err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("writing to temporary file %s: %w", tempFile.Name(), err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("%w: closing temporary file %s: %w", ErrFileSystem, tempFile.Name(), err)
	}

	log.Infof("Finished writing %s.", tempFile.Name())
	return nil
}

// detectMimeAndRename detects MIME type and renames temp file with correct extension
func detectMimeAndRename(tempFilePath, finalPath string) (string, error) {
	fileForDetect, err := os.Open(tempFilePath)
	if err != nil {
		log.WithError(err).Errorf("Failed to re-open temp file %s for MIME detection", tempFilePath)
		return "", fmt.Errorf("%w: opening temp file for mime detection: %w", ErrFileSystem, err)
	}
	defer fileForDetect.Close()

	buffer := make([]byte, 512)
	n, err := fileForDetect.Read(buffer)
	if err != nil && err != io.EOF {
		log.WithError(err).Errorf("Failed to read from temp file %s for MIME detection", tempFilePath)
		return "", fmt.Errorf("%w: reading temp file for mime detection: %w", ErrFileSystem, err)
	}

	mimeType := http.DetectContentType(buffer[:n])
	log.Debugf("Detected MIME type for %s: %s", tempFilePath, mimeType)

	// Get correct extension based on MIME type
	finalDir := filepath.Dir(finalPath)
	finalBaseName := filepath.Base(finalPath)
	finalExt := filepath.Ext(finalBaseName)
	finalBaseNameWithoutExt := strings.TrimSuffix(finalBaseName, finalExt)

	correctExt, ok := helpers.GetExtensionFromMimeType(mimeType)
	if !ok {
		log.Warnf("Could not determine standard extension for detected MIME type '%s'. Using original extension '%s'.", mimeType, finalExt)
		correctExt = finalExt
	}

	finalPathWithCorrectExt := filepath.Join(finalDir, finalBaseNameWithoutExt+correctExt)
	log.Debugf("Final path with corrected extension based on MIME type: %s", finalPathWithCorrectExt)

	log.Debugf("Renaming temporary file %s to final path %s", tempFilePath, finalPathWithCorrectExt)
	if err := os.Rename(tempFilePath, finalPathWithCorrectExt); err != nil {
		return "", fmt.Errorf("%w: renaming temporary file %s to %s: %w", ErrFileSystem, tempFilePath, finalPathWithCorrectExt, err)
	}

	log.Infof("Successfully renamed temp file to %s", finalPathWithCorrectExt)
	return finalPathWithCorrectExt, nil
}

// verifyHash verifies the downloaded file hash if provided
func verifyHash(filePath string, hashes models.Hashes) error {
	hashesProvided := hashes.SHA256 != "" || hashes.BLAKE3 != "" || hashes.CRC32 != "" || hashes.AutoV2 != ""
	if !hashesProvided {
		log.Debugf("Skipping hash verification for %s (no expected hashes provided).", filePath)
		return nil
	}

	log.Debugf("Verifying hash for final file: %s", filePath)
	if !helpers.CheckHash(filePath, hashes) {
		log.Errorf("Hash mismatch for downloaded file: %s", filePath)
		return ErrHashMismatch
	}

	log.Infof("Hash verified for %s.", filePath)
	return nil
}

// DownloadFile downloads a file from the specified URL to the target filepath.
// It checks for existing files, verifies hashes, and attempts to use the
// Content-Disposition header for the filename.
func (d *Downloader) DownloadFile(targetFilepath string, url string, hashes models.Hashes, modelVersionID int) (string, error) {
	// Check for existing file first
	existingPath, exists, err := d.checkExistingFile(targetFilepath, hashes)
	if err != nil {
		return "", err
	}
	if exists {
		return existingPath, nil
	}

	// Ensure target directory exists
	targetDir := filepath.Dir(targetFilepath)
	if !helpers.CheckAndMakeDir(targetDir) {
		return "", fmt.Errorf("%w: failed to create target directory %s", ErrFileSystem, targetDir)
	}

	// Create temporary file
	baseName := filepath.Base(targetFilepath)
	tempFile, err := os.CreateTemp(targetDir, baseName+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("%w: creating temporary file %s: %w", ErrFileSystem, targetFilepath, err)
	}

	shouldCleanupTemp := true
	defer func() {
		if shouldCleanupTemp {
			log.Debugf("Cleaning up temporary file via defer: %s", tempFile.Name())
			if removeErr := os.Remove(tempFile.Name()); removeErr != nil {
				log.WithError(removeErr).Warnf("Failed to remove temporary file %s during defer cleanup", tempFile.Name())
			}
		}
	}()

	log.Info("Starting download process...")
	log.Infof("Attempting to download from URL: %s", url)

	// Create and execute HTTP request
	req, err := d.createHTTPRequest(url)
	if err != nil {
		return "", err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		log.WithError(err).Errorf("Error performing download request from %s", url)
		return "", fmt.Errorf("%w: performing request for %s: %v", ErrHttpRequest, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Errorf("Error downloading file: Received status code %d from %s", resp.StatusCode, url)
		return "", fmt.Errorf("%w: received status %d from %s", ErrHttpStatus, resp.StatusCode, url)
	}

	// Extract filename from response and construct final path
	apiFilename := extractFilenameFromResponse(resp)
	finalFilepath := constructFinalPath(targetFilepath, apiFilename, modelVersionID)

	// Check if final path already exists
	existingFinalPath, existsFinal, err := d.checkExistingFile(finalFilepath, hashes)
	if err != nil {
		return "", err
	}
	if existsFinal {
		return existingFinalPath, nil
	}

	// Download to temporary file
	if err := downloadToTemp(resp, tempFile, finalFilepath); err != nil {
		return "", err
	}

	// Detect MIME type and rename with correct extension
	finalPath, err := detectMimeAndRename(tempFile.Name(), finalFilepath)
	if err != nil {
		return "", err
	}
	shouldCleanupTemp = false // Success, don't cleanup

	// Verify hash
	if err := verifyHash(finalPath, hashes); err != nil {
		return "", err
	}

	log.Infof("Successfully downloaded and verified %s", finalPath)
	return finalPath, nil
}

// DownloadImage downloads an image from a URL to a specified directory.
// It determines the filename from the URL path.
// Returns the final filename (not the full path) and an error if one occurred.
func (d *Downloader) DownloadImage(targetDir string, url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("%w: creating image request for %s: %w", ErrHttpRequest, url, err)
	}
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: performing image request for %s: %v", ErrHttpRequest, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: received status %d for image %s", ErrHttpStatus, resp.StatusCode, url)
	}

	// Determine filename from URL
	baseName := filepath.Base(url)
	// A simple cleanup to remove query params from filename
	if queryIndex := strings.Index(baseName, "?"); queryIndex != -1 {
		baseName = baseName[:queryIndex]
	}
	if baseName == "" {
		baseName = "unknown_image" // Fallback filename
	}

	finalPath := filepath.Join(targetDir, baseName)

	// Create the destination file
	outFile, err := os.Create(finalPath)
	if err != nil {
		return "", fmt.Errorf("%w: creating image file %s: %w", ErrFileSystem, finalPath, err)
	}
	defer outFile.Close()

	// Copy the response body to the file
	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		// Attempt to remove the partial file on error
		_ = os.Remove(finalPath)
		return "", fmt.Errorf("writing to image file %s: %w", finalPath, err)
	}

	return baseName, nil
}
