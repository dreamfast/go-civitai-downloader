package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/helpers"
	"go-civitai-download/internal/models"
	"go-civitai-download/internal/paths"

	log "github.com/sirupsen/logrus"
)

// --- Structs for Concurrent Image Downloads --- START ---
type imageDownloadJob struct {
	// Strings first
	SourceURL   string
	TargetPath  string
	LogFilename string // Keep base filename for logging
	// Integer
	ImageID     int    // Keep ID for logging
}

// --- Structs for Concurrent Image Downloads --- END ---

// --- Worker for Concurrent Image Downloads --- START ---
func imageDownloadWorkerInternal(id int, jobs <-chan imageDownloadJob, imageDownloader *downloader.Downloader, wg *sync.WaitGroup, successCounter *int64, failureCounter *int64, logPrefix string) {
	defer wg.Done()
	log.Debugf("[%s-Worker-%d] Starting internal image worker", logPrefix, id)
	for job := range jobs {
		log.Debugf("[%s-Worker-%d] Received job for image ID %d -> %s", logPrefix, id, job.ImageID, job.TargetPath)

		// --- Check if image exists already (handling potential extension correction) ---
		fileExists := false
		if _, statErr := os.Stat(job.TargetPath); statErr == nil {
			// Exact path match found
			fileExists = true
			log.Debugf("[%s-Worker-%d] Skipping image %s - exact path exists.", logPrefix, id, job.LogFilename)
		} else if os.IsNotExist(statErr) {
			// Exact path doesn't exist, check for base name match with different extension
			targetDir := filepath.Dir(job.TargetPath)
			baseNameTarget := strings.TrimSuffix(job.LogFilename, filepath.Ext(job.LogFilename))
			log.Debugf("[%s-Worker-%d] Exact path %s not found. Scanning dir %s for base name '%s'...", logPrefix, id, job.TargetPath, targetDir, baseNameTarget)

			entries, readErr := os.ReadDir(targetDir)
			if readErr != nil {
				// If we can't even read the dir, log warning and proceed to download attempt?
				log.WithError(readErr).Warnf("[%s-Worker-%d] Failed to read target directory %s to check for existing base name. Attempting download.", logPrefix, id, targetDir)
			} else {
				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					entryBaseName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
					if strings.EqualFold(entryBaseName, baseNameTarget) {
						fileExists = true
						log.Debugf("[%s-Worker-%d] Skipping image %s - existing file found with matching base name: %s", logPrefix, id, job.LogFilename, entry.Name())
						break // Found a match, no need to check further
					}
				}
			}
		} else {
			// Other stat error (permission denied, etc.)
			log.WithError(statErr).Warnf("[%s-Worker-%d] Failed to check status of target image file %s. Skipping download for this image.", logPrefix, id, job.TargetPath)
			atomic.AddInt64(failureCounter, 1)
			continue // Skip to next job
		}

		// If file exists (either exact match or base name match), skip to next job
		if fileExists {
			continue
		}
		// --- End Existence Check ---

		// Download the image
		log.Debugf("[%s-Worker-%d] Downloading image %s from %s", logPrefix, id, job.LogFilename, job.SourceURL)
		// Always pass empty hashes for images, as API doesn't provide standard ones
		_, dlErr := imageDownloader.DownloadFile(job.TargetPath, job.SourceURL, models.Hashes{}, 0)

		if dlErr != nil {
			log.WithError(dlErr).Errorf("[%s-Worker-%d] Failed to download image %s from %s", logPrefix, id, job.LogFilename, job.SourceURL)
			atomic.AddInt64(failureCounter, 1)
		} else {
			log.Debugf("[%s-Worker-%d] Downloaded image %s successfully.", logPrefix, id, job.LogFilename)
			atomic.AddInt64(successCounter, 1)
		}
	}
	log.Debugf("[%s-Worker-%d] Finishing internal image worker", logPrefix, id)
}

// --- Worker for Concurrent Image Downloads --- END ---

// saveModelInfoFile saves the complete model metadata to a JSON file.
// It's called by the worker and uses the ModelInfoPathPattern to determine the location.
func saveModelInfoFile(pd potentialDownload, cfg *models.Config) error {
	// Use the FullModel from the potentialDownload struct.
	// It should be populated, especially if it came from a full details fetch.
	model := pd.FullModel
	if model.ID == 0 {
		// This is a safeguard. In practice, FullModel should be populated.
		// If not, we can't reliably save model info.
		log.Warnf("Cannot save model info for version %d: FullModel data is missing.", pd.ModelVersionID)
		return fmt.Errorf("missing full model data for version %d", pd.ModelVersionID)
	}

	// --- Path Generation using ModelInfoPathPattern ---
	// We need to build the data map for the path generator.
	// We use the specific version data from the potential download to ensure
	// placeholders like {baseModel} are resolved correctly for this version.
	data := buildPathData(&model, &pd.FullVersion, &pd.File)

	relModelInfoDir, err := paths.GeneratePath(cfg.Download.ModelInfoPathPattern, data)
	if err != nil {
		log.WithError(err).Errorf("Failed to generate model info path for model %s (ID: %d) using pattern '%s'. Skipping info save.", model.Name, model.ID, cfg.Download.ModelInfoPathPattern)
		return err
	}
	infoDirPath := filepath.Join(cfg.SavePath, relModelInfoDir)
	// --- End Path Generation ---

	// Ensure the directory exists
	if err := os.MkdirAll(infoDirPath, 0750); err != nil {
		log.WithError(err).Errorf("Failed to create model info directory: %s", infoDirPath)
		return fmt.Errorf("failed to create directory %s: %w", infoDirPath, err)
	}

	// Construct the file path: {modelID}-{modelNameSlug}.json
	modelNameSlug := helpers.ConvertToSlug(model.Name)
	if modelNameSlug == "" {
		modelNameSlug = "unknown_model"
	}
	fileName := fmt.Sprintf("%d-%s.json", model.ID, modelNameSlug)
	filePath := filepath.Join(infoDirPath, fileName)

	// Marshal the full model info
	jsonData, jsonErr := json.MarshalIndent(model, "", "  ")
	if jsonErr != nil {
		log.WithError(jsonErr).Warnf("Failed to marshal full model info for model %d (%s)", model.ID, model.Name)
		return fmt.Errorf("failed to marshal model info for %d: %w", model.ID, jsonErr)
	}

	// Write the file (overwrite if exists)
	if writeErr := os.WriteFile(filePath, jsonData, 0600); writeErr != nil {
		log.WithError(writeErr).Warnf("Failed to write model info file %s", filePath)
		return fmt.Errorf("failed to write model info file %s: %w", filePath, writeErr)
	}

	log.Debugf("Saved full model info to %s", filePath)
	return nil
}

// downloadImages handles downloading a list of images concurrently to a specified directory.
func downloadImages(logPrefix string, images []models.ModelImage, targetImageDir string, imageDownloader *downloader.Downloader, numWorkers int) (finalSuccessCount, finalFailCount int) {
	if imageDownloader == nil {
		log.Warnf("[%s] Image downloader is nil, cannot download images.", logPrefix)
		return 0, len(images) // Count all as failed if downloader doesn't exist
	}
	if len(images) == 0 {
		log.Debugf("[%s] No images provided to download.", logPrefix)
		return 0, 0
	}
	if numWorkers <= 0 {
		log.Warnf("[%s] Invalid concurrency level %d for image download, defaulting to 1.", logPrefix, numWorkers)
		numWorkers = 1
	}

	log.Infof("[%s] Attempting concurrent download for %d images to %s (Concurrency: %d)", logPrefix, len(images), targetImageDir, numWorkers)
	log.Debugf("[%s] downloadImages received targetImageDir: %s", logPrefix, targetImageDir)

	// Ensure the specific image subdirectory exists
	if err := os.MkdirAll(targetImageDir, 0750); err != nil {
		log.WithError(err).Errorf("[%s] Failed to create image directory: %s", logPrefix, targetImageDir)
		return 0, len(images) // Cannot proceed, count all as failed
	}

	// --- Setup Concurrency ---
	jobs := make(chan imageDownloadJob, numWorkers*2) // Buffered channel
	var wg sync.WaitGroup
	var successCounter int64 = 0
	var failureCounter int64 = 0

	// --- Start Workers ---
	log.Debugf("[%s] Starting %d internal image download workers...", logPrefix, numWorkers)
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go imageDownloadWorkerInternal(w, jobs, imageDownloader, &wg, &successCounter, &failureCounter, logPrefix)
	}

	// --- Queue Jobs --- Loop through images and send jobs
	queuedCount := 0
	for imgIdx, image := range images {
		// Construct image filename: {imageID}.{ext} (Copied from previous sequential logic)
		imgUrlParsed, urlErr := url.Parse(image.URL)
		var imgFilename string

		if urlErr != nil || image.ID == 0 {
			fallbackName := fmt.Sprintf("image_%d.jpg", imgIdx) // Default fallback
			// Try to get filename from URL path as a better fallback
			if urlErr == nil { // Only try if URL parsing itself didn't fail
				pathSegments := strings.Split(imgUrlParsed.Path, "/")
				if len(pathSegments) > 0 {
					lastSegment := pathSegments[len(pathSegments)-1]
					// Basic check if it looks like a filename (has an extension, not empty)
					if strings.Contains(lastSegment, ".") && len(lastSegment) > 1 {
						fallbackName = lastSegment
						log.Debugf("[%s] Using filename '%s' extracted from URL path as fallback.", logPrefix, fallbackName)
					} else {
						log.Debugf("[%s] Last URL path segment '%s' does not look like a usable filename.", logPrefix, lastSegment)
					}
				}
			}
			// Log the warning, indicating which fallback name is being used
			log.WithError(urlErr).Debugf("[%s] Cannot determine filename/ID for image %d (URL: %s). Using fallback: %s", logPrefix, imgIdx, image.URL, fallbackName)
			imgFilename = fallbackName
		} else {
			// Normal logic using image.ID
			ext := filepath.Ext(imgUrlParsed.Path)
			if ext == "" || len(ext) > 5 { // Basic check for valid extension
				log.Warnf("[%s] Image URL %s has unusual/missing extension '%s', defaulting to .jpg", logPrefix, image.URL, ext)
				ext = ".jpg"
			}
			imgFilename = fmt.Sprintf("%d%s", image.ID, ext)
		}
		// Use imageSaveDir instead of baseDir
		imgTargetPath := filepath.Join(targetImageDir, imgFilename)
		log.Debugf("[%s] Calculated imgTargetPath: %s", logPrefix, imgTargetPath)

		// Create and send job
		job := imageDownloadJob{
			SourceURL:   image.URL,
			TargetPath:  imgTargetPath,
			ImageID:     image.ID,
			LogFilename: imgFilename, // Pass for consistent logging
		}
		log.Debugf("[%s] Queueing image job: ID %d -> %s", logPrefix, job.ImageID, job.TargetPath)
		jobs <- job
		queuedCount++
	}

	close(jobs) // Signal no more jobs
	log.Debugf("[%s] All %d image jobs queued. Waiting for workers...", logPrefix, queuedCount)

	// --- Wait for Completion ---
	wg.Wait()
	log.Infof("[%s] Image download complete. Success: %d, Failed: %d", logPrefix, atomic.LoadInt64(&successCounter), atomic.LoadInt64(&failureCounter))

	return int(atomic.LoadInt64(&successCounter)), int(atomic.LoadInt64(&failureCounter))
}
