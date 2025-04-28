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

// DownloadFile downloads a file from the specified URL to the target filepath.
// It checks for existing files, verifies hashes, and attempts to use the
// Content-Disposition header for the filename.
// It also now accepts a modelVersionID to prepend to the final filename.
// Returns the final filepath used (or empty string on failure) and an error if one occurred.
func (d *Downloader) DownloadFile(targetFilepath string, url string, hashes models.Hashes, modelVersionID int) (string, error) {
	initialFinalFilepath := targetFilepath // Store the initially constructed path
	targetDir := filepath.Dir(initialFinalFilepath)
	initialBaseName := filepath.Base(initialFinalFilepath)
	initialExt := filepath.Ext(initialBaseName)
	initialBaseNameWithoutExt := strings.TrimSuffix(initialBaseName, initialExt)

	log.Debugf("Checking for existing file based on initial path: Dir=%s, BaseName=%s, Ext=%s", targetDir, initialBaseNameWithoutExt, initialExt)
	// --- Initial Check for Existing File (using new helper with expected extension) ---
	foundPath, exists, errCheck := findExistingFileWithMatchingBaseAndHash(targetDir, initialBaseNameWithoutExt, initialExt, hashes)
	if errCheck != nil {
		log.WithError(errCheck).Errorf("Error during initial check for existing file matching %s%s in %s", initialBaseNameWithoutExt, initialExt, targetDir)
		return "", fmt.Errorf("%w: initial check for existing file: %v", ErrFileSystem, errCheck)
	}
	if exists {
		log.Infof("Found valid existing file matching base name '%s' and extension '%s': %s. Skipping download.", initialBaseNameWithoutExt, initialExt, foundPath)
		return foundPath, nil // Success, return the path of the valid existing file
	}
	log.Infof("No valid file matching base name '%s' and extension '%s' found initially. Proceeding with download process.", initialBaseNameWithoutExt, initialExt)
	// --- End Initial Check ---

	// Ensure target directory exists before creating temp file
	if !helpers.CheckAndMakeDir(targetDir) {
		return "", fmt.Errorf("%w: failed to create target directory %s", ErrFileSystem, targetDir)
	}

	// Create a temporary file in the target directory
	baseName := filepath.Base(targetFilepath)
	tempFile, err := os.CreateTemp(targetDir, baseName+".*.tmp") // Use targetDir here
	if err != nil {
		return "", fmt.Errorf("%w: creating temporary file %s: %w", ErrFileSystem, targetFilepath, err)
	}
	// Use a flag to track if we should remove the temp file on error exit
	shouldCleanupTemp := true
	defer func() {
		if shouldCleanupTemp {
			// If tempFile wasn't closed explicitly due to an early error *before* the explicit close,
			// or if it was closed but we still need to cleanup (e.g., hash mismatch),
			// we might need to close it here, but the explicit close should handle most cases.
			// The main goal here is the os.Remove.
			log.Debugf("Cleaning up temporary file via defer: %s", tempFile.Name())
			if removeErr := os.Remove(tempFile.Name()); removeErr != nil {
				log.WithError(removeErr).Warnf("Failed to remove temporary file %s during defer cleanup", tempFile.Name())
			}
		}
	}()

	log.Info("Starting download process...") // Log before creating temp file

	log.Infof("Attempting to download from URL: %s", url)

	// Create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("%w: creating download request for %s: %w", ErrHttpRequest, url, err)
	}

	// Add authentication header if API key is present
	log.Debugf("Downloader stored API Key: %s", d.apiKey) // Added Debug Log
	if d.apiKey != "" {
		log.Debug("Adding Authorization header to download request.") // Added Debug Log
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	} else {
		log.Debug("No API Key found, skipping Authorization header for download.") // Added Debug Log
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

	// --- Filename Handling from Content-Disposition ---
	// Recalculate finalFilepath based on header
	contentDisposition := resp.Header.Get("Content-Disposition")
	potentialApiFilename := "" // Store potential filename from header
	if contentDisposition != "" {
		_, params, err := mime.ParseMediaType(contentDisposition)
		if err == nil && params["filename"] != "" {
			potentialApiFilename = params["filename"]
			log.Infof("Received filename from Content-Disposition: %s", potentialApiFilename)
		} else {
			// If the disposition is 'inline' and has no filename, it's expected, log as debug.
			if strings.HasPrefix(contentDisposition, "inline") && params["filename"] == "" {
				log.Debugf("Content-Disposition is '%s' (no filename), using constructed filename.", contentDisposition)
			} else {
				// Log other parsing issues as warnings.
				log.WithError(err).Warnf("Could not parse Content-Disposition header: %s", contentDisposition)
			}
		}
	} else {
		log.Warn("Warning: No Content-Disposition header found, will use constructed filename.")
	}

	// Determine the base filename to use (API provided or original)
	var baseFilenameToUse string
	if potentialApiFilename != "" {
		baseFilenameToUse = potentialApiFilename
	} else {
		baseFilenameToUse = filepath.Base(targetFilepath) // Use original base filename if API doesn't provide one
	}
	// Construct the path *before* prepending ID
	pathBeforeId := filepath.Join(filepath.Dir(targetFilepath), baseFilenameToUse)

	var finalFilepath string // Declare finalFilepath here
	// --- Prepend Model Version ID to Filename ---
	if modelVersionID > 0 { // Only prepend if ID is valid
		finalFilepath = filepath.Join(filepath.Dir(pathBeforeId), fmt.Sprintf("%d_%s", modelVersionID, baseFilenameToUse))
		log.Debugf("Prepended model version ID, final target path: %s", finalFilepath)
	} else {
		finalFilepath = pathBeforeId // Use the path without ID if ID is 0
		log.Debugf("Model version ID is 0, final target path: %s", finalFilepath)
	}

	// --- Check Existence of FINAL Path (with potential API name and ID, using new helper) ---
	finalTargetDir := filepath.Dir(finalFilepath)
	finalBaseName := filepath.Base(finalFilepath)
	finalExt := filepath.Ext(finalBaseName) // Get extension from the FINAL path
	finalBaseNameWithoutExt := strings.TrimSuffix(finalBaseName, finalExt)

	log.Debugf("Checking for existing file based on determined final path: Dir=%s, BaseName=%s, Ext=%s", finalTargetDir, finalBaseNameWithoutExt, finalExt)
	foundPathFinal, existsFinal, errCheckFinal := findExistingFileWithMatchingBaseAndHash(finalTargetDir, finalBaseNameWithoutExt, finalExt, hashes)
	if errCheckFinal != nil {
		log.WithError(errCheckFinal).Errorf("Error during final check for existing file matching %s%s in %s", finalBaseNameWithoutExt, finalExt, finalTargetDir)
		return "", fmt.Errorf("%w: final check for existing file: %v", ErrFileSystem, errCheckFinal)
	}
	if existsFinal {
		log.Infof("Found valid existing file matching final base name '%s' and extension '%s': %s. Download not needed.", finalBaseNameWithoutExt, finalExt, foundPathFinal)
		shouldCleanupTemp = true   // Ensure any temp file created before this check is removed
		return foundPathFinal, nil // Success, return the path of the valid existing file
	}
	log.Debugf("Final target file base name '%s' with extension '%s' does not exist with valid hash. Proceeding with network download to temp file.", finalBaseNameWithoutExt, finalExt)
	// --- End Final Path Check ---

	// Get the size of the file
	size, _ := strconv.ParseUint(resp.Header.Get("Content-Length"), 10, 64)

	// Create a CounterWriter
	counter := &helpers.CounterWriter{
		Writer: tempFile,
		Total:  0,
	}

	// --- Perform Download ---
	// Write the body to the temporary file
	log.Infof("Downloading to %s (Target: %s, Size: %s)...",
		tempFile.Name(),
		finalFilepath,
		helpers.BytesToSize(size),
	)
	_, err = io.Copy(counter, resp.Body)
	if err != nil {
		// Explicitly close here before returning error, as defer won't run until func exits
		_ = tempFile.Close()
		return "", fmt.Errorf("writing to temporary file %s: %w", tempFile.Name(), err)
	}

	// Explicitly close the temporary file *after* writing is complete
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("%w: closing temporary file %s: %w", ErrFileSystem, tempFile.Name(), err)
	}
	log.Infof("Finished writing %s.", tempFile.Name())

	// --- MIME Type Detection and Renaming --- START ---
	// Re-open the temp file to detect content type
	fileForDetect, err := os.Open(tempFile.Name())
	if err != nil {
		log.WithError(err).Errorf("Failed to re-open temp file %s for MIME detection", tempFile.Name())
		return "", fmt.Errorf("%w: opening temp file for mime detection: %w", ErrFileSystem, err)
	}

	buffer := make([]byte, 512) // Read first 512 bytes
	n, err := fileForDetect.Read(buffer)
	if err != nil && err != io.EOF {
		_ = fileForDetect.Close()
		log.WithError(err).Errorf("Failed to read from temp file %s for MIME detection", tempFile.Name())
		return "", fmt.Errorf("%w: reading temp file for mime detection: %w", ErrFileSystem, err)
	}
	_ = fileForDetect.Close() // Close after reading

	mimeType := http.DetectContentType(buffer[:n])
	log.Debugf("Detected MIME type for %s: %s", tempFile.Name(), mimeType)

	// Get the correct extension based on the detected MIME type
	// Placeholder: Need a function/map like helpers.GetExtensionFromMimeType(mimeType)
	correctExt, ok := helpers.GetExtensionFromMimeType(mimeType) // Assuming this helper exists
	if !ok {
		log.Warnf("Could not determine standard extension for detected MIME type '%s'. Using original extension '%s'.", mimeType, finalExt)
		correctExt = finalExt // Fallback to the originally determined extension
	}

	// Reconstruct the final filename WITH the correct extension
	finalBaseNameWithCorrectExt := finalBaseNameWithoutExt + correctExt // Use base name derived earlier
	finalPathWithCorrectExt := filepath.Join(finalTargetDir, finalBaseNameWithCorrectExt)
	log.Debugf("Final path with corrected extension based on MIME type: %s", finalPathWithCorrectExt)

	// Use the corrected path for renaming
	finalFilepath = finalPathWithCorrectExt // *** OVERWRITE finalFilepath ***
	// --- MIME Type Detection and Renaming --- END ---

	// Rename the temporary file to the final path (which now has the corrected extension)
	log.Debugf("Renaming temporary file %s to final path %s", tempFile.Name(), finalFilepath)
	if err := os.Rename(tempFile.Name(), finalFilepath); err != nil {
		return "", fmt.Errorf("%w: renaming temporary file %s to %s: %w", ErrFileSystem, tempFile.Name(), finalFilepath, err)
	}
	log.Infof("Successfully renamed temp file to %s", finalFilepath)
	shouldCleanupTemp = false // Prevent defer from removing the final file

	// --- Hash Verification --- START ---
	// Check if any standard hashes were actually provided
	hashesProvided := hashes.SHA256 != "" || hashes.BLAKE3 != "" || hashes.CRC32 != "" || hashes.AutoV2 != ""
	if !hashesProvided {
		log.Debugf("Skipping hash verification for %s (no expected hashes provided).", finalFilepath)
	} else {
		// Hashes were provided, perform the check
		log.Debugf("Verifying hash for final file: %s", finalFilepath)
		if !helpers.CheckHash(finalFilepath, hashes) {
			log.Errorf("Hash mismatch for downloaded file: %s", finalFilepath)
			// Optionally remove the bad file
			// if removeErr := os.Remove(finalFilepath); removeErr != nil {
			// 	 log.WithError(removeErr).Warnf("Failed to remove mismatched file: %s", finalFilepath)
			// }
			return "", ErrHashMismatch
		}
		log.Infof("Hash verified for %s.", finalFilepath) // Log verification success
	}
	// --- Hash Verification --- END ---

	// Success!
	log.Infof("Successfully downloaded and verified %s", finalFilepath)
	return finalFilepath, nil
}
