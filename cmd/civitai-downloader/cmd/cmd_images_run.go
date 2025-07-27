package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gosuri/uilive"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"go-civitai-download/internal/api" // Added for api.ConvertImageAPIParamsToURLValues and api.CivitaiApiBaseUrl
	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/models"
)

// runImages orchestrates the fetching and downloading of images based on command-line flags.
func runImages(cmd *cobra.Command, args []string) {
	cfg := globalConfig

	// These direct assignments are no longer needed as CreateImageQueryParams reads from cfg.Images
	// modelID := cfg.Images.ModelID
	// modelVersionID := cfg.Images.ModelVersionID
	// username := cfg.Images.Username
	// period := cfg.Images.Period
	// sortOrder := cfg.Images.Sort
	// nsfwFilter := cfg.Images.Nsfw
	// postID := cfg.Images.PostID

	userTotalLimit := cfg.Images.Limit
	targetDir := cfg.Images.OutputDir
	saveMeta := cfg.Images.SaveMetadata
	numWorkers := cfg.Images.Concurrency
	maxPages := cfg.Images.MaxPages

	if printUrlFlag, _ := cmd.Flags().GetBool("debug-print-api-url"); printUrlFlag {
		log.Info("--- Debug API URL (--debug-print-api-url) for Images ---")
		tempApiParams := CreateImageQueryParams(&cfg)
		tempUrlValues := api.ConvertImageAPIParamsToURLValues(tempApiParams)
		requestURL := fmt.Sprintf("%s/images?%s", api.CivitaiApiBaseUrl, tempUrlValues.Encode())
		fmt.Println(requestURL)
		log.Info("Exiting after printing images API URL.")
		os.Exit(0)
	}

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

		effectiveAPIParamsForDisplay := CreateImageQueryParams(&cfg)
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
			log.Info("Operation cancelled by user.")
			os.Exit(0)
		}
		log.Info("Configuration confirmed.")
	} else {
		log.Info("Skipping configuration review due to --yes flag or config setting.")
	}

	log.Infof("Using image download concurrency level: %d", numWorkers)

	if targetDir == "" {
		if cfg.SavePath == "" {
			log.Fatal("Required configuration 'SavePath' is not set and --output-dir flag was not provided.")
		}
		targetDir = filepath.Join(cfg.SavePath, "images")
		log.Infof("Output directory not specified, using default: %s", targetDir)
	}

	if cfg.Images.ModelVersionID != 0 {
		log.Infof("Primary filter: Model Version ID %d", cfg.Images.ModelVersionID)
	} else if cfg.Images.ModelID != 0 {
		log.Infof("Primary filter: Model ID %d", cfg.Images.ModelID)
	} else if cfg.Images.PostID != 0 {
		log.Infof("Primary filter: Post ID %d", cfg.Images.PostID)
	} else if cfg.Images.Username != "" {
		log.Infof("Primary filter: Username '%s'", cfg.Images.Username)
	} else {
		log.Fatal("No primary filter (model-id, model-version-id, post-id, or username) is active for images command.")
	}

	if globalHttpTransport == nil {
		log.Warn("Global HTTP transport not initialized, using default.")
		globalHttpTransport = http.DefaultTransport
	}
	apiClient := &http.Client{
		Transport: globalHttpTransport,
		Timeout:   time.Duration(cfg.APIClientTimeoutSec) * time.Second,
	}

	log.Info("Fetching image list from Civitai API...")
	var allImages []models.ImageApiItem
	initialApiParams := CreateImageQueryParams(&cfg)

	pageCount := 0
	var nextCursor string
	var loopErr error

	log.Info("--- Starting Image Fetching ---")
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

		urlValues := api.ConvertImageAPIParamsToURLValues(currentApiParams)
		requestURL := fmt.Sprintf("%s/images?%s", api.CivitaiApiBaseUrl, urlValues.Encode())

		log.Debugf("Requesting Image URL (Page %d, Cursor: '%s'): %s", pageCount, currentApiParams.Cursor, requestURL)

		req, err := http.NewRequest("GET", requestURL, nil)
		if err != nil {
			loopErr = fmt.Errorf("failed to create request for page %d: %w", pageCount, err)
			break
		}
		if cfg.APIKey != "" {
			req.Header.Add("Authorization", "Bearer "+cfg.APIKey)
		}

		resp, err := apiClient.Do(req)
		if err != nil {
			if urlErr, ok := err.(*url.Error); ok && urlErr.Timeout() {
				log.WithError(err).Warnf("Timeout fetching image metadata page %d. Retrying after delay...", pageCount)
				time.Sleep(5 * time.Second)
				continue
			}
			loopErr = fmt.Errorf("failed to fetch image metadata page %d: %w", pageCount, err)
			break
		}

		bodyBytes, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.WithError(closeErr).Warn("Error closing image API response body")
		}

		if readErr != nil {
			loopErr = fmt.Errorf("failed to read response body (Page %d): %w", pageCount, readErr)
			break
		}

		if resp.StatusCode != http.StatusOK {
			errMsg := fmt.Sprintf("Image API request failed (Page %d) with status %s", pageCount, resp.Status)
			if len(bodyBytes) > 0 {
				maxLen := 200
				bodyStr := string(bodyBytes)
				if len(bodyStr) > maxLen {
					bodyStr = bodyStr[:maxLen] + "..."
				}
				errMsg += fmt.Sprintf(". Response: %s", bodyStr)
			}
			log.Error(errMsg)
			if resp.StatusCode == http.StatusTooManyRequests {
				log.Warn("Rate limited. Applying longer delay...")
				delay := time.Duration(cfg.APIDelayMs)*time.Millisecond*5 + 5*time.Second
				time.Sleep(delay)
				continue
			}
			loopErr = errors.New(errMsg)
			break
		}

		var response models.ImageApiResponse
		if errUnmarshal := json.Unmarshal(bodyBytes, &response); errUnmarshal != nil {
			loopErr = fmt.Errorf("failed to decode image API response (Page %d): %w", pageCount, errUnmarshal)
			log.WithError(errUnmarshal).Errorf("Response body sample: %s", string(bodyBytes[:minSafe(len(bodyBytes), 200)]))
			break
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

		nextCursor = response.Metadata.NextCursor
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

	if loopErr != nil {
		log.WithError(loopErr).Error("Image fetching loop stopped due to an error.")
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

	downloadHttpClient := &http.Client{
		Transport: globalHttpTransport,
		Timeout:   0,
	}
	dl := downloader.NewDownloader(downloadHttpClient, cfg.APIKey)

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
		go imageDownloadWorker(i, jobs, dl, &wg, writer, &successCount, &failureCount, saveMeta, finalBaseTargetDir)
	}

	log.Infof("Queueing %d image download jobs...", len(allImages))
	for _, imageItem := range allImages {
		if imageItem.URL == "" {
			log.Warnf("Image ID %d (URL: '%s') has no URL or it's empty, skipping queueing.", imageItem.ID, imageItem.URL)
			atomic.AddInt64(&failureCount, 1)
			continue
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
	// Rely on defer writer.Stop() - removed explicit stop here

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
		ModelID:        cfg.Images.ModelID,
		ModelVersionID: cfg.Images.ModelVersionID,
		PostID:         cfg.Images.PostID,
		Username:       cfg.Images.Username,
		Limit:          apiPageLimit,
		Sort:           cfg.Images.Sort,
		Period:         cfg.Images.Period,
		Nsfw:           cfg.Images.Nsfw,
	}

	log.Debugf("Created Image API Params: ModelID=%d, ModelVersionID=%d, PostID=%d, Username='%s', Limit=%d, Sort='%s', Period='%s', Nsfw='%s'",
		params.ModelID, params.ModelVersionID, params.PostID, params.Username, params.Limit, params.Sort, params.Period, params.Nsfw)
	return params
}

// minSafe is a helper for slicing, not general purpose min.
func minSafe(a, b int) int {
	if a < b {
		return a
	}
	return b
}
