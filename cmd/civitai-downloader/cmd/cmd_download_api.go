package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go-civitai-download/internal/api"
	"go-civitai-download/internal/database"
	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/helpers"
	"go-civitai-download/internal/models"

	"github.com/blevesearch/bleve/v2"
	log "github.com/sirupsen/logrus"
)

// --- Retry Logic Helper --- START ---

// doRequestWithRetry performs an HTTP request with exponential backoff retries.
// It now uses MaxRetries and InitialRetryDelayMs from the config.
func doRequestWithRetry(client *http.Client, req *http.Request, cfg *models.Config, logPrefix string) (*http.Response, []byte, error) {
	var resp *http.Response
	var err error
	var bodyBytes []byte
	_ = bodyBytes // Explicitly use bodyBytes to satisfy linter (used indirectly in logging/errors)

	maxRetries := cfg.MaxRetries                                                   // Get from config
	initialRetryDelay := time.Duration(cfg.InitialRetryDelayMs) * time.Millisecond // Get from config
	if initialRetryDelay <= 0 {                                                    // Ensure a minimum delay
		initialRetryDelay = 500 * time.Millisecond
	}
	if maxRetries < 0 {
		maxRetries = 0 // Ensure non-negative retries
	}
	maxAttempts := maxRetries + 1 // Total attempts include the initial one

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Calculate backoff: initial * 2^(attempt-1)
			backoff := initialRetryDelay * time.Duration(1<<(attempt-1))
			log.Infof("[%s] Retrying request for %s in %v (Attempt %d/%d)...", logPrefix, req.URL.String(), backoff, attempt+1, maxAttempts)
			time.Sleep(backoff)
		}

		// Clone the request for the attempt, especially important if the body is consumed.
		clonedReq := req.Clone(req.Context())
		if req.Body != nil && req.GetBody != nil {
			clonedReq.Body, err = req.GetBody()
			if err != nil {
				return nil, nil, fmt.Errorf("[%s] failed to get request body for retry clone (attempt %d): %w", logPrefix, attempt+1, err)
			}
		} else if req.Body != nil {
			log.Warnf("[%s] Cannot guarantee safe retry for request with non-nil body without GetBody defined (URL: %s)", logPrefix, req.URL.String())
		}

		log.Debugf("[%s] Attempt %d/%d: Sending request to %s", logPrefix, attempt+1, maxAttempts, clonedReq.URL.String())
		resp, err = client.Do(clonedReq)

		if err != nil {
			log.WithError(err).Warnf("[%s] Attempt %d/%d failed for %s: %v", logPrefix, attempt+1, maxAttempts, clonedReq.URL.String(), err)
			if resp != nil {
				if closeErr := resp.Body.Close(); closeErr != nil {
					log.WithError(closeErr).Warnf("[%s] Failed to close response body after network error for %s", logPrefix, clonedReq.URL.String())
				}
			}
			if attempt == maxRetries {
				return nil, nil, fmt.Errorf("[%s] network error failed after %d attempts for %s: %w", logPrefix, maxAttempts, clonedReq.URL.String(), err)
			}
			continue // Retry
		}

		bodyBytes, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.WithError(closeErr).Warnf("[%s] Failed to close response body after reading for %s", logPrefix, clonedReq.URL.String())
		}

		if readErr != nil {
			log.WithError(readErr).Warnf("[%s] Attempt %d/%d failed to read response body for %s: %v", logPrefix, attempt+1, maxAttempts, clonedReq.URL.String(), readErr)
			if attempt == maxRetries {
				return nil, nil, fmt.Errorf("[%s] failed to read body after %d attempts for %s: %w", logPrefix, maxAttempts, clonedReq.URL.String(), readErr)
			}
			continue // Retry
		}

		if resp.StatusCode == http.StatusOK {
			log.Debugf("[%s] Attempt %d/%d successful for %s", logPrefix, attempt+1, maxAttempts, clonedReq.URL.String())
			return resp, bodyBytes, nil // Success!
		}

		bodySample := string(bodyBytes)
		if len(bodySample) > 200 {
			bodySample = bodySample[:200] + "..."
		}
		log.Warnf("[%s] Attempt %d/%d for %s failed with status %s. Body: %s", logPrefix, attempt+1, maxAttempts, clonedReq.URL.String(), resp.Status, bodySample)

		isRetryableStatus := resp.StatusCode >= 500 ||
			resp.StatusCode == http.StatusRequestTimeout ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusGatewayTimeout

		if isRetryableStatus && attempt < maxRetries {
			log.Warnf("[%s] Status %s is retryable.", logPrefix, resp.Status)
		} else {
			errMsg := fmt.Sprintf("[%s] request failed with status %s after %d attempts", logPrefix, resp.Status, attempt+1)
			if !isRetryableStatus {
				errMsg = fmt.Sprintf("[%s] request failed with non-retryable status %s on attempt %d", logPrefix, resp.Status, attempt+1)
			}
			errMsg += fmt.Sprintf(". Body: %s", bodySample)
			return resp, bodyBytes, fmt.Errorf(errMsg)
		}
	} // End of retry loop

	return nil, nil, fmt.Errorf("[%s] retry loop completed without success or error return for %s", logPrefix, req.URL.String())
}

// --- Retry Logic Helper --- END ---

// passesFileFilters checks if a given file passes the configured file-level filters.
// Now uses the passed config struct.
func passesFileFilters(file models.File, modelType string, cfg *models.Config) bool {
	if file.Hashes.CRC32 == "" {
		log.Debugf("Skipping file %s: Missing CRC32 hash.", file.Name)
		return false
	}

	if cfg.Download.PrimaryOnly && !file.Primary {
		log.Debugf("Skipping non-primary file %s.", file.Name)
		return false
	}

	if file.Metadata.Format == "" {
		log.Debugf("Skipping file %s: Missing metadata format.", file.Name)
		return false
	}
	if strings.ToLower(file.Metadata.Format) != "safetensor" {
		log.Debugf("Skipping non-safetensor file %s (Format: %s).", file.Name, file.Metadata.Format)
		return false
	}

	if strings.EqualFold(modelType, "checkpoint") {
		sizeStr := fmt.Sprintf("%v", file.Metadata.Size)
		fpStr := fmt.Sprintf("%v", file.Metadata.Fp)

		if cfg.Download.Pruned && !strings.EqualFold(sizeStr, "pruned") {
			log.Debugf("Skipping non-pruned file %s (Size: %s) in checkpoint model.", file.Name, sizeStr)
			return false
		}
		if cfg.Download.Fp16 && !strings.EqualFold(fpStr, "fp16") {
			log.Debugf("Skipping non-fp16 file %s (FP: %s) in checkpoint model.", file.Name, fpStr)
			return false
		}
	}

	ignoredFilenameStrings := cfg.Download.IgnoreFileNameStrings // Use config
	if len(ignoredFilenameStrings) > 0 {
		for _, ignoreFileName := range ignoredFilenameStrings {
			if ignoreFileName != "" && strings.Contains(strings.ToLower(file.Name), strings.ToLower(ignoreFileName)) {
				log.Debugf("      - Skipping file %s: Filename contains ignored string '%s'.", file.Name, ignoreFileName)
				return false
			}
		}
	}
	return true
}

// handleSingleVersionDownload Fetches details for a specific model version ID and processes it for download.
// Now uses the passed config struct and api.Client.
func handleSingleVersionDownload(versionID int, db *database.DB, apiClient *api.Client, cfg *models.Config) ([]potentialDownload, uint64, error) {
	log.Debugf("Fetching details for model version ID: %d", versionID)
	apiURL := fmt.Sprintf("https://civitai.com/api/v1/model-versions/%d", versionID)
	logPrefix := fmt.Sprintf("Version %d", versionID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request for version %d: %w", versionID, err)
	}
	if cfg.APIKey != "" {
		req.Header.Add("Authorization", "Bearer "+cfg.APIKey)
	}

	// Use Retry Helper with config for retry settings
	_, bodyBytes, err := doRequestWithRetry(apiClient.HttpClient, req, cfg, logPrefix)

	if err != nil {
		finalErrMsg := fmt.Sprintf("failed to fetch version %d: %v", versionID, err)
		if !strings.Contains(err.Error(), "Body:") && len(bodyBytes) > 0 {
			bodySample := string(bodyBytes)
			if len(bodySample) > 200 {
				bodySample = bodySample[:200] + "..."
			}
			finalErrMsg += fmt.Sprintf(". Last Body: %s", bodySample)
		}
		return nil, 0, fmt.Errorf(finalErrMsg)
	}

	var versionResponse models.ModelVersion
	if err := json.Unmarshal(bodyBytes, &versionResponse); err != nil {
		log.WithError(err).Errorf("Response body sample: %s", string(bodyBytes[:min(len(bodyBytes), 200)]))
		return nil, 0, fmt.Errorf("failed to decode API response for version %d: %w", versionID, err)
	}

	log.Infof("Successfully fetched details for version %d (%s) of model %s (%s)",
		versionResponse.ID, versionResponse.Name, versionResponse.Model.Name, versionResponse.Model.Type)

	// --- Save Model Info if requested --- START ---
	if cfg.Download.SaveModelInfo {
		modelID := versionResponse.ModelId
		if modelID > 0 {
			log.Debugf("SaveModelInfo enabled. Fetching full details for parent model ID: %d", modelID)
			fullModelDetails, detailErr := apiClient.GetModelDetails(modelID)
			if detailErr != nil {
				log.WithError(detailErr).Warnf("Failed to fetch full model details for ID %d. Cannot save model info.", modelID)
			} else {
				// Calculate model base directory based on model type and name from version response
				modelTypeNameSlug := helpers.ConvertToSlug(versionResponse.Model.Type)
				modelNameSlug := helpers.ConvertToSlug(versionResponse.Model.Name)
				modelBaseDir := filepath.Join(cfg.SavePath, modelTypeNameSlug, modelNameSlug)

				if infoSaveErr := saveModelInfoFile(fullModelDetails, modelBaseDir); infoSaveErr != nil {
					log.WithError(infoSaveErr).Warnf("Failed to save model info JSON for model ID %d", modelID)
				} else {
					log.Debugf("Successfully saved model info JSON for model ID %d", modelID)
				}
			}
		} else {
			log.Warnf("Cannot fetch model info: Parent Model ID not found in version response.")
		}
	}
	// --- Save Model Info if requested --- END ---

	var potentialDownloadsPage []potentialDownload
	versionWithoutFilesImages := versionResponse
	versionWithoutFilesImages.Files = nil
	versionWithoutFilesImages.Images = nil

	for _, file := range versionResponse.Files {
		// Pass config to filter function
		if !passesFileFilters(file, versionResponse.Model.Type, cfg) {
			continue
		}

		// Construct the directory path using Model Type, Model Name Slug, and Version ID
		modelTypeName := helpers.ConvertToSlug(versionResponse.Model.Type)
		modelNameSlug := helpers.ConvertToSlug(versionResponse.Model.Name)
		versionIDStr := strconv.Itoa(versionResponse.ID)
		targetDir := filepath.Join(cfg.SavePath, modelTypeName, modelNameSlug, versionIDStr)
		// Construct filename with version ID prefix (consistent with DB logic)
		finalBaseFilename := fmt.Sprintf("%d_%s", versionResponse.ID, helpers.ConvertToSlug(file.Name))
		targetPath := filepath.Join(targetDir, finalBaseFilename)

		// Use placeholder for creator since full Model info isn't available here
		placeholderCreator := models.Creator{Username: "unknown_creator"}

		pd := potentialDownload{
			ModelName:         versionResponse.Model.Name,
			ModelType:         versionResponse.Model.Type,
			Creator:           placeholderCreator,        // Use placeholder
			FullVersion:       versionWithoutFilesImages, // Store version details (no files/images)
			ModelVersionID:    versionResponse.ID,
			File:              file,
			TargetFilepath:    targetPath,
			FinalBaseFilename: finalBaseFilename,
			OriginalImages:    versionResponse.Images,    // Populate OriginalImages
			BaseModel:         versionResponse.BaseModel, // Store the base model
			Slug:              modelNameSlug,             // Store the model name slug
			VersionName:       versionResponse.Name,      // Store the version name
		}
		potentialDownloadsPage = append(potentialDownloadsPage, pd)
	}

	// Use the refactored filterAndPrepareDownloads function
	processedDownloads, totalSize := filterAndPrepareDownloads(potentialDownloadsPage, db, cfg)
	return processedDownloads, totalSize, nil
}

// handleSingleModelDownload Fetches all versions for a specific model ID and processes them.
// Now uses the passed config struct and api.Client.
func handleSingleModelDownload(modelID int, db *database.DB, apiClient *api.Client, imageDownloader *downloader.Downloader, cfg *models.Config) ([]potentialDownload, uint64, error) {
	log.Debugf("Fetching details for model ID: %d", modelID)
	apiURL := fmt.Sprintf("https://civitai.com/api/v1/models/%d", modelID)
	logPrefix := fmt.Sprintf("Model %d", modelID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request for model %d: %w", modelID, err)
	}
	if cfg.APIKey != "" {
		req.Header.Add("Authorization", "Bearer "+cfg.APIKey)
	}

	// Use Retry Helper with config
	_, bodyBytes, err := doRequestWithRetry(apiClient.HttpClient, req, cfg, logPrefix)
	if err != nil {
		finalErrMsg := fmt.Sprintf("failed to fetch model %d: %v", modelID, err)
		if !strings.Contains(err.Error(), "Body:") && len(bodyBytes) > 0 {
			bodySample := string(bodyBytes)
			if len(bodySample) > 200 {
				bodySample = bodySample[:200] + "..."
			}
			finalErrMsg += fmt.Sprintf(". Last Body: %s", bodySample)
		}
		return nil, 0, fmt.Errorf(finalErrMsg)
	}

	var modelResponse models.Model
	if err := json.Unmarshal(bodyBytes, &modelResponse); err != nil {
		log.WithError(err).Errorf("Response body sample: %s", string(bodyBytes[:min(len(bodyBytes), 200)]))
		return nil, 0, fmt.Errorf("failed to decode API response for model %d: %w", modelID, err)
	}

	log.Infof("Processing Model: %s (ID: %d) by %s", modelResponse.Name, modelResponse.ID, modelResponse.Creator.Username)

	// --- Process Model Info & Images if requested ---
	modelInfoDir := filepath.Join(cfg.SavePath, "model_info", helpers.ConvertToSlug(modelResponse.Type), helpers.ConvertToSlug(modelResponse.Creator.Username), helpers.ConvertToSlug(modelResponse.Name))

	if cfg.Download.SaveModelInfo {
		err := saveModelInfoFile(modelResponse, modelInfoDir)
		if err != nil {
			log.Warnf("Failed to save full model info JSON for model %d: %v", modelResponse.ID, err)
		}
	}

	if cfg.Download.SaveModelImages && imageDownloader != nil {
		log.Warnf("Downloading model images for model %s (ID: %d). This may include images from multiple versions.", modelResponse.Name, modelResponse.ID)
		// Collect all images from all versions
		var allModelImages []models.ModelImage
		for _, version := range modelResponse.ModelVersions {
			if len(version.Images) > 0 {
				allModelImages = append(allModelImages, version.Images...)
			}
		}

		if len(allModelImages) > 0 {
			imageCount, imageErrors := downloadImages(
				fmt.Sprintf("Model-%d-Images", modelResponse.ID),
				allModelImages, // Pass the collected slice of all images
				modelInfoDir,
				imageDownloader,
				cfg.Download.Concurrency,
			)
			log.Infof("Model image download attempt finished for %s. Success: %d, Errors: %d", modelResponse.Name, imageCount, imageErrors)
		} else {
			log.Infof("No images found to download for model %s (ID: %d)", modelResponse.Name, modelResponse.ID)
		}
	}
	// --- End Model Info/Images Processing ---

	var potentialDownloadsPage []potentialDownload

	// Iterate through versions
	for _, version := range modelResponse.ModelVersions {
		log.Debugf("    Processing Version: %s (ID: %d)", version.Name, version.ID)
		versionWithoutFilesImages := version // Create a copy for metadata
		versionWithoutFilesImages.Files = nil
		versionWithoutFilesImages.Images = nil

		for _, file := range version.Files {
			// Pass config to filter function
			if !passesFileFilters(file, modelResponse.Type, cfg) {
				continue
			}

			// Construct the directory path using Model Type, Model Name Slug, and Version ID
			modelTypeName := helpers.ConvertToSlug(version.Model.Type)
			modelNameSlug := helpers.ConvertToSlug(version.Model.Name)
			versionIDStr := strconv.Itoa(version.ID)
			targetDir := filepath.Join(cfg.SavePath, modelTypeName, modelNameSlug, versionIDStr)
			// Construct filename with version ID prefix (consistent with DB logic)
			finalBaseFilename := fmt.Sprintf("%d_%s", version.ID, helpers.ConvertToSlug(file.Name))
			targetPath := filepath.Join(targetDir, finalBaseFilename)

			pd := potentialDownload{
				ModelName:         version.Model.Name,
				ModelType:         version.Model.Type,
				Creator:           modelResponse.Creator,
				FullVersion:       versionWithoutFilesImages,
				ModelVersionID:    version.ID,
				File:              file,
				TargetFilepath:    targetPath,
				FinalBaseFilename: finalBaseFilename,
				OriginalImages:    version.Images,
				BaseModel:         version.BaseModel,
				Slug:              modelNameSlug,
				VersionName:       version.Name,
			}
			potentialDownloadsPage = append(potentialDownloadsPage, pd)
		}
		// Only process the latest version if AllVersions is false
		if !cfg.Download.AllVersions {
			log.Debugf("Processing only latest version for model %d, breaking version loop.", modelResponse.ID)
			break
		}
	}

	// Use the refactored filterAndPrepareDownloads function
	processedDownloads, totalSize := filterAndPrepareDownloads(potentialDownloadsPage, db, cfg)
	return processedDownloads, totalSize, nil
}

// filterAndPrepareDownloads checks potential downloads against the database and calculates total size.
// Now uses the passed config struct.
func filterAndPrepareDownloads(potentialDownloadsPage []potentialDownload, db *database.DB, cfg *models.Config) ([]potentialDownload, uint64) {
	var downloadsToQueueFiltered []potentialDownload
	var totalSizeFiltered uint64

	for _, pd := range potentialDownloadsPage {
		// --- Check Database ---
		dbKey := fmt.Sprintf("v_%d", pd.ModelVersionID) // Key using version ID
		shouldQueue := true                             // Assume we should queue unless DB says otherwise
		existingEntryBytes, errGet := db.Get([]byte(dbKey))

		if errGet == nil {
			// Entry exists, check its status
			var existingEntry models.DatabaseEntry
			if errUnmarshal := json.Unmarshal(existingEntryBytes, &existingEntry); errUnmarshal == nil {
				// Check if the specific file within the version entry matches
				if existingEntry.File.ID == pd.File.ID && existingEntry.File.Hashes.CRC32 == pd.File.Hashes.CRC32 {
					if existingEntry.Status == models.StatusDownloaded {
						// File exists and is downloaded. Only queue if we need to check images.
						if cfg.Download.SaveVersionImages {
							log.Debugf("      - Queuing downloaded file %s (Version %d, File %d) for image check.", pd.File.Name, pd.ModelVersionID, pd.File.ID)
							shouldQueue = true // Queue specifically for image check
						} else {
							log.Debugf("      - Skipping file %s (Version %d, File %d): Already marked as downloaded in DB (images not requested).", pd.File.Name, pd.ModelVersionID, pd.File.ID)
							shouldQueue = false // Already downloaded, no image check needed
						}
					} else {
						// Status is Pending or Error, allow re-queue (shouldQueue remains true)
						log.Debugf("      - Re-queuing file %s (Version %d, File %d): DB status is %s.", pd.File.Name, pd.ModelVersionID, pd.File.ID, existingEntry.Status)
					}
				} else {
					// File ID or hash mismatch, treat as new download for this version
					log.Debugf("      - Queuing file %s (Version %d, File %d): File ID/Hash mismatch with DB entry.", pd.File.Name, pd.ModelVersionID, pd.File.ID)
					// Need to create/overwrite the entry with Pending status
					shouldQueue = true // Ensure it's queued
				}
			} else {
				log.WithError(errUnmarshal).Warnf("      - Failed to unmarshal existing DB entry for key %s. Re-queuing.", dbKey)
				shouldQueue = true // Re-queue if we can't parse existing entry
			}
		} else if errors.Is(errGet, database.ErrNotFound) {
			// Key not found, this is a new download, create Pending entry
			log.Debugf("      - Key %s not found in DB. Creating Pending entry.", dbKey)
			newEntry := models.DatabaseEntry{
				ModelName:    pd.ModelName,
				ModelType:    pd.ModelType,
				Version:      pd.FullVersion,       // Store the full version struct
				File:         pd.File,              // Store the file struct
				Timestamp:    time.Now().Unix(),    // Use Unix timestamp for AddedAt
				Creator:      pd.Creator,           // Store the creator struct
				Filename:     pd.FinalBaseFilename, // Use the calculated filename
				Folder:       pd.Slug,              // Use the calculated folder slug
				Status:       models.StatusPending, // Set initial status
				ErrorDetails: "",                   // Clear error details
			}
			entryBytes, marshalErr := json.Marshal(newEntry)
			if marshalErr != nil {
				log.WithError(marshalErr).Errorf("      - Failed to marshal NEW DB entry for key %s. Skipping queue.", dbKey)
				shouldQueue = false // Don't queue if marshalling fails
			} else {
				if errPut := db.Put([]byte(dbKey), entryBytes); errPut != nil {
					log.WithError(errPut).Errorf("      - Failed to PUT new Pending DB entry for key %s. Skipping queue.", dbKey)
					shouldQueue = false // Don't queue if DB write fails
				} else {
					log.Debugf("      - Successfully created Pending DB entry for key %s.", dbKey)
					// shouldQueue remains true
				}
			}
		} else {
			// Other unexpected DB error during Get
			log.WithError(errGet).Errorf("      - Unexpected error checking database for key %s. Skipping queue.", dbKey)
			shouldQueue = false
		}

		if !shouldQueue {
			continue // Skip to next potential download if shouldQueue is false
		}

		// --- Check Ignored Base Models (Only if we should queue based on DB status) ---
		ignoredBaseModels := cfg.Download.IgnoreBaseModels // Use config
		baseModelMatch := false
		if len(ignoredBaseModels) > 0 && pd.FullVersion.BaseModel != "" {
			for _, ignoredBM := range ignoredBaseModels {
				if ignoredBM != "" && strings.Contains(strings.ToLower(pd.FullVersion.BaseModel), strings.ToLower(ignoredBM)) {
					baseModelMatch = true
					break
				}
			}
		}
		if baseModelMatch {
			log.Debugf("      - Skipping file %s (Version %d): Belongs to ignored base model '%s'.", pd.File.Name, pd.ModelVersionID, pd.FullVersion.BaseModel)
			continue
		}

		// Passed checks, add to the list for this page and update size
		downloadsToQueueFiltered = append(downloadsToQueueFiltered, pd)
		totalSizeFiltered += uint64(pd.File.SizeKB) * 1024
	}
	return downloadsToQueueFiltered, totalSizeFiltered
}

// fetchModelsPaginated retrieves models page by page from the API.
// ADDED userTotalLimit parameter.
func fetchModelsPaginated(apiClient *api.Client, db *database.DB, imageDownloader *downloader.Downloader, queryParams models.QueryParameters, cfg *models.Config, bleveIndex bleve.Index, userTotalLimit int) ([]potentialDownload, uint64, error) {
	var allPotentialDownloads []potentialDownload
	var totalDownloadSize uint64
	var nextCursor string
	pageCount := 0
	maxPages := cfg.Download.MaxPages // Use config value
	modelID := cfg.Download.ModelID
	allVersions := cfg.Download.AllVersions
	// Declare response and err here
	var response models.ApiResponse
	var err error

	// --- Handle Single Model/All Versions Case --- START ---
	if modelID != 0 {
		if allVersions {
			log.Infof("Fetching all versions for Model ID: %d", modelID)
			// Pass imageDownloader here
			return handleSingleModelDownload(modelID, db, apiClient, imageDownloader, cfg)
		} else {
			// If not --all-versions, we need to get the LATEST version
			log.Infof("Fetching latest version for Model ID: %d", modelID)
			// Fetch the model details first to find the latest version ID
			modelDetails, err := apiClient.GetModelDetails(modelID)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to get model details for ID %d to find latest version: %w", modelID, err)
			}
			if len(modelDetails.ModelVersions) == 0 {
				return nil, 0, fmt.Errorf("no versions found for model ID %d", modelID)
			}
			latestVersionID := modelDetails.ModelVersions[0].ID // API usually returns newest first
			log.Infof("Found latest version ID for model %d: %d", modelID, latestVersionID)
			return handleSingleVersionDownload(latestVersionID, db, apiClient, cfg)
		}
	}
	// --- Handle Single Model/All Versions Case --- END ---

	// If not fetching a single model, proceed with paginated search
	log.Infof("Starting paginated model fetch. Max pages: %d", maxPages)

	for {
		pageCount++
		if maxPages > 0 && pageCount > maxPages {
			log.Infof("Reached max pages limit (%d). Stopping model fetch.", maxPages)
			break
		}

		log.Infof("--- Fetching Model Page %d (Cursor: %s) ---", pageCount, nextCursor)

		// Make the API call using the client's method
		nextCursor, response, err = apiClient.GetModels(nextCursor, queryParams)
		if err != nil {
			// Handle specific error types from the client
			if errors.Is(err, api.ErrRateLimited) {
				log.Error("API Rate Limited. Consider increasing ApiDelayMs or reducing concurrency. Stopping fetch.")
			} else if errors.Is(err, api.ErrUnauthorized) {
				log.Error("API Unauthorized (401/403). Check your API key (ApiKey in config). Stopping fetch.")
			} else {
				log.WithError(err).Errorf("Failed to fetch model page %d", pageCount)
			}
			return allPotentialDownloads, totalDownloadSize, err // Return whatever was collected so far and the error
		}

		log.Debugf("Received %d models for page %d", len(response.Items), pageCount)

		if len(response.Items) == 0 {
			log.Info("Received 0 models, assuming end of results.")
			break
		}

		var potentialDownloadsPage []potentialDownload
		for _, model := range response.Items {
			if model.Creator.Username == "" {
				model.Creator.Username = "unknown_creator"
			}

			// Client-side base model filtering
			if len(cfg.Download.IgnoreBaseModels) > 0 && len(model.ModelVersions) > 0 {
				if helpers.StringSliceContains(cfg.Download.IgnoreBaseModels, model.ModelVersions[0].BaseModel) {
					log.Debugf("Skipping model %s (ID: %d) due to ignored base model: %s", model.Name, model.ID, model.ModelVersions[0].BaseModel)
					continue
				}
			}

			// --- Fetch Full Model Details (if needed for Info or Model Images) ---
			var fullModelDetails models.Model
			var detailErr error
			shouldFetchDetails := cfg.Download.SaveModelInfo || cfg.Download.SaveModelImages
			modelBaseDir := filepath.Join(cfg.SavePath, helpers.ConvertToSlug(model.Type), helpers.ConvertToSlug(model.Name))

			if shouldFetchDetails {
				log.Debugf("Fetching full details for model %d (%s) for info/images...", model.ID, model.Name)
				fullModelDetails, detailErr = apiClient.GetModelDetails(model.ID)
				if detailErr != nil {
					log.WithError(detailErr).Warnf("Failed to fetch full details for model %d. Skipping info/image processing for this model.", model.ID)
					// Reset fullModelDetails to ensure we use the basic model data later
					fullModelDetails = models.Model{}
				}
			}
			// --- End Fetch Full Model Details ---

			// --- Save Model Info (if requested and details fetched) ---
			if cfg.Download.SaveModelInfo && fullModelDetails.ID != 0 {
				if infoSaveErr := saveModelInfoFile(fullModelDetails, modelBaseDir); infoSaveErr != nil {
					log.Warnf("Failed to save model info JSON for model %d: %v", model.ID, infoSaveErr)
				}
			}
			// --- End Save Model Info ---

			// --- Download Model Images (if requested and details fetched) ---
			if cfg.Download.SaveModelImages && fullModelDetails.ID != 0 && imageDownloader != nil {
				log.Debugf("SaveModelImages enabled for model %d. Collecting images...", model.ID)
				var allModelImages []models.ModelImage
				for _, version := range fullModelDetails.ModelVersions { // Use versions from full details
					if len(version.Images) > 0 {
						allModelImages = append(allModelImages, version.Images...)
					}
				}

				if len(allModelImages) > 0 {
					// Save images to {modelBaseDir}/images/
					modelImagesDir := filepath.Join(modelBaseDir, "images")
					imgLogPrefix := fmt.Sprintf("[Model-%d-Images]", model.ID)
					log.Infof("%s Downloading %d images for model %s to %s", imgLogPrefix, len(allModelImages), model.Name, modelImagesDir)
					imgSuccess, imgFail := downloadImages(
						imgLogPrefix,
						allModelImages,
						modelImagesDir, // Target the model's images subdir
						imageDownloader,
						cfg.Download.Concurrency,
					)
					log.Infof("%s Finished model image download. Success: %d, Failures: %d", imgLogPrefix, imgSuccess, imgFail)
				} else {
					log.Infof("[Model-%d-Images] No images found in full model details.", model.ID)
				}
			}
			// --- End Download Model Images ---

			// Process each version of the model (using versions from the initial list response)
			for _, version := range model.ModelVersions {
				// Process each file within the version
				for _, file := range version.Files {
					if !passesFileFilters(file, model.Type, cfg) {
						continue
					}

					// Construct the directory path using Model Type, Model Name Slug, and Version ID
					modelTypeName := helpers.ConvertToSlug(model.Type)
					modelNameSlug := helpers.ConvertToSlug(model.Name)
					versionIDStr := strconv.Itoa(version.ID)
					targetDir := filepath.Join(cfg.SavePath, modelTypeName, modelNameSlug, versionIDStr)
					// Construct filename with version ID prefix (consistent with DB logic)
					finalBaseFilename := fmt.Sprintf("%d_%s", version.ID, helpers.ConvertToSlug(file.Name))
					targetPath := filepath.Join(targetDir, finalBaseFilename)

					// Resolve which model data to use
					modelDataForPd := model       // Default to list API model
					if fullModelDetails.ID != 0 { // Use full details if fetched
						modelDataForPd = fullModelDetails
					}

					// Create potential download entry using the struct definition
					pd := potentialDownload{
						ModelID:           modelDataForPd.ID,
						FullModel:         modelDataForPd,
						ModelName:         modelDataForPd.Name,
						ModelType:         modelDataForPd.Type,
						Creator:           modelDataForPd.Creator,
						FullVersion:       version,
						ModelVersionID:    version.ID,
						File:              file,
						TargetFilepath:    targetPath,
						FinalBaseFilename: finalBaseFilename,
						OriginalImages:    version.Images,
						BaseModel:         version.BaseModel,
						Slug:              helpers.ConvertToSlug(modelDataForPd.Name),
						VersionName:       version.Name,
					}
					potentialDownloadsPage = append(potentialDownloadsPage, pd)
				}

				// --- Check if only latest version should be processed --- NEW
				if !cfg.Download.AllVersions {
					log.Debugf("Processing only latest version for model %d (%s) as --all-versions is false. Breaking version loop.", model.ID, model.Name)
					break // Stop processing versions for this model after the first one
				}
				// --- End Check ---
			}
		}

		// Filter the page's potential downloads against the DB and prepare them
		processedDownloads, pageDownloadSize := filterAndPrepareDownloads(potentialDownloadsPage, db, cfg)
		allPotentialDownloads = append(allPotentialDownloads, processedDownloads...)
		totalDownloadSize += pageDownloadSize

		// --- Check if user limit reached after processing this page --- NEW
		if userTotalLimit > 0 && len(allPotentialDownloads) >= userTotalLimit {
			log.Infof("Reached user download limit (%d) during pagination after processing page %d. Stopping model fetch.", userTotalLimit, pageCount)
			// Optional: Truncate if needed, although the later check handles this too
			// if len(allPotentialDownloads) > userTotalLimit {
			//    allPotentialDownloads = allPotentialDownloads[:userTotalLimit]
			// }
			break // Exit the pagination loop
		}
		// --- End Check ---

		// Prepare for next iteration or break
		nextCursor = response.Metadata.NextCursor
		if nextCursor == "" {
			log.Info("No next cursor returned, stopping model fetch.")
			break
		}

		// Add delay between pages if configured
		if cfg.APIDelayMs > 0 {
			delay := time.Duration(cfg.APIDelayMs) * time.Millisecond
			log.Debugf("Waiting %v before fetching next page...", delay)
			time.Sleep(delay)
		}
	}

	log.Infof("Finished fetching models. Found %d potential downloads.", len(allPotentialDownloads))
	return allPotentialDownloads, totalDownloadSize, nil
}

// CreateDownloadQueryParams extracts download-related settings from the config
// and populates a models.QueryParameters struct suitable for the Civitai models API.
func CreateDownloadQueryParams(cfg *models.Config) models.QueryParameters {
	// Note: cfg.Download.Usernames is []string but API takes single string.
	// Taking the first one if provided, otherwise empty.
	// This might need refinement based on desired behavior.
	username := ""
	if len(cfg.Download.Usernames) > 0 {
		username = cfg.Download.Usernames[0]
		if len(cfg.Download.Usernames) > 1 {
			log.Warnf("Multiple usernames found in config (Usernames list), but API only supports one. Using the first: %s", username)
		}
	}

	params := models.QueryParameters{
		Limit:           cfg.Download.Limit,
		Sort:            cfg.Download.Sort,
		Period:          cfg.Download.Period,
		Query:           cfg.Download.Query,
		Username:        username, // Use the derived single username
		Tag:             cfg.Download.Tag,
		Types:           cfg.Download.ModelTypes,
		BaseModels:      cfg.Download.BaseModels,
		PrimaryFileOnly: cfg.Download.PrimaryOnly,
		Nsfw:            cfg.Download.Nsfw, // Directly assign the bool
		// Favorites: // Does not exist in QueryParameters
		// Hidden: // Does not exist in QueryParameters
		// Rating: // Does not exist in QueryParameters
		// Allow fields *do* exist in QueryParameters, but not currently in DownloadConfig
		// AllowNoCredit:
		// AllowDerivatives:
		// AllowCommercialUse:
		// AllowDifferentLicense:
	}

	log.Debugf("Created Download Query Params: %+v", params)
	return params
}

// fetchAndProcessModels orchestrates the entire model fetching process.
// It sets up the API client and calls fetchModelsPaginated.
func fetchAndProcessModels(apiClient *api.Client, db *database.DB, queryParams models.QueryParameters, cfg *models.Config, bleveIndex bleve.Index) ([]potentialDownload, error) {

	// Setup image downloader (needed for all-versions case inside fetchModelsPaginated)
	// Pass the correct arguments: http client and api key
	imageDownloader := downloader.NewDownloader(apiClient.HttpClient, cfg.APIKey)

	// Fetch models - Pass userTotalLimit (cfg.Download.Limit) now
	allPotentialDownloads, _, err := fetchModelsPaginated(apiClient, db, imageDownloader, queryParams, cfg, bleveIndex, cfg.Download.Limit)
	if err != nil {
		// Log the error, but potentially return the downloads found so far?
		// For now, just return the error.
		log.WithError(err).Error("Error occurred during model fetching.")
		return allPotentialDownloads, err
	}

	return allPotentialDownloads, nil
}

// Helper function to find min of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
