package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gosuri/uilive"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"go-civitai-download/internal/api"
	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/models"
)

// runImages orchestrates the fetching and downloading of images based on command-line flags.
func runImages(cmd *cobra.Command, args []string) {
	cfg := globalConfig

	userTotalLimit := cfg.Images.Limit
	targetDir := cfg.Images.OutputDir
	saveMeta := cfg.Images.SaveMetadata
	numWorkers := cfg.Images.Concurrency
	maxPages := cfg.Images.MaxPages

	handleDebugAPIURL(cmd, &cfg)

	confirmConfiguration(&cfg)

	log.Infof("Using image download concurrency level: %d", numWorkers)

	targetDir = validateAndSetTargetDir(targetDir, &cfg)

	validatePrimaryFilters(&cfg)

	if globalHttpTransport == nil {
		log.Warn("Global HTTP transport not initialized, using default.")
		globalHttpTransport = http.DefaultTransport
	}
	httpClient := &http.Client{
		Transport: globalHttpTransport,
		Timeout:   0, // Set to 0 to avoid conflict with logging transport, like in downloadCmd
	}

	// This apiClient is used for all API calls in this command
	apiClient := api.NewClient(cfg.APIKey, httpClient, cfg)

	// Pre-fetch ModelID if only ModelVersionID is provided
	prefetchedModelID := resolveModelID(&cfg, apiClient)

	// Fetch image list from API
	allImages, loopErr := fetchImageList(&cfg, apiClient, userTotalLimit, maxPages)

	if loopErr != nil {
		log.WithError(loopErr).Error("Image fetching stopped due to an error.")
		if len(allImages) == 0 {
			log.Fatal("Exiting as no images were fetched before the error occurred.")
		}
		log.Warnf("Proceeding with %d images fetched before the error.", len(allImages))
	} else {
		log.Info("--- Finished Image Fetching ---")
	}

	if len(allImages) == 0 {
		log.Info("No images found matching the criteria after fetching from API.")
		return
	}
	log.Infof("Found %d total images to potentially download.", len(allImages))

	// Download images using worker pool
	downloadAllImages(&cfg, allImages, targetDir, saveMeta, numWorkers, prefetchedModelID, apiClient)
}

// resolveModelID pre-fetches the parent model ID if only ModelVersionID is provided.
func resolveModelID(cfg *models.Config, apiClient *api.Client) int {
	if cfg.Images.ModelID != 0 || cfg.Images.ModelVersionID == 0 {
		return cfg.Images.ModelID
	}
	log.Infof("Fetching model details for version %d to find parent model ID...", cfg.Images.ModelVersionID)
	versionDetails, err := apiClient.GetModelVersionDetails(cfg.Images.ModelVersionID)
	if err != nil {
		log.WithError(err).Fatalf("Failed to get model details for version %d. Cannot proceed.", cfg.Images.ModelVersionID)
	}
	log.Infof("Found parent ModelID: %d", versionDetails.ModelId)
	return versionDetails.ModelId
}

// fetchImageList handles cursor-advance and main API fetching to collect all images.
func fetchImageList(cfg *models.Config, apiClient *api.Client, userTotalLimit int, maxPages int) ([]models.ImageApiItem, error) {
	log.Info("Fetching image list from Civitai API...")
	initialApiParams := CreateImageQueryParams(cfg)

	pageCount := 0
	var nextCursor string

	// Cursor-advance for Page > 1
	nextCursor, loopErr := advanceCursorToPage(cfg, apiClient, initialApiParams, maxPages, &pageCount)
	if loopErr != nil {
		return nil, loopErr
	}

	// Main fetching loop
	var allImages []models.ImageApiItem
	for {
		pageCount++
		if maxPages > 0 && pageCount > maxPages {
			log.Infof("Reached max pages limit (%d). Stopping.", maxPages)
			break
		}

		currentApiParams := initialApiParams
		if nextCursor != "" {
			currentApiParams.Cursor = nextCursor
		}

		_, response, err := apiClient.GetImages(nextCursor, currentApiParams)
		if err != nil {
			return allImages, fmt.Errorf("failed to fetch image metadata page %d: %w", pageCount, err)
		}

		if len(response.Items) == 0 {
			log.Info("Received empty items list from API. Assuming end of results.")
			break
		}
		allImages = append(allImages, response.Items...)
		log.Infof("Received %d images from API page %d. Total collected so far: %d", len(response.Items), pageCount, len(allImages))

		if userTotalLimit > 0 && len(allImages) >= userTotalLimit {
			log.Infof("Reached total image limit (%d). Stopping image fetching.", userTotalLimit)
			allImages = allImages[:userTotalLimit]
			break
		}

		nextCursor = response.Metadata.NextCursor.String()
		if nextCursor == "" {
			log.Info("No next cursor found. Finished fetching all available images for the query.")
			break
		}
		log.Debugf("Next cursor for images API: %s", nextCursor)

		if cfg.APIDelayMs > 0 {
			log.Debugf("Applying API delay: %d ms", cfg.APIDelayMs)
			time.Sleep(time.Duration(cfg.APIDelayMs) * time.Millisecond)
		}
	}

	return allImages, nil
}

// advanceCursorToPage advances the cursor to the requested page when Page > 1.
func advanceCursorToPage(cfg *models.Config, apiClient *api.Client, initialApiParams models.ImageAPIParameters, maxPages int, pageCount *int) (string, error) {
	if cfg.Images.Page <= 1 {
		return "", nil
	}

	if cfg.Images.Page > 50 {
		log.Warnf("Page %d exceeds maximum allowed (50). Capping to 50.", cfg.Images.Page)
		cfg.Images.Page = 50
	}
	if cfg.Images.Page > 10 {
		log.Warnf("Page %d may trigger rate limiting due to %d cursor-advance API calls.", cfg.Images.Page, cfg.Images.Page-1)
	}

	skipCount := cfg.Images.Page - 1
	log.Infof("Skipping to page %d: advancing cursor through %d pages...", cfg.Images.Page, skipCount)

	skipCursor := ""
	for i := 0; i < skipCount; i++ {
		if maxPages > 0 && (i+1) > maxPages {
			log.Warnf("--page %d with --max-pages %d: skip consumes all allowed pages. No images will be fetched.", cfg.Images.Page, maxPages)
			return skipCursor, fmt.Errorf("page skip (%d) exceeds max-pages limit (%d)", cfg.Images.Page, maxPages)
		}

		skipParams := initialApiParams
		if skipCursor != "" {
			skipParams.Cursor = skipCursor
		}

		_, skipResp, skipErr := apiClient.GetImages(skipCursor, skipParams)
		if skipErr != nil {
			return skipCursor, fmt.Errorf("failed to advance cursor to page %d: %w", i+2, skipErr)
		}

		if len(skipResp.Items) == 0 {
			log.Infof("No more images available during cursor advance. Stopping at effective page %d.", i+1)
			break
		}

		skipCursor = skipResp.Metadata.NextCursor.String()
		if skipCursor == "" {
			log.Info("No next cursor during advance. End of results reached before target page.")
			break
		}

		*pageCount++ // Count skipped pages against maxPages
		log.Debugf("Cursor advanced: page %d/%d skipped. Next cursor: %s", i+1, skipCount, skipCursor)

		if cfg.APIDelayMs > 0 {
			time.Sleep(time.Duration(cfg.APIDelayMs) * time.Millisecond)
		}
	}

	log.Infof("Cursor advance complete. Starting download from page %d (cursor: %s).", cfg.Images.Page, skipCursor)
	return skipCursor, nil
}

// downloadAllImages sets up worker pool and downloads all collected images.
func downloadAllImages(cfg *models.Config, allImages []models.ImageApiItem, targetDir string, saveMeta bool, numWorkers int, prefetchedModelID int, apiClient *api.Client) {
	downloadHttpClient := &http.Client{
		Transport: globalHttpTransport,
		Timeout:   0,
	}
	dl := downloader.NewDownloader(downloadHttpClient, cfg.APIKey, cfg.SessionCookie)
	dl.SetDetectImageMimeType(cfg.Images.DetectImageMimeType)

	finalBaseTargetDir := targetDir
	log.Infof("Preparing to download images to base directory: %s", finalBaseTargetDir)
	if err := os.MkdirAll(finalBaseTargetDir, 0750); err != nil {
		log.Fatalf("Failed to create base target directory %s: %v", finalBaseTargetDir, err)
	}

	var wg sync.WaitGroup
	jobs := make(chan imageJob, len(allImages))

	var successCount, failureCount int64

	writer := uilive.New()
	writer.Start()
	defer writer.Stop()

	log.Infof("Starting %d image download workers...", numWorkers)
	for i := 1; i <= numWorkers; i++ {
		wg.Add(1)
		go imageDownloadWorker(i, jobs, dl, &wg, writer, &successCount, &failureCount, saveMeta, finalBaseTargetDir, apiClient, cfg)
	}

	log.Infof("Queueing %d image download jobs...", len(allImages))
	for _, imageItem := range allImages {
		if imageItem.URL == "" {
			log.Warnf("Image ID %d (URL: '%s') has no URL or it's empty, skipping queueing.", imageItem.ID, imageItem.URL)
			atomic.AddInt64(&failureCount, 1)
			continue
		}

		// Enrich the image item with the prefetched model ID if it's missing
		if imageItem.ModelID == 0 && prefetchedModelID != 0 {
			log.Debugf("Enriching image %d with prefetched ModelID %d", imageItem.ID, prefetchedModelID)
			imageItem.ModelID = prefetchedModelID
		}

		jobs <- imageJob{
			ImageID:   imageItem.ID,
			SourceURL: imageItem.URL,
			Metadata:  imageItem,
		}
	}
	close(jobs)
	log.Info("All image jobs queued.")

	log.Info("Waiting for image download workers to complete...")
	wg.Wait()
	log.Info("All image download workers finished.")

	fmt.Println("--------------------------")
	log.Infof("Image Download Summary:")
	log.Infof("  Successfully Downloaded: %d", atomic.LoadInt64(&successCount))
	log.Infof("  Failed Downloads:      %d", atomic.LoadInt64(&failureCount))
	if saveMeta {
		log.Infof("  (Metadata saving was attempted for successful downloads if enabled)")
	}
	fmt.Println("--------------------------")
}

// CreateImageQueryParams extracts image-related settings from the config
// and populates a models.ImageAPIParameters struct suitable for the Civitai images API.
func CreateImageQueryParams(cfg *models.Config) models.ImageAPIParameters {
	apiPageLimit := 100

	if cfg.Images.Limit > 0 && cfg.Images.Limit <= 200 {
		apiPageLimit = cfg.Images.Limit
	}

	params := models.ImageAPIParameters{
		ImageID:        cfg.Images.ImageID,
		ModelID:        cfg.Images.ModelID,
		ModelVersionID: cfg.Images.ModelVersionID,
		PostID:         cfg.Images.PostID,
		Username:       cfg.Images.Username,
		Limit:          apiPageLimit,
		Sort:           cfg.Images.Sort,
		Period:         cfg.Images.Period,
		Nsfw:           cfg.Images.Nsfw,
	}

	log.Debugf("Created Image API Params: ImageID=%d, ModelID=%d, ModelVersionID=%d, PostID=%d, Username='%s', Limit=%d, Sort='%s', Period='%s', Nsfw='%s'",
		params.ImageID, params.ModelID, params.ModelVersionID, params.PostID, params.Username, params.Limit, params.Sort, params.Period, params.Nsfw)
	return params
}

// handleDebugAPIURL handles the debug API URL flag
func handleDebugAPIURL(cmd *cobra.Command, cfg *models.Config) {
	if printUrlFlag, _ := cmd.Flags().GetBool("debug-print-api-url"); printUrlFlag {
		log.Info("--- Debug API URL (--debug-print-api-url) for Images ---")
		tempApiParams := CreateImageQueryParams(cfg)
		tempUrlValues := api.ConvertImageAPIParamsToURLValues(tempApiParams)
		requestURL := fmt.Sprintf("%s/images?%s", api.CivitaiApiBaseUrl, tempUrlValues.Encode())
		fmt.Println(requestURL)
		log.Info("Exiting after printing images API URL.")
		os.Exit(0)
	}
}

// confirmConfiguration displays configuration and asks for user confirmation
func confirmConfiguration(cfg *models.Config) {
	if !cfg.Download.SkipConfirmation {
		log.Info("--- Review Effective Configuration (Images Command) ---")
		globalSettings := map[string]interface{}{
			"SavePath":            cfg.SavePath,
			"OutputDir":           cfg.Images.OutputDir,
			"ApiKeySet":           cfg.APIKey != "",
			"ApiClientTimeoutSec": cfg.APIClientTimeoutSec,
			"ApiDelayMs":          cfg.APIDelayMs,
			"LogApiRequests":      cfg.LogApiRequests,
			"Concurrency":         cfg.Images.Concurrency,
		}
		globalJSON, _ := json.MarshalIndent(globalSettings, "  ", "  ")
		fmt.Println("  --- Global Settings (Relevant to Images) ---")
		fmt.Println("  " + strings.ReplaceAll(string(globalJSON), "\n", "\n  "))

		effectiveAPIParamsForDisplay := CreateImageQueryParams(cfg)
		imageAPIParamsDisplay := map[string]interface{}{
			"ModelID":        effectiveAPIParamsForDisplay.ModelID,
			"ModelVersionID": effectiveAPIParamsForDisplay.ModelVersionID,
			"PostID":         effectiveAPIParamsForDisplay.PostID,
			"Username":       effectiveAPIParamsForDisplay.Username,
			"APIPageLimit":   effectiveAPIParamsForDisplay.Limit,
			"UserTotalLimit": cfg.Images.Limit,
			"Period":         effectiveAPIParamsForDisplay.Period,
			"Sort":           effectiveAPIParamsForDisplay.Sort,
			"NSFW":           effectiveAPIParamsForDisplay.Nsfw,
			"MaxPages":       cfg.Images.MaxPages,
			"SaveMetadata":   cfg.Images.SaveMetadata,
		}
		apiParamsJSON, _ := json.MarshalIndent(imageAPIParamsDisplay, "  ", "  ")
		fmt.Println("\n  --- Image API Parameters (Effective) ---")
		fmt.Println("  " + strings.ReplaceAll(string(apiParamsJSON), "\n", "\n  "))

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nProceed with these settings? (y/N): ")
		input, _ := reader.ReadString('\n')
		input = strings.ToLower(strings.TrimSpace(input))
		if input != "y" {
			log.Info("Operation canceled by user.")
			os.Exit(0)
		}
		log.Info("Configuration confirmed.")
	} else {
		log.Info("Skipping configuration review due to --yes flag or config setting.")
	}
}

// validateAndSetTargetDir validates and sets the target directory
func validateAndSetTargetDir(targetDir string, cfg *models.Config) string {
	if targetDir == "" {
		if cfg.SavePath == "" {
			log.Fatal("Required configuration 'SavePath' is not set and --output-dir flag was not provided.")
		}
		targetDir = filepath.Join(cfg.SavePath, "images")
		log.Infof("Output directory not specified, using default: %s", targetDir)
	}
	return targetDir
}

// validatePrimaryFilters validates that at least one primary filter is active
func validatePrimaryFilters(cfg *models.Config) {
	if cfg.Images.ImageID != 0 {
		log.Infof("Primary filter: Image ID %d", cfg.Images.ImageID)
	} else if cfg.Images.ModelVersionID != 0 {
		log.Infof("Primary filter: Model Version ID %d", cfg.Images.ModelVersionID)
	} else if cfg.Images.ModelID != 0 {
		log.Infof("Primary filter: Model ID %d", cfg.Images.ModelID)
	} else if cfg.Images.PostID != 0 {
		log.Infof("Primary filter: Post ID %d", cfg.Images.PostID)
	} else if cfg.Images.Username != "" {
		log.Infof("Primary filter: Username '%s'", cfg.Images.Username)
	} else {
		log.Fatal("No primary filter (image-id, model-id, model-version-id, post-id, or username) is active for images command.")
	}
}
