package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"go-civitai-download/internal/api"
	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/models"
	"go-civitai-download/internal/paths"

	"github.com/gosuri/uilive"
	log "github.com/sirupsen/logrus"
)

// imageJob defines the data needed to process a single image download.
type imageJob struct {
	ImageID   int
	SourceURL string
	Metadata  models.ImageApiItem // The full metadata from the images API
}

// imageDownloadWorker is responsible for fetching full model details for an image,
// generating a correct path, and downloading the image and its metadata.
func imageDownloadWorker(
	id int,
	jobs <-chan imageJob,
	dl *downloader.Downloader,
	wg *sync.WaitGroup,
	writer *uilive.Writer,
	successCount *int64,
	failureCount *int64,
	saveMeta bool,
	baseDir string, // The root directory for all image downloads (e.g., "downloads/images")
	apiClient *api.Client, // API client to fetch model details
	cfg *models.Config,
) {
	defer wg.Done()
	logPrefix := fmt.Sprintf("ImgWorker-%d", id)
	log.Debugf("[%s] Starting", logPrefix)

	for job := range jobs {
		log.Infof("[%s] Processing image ID %d", logPrefix, job.ImageID)
		fmt.Fprintf(writer, "[%s] Processing image %d...\n", logPrefix, job.ImageID)

		// Step 1: Fetch full model details to get creator and base model info
		if job.Metadata.ModelID == 0 {
			log.Warnf("[%s] Image ID %d has no associated ModelID. Cannot determine path. Skipping.", logPrefix, job.ImageID)
			atomic.AddInt64(failureCount, 1)
			continue
		}

		modelDetails, err := apiClient.GetModelDetails(job.Metadata.ModelID)
		if err != nil {
			log.WithError(err).Errorf("[%s] Failed to get model details for model ID %d (for image %d). Skipping.", logPrefix, job.Metadata.ModelID, job.ImageID)
			atomic.AddInt64(failureCount, 1)
			continue
		}

		// Find the specific version to get the baseModel
		var modelVersion *models.ModelVersion
		for _, v := range modelDetails.ModelVersions {
			if v.ID == job.Metadata.ModelVersionID {
				versionCopy := v // Make a copy to avoid pointer issues
				modelVersion = &versionCopy
				break
			}
		}

		if modelVersion == nil && len(modelDetails.ModelVersions) > 0 {
			// Fallback to the latest version if the specific one isn't found
			log.Debugf("[%s] Could not find specific version %d for image %d. Falling back to latest version %d for baseModel info.", logPrefix, job.Metadata.ModelVersionID, job.ImageID, modelDetails.ModelVersions[0].ID)
			latestVersion := modelDetails.ModelVersions[0]
			modelVersion = &latestVersion
		}

		// Step 2: Generate the path using the fetched details
		data := buildPathData(&modelDetails, modelVersion, nil) // file is nil for image paths
		// We use the VersionPathPattern here to keep the structure consistent with model downloads
		relPath, err := paths.GeneratePath(cfg.Download.VersionPathPattern, data)
		if err != nil {
			log.WithError(err).Errorf("[%s] Failed to generate path for image %d. Skipping.", logPrefix, job.ImageID)
			atomic.AddInt64(failureCount, 1)
			continue
		}

		// The final directory for this image will be inside the model's folder
		finalImageDir := filepath.Join(baseDir, relPath)
		if err := os.MkdirAll(finalImageDir, 0750); err != nil {
			log.WithError(err).Errorf("[%s] Failed to create directory %s. Skipping image %d.", logPrefix, finalImageDir, job.ImageID)
			atomic.AddInt64(failureCount, 1)
			continue
		}

		// Step 3: Download the image
		imageFilename, err := dl.DownloadImage(finalImageDir, job.SourceURL)
		if err != nil {
			log.WithError(err).Errorf("[%s] Failed to download image from %s", logPrefix, job.SourceURL)
			atomic.AddInt64(failureCount, 1)
			fmt.Fprintf(writer.Newline(), "[%s] Error downloading image %d: %v\n", logPrefix, job.ImageID, err)
			continue
		}
		log.Infof("[%s] Successfully downloaded image %s", logPrefix, imageFilename)
		atomic.AddInt64(successCount, 1)

		// Step 4: Save metadata if requested
		if saveMeta {
			metaFilename := fmt.Sprintf("%s.json", imageFilename)
			metaPath := filepath.Join(finalImageDir, metaFilename)
			metaBytes, err := json.MarshalIndent(job.Metadata, "", "  ")
			if err != nil {
				log.WithError(err).Errorf("[%s] Failed to marshal metadata for image %d.", logPrefix, job.ImageID)
				// Don't count this as a full failure, as the image downloaded
			} else {
				if err := os.WriteFile(metaPath, metaBytes, 0600); err != nil {
					log.WithError(err).Errorf("[%s] Failed to write metadata file %s.", logPrefix, metaPath)
				} else {
					log.Debugf("[%s] Successfully saved metadata to %s", logPrefix, metaPath)
				}
			}
		}
		fmt.Fprintf(writer.Newline(), "[%s] Successfully processed image %d -> %s\n", logPrefix, job.ImageID, imageFilename)
	}
	log.Debugf("[%s] Exiting", logPrefix)
}
