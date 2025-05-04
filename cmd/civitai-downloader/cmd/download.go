package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go-civitai-download/internal/api"
	"go-civitai-download/internal/database"
	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/models"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/gosuri/uilive"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	index "go-civitai-download/index"
)

// downloadCmd represents the download command
var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download models based on specified criteria",
	Long: `Downloads models from Civitai based on various filters like tags, usernames, model types, etc.
It checks for existing files based on a local database and saves metadata.`,
	Run: runDownload,
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	// Logging flags (local to download command? or should be persistent? Currently local)
	downloadCmd.Flags().StringVar(&logLevel, "log-level", "info", "Logging level (debug, info, warn, error)")
	downloadCmd.Flags().StringVar(&logFormat, "log-format", "text", "Log format (text, json)")

	// Concurrency flag
	downloadCmd.Flags().IntP("concurrency", "c", 0, "Number of concurrent downloads (overrides config)")
	// Bind the flag to Viper using the struct field name as the key
	_ = viper.BindPFlag("concurrency", downloadCmd.Flags().Lookup("concurrency"))

	// --- Query Parameter Flags (Mostly mirroring Config struct) ---
	// Authentication
	downloadCmd.Flags().String("api-key", "", "Civitai API Key (overrides config)")
	_ = viper.BindPFlag("apikey", downloadCmd.Flags().Lookup("api-key"))

	// Filtering & Selection
	downloadCmd.Flags().StringP("tag", "t", "", "Filter by specific tag name")
	_ = viper.BindPFlag("tag", downloadCmd.Flags().Lookup("tag"))
	downloadCmd.Flags().StringP("query", "q", "", "Search query term (e.g., model name)")
	_ = viper.BindPFlag("query", downloadCmd.Flags().Lookup("query"))
	downloadCmd.Flags().StringSliceP("model-types", "m", []string{}, "Filter by model types (Checkpoint, LORA, etc.)")
	_ = viper.BindPFlag("modeltypes", downloadCmd.Flags().Lookup("model-types"))
	downloadCmd.Flags().StringSliceP("base-models", "b", []string{}, "Filter by base models (SD 1.5, SDXL 1.0, etc.)")
	_ = viper.BindPFlag("basemodels", downloadCmd.Flags().Lookup("base-models"))
	downloadCmd.Flags().StringP("username", "u", "", "Filter by specific creator username")
	_ = viper.BindPFlag("username", downloadCmd.Flags().Lookup("username"))
	downloadCmd.Flags().Bool("nsfw", false, "Include NSFW models (overrides config)")
	_ = viper.BindPFlag("nsfw", downloadCmd.Flags().Lookup("nsfw"))
	downloadCmd.Flags().IntP("limit", "l", 0, "Limit the number of models to download per query page (overrides config)")
	_ = viper.BindPFlag("limit", downloadCmd.Flags().Lookup("limit"))
	downloadCmd.Flags().IntP("max-pages", "p", 0, "Maximum number of pages to process (0 for unlimited)")
	_ = viper.BindPFlag("maxpages", downloadCmd.Flags().Lookup("max-pages"))
	downloadCmd.Flags().String("sort", "", "Sort order (newest, oldest, highest_rated, etc. - overrides config)")
	_ = viper.BindPFlag("sort", downloadCmd.Flags().Lookup("sort"))
	downloadCmd.Flags().String("period", "", "Time period for sort (Day, Week, Month, Year, AllTime - overrides config)")
	_ = viper.BindPFlag("period", downloadCmd.Flags().Lookup("period"))
	downloadCmd.Flags().Int("model-id", 0, "Download only a specific model ID")
	_ = viper.BindPFlag("modelid", downloadCmd.Flags().Lookup("model-id")) // Should match config struct field if exists
	downloadCmd.Flags().Int("model-version-id", 0, "Download only a specific model version ID")
	_ = viper.BindPFlag("modelversionid", downloadCmd.Flags().Lookup("model-version-id")) // Should match config struct field if exists

	// File & Version Selection
	downloadCmd.Flags().Bool("primary-only", false, "Only download the primary file for a version (overrides config)")
	_ = viper.BindPFlag("primaryonly", downloadCmd.Flags().Lookup("primary-only"))
	downloadCmd.Flags().Bool("pruned", false, "Prefer pruned models (overrides config)")
	_ = viper.BindPFlag("pruned", downloadCmd.Flags().Lookup("pruned"))
	downloadCmd.Flags().Bool("fp16", false, "Prefer fp16 models (overrides config)")
	_ = viper.BindPFlag("fp16", downloadCmd.Flags().Lookup("fp16"))
	downloadCmd.Flags().Bool("all-versions", false, "Download all versions of a model, not just the latest (overrides config)")
	_ = viper.BindPFlag("downloadallversions", downloadCmd.Flags().Lookup("all-versions"))
	downloadCmd.Flags().StringSlice("ignore-base-models", []string{}, "Base models to ignore (comma-separated or multiple flags, overrides config)")
	_ = viper.BindPFlag("ignorebasemodels", downloadCmd.Flags().Lookup("ignore-base-models"))
	downloadCmd.Flags().StringSlice("ignore-filename-strings", []string{}, "Substrings in filenames to ignore (comma-separated or multiple flags, overrides config)")
	_ = viper.BindPFlag("ignorefilenamestrings", downloadCmd.Flags().Lookup("ignore-filename-strings"))

	// Saving & Behavior
	downloadCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt before downloading (overrides config)")
	_ = viper.BindPFlag("skipconfirmation", downloadCmd.Flags().Lookup("yes"))
	downloadCmd.Flags().Bool("metadata", false, "Save model version metadata to a JSON file (overrides config)")
	_ = viper.BindPFlag("savemetadata", downloadCmd.Flags().Lookup("metadata"))
	downloadCmd.Flags().Bool("model-info", false, "Save model info (description, etc.) to a JSON file (overrides config)") // Renamed flag
	_ = viper.BindPFlag("savemodelinfo", downloadCmd.Flags().Lookup("model-info"))
	downloadCmd.Flags().Bool("version-images", false, "Save version preview images (overrides config)") // Renamed flag
	_ = viper.BindPFlag("saveversionimages", downloadCmd.Flags().Lookup("version-images"))
	downloadCmd.Flags().Bool("model-images", false, "Save model gallery images (overrides config)") // Renamed flag
	_ = viper.BindPFlag("savemodelimages", downloadCmd.Flags().Lookup("model-images"))
	downloadCmd.Flags().Bool("meta-only", false, "Only download/update metadata files, skip model downloads (overrides config)") // Renamed flag
	_ = viper.BindPFlag("downloadmetaonly", downloadCmd.Flags().Lookup("meta-only"))

	// Debugging flags
	downloadCmd.Flags().Bool("show-config", false, "Show the effective configuration values and exit")
	downloadCmd.Flags().Bool("debug-print-api-url", false, "Print the constructed API URL for model fetching and exit")
	_ = downloadCmd.Flags().MarkHidden("debug-print-api-url") // Hide from help output
}

var logLevel string
var logFormat string

func maskApiKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 4 {
		return "****" // shouldn't happen with valid keys
	}
	return "****" + key[len(key)-4:] // show last 4 chars
}

// setupDownloadEnvironment handles the initialization of database, downloaders, and concurrency settings.
func setupDownloadEnvironment(cmd *cobra.Command, cfg *models.Config) (db *database.DB, fileDownloader *downloader.Downloader, imageDownloader *downloader.Downloader, concurrencyLevel int, err error) {
	// --- Database Setup ---
	dbPath := cfg.DatabasePath
	if dbPath == "" {
		if cfg.SavePath != "" {
			dbPath = filepath.Join(cfg.SavePath, "civitai_download_db")
			log.Warnf("DatabasePath not set in config, defaulting to: %s", dbPath)
		} else {
			err = fmt.Errorf("DatabasePath and SavePath are not set in config. Cannot determine database location")
			return
		}
	}
	log.Infof("Opening database at: %s", dbPath)
	db, err = database.Open(dbPath)
	if err != nil {
		err = fmt.Errorf("failed to open database: %w", err)
		return
	}
	log.Info("Database opened successfully.")

	// --- Concurrency & Downloader Setup ---
	// Get concurrency level using Viper (respects flag > config > default)
	concurrencyLevel = viper.GetInt("concurrency") // Use Viper to get value

	// Apply default only if the value from flag/config is invalid
	if concurrencyLevel <= 0 {
		// Try reading from the explicitly loaded config as a fallback before hardcoded default
		concurrencyLevel = cfg.Concurrency
		if concurrencyLevel <= 0 {
			concurrencyLevel = 3 // Hardcoded fallback default
			log.Warnf("Concurrency not set or invalid in config/flags, using default: %d", concurrencyLevel)
		}
	}
	log.Infof("Using concurrency level: %d", concurrencyLevel)

	// --- Downloader Client Setup ---
	// Directly use the globalHttpTransport set up in root.go
	if globalHttpTransport == nil {
		// Fallback in case root command setup failed silently
		log.Error("Global HTTP transport not initialized, using default transport without logging.")
		globalHttpTransport = http.DefaultTransport // Use default as fallback
	}
	// Create client for file downloader using the global transport
	mainHttpClient := &http.Client{
		Timeout:   0, // Rely on transport timeouts
		Transport: globalHttpTransport,
	}
	fileDownloader = downloader.NewDownloader(mainHttpClient, cfg.ApiKey)

	// --- Setup Image Downloader ---
	// Use correct viper keys corresponding to bound flags
	if viper.GetBool("saveversionimages") || viper.GetBool("savemodelimages") {
		log.Debug("Image saving enabled, creating image downloader instance.")
		// Create a separate client instance for image downloader, but reuse the global transport
		imgHttpClient := &http.Client{
			Timeout:   0,
			Transport: globalHttpTransport,
		}
		imageDownloader = downloader.NewDownloader(imgHttpClient, cfg.ApiKey)
	}
	// Add debug log here
	if imageDownloader != nil {
		log.Debug("Image downloader initialized successfully.")
	} else {
		log.Debug("Image downloader is nil (image download flags likely not set).")
	}

	return
}

// handleMetadataOnlyMode handles the logic when --download-meta-only is specified.
// It saves metadata for the queued files and returns true if the program should exit.
func handleMetadataOnlyMode(downloadsToQueue []potentialDownload, cfg *models.Config) (shouldExit bool) {
	log.Info("--- Metadata-Only Mode Activated --- ")
	if len(downloadsToQueue) == 0 {
		log.Info("No new files found for which to save metadata.")
		return true // Exit cleanly
	}

	log.Infof("Attempting to save metadata for %d files...", len(downloadsToQueue))
	savedCount := 0
	failedCount := 0
	for _, pd := range downloadsToQueue {
		// --- Reconstruct the intended file path for metadata saving ---
		// This mirrors the logic that would happen during download to determine the final filename
		// before the .json suffix is added by saveMetadataFile.
		baseFilename := pd.FinalBaseFilename // e.g., my_model_v1.safetensors
		finalFilenameWithID := baseFilename
		if pd.ModelVersionID > 0 { // Prepend ID if available
			finalFilenameWithID = fmt.Sprintf("%d_%s", pd.ModelVersionID, baseFilename)
		}
		dir := filepath.Dir(pd.TargetFilepath) // Get the target directory
		// Construct the final path that the model file *would* have had
		finalPathForMeta := filepath.Join(dir, finalFilenameWithID)
		log.Debugf("Using base path for meta-only JSON derivation: %s", finalPathForMeta)
		// --- End Path Reconstruction ---

		// Pass the potential download struct and the reconstructed path
		err := saveMetadataFile(pd, finalPathForMeta)
		if err != nil {
			// Use ModelVersionID for logging
			log.Warnf("Failed to save metadata for %s (VersionID: %d): %v", pd.File.Name, pd.ModelVersionID, err)
			failedCount++
		} else {
			savedCount++
		}
		// Note: We don't change DB status here.
	}

	log.Infof("Metadata-only mode finished. Saved: %d, Failed: %d", savedCount, failedCount)
	return true // Exit after processing
}

// confirmDownload displays the download summary and prompts the user for confirmation.
// Returns true if the user confirms, false otherwise.
func confirmDownload(downloadsToQueue []potentialDownload) bool {
	if len(downloadsToQueue) == 0 {
		log.Info("No new files meet the criteria or need downloading.")
		return false // Nothing to confirm
	}

	// Check if confirmation should be skipped
	log.Debugf("Checking viper skipconfirmation value: %v", viper.GetBool("skipconfirmation"))
	if viper.GetBool("skipconfirmation") {
		log.Info("Skipping download confirmation due to --yes flag or config setting.")
		return true
	}

	// Calculate total size for confirmation
	var totalQueuedSizeBytes uint64 = 0
	for _, pd := range downloadsToQueue {
		// Cast SizeKB (float64) to uint64 before calculation
		totalQueuedSizeBytes += uint64(pd.File.SizeKB) * 1024 // Convert KB to Bytes
	}

	log.Infof("--- Download Summary ---")
	log.Infof("Total files to download: %d", len(downloadsToQueue))
	log.Infof("Total size: %.2f GB", float64(totalQueuedSizeBytes)/(1024*1024*1024))
	// List first few files for context
	maxFilesToShow := 5
	if len(downloadsToQueue) < maxFilesToShow {
		maxFilesToShow = len(downloadsToQueue)
	}
	log.Info("Files to be downloaded include:")
	for i := 0; i < maxFilesToShow; i++ {
		pd := downloadsToQueue[i]
		log.Infof("  - %s (%.2f MB)", pd.FinalBaseFilename, pd.File.SizeKB/1024.0)
	}
	if len(downloadsToQueue) > maxFilesToShow {
		log.Infof("  ... and %d more.", len(downloadsToQueue)-maxFilesToShow)
	}

	// Confirmation Prompt
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Proceed with download? (y/N): ")
	input, _ := reader.ReadString('\n')
	input = strings.ToLower(strings.TrimSpace(input))

	if input != "y" {
		log.Info("Download cancelled by user.")
		return false
	}
	return true
}

// confirmParameters displays the effective configuration and API query parameters,
// then prompts the user for confirmation before proceeding with API calls.
// Returns true if the user confirms or if confirmation is skipped, false otherwise.
func confirmParameters(queryParams models.QueryParameters) bool {
	// Check if confirmation should be skipped first
	if viper.GetBool("skipconfirmation") {
		log.Info("Skipping parameter confirmation due to --yes flag or config setting.")
		return true
	}

	log.Info("--- Review Effective Configuration & Parameters ---")

	// Display Global Config (similar to --show-config)
	effectiveGlobalConfig := map[string]interface{}{
		// Paths (might still be empty if not set)
		"SavePath":       viper.GetString("savepath"),
		"DatabasePath":   viper.GetString("databasepath"),
		"BleveIndexPath": viper.GetString("bleveindexpath"),
		"ApiKey":         maskApiKey(viper.GetString("apikey")), // Show masked API key
		// Filtering - Model/Version
		"DownloadAllVersions": viper.GetBool("downloadallversions"),
		"ModelVersionID":      viper.GetInt("modelversionid"),
		"ModelID":             viper.GetInt("modelid"), // Added ModelID for completeness
		// Filtering - File Level
		"PrimaryOnly":           viper.GetBool("primaryonly"),
		"Pruned":                viper.GetBool("pruned"),
		"Fp16":                  viper.GetBool("fp16"),
		"IgnoreBaseModels":      viper.GetStringSlice("ignorebasemodels"),
		"IgnoreFileNameStrings": viper.GetStringSlice("ignorefilenamestrings"),
		// Downloader Behavior
		"Concurrency":         viper.GetInt("concurrency"),
		"SaveMetadata":        viper.GetBool("savemetadata"),
		"DownloadMetaOnly":    viper.GetBool("downloadmetaonly"),
		"SaveModelInfo":       viper.GetBool("savemodelinfo"),
		"SaveVersionImages":   viper.GetBool("saveversionimages"),
		"SaveModelImages":     viper.GetBool("savemodelimages"),
		"SkipConfirmation":    viper.GetBool("skipconfirmation"), // Should be false here
		"ApiDelayMs":          viper.GetInt("apidelayms"),
		"ApiClientTimeoutSec": viper.GetInt("apiclienttimeoutsec"),
		// Other
		"LogApiRequests": viper.GetBool("logapirequests"),
	}
	globalConfigJSON, err := json.MarshalIndent(effectiveGlobalConfig, "", "  ")
	if err != nil {
		log.Errorf("Failed to marshal effectiveGlobalConfig to JSON: %v", err)
	} else {
		fmt.Println("\n--- Global Config Settings ---")
		fmt.Println(string(globalConfigJSON))
	}

	// Display Query Parameters (using the input struct)
	queryParamsJSON, err := json.MarshalIndent(queryParams, "", "  ")
	if err != nil {
		log.Errorf("Failed to marshal queryParams to JSON: %v", err)
	}
	fmt.Println("\n--- Query Parameters for API ---")
	fmt.Println(string(queryParamsJSON))

	// Confirmation Prompt
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nProceed with these settings? (y/N): ")
	input, _ := reader.ReadString('\n')
	input = strings.ToLower(strings.TrimSpace(input))

	if input != "y" {
		log.Info("Operation cancelled by user.")
		return false // User cancelled
	}

	log.Info("Configuration confirmed, proceeding with API calls...")
	return true // User confirmed
}

// executeDownloads manages the worker pool and queues download jobs.
func executeDownloads(downloadsToQueue []potentialDownload, db *database.DB, fileDownloader *downloader.Downloader, imageDownloader *downloader.Downloader, concurrencyLevel int, cfg *models.Config, bleveIndex bleve.Index) {
	log.Info("--- Starting Phase 3: Download Execution --- ")

	// Initialize uilive writer for progress updates
	writer := uilive.New()
	writer.Start()
	defer writer.Stop() // Ensure writer stops even if there are errors

	var wg sync.WaitGroup
	downloadJobs := make(chan downloadJob, concurrencyLevel) // Buffered channel

	// Start download workers
	log.Infof("Starting %d download workers...", concurrencyLevel)
	for i := 0; i < concurrencyLevel; i++ {
		wg.Add(1)
		// Pass necessary components to the worker
		// Pass imageDownloader, writer, concurrencyLevel, and bleveIndex
		go downloadWorker(i+1, downloadJobs, db, fileDownloader, imageDownloader, &wg, writer, concurrencyLevel, bleveIndex)
	}

	// Queue downloads
	queuedCount := 0
	failedToQueueCount := 0
	for _, pd := range downloadsToQueue {
		// --- Calculate DB Key and Check Preconditions ---
		// Ensure ModelVersion ID exists before calculating key and checking DB
		if pd.CleanedVersion.ID == 0 {
			log.Errorf("Cannot process download for %s (Model: %s) - CleanedVersion ID is missing! Skipping queue.", pd.File.Name, pd.ModelName)
			failedToQueueCount++
			continue
		}
		// Calculate key using version ID with prefix (as it was originally)
		dbKey := fmt.Sprintf("v_%d", pd.CleanedVersion.ID)

		// Check DB status before queueing (should be Pending)
		rawValue, errGet := db.Get([]byte(dbKey))
		if errGet != nil {
			log.Warnf("Failed to get DB entry %s before queueing download job for %s. Skipping queue.", dbKey, pd.FinalBaseFilename)
			failedToQueueCount++
			continue
		}
		var entry models.DatabaseEntry
		if errUnmarshal := json.Unmarshal(rawValue, &entry); errUnmarshal != nil {
			log.Warnf("Failed to unmarshal DB entry %s before queueing download job for %s. Skipping queue.", dbKey, pd.FinalBaseFilename)
			failedToQueueCount++
			continue
		}

		if entry.Status != models.StatusPending {
			log.Warnf("DB entry %s for %s is not in Pending state (Status: %s). Skipping queue.", dbKey, pd.FinalBaseFilename, entry.Status)
			failedToQueueCount++
			continue
		}

		// Add job to the channel
		job := downloadJob{
			PotentialDownload: pd,
			DatabaseKey:       dbKey,
		}
		downloadJobs <- job
		queuedCount++
	}

	close(downloadJobs) // Close channel once all jobs are sent
	log.Infof("Queued %d download jobs. Waiting for workers to finish... (%d jobs failed to queue)", queuedCount, failedToQueueCount)

	wg.Wait() // Wait for all workers to complete
	log.Info("--- Finished Phase 3: Download Execution --- ")
}

// runDownload is the main execution function for the download command.
func runDownload(cmd *cobra.Command, args []string) {
	initLogging() // Ensures logging is set up based on flags FIRST

	// --- Explicitly check changed flags and set Viper --- START ---
	// Ensure command-line flags take precedence in Viper before confirmation display
	if cmd.Flags().Changed("concurrency") {
		concurrencyVal, _ := cmd.Flags().GetInt("concurrency")
		viper.Set("concurrency", concurrencyVal)
		log.Debugf("Explicitly set viper concurrency from flag: %d", concurrencyVal)
	}
	// TODO: Add similar checks for other flags if needed, although BindPFlag should ideally handle this.
	// --- Explicitly check changed flags and set Viper --- END ---

	// --- Early Exit Flags Check --- START ---
	// Check for flags that should cause an early exit BEFORE initializing the environment

	// Handle --show-config
	if showConfigFlag, _ := cmd.Flags().GetBool("show-config"); showConfigFlag {
		log.Info("--- Effective Configuration (--show-config) ---")

		// Get API parameters using Viper directly
		queryParams := setupQueryParams(&globalConfig, cmd) // Pass globalConfig only for context if needed by setup

		// Build effective global config using Viper directly
		effectiveGlobalConfig := map[string]interface{}{
			// Paths (might still be empty if not set)
			"SavePath":       viper.GetString("savepath"),
			"DatabasePath":   viper.GetString("databasepath"),
			"BleveIndexPath": viper.GetString("bleveindexpath"),
			"ApiKey":         maskApiKey(viper.GetString("apikey")),
			// Filtering - Model/Version
			"DownloadAllVersions": viper.GetBool("downloadallversions"),
			"ModelVersionID":      viper.GetInt("modelversionid"),
			// Filtering - File Level
			"PrimaryOnly":           viper.GetBool("primaryonly"),
			"Pruned":                viper.GetBool("pruned"),
			"Fp16":                  viper.GetBool("fp16"),
			"IgnoreBaseModels":      viper.GetStringSlice("ignorebasemodels"),
			"IgnoreFileNameStrings": viper.GetStringSlice("ignorefilenamestrings"),
			// Downloader Behavior
			"Concurrency":         viper.GetInt("concurrency"),
			"SaveMetadata":        viper.GetBool("savemetadata"),
			"DownloadMetaOnly":    viper.GetBool("downloadmetaonly"),
			"SaveModelInfo":       viper.GetBool("savemodelinfo"),
			"SaveVersionImages":   viper.GetBool("saveversionimages"),
			"SaveModelImages":     viper.GetBool("savemodelimages"),
			"SkipConfirmation":    viper.GetBool("skipconfirmation"),
			"ApiDelayMs":          viper.GetInt("apidelayms"),
			"ApiClientTimeoutSec": viper.GetInt("apiclienttimeoutsec"),
			// Other
			"LogApiRequests": viper.GetBool("logapirequests"),
			// NOTE: Query, Tags, Usernames, ModelTypes, BaseModels, Nsfw, Sort, Period, Limit, MaxPages
			// are part of API params, not strictly global config shown here.
		}

		// Print effectiveGlobalConfig
		globalConfigJSON, err := json.MarshalIndent(effectiveGlobalConfig, "", "  ")
		if err != nil {
			log.Errorf("Failed to marshal effectiveGlobalConfig to JSON: %v", err)
		} else {
			fmt.Println("\n--- Global Config Settings ---")
			fmt.Println(string(globalConfigJSON))
		}

		// Print queryParams
		queryParamsJSON, err := json.MarshalIndent(queryParams, "", "  ")
		if err != nil {
			log.Errorf("Failed to marshal queryParams to JSON: %v", err)
		}
		fmt.Println("\n--- Query Parameters for API ---")
		fmt.Println(string(queryParamsJSON))

		log.Info("Exiting after showing configuration.")
		os.Exit(0) // Exit successfully
	}

	// Handle --debug-print-api-url
	if printUrl, _ := cmd.Flags().GetBool("debug-print-api-url"); printUrl {
		// Similar to --show-config, we need queryParams
		queryParams := setupQueryParams(&globalConfig, cmd)
		// Construct the URL parts using the new helper function
		apiURL := api.CivitaiApiBaseUrl + "/models" // Use constant from api package
		params := api.ConvertQueryParamsToURLValues(queryParams)
		fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())
		log.Infof("--- Debug API URL (--debug-print-api-url) ---")
		fmt.Println(fullURL) // Print only the URL to stdout
		log.Info("Exiting after printing API URL.")
		os.Exit(0) // Exit immediately
	}
	// --- Early Exit Flags Check --- END ---

	// ---> initLogging() is only called AFTER the early exit checks <--- <-- Move this up
	// initLogging()
	log.Info("Starting Civitai Downloader - Download Command")

	// --- Initialize Environment ---
	db, fileDownloader, imageDownloader, concurrencyLevel, err := setupDownloadEnvironment(cmd, &globalConfig)
	if err != nil {
		log.Fatalf("Failed to set up download environment: %v", err)
	}
	defer func() {
		log.Info("Closing database.")
		if err := db.Close(); err != nil {
			log.Errorf("Error closing database: %v", err)
		}
	}()
	// --- End Environment Initialization ---

	// --- Initialize Bleve Index --- START ---
	indexPath := globalConfig.BleveIndexPath
	if indexPath == "" {
		indexPath = filepath.Join(globalConfig.SavePath, "civitai.bleve") // Default if config is empty
		log.Warnf("BleveIndexPath not set in config, defaulting index path for model downloads to: %s", indexPath)
	} else {
		// Optionally, ensure it's an absolute path or resolve relative to working dir/config file?
		// For now, assume it's a valid path as provided.
	}
	log.Infof("Opening/Creating Bleve index at: %s", indexPath)
	bleveIndex, err := index.OpenOrCreateIndex(indexPath)
	if err != nil {
		log.Fatalf("Failed to open or create Bleve index: %v", err)
	}
	defer func() {
		log.Info("Closing Bleve index.")
		if err := bleveIndex.Close(); err != nil {
			log.Errorf("Error closing Bleve index: %v", err)
		}
	}()
	log.Info("Bleve index opened successfully.")
	// --- Initialize Bleve Index --- END ---

	// Pass address of globalConfig (needed by legacy parts, but Viper is preferred for new checks)
	// Also ensure queryParams uses Viper directly
	queryParams := setupQueryParams(&globalConfig, cmd) // setupQueryParams already uses Viper

	// --- Confirm Parameters Before API Calls --- START ---
	if !confirmParameters(queryParams) {
		// User cancelled during parameter confirmation
		return // Exit runDownload gracefully
	}
	// --- Confirm Parameters Before API Calls --- END ---

	// =============================================
	// Phase 1: Metadata Gathering & Filtering
	// =============================================

	// --- Setup Metadata HTTP Client ---
	// Get timeout from Viper (handles flag > config > default)
	timeoutSec := viper.GetInt("apiclienttimeoutsec")
	metadataTimeout := time.Duration(timeoutSec) * time.Second
	log.Debugf("Using API client timeout: %v", metadataTimeout)

	// Create the custom transport tuned for API calls
	metadataTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second, // Shorter timeout for API responses
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   5, // Fewer idle connections needed
	}

	// Wrap the transport for logging if enabled (similar to root.go)
	var finalMetadataTransport http.RoundTripper = metadataTransport
	if viper.GetBool("logapirequests") { // Check Viper directly
		log.Debug("API request logging enabled, wrapping metadata HTTP transport.")
		// Use the main api.log file for metadata calls as well
		logFilePath := "api.log"
		// --- Use viper.GetString to get the save path consistent with root.go ---
		savePath := viper.GetString("savepath")
		if savePath != "" {
			if _, statErr := os.Stat(savePath); statErr == nil {
				logFilePath = filepath.Join(savePath, logFilePath)
			} else {
				log.Warnf("SavePath '%s' (from Viper) not found, saving %s to current directory.", savePath, logFilePath)
			}
		}
		// --- End save path consistency change ---
		log.Infof("Metadata API logging will append to file: %s", logFilePath)
		// Need to import "go-civitai-download/internal/api"
		loggingMetaTransport, err := api.NewLoggingTransport(metadataTransport, logFilePath)
		if err != nil {
			log.WithError(err).Error("Failed to initialize API logging transport for metadata client, logging disabled for it.")
			// Keep finalMetadataTransport as metadataTransport
		} else {
			finalMetadataTransport = loggingMetaTransport // Use the wrapped transport
		}
	}

	// Create the metadata client using the (potentially wrapped) transport
	metadataClient := &http.Client{
		Timeout:   metadataTimeout,        // Set client-level timeout
		Transport: finalMetadataTransport, // Use the final transport
	}
	// --- End Setup Metadata HTTP Client ---

	modelVersionID := viper.GetInt("modelversionid") // Viper key from init()
	modelID := viper.GetInt("modelid")               // Viper key from init()

	var downloadsToQueue []potentialDownload // Holds downloads confirmed for queueing after DB check
	var loopErr error                        // Store loop errors

	if modelVersionID > 0 {
		log.Infof("--- Processing specific Model Version ID: %d (Model ID flag ignored) ---", modelVersionID)
		// Use the metadataClient initialized above
		downloadsToQueue, _, loopErr = handleSingleVersionDownload(modelVersionID, db, metadataClient, &globalConfig, cmd)

		if loopErr != nil {
			log.Errorf("Failed to process single model version %d: %v", modelVersionID, loopErr)
			return // Exit if single version fetch/process failed
		}
		log.Info("--- Finished processing single model version ---")
	} else if modelID > 0 { // Check for model ID *after* version ID
		log.Infof("--- Processing specific Model ID: %d ---", modelID)
		// Call a new function similar to handleSingleVersionDownload but for a model ID
		// Pass the imageDownloader instance now
		downloadsToQueue, _, loopErr = handleSingleModelDownload(modelID, db, metadataClient, imageDownloader, &globalConfig, cmd)

		if loopErr != nil {
			log.Errorf("Failed to process single model %d: %v", modelID, loopErr)
			return // Exit if single model fetch/process failed
		}
		log.Info("--- Finished processing single model ID ---")
	} else {
		// ================== DEBUG LOG ==================
		// log.Debugf("DEBUG: QueryParams Username before fetch: '%s'", queryParams.Username) // REDUNDANT - REMOVED
		// ==============================================

		// --- Existing Pagination Logic ---
		log.Info("--- Starting Phase 1: Metadata Gathering & DB Check --- (Pagination)")
		downloadsToQueue, _, loopErr = fetchModelsPaginated(db, metadataClient, imageDownloader, queryParams, &globalConfig, cmd)

		if loopErr != nil {
			log.Errorf("Metadata gathering phase finished with error: %v", loopErr)
			log.Error("Aborting due to error during metadata gathering.")
			return
		}
		log.Info("--- Finished Phase 1: Metadata Gathering & DB Check ---")
	}

	// =============================================
	// Phase 1.5: Handle Metadata-Only Mode
	// =============================================
	// Use viper to check meta-only flag
	if viper.GetBool("downloadmetaonly") { // Viper key from init()
		if handleMetadataOnlyMode(downloadsToQueue, &globalConfig) {
			return // Exit if the handler function indicates we should.
		}
	}

	// =============================================
	// Phase 2: Summary & Confirmation
	// =============================================
	// Confirmation logic moved to confirmDownload function
	if !confirmDownload(downloadsToQueue) {
		return // Exit if user cancels
	}

	// =============================================
	// Phase 3: Download Execution
	// =============================================
	// Call the function to execute downloads, passing the index
	executeDownloads(downloadsToQueue, db, fileDownloader, imageDownloader, concurrencyLevel, &globalConfig, bleveIndex)

	// =============================================
	// Phase 4: Final Summary
	// =============================================
	log.Info("Download process complete.")
}
