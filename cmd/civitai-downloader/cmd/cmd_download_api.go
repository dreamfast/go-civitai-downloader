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
	"go-civitai-download/internal/paths"

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
			return resp, bodyBytes, errors.New(errMsg)
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

// Helper to build data map for path generation
func buildPathData(model *models.Model, version *models.ModelVersion, file *models.File) map[string]string {
	data := map[string]string{}
	if model != nil {
		data["modelId"] = strconv.Itoa(model.ID)
		data["modelName"] = model.Name
		data["modelType"] = model.Type
		data["creatorName"] = model.Creator.Username // Assuming Creator is populated
	}
	if version != nil {
		data["versionId"] = strconv.Itoa(version.ID)
		data["versionName"] = version.Name
		data["baseModel"] = version.BaseModel
		if data["modelId"] == "" || data["modelId"] == "0" { // Populate from version if model data was minimal
			data["modelId"] = strconv.Itoa(version.ModelId)
		}
		// If top-level model data wasn't available (e.g., single version call), use version's model info
		if data["modelName"] == "" {
			data["modelName"] = version.Model.Name
		}
		if data["modelType"] == "" {
			data["modelType"] = version.Model.Type
		}
	}
	// Could add file-specific tags later if needed, like {fileId}, {fileName}

	// Ensure creator is never empty if possible (fallback needed?)
	if data["creatorName"] == "" {
		data["creatorName"] = "unknown_creator"
	}

	return data
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
		return nil, 0, errors.New(finalErrMsg)
	}

	var versionResponse models.ModelVersion
	if err := json.Unmarshal(bodyBytes, &versionResponse); err != nil {
		log.WithError(err).Errorf("Response body sample: %s", string(bodyBytes[:min(len(bodyBytes), 200)]))
		return nil, 0, fmt.Errorf("failed to decode API response for version %d: %w", versionID, err)
	}

	log.Infof("Successfully fetched details for version %d (%s) of model %s (%s)",
		versionResponse.ID, versionResponse.Name, versionResponse.Model.Name, versionResponse.Model.Type)

	potentialDownloadsPage := make([]potentialDownload, 0, len(versionResponse.Files))
	versionWithoutFilesImages := versionResponse
	// Clear files and images to reduce database storage size
	versionWithoutFilesImages.Files = []models.File{}
	versionWithoutFilesImages.Images = []models.ModelImage{}

	// Create a pseudo-Model struct for path data generation, as we only have version data here
	pseudoModel := models.Model{
		ID:   versionResponse.ModelId, // Use ModelId from version
		Name: versionResponse.Model.Name,
		Type: versionResponse.Model.Type,
		// Creator is missing here, buildPathData will use fallback
	}

	for _, file := range versionResponse.Files {
		if !passesFileFilters(file, pseudoModel.Type, cfg) {
			continue
		}

		// --- Path Generation using pattern --- START ---
		data := buildPathData(&pseudoModel, &versionResponse, &file)
		relPath, err := paths.GeneratePath(cfg.Download.VersionPathPattern, data)
		if err != nil {
			log.WithError(err).Errorf("Failed to generate path for version %d, file %s. Skipping.", versionResponse.ID, file.Name)
			continue
		}
		// --- Path Generation using pattern --- END ---

		finalBaseFilename := fmt.Sprintf("%d_%s", versionResponse.ID, helpers.ConvertToSlug(file.Name))
		targetPath := filepath.Join(cfg.SavePath, relPath, finalBaseFilename)

		pd := potentialDownload{
			ModelID:           pseudoModel.ID,
			ModelName:         pseudoModel.Name,
			ModelType:         pseudoModel.Type,
			Creator:           pseudoModel.Creator, // Will be fallback "unknown_creator"
			FullVersion:       versionWithoutFilesImages,
			ModelVersionID:    versionResponse.ID,
			File:              file,
			TargetFilepath:    targetPath, // Use calculated full path
			FinalBaseFilename: finalBaseFilename,
			OriginalImages:    versionResponse.Images,
			BaseModel:         versionResponse.BaseModel,
			Slug:              helpers.ConvertToSlug(pseudoModel.Name),
			VersionName:       versionResponse.Name,
		}
		potentialDownloadsPage = append(potentialDownloadsPage, pd)
	}

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
		return nil, 0, errors.New(finalErrMsg)
	}

	var modelResponse models.Model
	if err := json.Unmarshal(bodyBytes, &modelResponse); err != nil {
		log.WithError(err).Errorf("Response body sample: %s", string(bodyBytes[:min(len(bodyBytes), 200)]))
		return nil, 0, fmt.Errorf("failed to decode API response for model %d: %w", modelID, err)
	}

	log.Infof("Successfully fetched details for model %s (ID: %d, Type: %s, Creator: %s)",
		modelResponse.Name, modelResponse.ID, modelResponse.Type, modelResponse.Creator.Username)

	// --- Model Images Processing --- START ---
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
			// Save model images to potentially multiple directories if structure is basemodel_centric
			processedImageDirs := make(map[string]bool)
			for _, version := range modelResponse.ModelVersions {
				data := buildPathData(&modelResponse, &version, nil) // Build data map

				// If ModelInfoPathPattern (used for image base dir) uses {baseModel}, it's ambiguous.
				// Ensure it resolves to "unknown_baseModel".
				if strings.Contains(cfg.Download.ModelInfoPathPattern, "{baseModel}") {
					baseModelValue, bmExists := data["baseModel"]
					if !bmExists || strings.TrimSpace(baseModelValue) == "" {
						data["baseModel"] = "unknown_baseModel"
						log.Debugf("For single model image base path (model %d), {baseModel} is ambiguous or empty, setting to 'unknown_baseModel' for path generation.", modelResponse.ID)
					}
				}

				relModelInfoDir, err := paths.GeneratePath(cfg.Download.ModelInfoPathPattern, data)
				if err != nil {
					log.WithError(err).Errorf("Failed to generate model image path for model %s (ID: %d) using pattern '%s'. Skipping image download for this path.", modelResponse.Name, modelResponse.ID, cfg.Download.ModelInfoPathPattern)
					continue
				}
				modelImagesDirAbs := filepath.Join(cfg.SavePath, relModelInfoDir, "images")

				if !processedImageDirs[modelImagesDirAbs] {
					imgLogPrefix := fmt.Sprintf("[Model-%d-Images]", modelResponse.ID)
					log.Infof("%s Downloading %d images for model %s to %s (derived from version %d)",
						imgLogPrefix, len(allModelImages), modelResponse.Name, modelImagesDirAbs, version.ID)
					imgSuccess, imgFail := downloadImages(
						imgLogPrefix,
						allModelImages,
						modelImagesDirAbs,
						imageDownloader,
						cfg.Download.Concurrency,
					)
					log.Infof("%s Finished model image download for dir %s. Success: %d, Failures: %d",
						imgLogPrefix, modelImagesDirAbs, imgSuccess, imgFail)
					processedImageDirs[modelImagesDirAbs] = true
				}
			}
		} else {
			log.Infof("[Model-%d-Images] No images found in model details.", modelResponse.ID)
		}
	}
	// --- Model Images Processing --- END ---

	var totalFiles int
	for _, version := range modelResponse.ModelVersions {
		totalFiles += len(version.Files)
	}
	potentialDownloadsPage := make([]potentialDownload, 0, totalFiles)

	// Iterate through versions
	for _, version := range modelResponse.ModelVersions {
		log.Debugf("    Processing Version: %s (ID: %d)", version.Name, version.ID)

		for _, file := range version.Files {
			// Pass config to filter function
			if !passesFileFilters(file, modelResponse.Type, cfg) {
				continue
			}

			// --- Path Generation using pattern --- START ---
			data := buildPathData(&modelResponse, &version, &file)
			relPath, err := paths.GeneratePath(cfg.Download.VersionPathPattern, data)
			if err != nil {
				log.WithError(err).Errorf("Failed to generate path for model %d, version %d, file %s. Skipping.", modelResponse.ID, version.ID, file.Name)
				continue
			}
			// --- Path Generation using pattern --- END ---

			// Construct full target path and base filename
			finalBaseFilename := fmt.Sprintf("%d_%s", version.ID, helpers.ConvertToSlug(file.Name))
			targetPath := filepath.Join(cfg.SavePath, relPath, finalBaseFilename)

			// --- Ensure ModelId is set in the version struct --- START ---
			versionForPd := version // Make a copy to modify
			if versionForPd.ModelId == 0 && modelResponse.ID != 0 {
				log.Debugf("Populating missing ModelId (%d) in version data (Version ID: %d) for model %d before creating potentialDownload", modelResponse.ID, versionForPd.ID, modelResponse.ID)
				versionForPd.ModelId = modelResponse.ID
			}
			// --- Ensure ModelId is set in the version struct --- END

			pd := potentialDownload{
				ModelID:           modelResponse.ID,
				FullModel:         modelResponse,
				ModelName:         modelResponse.Name,
				ModelType:         modelResponse.Type,
				Creator:           modelResponse.Creator,
				FullVersion:       versionForPd,
				ModelVersionID:    versionForPd.ID,
				File:              file,
				TargetFilepath:    targetPath,
				FinalBaseFilename: finalBaseFilename,
				OriginalImages:    version.Images,
				BaseModel:         version.BaseModel,
				Slug:              helpers.ConvertToSlug(modelResponse.Name),
				VersionName:       versionForPd.Name,
			}
			potentialDownloadsPage = append(potentialDownloadsPage, pd)
		}
		if !cfg.Download.AllVersions {
			log.Debugf("Processing only latest version for model %d, breaking version loop.", modelResponse.ID)
			break
		}
	}

	processedDownloads, totalSize := filterAndPrepareDownloads(potentialDownloadsPage, db, cfg)
	return processedDownloads, totalSize, nil
}

// filterAndPrepareDownloads checks potential downloads against the database, generates the final path,
// and prepares them for the download queue.
// Now uses the passed config struct.
func filterAndPrepareDownloads(potentialDownloadsPage []potentialDownload, db *database.DB, cfg *models.Config) ([]potentialDownload, uint64) {
	downloadsToQueueFiltered := make([]potentialDownload, 0, len(potentialDownloadsPage))
	var totalSizeFiltered uint64

	for _, pd := range potentialDownloadsPage {
		// --- Path Generation using pattern --- START ---
		// This is now the single source of truth for path generation before queueing.
		data := buildPathData(&pd.FullModel, &pd.FullVersion, &pd.File)
		relPath, err := paths.GeneratePath(cfg.Download.VersionPathPattern, data)
		if err != nil {
			log.WithError(err).Errorf("Failed to generate path for version %d, file %s. Skipping.", pd.ModelVersionID, pd.File.Name)
			continue
		}
		finalBaseFilename := fmt.Sprintf("%d_%s", pd.ModelVersionID, helpers.ConvertToSlug(pd.File.Name))
		targetPath := filepath.Join(cfg.SavePath, relPath, finalBaseFilename)

		// Update the potentialDownload with the final, correct path information
		pd.TargetFilepath = targetPath
		pd.FinalBaseFilename = finalBaseFilename
		// --- Path Generation using pattern --- END ---

		dbKey := fmt.Sprintf("v_%d", pd.ModelVersionID)
		shouldQueue := true
		existingEntryBytes, errGet := db.Get([]byte(dbKey))

		correctFolderRelPath := relPath // The relative path is the correct folder path

		if errGet == nil {
			var existingEntry models.DatabaseEntry
			if errUnmarshal := json.Unmarshal(existingEntryBytes, &existingEntry); errUnmarshal == nil {
				if existingEntry.File.ID == pd.File.ID && existingEntry.File.Hashes.CRC32 == pd.File.Hashes.CRC32 {
					if existingEntry.Status == models.StatusDownloaded {
						// Re-queue if images are requested, as they might need downloading.
						if cfg.Download.SaveVersionImages || cfg.Download.SaveModelImages {
							log.Debugf("      - Queuing downloaded file %s (Version %d, File %d) for image check.", pd.File.Name, pd.ModelVersionID, pd.File.ID)
							shouldQueue = true
						} else {
							log.Debugf("      - Skipping file %s (Version %d, File %d): Already marked as downloaded in DB (images not requested).", pd.File.Name, pd.ModelVersionID, pd.File.ID)
							shouldQueue = false
						}
					} else {
						log.Debugf("      - Re-queuing file %s (Version %d, File %d): DB status is %s.", pd.File.Name, pd.ModelVersionID, pd.File.ID, existingEntry.Status)
						// Correct Folder path if necessary
						if existingEntry.Folder != correctFolderRelPath {
							log.Debugf("      - Correcting Folder path in DB for re-queued item %s: from '%s' to '%s'", dbKey, existingEntry.Folder, correctFolderRelPath)
							entryToUpdate := existingEntry // Make a copy to modify
							entryToUpdate.Folder = correctFolderRelPath
							entryToUpdate.Status = models.StatusPending
							entryToUpdate.ErrorDetails = ""
							entryToUpdate.Version = pd.FullVersion
							entryToUpdate.File = pd.File

							updatedEntryBytes, marshalErr := json.Marshal(entryToUpdate)
							if marshalErr == nil {
								if err := db.Put([]byte(dbKey), updatedEntryBytes); err != nil {
									log.WithError(err).Warnf("Failed to update folder path in DB for re-queued item %s", dbKey)
								}
							} else {
								log.WithError(marshalErr).Warnf("Failed to marshal for folder path update for re-queued item %s", dbKey)
							}
						}
					}
				} else {
					log.Debugf("      - Queuing file %s (Version %d, File %d): File ID/Hash mismatch with DB entry.", pd.File.Name, pd.ModelVersionID, pd.File.ID)
					shouldQueue = true
				}
			} else {
				log.WithError(errUnmarshal).Warnf("      - Failed to unmarshal existing DB entry for key %s. Re-queuing.", dbKey)
				shouldQueue = true
			}
		} else if errors.Is(errGet, database.ErrNotFound) {
			log.Debugf("      - Key %s not found in DB. Creating Pending entry.", dbKey)

			versionToSave := pd.FullVersion
			if versionToSave.ModelId == 0 && pd.ModelID != 0 {
				log.Debugf("      - Populating missing ModelId (%d) in Version struct for DB save (Version ID: %d)", pd.ModelID, versionToSave.ID)
				versionToSave.ModelId = pd.ModelID
			}

			newEntry := models.DatabaseEntry{
				ModelID:      pd.ModelID,
				ModelName:    pd.ModelName,
				ModelType:    pd.ModelType,
				Version:      versionToSave,
				File:         pd.File,
				Timestamp:    time.Now().Unix(),
				Creator:      pd.Creator,
				Filename:     pd.FinalBaseFilename,
				Folder:       correctFolderRelPath, // Use the calculated relative path
				Status:       models.StatusPending,
				ErrorDetails: "",
			}
			entryBytes, marshalErr := json.Marshal(newEntry)
			if marshalErr != nil {
				log.WithError(marshalErr).Errorf("      - Failed to marshal NEW DB entry for key %s. Skipping queue.", dbKey)
				shouldQueue = false
			} else {
				if errPut := db.Put([]byte(dbKey), entryBytes); errPut != nil {
					log.WithError(errPut).Errorf("      - Failed to PUT new Pending DB entry for key %s. Skipping queue.", dbKey)
					shouldQueue = false
				} else {
					log.Debugf("      - Successfully created Pending DB entry for key %s.", dbKey)
				}
			}
		} else {
			log.WithError(errGet).Errorf("      - Unexpected error checking database for key %s. Skipping queue.", dbKey)
			shouldQueue = false
		}

		if !shouldQueue {
			continue
		}

		ignoredBaseModels := cfg.Download.IgnoreBaseModels
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

		downloadsToQueueFiltered = append(downloadsToQueueFiltered, pd)
		totalSizeFiltered += uint64(pd.File.SizeKB) * 1024
	}
	return downloadsToQueueFiltered, totalSizeFiltered
}

// fetchModelsPaginated retrieves models page by page from the API.
// ADDED userTotalLimit parameter.
func fetchModelsPaginated(apiClient *api.Client, db *database.DB, imageDownloader *downloader.Downloader, queryParams models.QueryParameters, cfg *models.Config, userTotalLimit int) ([]potentialDownload, uint64, error) {
	// Handle single model cases
	if cfg.Download.ModelID != 0 {
		return handleSingleModelCase(cfg.Download.ModelID, cfg.Download.AllVersions, db, apiClient, imageDownloader, cfg)
	}

	// Handle paginated search
	return handlePaginatedSearch(apiClient, db, queryParams, cfg, userTotalLimit)
}

// handleSingleModelCase handles downloading a single model by ID
func handleSingleModelCase(modelID int, allVersions bool, db *database.DB, apiClient *api.Client, imageDownloader *downloader.Downloader, cfg *models.Config) ([]potentialDownload, uint64, error) {
	if allVersions {
		log.Infof("Fetching all versions for Model ID: %d", modelID)
		return handleSingleModelDownload(modelID, db, apiClient, imageDownloader, cfg)
	}

	log.Infof("Fetching latest version for Model ID: %d", modelID)
	modelDetails, err := apiClient.GetModelDetails(modelID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get model details for ID %d to find latest version: %w", modelID, err)
	}
	if len(modelDetails.ModelVersions) == 0 {
		return nil, 0, fmt.Errorf("no versions found for model ID %d", modelID)
	}

	latestVersionID := modelDetails.ModelVersions[0].ID
	log.Infof("Found latest version ID for model %d: %d", modelID, latestVersionID)
	return handleSingleVersionDownload(latestVersionID, db, apiClient, cfg)
}

// handlePaginatedSearch handles the paginated API search for models
func handlePaginatedSearch(apiClient *api.Client, db *database.DB, queryParams models.QueryParameters, cfg *models.Config, userTotalLimit int) ([]potentialDownload, uint64, error) {
	var allPotentialDownloads []potentialDownload
	var totalDownloadSize uint64
	var nextCursor string
	pageCount := 0
	maxPages := cfg.Download.MaxPages

	log.Infof("Starting paginated model fetch. Max pages: %d", maxPages)

	for {
		pageCount++
		if maxPages > 0 && pageCount > maxPages {
			log.Infof("Reached max pages limit (%d). Stopping model fetch.", maxPages)
			break
		}

		log.Infof("--- Fetching Model Page %d (Cursor: %s) ---", pageCount, nextCursor)

		// Fetch page
		newCursor, response, err := apiClient.GetModels(nextCursor, queryParams)
		if err != nil {
			handleAPIError(err, pageCount)
			return allPotentialDownloads, totalDownloadSize, err
		}

		nextCursor = newCursor
		log.Debugf("Received %d models for page %d", len(response.Items), pageCount)

		if len(response.Items) == 0 {
			log.Info("Received 0 models, assuming end of results.")
			break
		}

		// Handle early exit for limited searches
		if shouldExitEarly(userTotalLimit, cfg.Download.AllVersions, pageCount) {
			nextCursor = ""
		}

		// Process models on this page
		potentialDownloadsPage, reachedLimit := processModelsOnPage(response.Items, apiClient, cfg, userTotalLimit, len(allPotentialDownloads))

		// Filter and add to results
		processedDownloads, pageDownloadSize := filterAndPrepareDownloads(potentialDownloadsPage, db, cfg)
		allPotentialDownloads = append(allPotentialDownloads, processedDownloads...)
		totalDownloadSize += pageDownloadSize

		// Check various exit conditions
		if shouldStopPagination(userTotalLimit, cfg, pageCount, len(allPotentialDownloads), nextCursor, reachedLimit) {
			break
		}

		// API delay if configured
		if cfg.APIDelayMs > 0 {
			delay := time.Duration(cfg.APIDelayMs) * time.Millisecond
			log.Debugf("Waiting %v before fetching next page...", delay)
			time.Sleep(delay)
		}
	}

	log.Infof("Finished fetching models. Found %d potential downloads.", len(allPotentialDownloads))
	return allPotentialDownloads, totalDownloadSize, nil
}

// handleAPIError handles different types of API errors
func handleAPIError(err error, pageCount int) {
	if errors.Is(err, api.ErrRateLimited) {
		log.Error("API Rate Limited. Consider increasing ApiDelayMs or reducing concurrency. Stopping fetch.")
	} else if errors.Is(err, api.ErrUnauthorized) {
		log.Error("API Unauthorized (401/403). Check your API key (ApiKey in config). Stopping fetch.")
	} else {
		log.WithError(err).Errorf("Failed to fetch model page %d", pageCount)
	}
}

// shouldExitEarly determines if pagination should exit early based on limit settings
func shouldExitEarly(userTotalLimit int, allVersions bool, pageCount int) bool {
	return userTotalLimit > 0 && !allVersions && pageCount == 1
}

// processModelsOnPage processes all models on a single page
func processModelsOnPage(models []models.Model, apiClient *api.Client, cfg *models.Config, userTotalLimit, currentDownloadCount int) ([]potentialDownload, bool) {
	totalFiles := calculateTotalFiles(models)
	potentialDownloadsPage := make([]potentialDownload, 0, totalFiles)
	reachedLimit := false

	for _, model := range models {
		if model.Creator.Username == "" {
			model.Creator.Username = "unknown_creator"
		}

		if shouldSkipModelForBaseModel(model, cfg) {
			continue
		}

		fullModelDetails, err := fetchFullModelDetails(model.ID, apiClient)
		if err != nil {
			continue
		}

		modelDownloads, modelReachedLimit := processModelVersions(fullModelDetails, cfg, userTotalLimit, currentDownloadCount+len(potentialDownloadsPage))
		potentialDownloadsPage = append(potentialDownloadsPage, modelDownloads...)

		if modelReachedLimit {
			reachedLimit = true
			break
		}
	}

	return potentialDownloadsPage, reachedLimit
}

// calculateTotalFiles calculates the total number of files across all models
func calculateTotalFiles(models []models.Model) int {
	totalFiles := 0
	for _, model := range models {
		for _, version := range model.ModelVersions {
			totalFiles += len(version.Files)
		}
	}
	return totalFiles
}

// shouldSkipModelForBaseModel checks if a model should be skipped based on base model filters
func shouldSkipModelForBaseModel(model models.Model, cfg *models.Config) bool {
	if len(cfg.Download.IgnoreBaseModels) == 0 || len(model.ModelVersions) == 0 {
		return false
	}

	var representativeBaseModel string
	for _, mv := range model.ModelVersions {
		if mv.BaseModel != "" {
			representativeBaseModel = mv.BaseModel
			break
		}
	}

	if representativeBaseModel != "" && helpers.StringSliceContains(cfg.Download.IgnoreBaseModels, representativeBaseModel) {
		log.Debugf("Skipping model %s (ID: %d) due to ignored base model: %s", model.Name, model.ID, representativeBaseModel)
		return true
	}

	if representativeBaseModel == "" {
		log.Debugf("Model %s (ID: %d) has no versions with BaseModels specified, cannot apply base model ignore filter.", model.Name, model.ID)
	}

	return false
}

// fetchFullModelDetails fetches complete model details from the API
func fetchFullModelDetails(modelID int, apiClient *api.Client) (models.Model, error) {
	log.Debugf("Fetching full details for model %d to ensure accurate version data...", modelID)
	fullModelDetails, err := apiClient.GetModelDetails(modelID)
	if err != nil {
		log.WithError(err).Warnf("Failed to fetch full details for model %d. Skipping this model.", modelID)
		return models.Model{}, err
	}
	return fullModelDetails, nil
}

// processModelVersions processes all versions of a model and returns potential downloads
func processModelVersions(fullModelDetails models.Model, cfg *models.Config, userTotalLimit, currentDownloadCount int) ([]potentialDownload, bool) {
	var potentialDownloads []potentialDownload

	for _, version := range fullModelDetails.ModelVersions {
		versionDownloads, reachedLimit := processVersionFiles(fullModelDetails, version, cfg, userTotalLimit, currentDownloadCount+len(potentialDownloads))
		potentialDownloads = append(potentialDownloads, versionDownloads...)

		if reachedLimit {
			return potentialDownloads, true
		}

		if !cfg.Download.AllVersions {
			log.Debugf("Processing only latest version for model %d (%s) as --all-versions is false. Breaking version loop.", fullModelDetails.ID, fullModelDetails.Name)
			break
		}
	}

	return potentialDownloads, false
}

// processVersionFiles processes all files in a model version
func processVersionFiles(fullModelDetails models.Model, version models.ModelVersion, cfg *models.Config, userTotalLimit, currentDownloadCount int) ([]potentialDownload, bool) {
	potentialDownloads := make([]potentialDownload, 0, len(version.Files))

	for _, file := range version.Files {
		if !passesFileFilters(file, fullModelDetails.Type, cfg) {
			continue
		}

		// Ensure ModelId is set in the version struct
		versionForPd := version
		if versionForPd.ModelId == 0 && fullModelDetails.ID != 0 {
			log.Debugf("Populating missing ModelId (%d) in version data (Version ID: %d) before creating potentialDownload", fullModelDetails.ID, versionForPd.ID)
			versionForPd.ModelId = fullModelDetails.ID
		}

		pd := potentialDownload{
			ModelID:        fullModelDetails.ID,
			FullModel:      fullModelDetails,
			ModelName:      fullModelDetails.Name,
			ModelType:      fullModelDetails.Type,
			Creator:        fullModelDetails.Creator,
			FullVersion:    versionForPd,
			ModelVersionID: versionForPd.ID,
			File:           file,
			OriginalImages: version.Images,
			BaseModel:      version.BaseModel,
			Slug:           helpers.ConvertToSlug(fullModelDetails.Name),
			VersionName:    version.Name,
		}

		if userTotalLimit > 0 && currentDownloadCount+len(potentialDownloads) >= userTotalLimit {
			log.Infof("Reached user download limit (%d) while processing model %d. Stopping further processing for this model and page.", userTotalLimit, fullModelDetails.ID)
			return potentialDownloads, true
		}

		potentialDownloads = append(potentialDownloads, pd)
	}

	return potentialDownloads, false
}

// shouldStopPagination determines if pagination should stop based on various conditions
func shouldStopPagination(userTotalLimit int, cfg *models.Config, pageCount, currentDownloadCount int, nextCursor string, reachedLimit bool) bool {
	// Check if user limit reached after processing this page
	if userTotalLimit > 0 && currentDownloadCount >= userTotalLimit {
		log.Infof("Reached user download limit (%d) after processing page %d. Stopping model fetch.", userTotalLimit, pageCount)
		return true
	}

	// Safety check for --all-versions + --limit
	if userTotalLimit > 0 && cfg.Download.AllVersions && pageCount > 1 && currentDownloadCount == 0 {
		log.Warnf("Fetched %d pages but found 0 downloadable files matching filters while using --limit %d and --all-versions. Stopping pagination to prevent potential infinite loop. Check filters or query if this is unexpected.", pageCount, userTotalLimit)
		return true
	}

	// Check if no next cursor or reached limit
	if nextCursor == "" {
		log.Info("No next cursor available (or loop forced to stop early), stopping model fetch.")
		return true
	}

	return reachedLimit
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
		Sort:            cfg.Download.Sort,
		Period:          cfg.Download.Period,
		Query:           cfg.Download.Query,
		Username:        username,
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
func fetchAndProcessModels(apiClient *api.Client, db *database.DB, queryParams models.QueryParameters, cfg *models.Config) ([]potentialDownload, error) {

	// Setup image downloader (needed for all-versions case inside fetchModelsPaginated)
	// Pass the correct arguments: http client and api key
	imageDownloader := downloader.NewDownloader(apiClient.HttpClient, cfg.APIKey)

	// Fetch models - Pass userTotalLimit (cfg.Download.Limit) now
	allPotentialDownloads, _, err := fetchModelsPaginated(apiClient, db, imageDownloader, queryParams, cfg, cfg.Download.Limit)
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
