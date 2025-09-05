package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go-civitai-download/internal/api"
	"go-civitai-download/internal/database"
	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/models"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gosuri/uilive"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// --- Package Level Variables for Download Flags --- (Moved from init)
var (
	downloadConcurrencyFlag int
	downloadTagFlag         string
	downloadQueryFlag                 string
	downloadModelTypesFlag            []string
	downloadBaseModelsFlag            []string
	downloadUsernameFlag              string
	downloadNsfwFlag                  bool // Note: Config uses Nsfw, flag name is nsfw
	downloadLimitFlag                 int
	downloadMaxPagesFlag              int
	downloadSortFlag                  string
	downloadPeriodFlag                string
	downloadModelIDFlag               int
	downloadModelVersionIDFlag        int
	downloadPrimaryOnlyFlag           bool
	downloadPrunedFlag                bool
	downloadFp16Flag                  bool
	downloadAllVersionsFlag           bool
	downloadIgnoreBaseModelsFlag      []string
	downloadIgnoreFileNameStringsFlag []string
	downloadYesFlag                   bool // Corresponds to SkipConfirmation
	downloadMetadataFlag              bool // Corresponds to SaveMetadata
	downloadModelInfoFlag             bool // Corresponds to SaveModelInfo
	downloadVersionImagesFlag         bool // Corresponds to SaveVersionImages
	downloadModelImagesFlag           bool // Corresponds to SaveModelImages
	downloadMetaOnlyFlag              bool // Corresponds to DownloadMetaOnly
)

// downloadCmd represents the download command
var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download models based on specified criteria",
	Long: `Downloads models from Civitai based on various filters like tags, usernames, model types, etc.
It checks for existing files based on a local database and saves metadata.`,
	RunE: runDownload, // Changed to RunE to handle errors from setup
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	// Concurrency flag
	downloadCmd.Flags().IntVarP(&downloadConcurrencyFlag, "concurrency", "c", 0, "Number of concurrent downloads (0 uses config default)")

	// --- Query Parameter Flags (Mostly mirroring Config struct) ---
	// Filtering & Selection
	downloadCmd.Flags().StringVarP(&downloadTagFlag, "tag", "t", "", "Filter by specific tag name")
	downloadCmd.Flags().StringVarP(&downloadQueryFlag, "query", "q", "", "Search query term (e.g., model name)")
	downloadCmd.Flags().StringSliceVarP(&downloadModelTypesFlag, "model-types", "m", []string{}, "Filter by model types (Checkpoint, LORA, etc.)")
	downloadCmd.Flags().StringSliceVarP(&downloadBaseModelsFlag, "base-models", "b", []string{}, "Filter by base models (SD 1.5, SDXL 1.0, etc.)")
	downloadCmd.Flags().StringVarP(&downloadUsernameFlag, "username", "u", "", "Filter by specific creator username")
	downloadCmd.Flags().BoolVar(&downloadNsfwFlag, "nsfw", false, "Include NSFW models (overrides config)") // Default to false as override
	downloadCmd.Flags().IntVarP(&downloadLimitFlag, "limit", "l", 0, "Total number of models/files to download. 0 means unlimited. If not set, uses config value (defaulting to unlimited if also not in config).")
	downloadCmd.Flags().IntVarP(&downloadMaxPagesFlag, "max-pages", "p", 0, "Maximum number of API pages to process (0 uses config default, which is 0 for no limit)")
	downloadCmd.Flags().StringVar(&downloadSortFlag, "sort", "", "Sort order (newest, oldest, highest_rated, etc. - overrides config)")
	downloadCmd.Flags().StringVar(&downloadPeriodFlag, "period", "", "Time period for sort (Day, Week, Month, Year, AllTime - overrides config)")
	downloadCmd.Flags().IntVar(&downloadModelIDFlag, "model-id", 0, "Download only a specific model ID")
	downloadCmd.Flags().IntVar(&downloadModelVersionIDFlag, "model-version-id", 0, "Download only a specific model version ID")

	// File & Version Selection
	downloadCmd.Flags().BoolVar(&downloadPrimaryOnlyFlag, "primary-only", false, "Only download the primary file for a version (overrides config)")
	downloadCmd.Flags().BoolVar(&downloadPrunedFlag, "pruned", false, "Prefer pruned models (overrides config)")
	downloadCmd.Flags().BoolVar(&downloadFp16Flag, "fp16", false, "Prefer fp16 models (overrides config)")
	downloadCmd.Flags().BoolVar(&downloadAllVersionsFlag, "all-versions", false, "Download all versions of a model, not just the latest (overrides config)")
	downloadCmd.Flags().StringSliceVar(&downloadIgnoreBaseModelsFlag, "ignore-base-models", []string{}, "Base models to ignore (comma-separated or multiple flags, overrides config)")
	downloadCmd.Flags().StringSliceVar(&downloadIgnoreFileNameStringsFlag, "ignore-filename-strings", []string{}, "Substrings in filenames to ignore (comma-separated or multiple flags, overrides config)")

	// Saving & Behavior
	downloadCmd.Flags().BoolVarP(&downloadYesFlag, "yes", "y", false, "Skip confirmation prompt before downloading (overrides config)")
	downloadCmd.Flags().BoolVar(&downloadMetadataFlag, "metadata", false, "Save model version metadata to a JSON file (overrides config)")
	downloadCmd.Flags().BoolVar(&downloadModelInfoFlag, "model-info", false, "Save model info (description, etc.) to a JSON file (overrides config)")
	downloadCmd.Flags().BoolVar(&downloadVersionImagesFlag, "version-images", false, "Save version preview images (overrides config)")
	downloadCmd.Flags().BoolVar(&downloadModelImagesFlag, "model-images", false, "Save model gallery images (overrides config)")
	downloadCmd.Flags().BoolVar(&downloadMetaOnlyFlag, "meta-only", false, "Only download/update metadata files, skip model downloads (overrides config)")

	// Debugging flags
	downloadCmd.Flags().Bool("show-config", false, "Show the effective configuration values and exit")
	downloadCmd.Flags().Bool("debug-print-api-url", false, "Print the constructed API URL for model fetching and exit")
	_ = downloadCmd.Flags().MarkHidden("debug-print-api-url")
}

// setupDownloadEnvironment handles the initialization of database, downloaders, and concurrency settings.
// It now directly uses the globalConfig passed to it.
func setupDownloadEnvironment(cfg *models.Config) (db *database.DB, fileDownloader *downloader.Downloader, imageDownloader *downloader.Downloader, err error) {
	// --- Database Setup ---
	dbPath := cfg.DatabasePath // Already derived in Initialize
	if dbPath == "" {
		// This case should ideally not happen if validation in Initialize works
		err = fmt.Errorf("DatabasePath is empty after configuration initialization")
		return
	}
	log.Infof("Opening database at: %s", dbPath)
	db, err = database.Open(dbPath)
	if err != nil {
		err = fmt.Errorf("failed to open database: %w", err)
		return
	}
	log.Info("Database opened successfully.")

	// --- Concurrency & Downloader Setup ---
	// Concurrency level is directly from the final config
	concurrencyLevel := cfg.Download.Concurrency
	if concurrencyLevel <= 0 {
		concurrencyLevel = 1 // Ensure at least one worker
		log.Warnf("Concurrency level invalid (%d), defaulting to 1", cfg.Download.Concurrency)
	}
	log.Infof("Using concurrency level: %d", concurrencyLevel)

	// --- Downloader Client Setup ---
	// Directly use the globalHttpTransport set up in root.go/config.Initialize
	if globalHttpTransport == nil {
		log.Error("Global HTTP transport not initialized, using default transport without logging.")
		globalHttpTransport = http.DefaultTransport
	}
	mainHttpClient := &http.Client{
		Timeout:   0, // Timeout should be handled by transport or context
		Transport: globalHttpTransport,
	}
	fileDownloader = downloader.NewDownloader(mainHttpClient, cfg.APIKey)

	// --- Setup Image Downloader ---
	if cfg.Download.SaveVersionImages || cfg.Download.SaveModelImages {
		log.Debug("Image saving enabled, creating image downloader instance.")
		imgHttpClient := &http.Client{
			Timeout:   0,
			Transport: globalHttpTransport,
		}
		imageDownloader = downloader.NewDownloader(imgHttpClient, cfg.APIKey)
	}
	if imageDownloader != nil {
		log.Debug("Image downloader initialized successfully.")
	} else {
		log.Debug("Image downloader is nil (image download flags likely not set).")
	}

	return // db, fileDownloader, imageDownloader, nil
}

// handleMetadataOnlyMode processes downloads when only metadata/images are requested.
// It now returns bool indicating if the program should exit, and requires imageDownloader.
func handleMetadataOnlyMode(downloadsToQueue []potentialDownload, cfg *models.Config, imageDownloader *downloader.Downloader) (shouldExit bool) {
	log.Info("--- Metadata-Only Mode Activated ---")
	if len(downloadsToQueue) == 0 {
		log.Info("No new files found for which to save metadata.")
		return true // Exit cleanly
	}

	log.Infof("Attempting to save metadata for %d files...", len(downloadsToQueue))
	savedCount := 0
	failedCount := 0
	processedModelImages := make(map[int]bool) // Track models processed for model images

	for _, pd := range downloadsToQueue {
		// --- Reconstruct the intended file path for metadata saving ---
		baseFilename := pd.FinalBaseFilename
		finalFilenameWithID := baseFilename
		if pd.ModelVersionID > 0 { // Prepend ID if available
			finalFilenameWithID = fmt.Sprintf("%d_%s", pd.ModelVersionID, baseFilename)
		}
		dir := filepath.Dir(pd.TargetFilepath)
		finalPathForMeta := filepath.Join(dir, finalFilenameWithID)
		log.Debugf("Using base path for meta-only JSON derivation: %s", finalPathForMeta)
		// --- End Path Reconstruction ---

		// --- Ensure Directory Exists (for metadata) ---
		metaDir := filepath.Dir(finalPathForMeta)
		if err := os.MkdirAll(metaDir, 0750); err != nil {
			log.WithError(err).Errorf("Failed to create directory %s for metadata file", metaDir)
			failedCount++
			continue // Skip to next potential download
		}
		// --- End Ensure Directory Exists ---

		// Save Metadata JSON
		err := saveVersionMetadataFile(pd, finalPathForMeta)
		if err != nil {
			log.Warnf("Failed to save metadata for %s (VersionID: %d): %v", pd.File.Name, pd.ModelVersionID, err)
			failedCount++
			// NOTE: Don't continue here if metadata save fails, still attempt image downloads if requested
		} else {
			savedCount++
		}

		// --- Handle Version Images (--version-images) ---
		if cfg.Download.SaveVersionImages && len(pd.FullVersion.Images) > 0 {
			// Version images go into the same directory as the metadata JSON
			versionImageDir := filepath.Join(metaDir, "images") // Append 'images' subdirectory
			logPrefix := fmt.Sprintf("MetaOnly-Ver-%d-Img", pd.ModelVersionID)

			// Ensure version image directory exists (downloadImages does this, but belt-and-suspenders)
			if err := os.MkdirAll(versionImageDir, 0750); err != nil {
				log.WithError(err).Errorf("[%s] Failed to create directory %s for version images", logPrefix, versionImageDir)
			} else {
				log.Infof("[%s] Downloading %d version images to %s", logPrefix, len(pd.FullVersion.Images), versionImageDir)
				downloadImages(logPrefix, pd.FullVersion.Images, versionImageDir, imageDownloader, cfg.Download.Concurrency)
				// Note: We are not tracking success/failure counts from downloadImages here for simplicity in meta-only mode.
			}
		}
		// --- End Handle Version Images ---

		// --- Handle Model Images (--model-images) ---
		if cfg.Download.SaveModelImages && !processedModelImages[pd.ModelID] {
			// Collect all images from all versions within the FullModel details
			var allModelImages []models.ModelImage
			for _, version := range pd.FullModel.ModelVersions {
				if len(version.Images) > 0 {
					allModelImages = append(allModelImages, version.Images...)
				}
			}

			if len(allModelImages) > 0 { // Proceed only if images were found
				// Model images go into the model's base directory/images
				modelBaseDir := filepath.Dir(metaDir) // Go up one level from the version-specific dir
				modelImageDir := filepath.Join(modelBaseDir, "images")
				logPrefix := fmt.Sprintf("MetaOnly-Mod-%d-Img", pd.ModelID)

				// Ensure model image directory exists
				if err := os.MkdirAll(modelImageDir, 0750); err != nil {
					log.WithError(err).Errorf("[%s] Failed to create directory %s for model images", logPrefix, modelImageDir)
				} else {
					log.Infof("[%s] Downloading %d model images to %s", logPrefix, len(allModelImages), modelImageDir)
					downloadImages(logPrefix, allModelImages, modelImageDir, imageDownloader, cfg.Download.Concurrency)
					processedModelImages[pd.ModelID] = true // Mark model as processed
					// Note: We are not tracking success/failure counts from downloadImages here.
				}
			} else {
				// Log if SaveModelImages was true but no images found in any version
				log.Debugf("[MetaOnly-Mod-%d-Img] No model images found across all versions.", pd.ModelID)
				processedModelImages[pd.ModelID] = true // Still mark as processed to avoid re-checking
			}
		}
		// --- End Handle Model Images ---

	} // End loop through downloadsToQueue

	log.Infof("Metadata-only mode finished. Metadata Saved: %d, Metadata Failed: %d", savedCount, failedCount)
	return true // Exit after processing
}

// confirmDownload displays the download summary and prompts the user for confirmation.
// Returns true if the user confirms, false otherwise. It now receives the globalConfig.
func confirmDownload(downloadsToQueue []potentialDownload, cfg *models.Config) bool {
	if len(downloadsToQueue) == 0 {
		log.Info("No new files meet the criteria or need downloading.")
		return false // Nothing to confirm
	}

	// Check if confirmation should be skipped using the config
	if cfg.Download.SkipConfirmation {
		log.Info("Skipping download confirmation due to --yes flag or config setting.")
		return true
	}

	// Calculate total size for confirmation
	var totalQueuedSizeBytes uint64 = 0
	for _, pd := range downloadsToQueue {
		totalQueuedSizeBytes += uint64(pd.File.SizeKB) * 1024
	}
	totalSizeMB := float64(totalQueuedSizeBytes) / 1024 / 1024
	totalSizeGB := totalSizeMB / 1024

	fmt.Printf("\n--- Download Summary ---\n")
	fmt.Printf("Files to download: %d\n", len(downloadsToQueue))
	if totalSizeGB >= 1.0 {
		fmt.Printf("Total size: %.2f GB\n", totalSizeGB)
	} else {
		fmt.Printf("Total size: %.2f MB\n", totalSizeMB)
	}
	fmt.Println("----------------------")

	// Prompt user
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Proceed with download? (y/n): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			log.WithError(err).Error("Error reading input, aborting download.")
			return false
		}
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "y" || input == "yes" {
			return true
		} else if input == "n" || input == "no" {
			log.Info("Download cancelled by user.")
			return false
		} else {
			fmt.Println("Invalid input. Please enter 'y' or 'n'.")
		}
	}
}

// confirmParameters prints the effective settings and asks for user confirmation.
// Uses globalConfig which should be populated.
func confirmParameters(cmd *cobra.Command, cfg *models.Config, queryParams models.QueryParameters) bool {
	fmt.Println("--- Current Settings ---")
	settingsSummary := map[string]interface{}{
		"ApiClientTimeoutSec":   cfg.APIClientTimeoutSec,
		"ApiDelayMs":            cfg.APIDelayMs,
		"ApiKeySet":             cfg.APIKey != "",
		"Concurrency":           cfg.Download.Concurrency,
		"DatabasePath":          cfg.DatabasePath,
		"DownloadAllVersions":   cfg.Download.AllVersions,
		"DownloadMetaOnly":      cfg.Download.DownloadMetaOnly,
		"Fp16":                  cfg.Download.Fp16,
		"IgnoreBaseModels":      cfg.Download.IgnoreBaseModels,
		"IgnoreFileNameStrings": cfg.Download.IgnoreFileNameStrings,
		"InitialRetryDelayMs":   cfg.InitialRetryDelayMs,
		"LogApiRequests":        cfg.LogApiRequests,
		"LogFormat":             cfg.LogFormat,
		"LogLevel":              cfg.LogLevel,
		"MaxPages":              cfg.Download.MaxPages,
		"MaxRetries":            cfg.MaxRetries,
		"ModelID":               cfg.Download.ModelID,
		"ModelInfoPathPattern":  cfg.Download.ModelInfoPathPattern,
		"ModelVersionID":        cfg.Download.ModelVersionID,
		"Nsfw":                  cfg.Download.Nsfw,
		"PrimaryOnly":           cfg.Download.PrimaryOnly,
		"Pruned":                cfg.Download.Pruned,
		"SaveMetadata":          cfg.Download.SaveMetadata,
		"SaveModelImages":       cfg.Download.SaveModelImages,
		"SaveModelInfo":         cfg.Download.SaveModelInfo,
		"SavePath":              cfg.SavePath,
		"SaveVersionImages":     cfg.Download.SaveVersionImages,
		"SkipConfirmation":      cfg.Download.SkipConfirmation,
		"VersionPathPattern":    cfg.Download.VersionPathPattern,
	}

	settingsJSON, _ := json.MarshalIndent(settingsSummary, "", "  ")
	fmt.Println(string(settingsJSON))

	// --- NEW: Explicitly state the Total Download Limit ---
	fmt.Println("--- Total Download Limit (Application) ---")
	if cfg.Download.Limit <= 0 {
		fmt.Println("Limit: Unlimited (0)")
	} else {
		fmt.Printf("Limit: %d\n", cfg.Download.Limit)
	}
	// --- END NEW ---

	fmt.Println("\n--- Query Parameters for API (Page size defaults to API) ---")
	// Create a temporary map to display query params, letting API default the page size
	displayQueryParams := map[string]interface{}{
		"page":                  queryParams.Page,
		"query":                 queryParams.Query,
		"username":              queryParams.Username,
		"tag":                   queryParams.Tag,
		"types":                 queryParams.Types,
		"baseModels":            queryParams.BaseModels,
		"sort":                  queryParams.Sort,
		"period":                queryParams.Period,
		"primaryFileOnly":       queryParams.PrimaryFileOnly,
		"allowNoCredit":         queryParams.AllowNoCredit,
		"allowDerivatives":      queryParams.AllowDerivatives,
		"allowDifferentLicense": queryParams.AllowDifferentLicenses,
		"allowCommercialUse":    queryParams.AllowCommercialUse,
		"nsfw":                  queryParams.Nsfw,
	}
	queryJSON, _ := json.MarshalIndent(displayQueryParams, "", "  ")
	fmt.Println(string(queryJSON))

	if cfg.Download.SkipConfirmation {
		return true
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Confirm parameters? (y/n): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			log.WithError(err).Error("Error reading input, aborting.")
			return false
		}
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "y" || input == "yes" {
			return true
		} else if input == "n" || input == "no" {
			log.Info("Operation cancelled by user.")
			return false
		} else {
			fmt.Println("Invalid input. Please enter 'y' or 'n'.")
		}
	}
}

// executeDownloads manages the download worker pool and progress display.
// It now receives the globalConfig.
func executeDownloads(downloadsToQueue []potentialDownload, db *database.DB, fileDownloader *downloader.Downloader, imageDownloader *downloader.Downloader, cfg *models.Config) {
	var wg sync.WaitGroup
	// Change channel type to downloadJob
	jobQueue := make(chan downloadJob, len(downloadsToQueue))

	numWorkers := cfg.Download.Concurrency
	totalCount := len(downloadsToQueue)
	log.Infof("Starting %d download workers for %d jobs...", numWorkers, totalCount)

	// --- Progress Display Setup ---
	writer := uilive.New()
	writer.Start()
	defer writer.Stop() // Ensure writer stops

	// Start workers - Pass writer and totalCount, remove results/status channels, ADD CFG
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		// Pass cfg to the worker
		go downloadWorker(i+1, jobQueue, db, fileDownloader, imageDownloader, &wg, writer, totalCount, cfg)
	}

	// Queue downloads as downloadJob structs
	log.Debugf("Queueing %d download jobs...", totalCount)
	for _, pd := range downloadsToQueue {
		// Use the same key format as processPage (v_{VersionID})
		dbKey := fmt.Sprintf("v_%d", pd.ModelVersionID)
		job := downloadJob{
			PotentialDownload: pd,
			DatabaseKey:       dbKey,
		}
		jobQueue <- job
	}
	close(jobQueue) // Signal workers that no more jobs are coming
	log.Debug("Finished queueing jobs.")

	wg.Wait() // Wait for all download workers to finish
	// Close unnecessary channels
	// close(statusUpdates)
	// close(results)
	// displayWg.Wait()

	log.Info("All download workers finished.")
}

// updateConcurrency dynamically updates concurrency based on flag, if set.
func updateConcurrency(cmd *cobra.Command, cfg *models.Config) {
	// Check if the concurrency flag was specifically set by the user for this run
	if cmd.Flags().Changed("concurrency") {
		concurrencyVal, _ := cmd.Flags().GetInt("concurrency")
		if concurrencyVal > 0 {
			log.Infof("Overriding concurrency with flag value: %d", concurrencyVal)
			cfg.Download.Concurrency = concurrencyVal // Directly update the loaded config struct
		} else {
			log.Warnf("Ignoring invalid concurrency flag value: %d", concurrencyVal)
		}
	}
}

// validateDownloadConfig validates and prepares the download configuration
func validateDownloadConfig(cmd *cobra.Command) (*models.Config, error) {
	cfg := globalConfig // Use the config loaded in PersistentPreRunE

	// Update config based on flags specific to this run
	updateConcurrency(cmd, &cfg)

	// Update Limit based on flag
	if cmd.Flags().Changed("limit") {
		limitVal, _ := cmd.Flags().GetInt("limit")
		// If user explicitly sets -l 0, it means 0 (unlimited), overriding config
		log.Infof("Overriding download limit with flag value: %d (0 means unlimited)", limitVal)
		cfg.Download.Limit = limitVal
	}

	return &cfg, nil
}

// setupDownloadContext prepares the download environment and validates parameters
func setupDownloadContext(cmd *cobra.Command, cfg *models.Config) (*http.Client, models.QueryParameters, error) {
	// Shared HTTP Client using global transport
	sharedHttpClient := &http.Client{
		Timeout:   0, // Timeout managed by transport
		Transport: globalHttpTransport,
	}

	// API Parameter Construction
	queryParams := buildQueryParameters(cfg)

	// Debug: Print API URL if requested
	debugPrintApiUrlFlag, _ := cmd.Flags().GetBool("debug-print-api-url")
	if debugPrintApiUrlFlag {
		fmt.Println(models.ConstructApiUrl(queryParams))
		return nil, queryParams, fmt.Errorf("debug mode - exiting after URL print")
	}

	// Confirm Parameters (Handles --show-config)
	if !confirmParameters(cmd, cfg, queryParams) {
		return nil, queryParams, fmt.Errorf("user cancelled or show-config mode")
	}

	return sharedHttpClient, queryParams, nil
}

// fetchDownloadCandidates fetches and processes models based on configuration
func fetchDownloadCandidates(cfg *models.Config, apiClient *api.Client, db *database.DB, imageDownloader *downloader.Downloader) ([]potentialDownload, error) {
	log.Info("Fetching model information from Civitai API...")

	var downloadsToQueue []potentialDownload
	var fetchErr error

	if cfg.Download.ModelVersionID > 0 {
		log.Infof("Processing specific model version ID: %d", cfg.Download.ModelVersionID)
		downloadsToQueue, _, fetchErr = handleSingleVersionDownload(cfg.Download.ModelVersionID, db, apiClient, cfg)
	} else if cfg.Download.ModelID > 0 {
		log.Infof("Processing specific model ID: %d (All versions: %v)", cfg.Download.ModelID, cfg.Download.AllVersions)
		downloadsToQueue, _, fetchErr = handleSingleModelDownload(cfg.Download.ModelID, db, apiClient, imageDownloader, cfg)
	} else {
		log.Info("Processing models based on general query parameters.")
		downloadsToQueue, fetchErr = fetchAndProcessModels(apiClient, db, buildQueryParameters(cfg), cfg)
	}

	if fetchErr != nil {
		return nil, fmt.Errorf("error fetching or processing models: %w", fetchErr)
	}

	log.Infof("Finished initial fetch/processing. Found %d potential downloads.", len(downloadsToQueue))
	return downloadsToQueue, nil
}

// applyDownloadLimits applies user-specified download limits to the download queue
func applyDownloadLimits(downloadsToQueue []potentialDownload, cfg *models.Config) []potentialDownload {
	userTotalLimit := cfg.Download.Limit
	// Only apply limit if it's positive AND if we WEREN'T fetching a specific version ID
	if userTotalLimit > 0 && cfg.Download.ModelVersionID == 0 && len(downloadsToQueue) > userTotalLimit {
		log.Infof("User limit (--limit %d) is less than the total potential downloads found (%d). Truncating list.", userTotalLimit, len(downloadsToQueue))
		downloadsToQueue = downloadsToQueue[:userTotalLimit]
		log.Infof("Proceeding with the first %d potential downloads.", len(downloadsToQueue))
	} else if userTotalLimit > 0 && cfg.Download.ModelVersionID == 0 {
		log.Debugf("User limit (--limit %d) is not exceeded by potential downloads (%d).", userTotalLimit, len(downloadsToQueue))
	}
	return downloadsToQueue
}

// runDownload is the main execution function for the download command.
// It now uses globalConfig populated by loadGlobalConfig.
func runDownload(cmd *cobra.Command, args []string) error {
	log.Info("Starting download command...")

	// Validate and prepare configuration
	cfg, err := validateDownloadConfig(cmd)
	if err != nil {
		return err
	}

	// Setup download context and validate parameters
	sharedHttpClient, _, err := setupDownloadContext(cmd, cfg)
	if err != nil {
		// Check if this is a controlled exit (debug mode or user cancellation)
		if err.Error() == "debug mode - exiting after URL print" || err.Error() == "user cancelled or show-config mode" {
			return nil
		}
		return err
	}

	// Setup Database, Downloaders
	db, fileDownloader, imageDownloader, err := setupDownloadEnvironment(cfg)
	if err != nil {
		log.Errorf("Failed to set up download environment: %v", err)
		return err
	}
	defer db.Close()

	// Create API client instance using shared client and config
	apiClient := api.NewClient(cfg.APIKey, sharedHttpClient, *cfg)

	// Fetch and process models
	downloadsToQueue, err := fetchDownloadCandidates(cfg, apiClient, db, imageDownloader)
	if err != nil {
		log.Errorf("Failed to fetch download candidates: %v", err)
		return err
	}

	// Apply download limits
	downloadsToQueue = applyDownloadLimits(downloadsToQueue, cfg)

	// Handle Metadata-Only Mode
	if cfg.Download.DownloadMetaOnly {
		if handleMetadataOnlyMode(downloadsToQueue, cfg, imageDownloader) {
			return nil // Exit after meta-only processing
		}
	}

	// Confirm Actual Download
	if !confirmDownload(downloadsToQueue, cfg) {
		return nil // Exit if user cancels
	}

	// Execute Downloads
	executeDownloads(downloadsToQueue, db, fileDownloader, imageDownloader, cfg)

	log.Info("Download command finished.")
	return nil
}
