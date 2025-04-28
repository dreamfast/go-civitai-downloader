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

	index "go-civitai-download/index"
	"go-civitai-download/internal/database"
	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/models"

	"github.com/blevesearch/bleve/v2"
	"github.com/gosuri/uilive"
	log "github.com/sirupsen/logrus"
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
	// entryJsonForDebug, _ := json.MarshalIndent(entry, "", "  ")
	// log.Debugf("DB Entry for key '%s' before marshalling: %s", key, string(entryJsonForDebug))

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

// handleMetadataSaving checks the config and calls saveMetadataFile if needed.
// Now accepts cfg *models.Config
func handleMetadataSaving(logPrefix string, pd potentialDownload, finalPath string, finalStatus string, writer *uilive.Writer, cfg *models.Config) {
	// Check config directly
	if cfg.Download.SaveMetadata {
		if finalStatus == models.StatusDownloaded {
			log.Debugf("[%s] Saving metadata for successfully downloaded file: %s", logPrefix, finalPath)
			if metaErr := saveMetadataFile(pd, finalPath); metaErr != nil {
				// Error already logged by saveMetadataFile
				if writer != nil {
					fmt.Fprintf(writer.Newline(), "[%s] Error saving metadata for %s: %v\n", logPrefix, filepath.Base(finalPath), metaErr)
				}
			}
		} else {
			log.Debugf("[%s] Skipping metadata save for %s due to download status: %s.", logPrefix, pd.TargetFilepath, finalStatus)
		}
	} else {
		log.Debugf("[%s] Skipping metadata save (disabled by config) for %s.", logPrefix, finalPath)
	}
}

// downloadWorker handles the actual download of a file and updates the database.
// It now also accepts an imageDownloader, bleveIndex, and the config.
func downloadWorker(id int, jobs <-chan downloadJob, db *database.DB, fileDownloader *downloader.Downloader, imageDownloader *downloader.Downloader, wg *sync.WaitGroup, writer *uilive.Writer, totalJobs int, bleveIndex bleve.Index, cfg *models.Config) {
	defer wg.Done()
	logPrefix := fmt.Sprintf("Worker-%d", id)
	log.Debugf("[%s] Starting", logPrefix)

	// Initialize progress tracking for this worker
	processedCount := 0

	for job := range jobs {
		pd := job.PotentialDownload
		dbKey := job.DatabaseKey // Use the key passed in the job
		log.Infof("[%s] Processing job for %s (DB Key: %s)", logPrefix, pd.TargetFilepath, dbKey)
		fmt.Fprintf(writer, "[%s] Preparing %s... (%d/%d)\n", logPrefix, filepath.Base(pd.TargetFilepath), processedCount+1, totalJobs)

		// Declare directoryPath and finalPath early, derive directoryPath from potential download path
		var directoryPath string = filepath.Dir(pd.TargetFilepath)
		var finalPath string = pd.TargetFilepath          // Default finalPath to target path initially
		var initialDbStatus string = models.StatusPending // Assume pending unless DB tells otherwise

		// --- Check Initial DB Status --- START ---
		rawValue, errGet := db.Get([]byte(dbKey))
		if errGet == nil {
			var entry models.DatabaseEntry
			if errUnmarshal := json.Unmarshal(rawValue, &entry); errUnmarshal == nil {
				initialDbStatus = entry.Status
				// Use filename from DB if it exists and status is Downloaded
				if initialDbStatus == models.StatusDownloaded && entry.Filename != "" {
					finalPath = filepath.Join(directoryPath, entry.Filename)
					log.Debugf("[%s] Initial DB status is Downloaded. Using existing filename from DB: %s", logPrefix, entry.Filename)
				}
			} else {
				log.WithError(errUnmarshal).Warnf("[%s] Failed to unmarshal existing DB entry for key %s during initial check. Assuming Pending.", logPrefix, dbKey)
			}
		} else if !errors.Is(errGet, database.ErrNotFound) {
			log.WithError(errGet).Warnf("[%s] Error checking initial DB status for key %s. Assuming Pending.", logPrefix, dbKey)
		}
		log.Debugf("[%s] Initial status for job %s determined as: %s", logPrefix, dbKey, initialDbStatus)
		// --- Check Initial DB Status --- END ---

		// Ensure directory exists (always do this)
		if err := os.MkdirAll(directoryPath, 0750); err != nil {
			log.WithError(err).Errorf("Worker %d: Failed to create directory %s", id, directoryPath)
			// Attempt to update DB status to Error
			updateErr := updateDbEntry(db, dbKey, models.StatusError, func(entry *models.DatabaseEntry) {
				// Only update error if it wasn't found initially or we couldn't parse it
				if errors.Is(errGet, database.ErrNotFound) || errGet != nil {
					entry.ErrorDetails = fmt.Sprintf("Failed to create directory: %v", err)
				}
			})
			if updateErr != nil {
				log.Errorf("Worker %d: Failed to update DB status after mkdir error: %v", id, updateErr)
			}
			fmt.Fprintf(writer.Newline(), "Worker %d: Error creating directory for %s: %v\n", id, filepath.Base(pd.TargetFilepath), err)
			processedCount++ // Increment counter even on error
			continue         // Skip to next job
		}

		// --- Main File Download Logic (Skip if already Downloaded) --- START ---
		finalStatus := initialDbStatus // Start with the initial status
		var downloadErr error = nil    // Initialize downloadErr to nil

		if initialDbStatus != models.StatusDownloaded {
			log.Infof("[%s] Status is '%s', proceeding with download check/process.", logPrefix, initialDbStatus)
			startTime := time.Now()
			fmt.Fprintf(writer.Newline(), "Worker %d: Checking/Downloading %s...\n", id, filepath.Base(pd.TargetFilepath))

			// Initiate download - it returns the actual final path used and error
			var actualFinalPath string
			actualFinalPath, downloadErr = fileDownloader.DownloadFile(pd.TargetFilepath, pd.File.DownloadUrl, pd.File.Hashes, pd.ModelVersionID)

			if downloadErr != nil {
				finalStatus = models.StatusError
				// Error logging and temp file removal happens within the updateDbEntry call below
			} else {
				finalStatus = models.StatusDownloaded
				finalPath = actualFinalPath // Update finalPath to the one returned by downloader
				duration := time.Since(startTime)
				log.Infof("[%s] Successfully downloaded %s in %v", logPrefix, finalPath, duration)
				fmt.Fprintf(writer.Newline(), "[%s] Success downloading %s\n", logPrefix, filepath.Base(finalPath))
			}

			// --- Update DB Based on Download Result --- START ---
			updateErr := updateDbEntry(db, dbKey, finalStatus, func(entry *models.DatabaseEntry) {
				if downloadErr != nil {
					entry.ErrorDetails = downloadErr.Error()
					// Error logging & cleanup moved inside updateDbEntry logic
				} else {
					// Success!
					entry.ErrorDetails = ""                   // Clear any previous error
					entry.Filename = filepath.Base(finalPath) // Update filename in DB
					entry.File = pd.File                      // Update File struct
					entry.Version = pd.FullVersion            // Update Version struct
				}
			})
			if updateErr != nil {
				log.Errorf("Worker %d: Failed DB update for key %s after download attempt: %v", id, dbKey, updateErr)
				fmt.Fprintf(writer.Newline(), "Worker %d: DB Error updating status for %s\n", id, pd.FinalBaseFilename)
				// If DB update fails after successful download, proceed with metadata/image checks anyway?
				// For now, we will proceed, but the DB state might be inconsistent.
			} else {
				// DB update was successful
				log.Debugf("[%s] DB status updated to %s for key %s", logPrefix, finalStatus, dbKey)
			}
			// --- Update DB Based on Download Result --- END ---

			// --- Index Item with Bleve (Only after successful download and DB update) --- START ---
			if finalStatus == models.StatusDownloaded && updateErr == nil && bleveIndex != nil {
				// ... (rest of Bleve indexing logic - ASSUME it uses finalPath, directoryPath correctly)
				// Calculate directory paths (directoryPath already defined)
				baseModelPath := filepath.Dir(directoryPath)
				modelPath := filepath.Dir(baseModelPath)
				// ... parse time, get metadata ...
				publishedAtTime := time.Time{}
				if pd.FullVersion.PublishedAt != "" {
					var errParse error
					publishedAtTime, errParse = time.Parse(time.RFC3339Nano, pd.FullVersion.PublishedAt)
					if errParse != nil {
						publishedAtTime, errParse = time.Parse(time.RFC3339, pd.FullVersion.PublishedAt)
						if errParse != nil {
							log.WithError(errParse).Warnf("Worker %d: Failed to parse PublishedAt time '%s' for indexing", id, pd.FullVersion.PublishedAt)
						}
					}
				}
				fileFormat := pd.File.Metadata.Format
				filePrecision := pd.File.Metadata.Fp
				fileSizeType := pd.File.Metadata.Size
				itemToIndex := index.Item{
					ID:                   fmt.Sprintf("v_%d_f_%d", pd.ModelVersionID, pd.File.ID), // Use DB Key
					Type:                 "model_file",
					Name:                 pd.File.Name, // Use the original file name
					Description:          pd.FullVersion.Description,
					FilePath:             finalPath,
					DirectoryPath:        directoryPath,
					BaseModelPath:        baseModelPath,
					ModelPath:            modelPath,
					ModelName:            pd.ModelName,
					VersionName:          pd.VersionName,
					BaseModel:            pd.BaseModel,
					CreatorName:          pd.Creator.Username,
					Tags:                 pd.FullVersion.TrainedWords,
					PublishedAt:          publishedAtTime,
					VersionDownloadCount: float64(pd.FullVersion.Stats.DownloadCount),
					VersionRating:        pd.FullVersion.Stats.Rating,
					VersionRatingCount:   float64(pd.FullVersion.Stats.RatingCount),
					FileSizeKB:           pd.File.SizeKB,
					FileFormat:           fileFormat,
					FilePrecision:        filePrecision,
					FileSizeType:         fileSizeType,
				}
				if indexErr := index.IndexItem(bleveIndex, itemToIndex); indexErr != nil {
					log.WithError(indexErr).Errorf("Worker %d: Failed to index downloaded item %s (ID: %s)", id, finalPath, itemToIndex.ID)
				} else {
					log.Debugf("Worker %d: Successfully indexed item %s (ID: %s)", id, finalPath, itemToIndex.ID)
				}
			}
			// --- Index Item with Bleve --- END ---

		} else {
			log.Infof("[%s] Initial status is '%s', skipping main file download.", logPrefix, initialDbStatus)
			// Ensure finalStatus reflects the initial state if download is skipped
			finalStatus = initialDbStatus
		}
		// --- Main File Download Logic --- END ---

		// --- Handle Metadata Saving (Always check if enabled, use finalStatus) --- START ---
		handleMetadataSaving(logPrefix, pd, finalPath, finalStatus, writer, cfg)
		// --- Handle Metadata Saving --- END ---

		// --- Download Version Images (Always check if enabled, use finalStatus) --- START ---
		saveVersionImages := cfg.Download.SaveVersionImages
		if saveVersionImages {
			// Only proceed if the main file status is 'Downloaded' (either initially or after download)
			if finalStatus == models.StatusDownloaded {
				imgLogPrefix := fmt.Sprintf("[%s-VerImg]", logPrefix)
				if len(pd.OriginalImages) > 0 {
					// --- Determine Correct Image Directory --- NEW
					// Use the directory of the final downloaded file path
					versionOutputDir := filepath.Dir(finalPath)
					imageSubDir := filepath.Join(versionOutputDir, "images")

					// Ensure the images subdirectory exists
					if err := os.MkdirAll(imageSubDir, 0700); err != nil {
						log.WithError(err).Errorf("%s Failed to create image directory: %s", imgLogPrefix, imageSubDir)
					} else {
						log.Infof("%s Downloading %d version images for %s to %s", imgLogPrefix, len(pd.OriginalImages), filepath.Base(finalPath), imageSubDir)
						imgSuccess, imgFail := downloadImages(
							imgLogPrefix,
							pd.OriginalImages,
							imageSubDir, // Pass the specific image subdirectory again
							imageDownloader,
							cfg.Download.Concurrency, // Reuse download concurrency for images for now
						)
						log.Infof("%s Finished downloading version images. Success: %d, Failures: %d", imgLogPrefix, imgSuccess, imgFail)
					}
					// --- End Determine Correct Image Directory ---

				} else {
					log.Debugf("%s No version images found to download for %s", imgLogPrefix, pd.FinalBaseFilename)
				}
			} else {
				// Log why image download is skipped if main file status is not Downloaded
				log.Debugf("[%s-VerImg] Skipping version image download for %s because main file status is '%s'", logPrefix, pd.FinalBaseFilename, finalStatus)
			}
		}
		// --- Download Version Images --- END ---

		processedCount++ // Increment counter after processing job
		fmt.Fprintf(writer.Newline(), "Worker %d: Finished job processing.\n", id)

	}
	log.Debugf("[%s] Exiting", logPrefix)
}

// saveMetadataFile saves the full model version metadata to a .json file.
// It derives the filename from the model file path.
func saveMetadataFile(pd potentialDownload, modelFilePath string) error {
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
