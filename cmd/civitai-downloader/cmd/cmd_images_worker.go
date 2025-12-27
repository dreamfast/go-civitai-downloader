package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// String first
	SourceURL string
	// Struct
	Metadata models.ImageApiItem // The full metadata from the images API
	// Integer
	ImageID int
}

// imageMetadataWithURL wraps ImageApiItem with an additional page_url field for Civitai linking.
type imageMetadataWithURL struct {
	// Strings first (for field alignment)
	PageURL string `json:"page_url"`
	// Embedded struct
	models.ImageApiItem
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

		// Step 1: Generate path using simple data from images API (no expensive model API calls)
		imageData := map[string]string{
			"username":  job.Metadata.Username,
			"baseModel": job.Metadata.BaseModel,
			"imageId":   strconv.Itoa(job.ImageID),
		}

		// Fallback values for missing data
		if imageData["username"] == "" {
			imageData["username"] = "unknown_user"
		}
		if imageData["baseModel"] == "" {
			imageData["baseModel"] = "unknown_basemodel"
		}

		// Use the Images.PathPattern instead of the complex VersionPathPattern
		relPath, err := paths.GeneratePath(cfg.Images.PathPattern, imageData)
		if err != nil {
			log.WithError(err).Errorf("[%s] Failed to generate path for image %d using pattern '%s'. Skipping.", logPrefix, job.ImageID, cfg.Images.PathPattern)
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

			// Wrap metadata with page_url field for easy linking to Civitai
			metaWithURL := imageMetadataWithURL{
				PageURL:      fmt.Sprintf("https://civitai.com/images/%d", job.ImageID),
				ImageApiItem: job.Metadata,
			}

			metaBytes, err := json.MarshalIndent(metaWithURL, "", "  ")
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
