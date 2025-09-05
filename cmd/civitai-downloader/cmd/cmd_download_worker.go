package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-civitai-download/internal/database"
	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/models"

	"github.com/gosuri/uilive"
	log "github.com/sirupsen/logrus"
)

// Package-level map to track processed models for model images, with a mutex for safe concurrent access.
var (
	processedModelImages     = make(map[int]bool)
	processedModelImagesLock = &sync.Mutex{}
)

// updateDbEntry encapsulates the logic for getting, updating, and putting a database entry.
// It takes the database connection, the key, the new status (string), and an optional function
// to apply further modifications to the entry before saving.
func updateDbEntry(db *database.DB, key string, newStatus string, updateFunc func(*models.DatabaseEntry)) error {
	log.Debugf("Attempting to update DB entry '%s' to status '%s'", key, newStatus)

	log.Debugf("Getting existing entry for key '%s'...", key)
	rawValue, errGet := db.Get([]byte(key))
	if errGet != nil {
		log.WithError(errGet).Errorf("Failed to get DB entry '%s' for update", key)
		return fmt.Errorf("failed to get DB entry '%s': %w", key, errGet)
	}
	log.Debugf("Successfully got existing entry for key '%s' (%d bytes)", key, len(rawValue))

	var entry models.DatabaseEntry
	log.Debugf("Unmarshalling existing entry for key '%s'...", key)
	if errUnmarshal := json.Unmarshal(rawValue, &entry); errUnmarshal != nil {
		log.WithError(errUnmarshal).Errorf("Failed to unmarshal DB entry '%s' for update", key)
		// Log raw value snippet on unmarshal error for debugging
		rawSnippet := string(rawValue)
		if len(rawSnippet) > 200 {
			rawSnippet = rawSnippet[:200] + "..."
		}
		log.Debugf("Raw data snippet for key '%s': %s", key, rawSnippet)
		return fmt.Errorf("failed to unmarshal DB entry '%s': %w", key, errUnmarshal)
	}
	log.Debugf("Successfully unmarshalled existing entry for key '%s'", key)

	// Update the status
	entry.Status = newStatus

	// Apply additional modifications if provided
	if updateFunc != nil {
		log.Debugf("Applying update function to entry for key '%s'...", key)
		updateFunc(&entry)
		log.Debugf("Update function applied for key '%s'", key)
	}

	// Log the entry *before* marshalling (optional, can be verbose)

	log.Debugf("Marshalling updated entry for key '%s'...", key)
	updatedEntryBytes, marshalErr := json.Marshal(entry)
	if marshalErr != nil {
		log.WithError(marshalErr).Errorf("Failed to marshal updated DB entry '%s' (Status: %s)", key, newStatus)
		return fmt.Errorf("failed to marshal DB entry '%s': %w", key, marshalErr)
	}
	log.Debugf("Successfully marshalled updated entry for key '%s' (%d bytes)", key, len(updatedEntryBytes))

	log.Debugf("Putting updated entry for key '%s' into DB...", key)
	if errPut := db.Put([]byte(key), updatedEntryBytes); errPut != nil {
		log.WithError(errPut).Errorf("Failed to update DB entry '%s' to status %s", key, newStatus)
		return fmt.Errorf("failed to put DB entry '%s': %w", key, errPut)
	}

	log.Infof("Successfully updated DB entry '%s' to status %s", key, newStatus)
	return nil
}

// handleMetadataSaving checks config flags and calls the appropriate metadata saving functions.
// It's called by the worker after a file download has successfully completed.
func handleMetadataSaving(logPrefix string, pd potentialDownload, finalPath string, finalStatus string, writer *uilive.Writer, cfg *models.Config) {
	if finalStatus != models.StatusDownloaded {
		log.Debugf("[%s] Skipping all metadata saving for %s due to download status: %s.", logPrefix, pd.TargetFilepath, finalStatus)
		return
	}

	// Save Version-Specific Metadata JSON (--metadata)
	if cfg.Download.SaveMetadata {
		log.Debugf("[%s] Saving version metadata for successfully downloaded file: %s", logPrefix, finalPath)
		if metaErr := saveVersionMetadataFile(pd, finalPath); metaErr != nil {
			if writer != nil {
				fmt.Fprintf(writer.Newline(), "[%s] Error saving version metadata for %s: %v\n", logPrefix, filepath.Base(finalPath), metaErr)
			}
			// Error is already logged by saveVersionMetadataFile
		}
	} else {
		log.Debugf("[%s] Skipping version metadata save (disabled by --metadata) for %s.", logPrefix, finalPath)
	}

	// Save Model Info JSON (--model-info)
	if cfg.Download.SaveModelInfo {
		log.Debugf("[%s] Saving model info for successfully downloaded file: %s", logPrefix, finalPath)
		// This function is now in cmd_download_processing.go
		if infoErr := saveModelInfoFile(pd, cfg); infoErr != nil {
			if writer != nil {
				fmt.Fprintf(writer.Newline(), "[%s] Error saving model info for %s: %v\n", logPrefix, pd.ModelName, infoErr)
			}
			// Error is already logged by saveModelInfoFile
		}
	} else {
		log.Debugf("[%s] Skipping model info save (disabled by --model-info) for %s.", logPrefix, finalPath)
	}
}

// handleModelImages handles the download of all images for a given model if the --model-images flag is set.
// It uses a shared map to ensure images for a model are only processed once per application run.
// It now accepts the finalPath of the downloaded file to correctly determine the parent directory.
func handleModelImages(logPrefix string, pd potentialDownload, finalPath string, imageDownloader *downloader.Downloader, cfg *models.Config) {
	if !cfg.Download.SaveModelImages {
		return // Exit if the feature is not enabled
	}

	processedModelImagesLock.Lock()
	alreadyProcessed := processedModelImages[pd.ModelID]
	processedModelImagesLock.Unlock()

	if alreadyProcessed {
		log.Debugf("%s Model images for model ID %d already processed. Skipping.", logPrefix, pd.ModelID)
		return
	}

	// Collect all images from all versions
	var allModelImages []models.ModelImage
	for _, version := range pd.FullModel.ModelVersions {
		if len(version.Images) > 0 {
			allModelImages = append(allModelImages, version.Images...)
		}
	}

	if len(allModelImages) == 0 {
		log.Debugf("%s No model images found across all versions for model %d.", logPrefix, pd.ModelID)
		processedModelImagesLock.Lock()
		processedModelImages[pd.ModelID] = true // Mark as processed to avoid re-checking
		processedModelImagesLock.Unlock()
		return
	}

	// --- Correctly determine the model's base directory ---
	// The `finalPath` is the absolute path to the downloaded *version file*.
	// The directory containing this file is the version-specific directory.
	// The directory containing the version-specific directory is the model's base directory.
	versionSpecificDir := filepath.Dir(finalPath)
	modelBaseDir := filepath.Dir(versionSpecificDir)
	modelImageDir := filepath.Join(modelBaseDir, "images")
	imgLogPrefix := fmt.Sprintf("[%s-ModelImg]", logPrefix)

	// Now, `modelImageDir` should be correct, e.g., `/path/to/downloads/lora/sdxl/model-name/images`

	if err := os.MkdirAll(modelImageDir, 0750); err != nil {
		log.WithError(err).Errorf("%s Failed to create directory %s for model images", imgLogPrefix, modelImageDir)
		return
	}

	log.Infof("%s Downloading %d model images to %s", imgLogPrefix, len(allModelImages), modelImageDir)
	imgSuccess, imgFail := downloadImages(imgLogPrefix, allModelImages, modelImageDir, imageDownloader, cfg.Download.Concurrency)
	log.Infof("%s Finished downloading model images. Success: %d, Failures: %d", imgLogPrefix, imgSuccess, imgFail)

	processedModelImagesLock.Lock()
	processedModelImages[pd.ModelID] = true // Mark model as processed
	processedModelImagesLock.Unlock()
}

// WorkerContext holds the context for a download worker
type WorkerContext struct {
	ID              int
	LogPrefix       string
	ProcessedCount  int
	TotalJobs       int
	DB              *database.DB
	FileDownloader  *downloader.Downloader
	ImageDownloader *downloader.Downloader
	Writer          *uilive.Writer
	Config          *models.Config
}

// checkInitialDBStatus checks and returns the initial database status for a job
func (ctx *WorkerContext) checkInitialDBStatus(dbKey string, targetFilepath string) (string, string, error) {
	directoryPath := filepath.Dir(targetFilepath)
	initialDbStatus := models.StatusPending
	finalPath := targetFilepath

	rawValue, errGet := ctx.DB.Get([]byte(dbKey))
	if errGet == nil {
		var entry models.DatabaseEntry
		if errUnmarshal := json.Unmarshal(rawValue, &entry); errUnmarshal == nil {
			initialDbStatus = entry.Status
			if initialDbStatus == models.StatusDownloaded && entry.Filename != "" {
				finalPath = filepath.Join(directoryPath, entry.Filename)
				log.Debugf("[%s] Initial DB status is Downloaded. Using existing filename from DB: %s", ctx.LogPrefix, entry.Filename)
			}
		} else {
			log.WithError(errUnmarshal).Warnf("[%s] Failed to unmarshal existing DB entry for key %s during initial check. Assuming Pending.", ctx.LogPrefix, dbKey)
		}
	} else if !errors.Is(errGet, database.ErrNotFound) {
		log.WithError(errGet).Warnf("[%s] Error checking initial DB status for key %s. Assuming Pending.", ctx.LogPrefix, dbKey)
	}

	log.Debugf("[%s] Initial status for job %s determined as: %s", ctx.LogPrefix, dbKey, initialDbStatus)
	return initialDbStatus, finalPath, errGet
}

// ensureDirectory creates the directory if it doesn't exist
func (ctx *WorkerContext) ensureDirectory(directoryPath, dbKey string, errGet error) error {
	if err := os.MkdirAll(directoryPath, 0750); err != nil {
		log.WithError(err).Errorf("Worker %d: Failed to create directory %s", ctx.ID, directoryPath)

		updateErr := updateDbEntry(ctx.DB, dbKey, models.StatusError, func(entry *models.DatabaseEntry) {
			if errors.Is(errGet, database.ErrNotFound) || errGet != nil {
				entry.ErrorDetails = fmt.Sprintf("Failed to create directory: %v", err)
			}
		})
		if updateErr != nil {
			log.Errorf("Worker %d: Failed to update DB status after mkdir error: %v", ctx.ID, updateErr)
		}
		fmt.Fprintf(ctx.Writer.Newline(), "Worker %d: Error creating directory for %s: %v\n", ctx.ID, filepath.Base(directoryPath), err)
		return err
	}
	return nil
}

// performFileDownload handles the main file download logic
func (ctx *WorkerContext) performFileDownload(pd potentialDownload, dbKey string, initialStatus string, targetPath string) (string, string, error) {
	if initialStatus == models.StatusDownloaded {
		log.Infof("[%s] Initial status is '%s', skipping main file download.", ctx.LogPrefix, initialStatus)
		return targetPath, initialStatus, nil
	}

	log.Infof("[%s] Status is '%s', proceeding with download check/process.", ctx.LogPrefix, initialStatus)
	startTime := time.Now()
	fmt.Fprintf(ctx.Writer.Newline(), "Worker %d: Checking/Downloading %s...\n", ctx.ID, filepath.Base(pd.TargetFilepath))

	actualFinalPath, downloadErr := ctx.FileDownloader.DownloadFile(pd.TargetFilepath, pd.File.DownloadUrl, pd.File.Hashes, pd.ModelVersionID)

	var finalStatus string
	if downloadErr != nil {
		finalStatus = models.StatusError
	} else {
		finalStatus = models.StatusDownloaded
		duration := time.Since(startTime)
		log.Infof("[%s] Successfully downloaded %s in %v", ctx.LogPrefix, actualFinalPath, duration)
		fmt.Fprintf(ctx.Writer.Newline(), "[%s] Success downloading %s\n", ctx.LogPrefix, filepath.Base(actualFinalPath))
	}

	return actualFinalPath, finalStatus, downloadErr
}

// updateDatabaseAfterDownload updates the database entry after download attempt
func (ctx *WorkerContext) updateDatabaseAfterDownload(dbKey string, pd potentialDownload, finalPath, finalStatus string, downloadErr error) error {
	updateErr := updateDbEntry(ctx.DB, dbKey, finalStatus, func(entry *models.DatabaseEntry) {
		if downloadErr != nil {
			entry.ErrorDetails = downloadErr.Error()
		} else {
			entry.ErrorDetails = ""
			entry.Filename = filepath.Base(finalPath)
			entry.File = pd.File
			entry.Version = pd.FullVersion

			actualFileDir := filepath.Dir(finalPath)
			folderRelToSavePath, err := filepath.Rel(ctx.Config.SavePath, actualFileDir)
			if err != nil {
				log.WithError(err).Warnf("Failed to calculate relative path for Folder for DB entry %s. Storing absolute: %s", dbKey, actualFileDir)
				entry.Folder = actualFileDir
			} else {
				entry.Folder = folderRelToSavePath
			}
			log.Debugf("Updating DB entry %s with Folder: %s", dbKey, entry.Folder)
		}
	})

	if updateErr != nil {
		log.Errorf("Worker %d: Failed DB update for key %s after download attempt: %v", ctx.ID, dbKey, updateErr)
		fmt.Fprintf(ctx.Writer.Newline(), "Worker %d: DB Error updating status for %s\n", ctx.ID, pd.FinalBaseFilename)
	} else {
		log.Debugf("[%s] DB status updated to %s for key %s", ctx.LogPrefix, finalStatus, dbKey)
	}

	return updateErr
}

// handleVersionImages downloads version-specific images if enabled
func (ctx *WorkerContext) handleVersionImages(pd potentialDownload, finalPath, finalStatus string) {
	if !ctx.Config.Download.SaveVersionImages || finalStatus != models.StatusDownloaded {
		if ctx.Config.Download.SaveVersionImages && finalStatus != models.StatusDownloaded {
			log.Debugf("[%s-VerImg] Skipping version image download for %s because main file status is '%s'", ctx.LogPrefix, pd.FinalBaseFilename, finalStatus)
		}
		return
	}

	imgLogPrefix := fmt.Sprintf("[%s-VerImg]", ctx.LogPrefix)
	if len(pd.OriginalImages) == 0 {
		log.Debugf("%s No version images found to download for %s", imgLogPrefix, pd.FinalBaseFilename)
		return
	}

	versionOutputDir := filepath.Dir(finalPath)
	imageSubDir := filepath.Join(versionOutputDir, "images")

	if err := os.MkdirAll(imageSubDir, 0750); err != nil {
		log.WithError(err).Errorf("%s Failed to create image directory: %s", imgLogPrefix, imageSubDir)
		return
	}

	log.Infof("%s Downloading %d version images for %s to %s", imgLogPrefix, len(pd.OriginalImages), filepath.Base(finalPath), imageSubDir)
	imgSuccess, imgFail := downloadImages(imgLogPrefix, pd.OriginalImages, imageSubDir, ctx.ImageDownloader, ctx.Config.Download.Concurrency)
	log.Infof("%s Finished downloading version images. Success: %d, Failures: %d", imgLogPrefix, imgSuccess, imgFail)
}

// processJob processes a single download job
func (ctx *WorkerContext) processJob(job downloadJob) {
	pd := job.PotentialDownload
	dbKey := job.DatabaseKey

	log.Infof("[%s] Processing job for %s (DB Key: %s)", ctx.LogPrefix, pd.TargetFilepath, dbKey)
	fmt.Fprintf(ctx.Writer, "[%s] Preparing %s... (%d/%d)\n", ctx.LogPrefix, filepath.Base(pd.TargetFilepath), ctx.ProcessedCount+1, ctx.TotalJobs)

	// Check initial database status
	initialDbStatus, finalPath, errGet := ctx.checkInitialDBStatus(dbKey, pd.TargetFilepath)

	// Ensure directory exists
	directoryPath := filepath.Dir(pd.TargetFilepath)
	if err := ctx.ensureDirectory(directoryPath, dbKey, errGet); err != nil {
		ctx.ProcessedCount++
		return
	}

	// Perform file download
	actualFinalPath, finalStatus, downloadErr := ctx.performFileDownload(pd, dbKey, initialDbStatus, finalPath)
	if downloadErr == nil {
		finalPath = actualFinalPath
	}

	// Update database if download was attempted
	if initialDbStatus != models.StatusDownloaded {
		ctx.updateDatabaseAfterDownload(dbKey, pd, finalPath, finalStatus, downloadErr)
	}

	// Handle post-download operations
	handleMetadataSaving(ctx.LogPrefix, pd, finalPath, finalStatus, ctx.Writer, ctx.Config)
	ctx.handleVersionImages(pd, finalPath, finalStatus)

	if finalStatus == models.StatusDownloaded {
		handleModelImages(ctx.LogPrefix, pd, finalPath, ctx.ImageDownloader, ctx.Config)
	}

	ctx.ProcessedCount++
	fmt.Fprintf(ctx.Writer.Newline(), "Worker %d: Finished job processing.\n", ctx.ID)
}

// downloadWorker handles the actual download of files and updates the database.
func downloadWorker(id int, jobs <-chan downloadJob, db *database.DB, fileDownloader *downloader.Downloader, imageDownloader *downloader.Downloader, wg *sync.WaitGroup, writer *uilive.Writer, totalJobs int, cfg *models.Config) {
	defer wg.Done()

	ctx := &WorkerContext{
		ID:              id,
		LogPrefix:       fmt.Sprintf("Worker-%d", id),
		ProcessedCount:  0,
		TotalJobs:       totalJobs,
		DB:              db,
		FileDownloader:  fileDownloader,
		ImageDownloader: imageDownloader,
		Writer:          writer,
		Config:          cfg,
	}

	log.Debugf("[%s] Starting", ctx.LogPrefix)

	for job := range jobs {
		ctx.processJob(job)
	}

	log.Debugf("[%s] Exiting", ctx.LogPrefix)
}

// saveVersionMetadataFile saves the full model version metadata to a .json file.
// It derives the filename from the model file path.
func saveVersionMetadataFile(pd potentialDownload, modelFilePath string) error {
	// Derive metadata path from the model file path
	metadataPath := strings.TrimSuffix(modelFilePath, filepath.Ext(modelFilePath)) + ".json"
	log.Debugf("Attempting to save metadata to: %s", metadataPath)

	// Marshal the FULL version info from the potential download struct
	// Use the FullVersion field which should hold the necessary data
	jsonData, jsonErr := json.MarshalIndent(pd.FullVersion, "", "  ")
	if jsonErr != nil {
		log.WithError(jsonErr).Errorf("Failed to marshal full version metadata for %s (VersionID: %d)", pd.ModelName, pd.ModelVersionID)
		return fmt.Errorf("failed to marshal metadata: %w", jsonErr)
	}

	// Write the file
	if writeErr := os.WriteFile(metadataPath, jsonData, 0600); writeErr != nil {
		log.WithError(writeErr).Errorf("Failed to write version metadata file %s", metadataPath)
		return fmt.Errorf("failed to write metadata file %s: %w", metadataPath, writeErr)
	}

	log.Debugf("Successfully saved metadata file: %s", metadataPath)
	return nil
}
