package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"go-civitai-download/internal/database"
	"go-civitai-download/internal/downloader"
	"go-civitai-download/internal/helpers"
	"go-civitai-download/internal/models"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Package-level variables for db verify flags
var (
	DbVerifyCheckHashFlag bool
	DbVerifyYesFlag       bool
)

// dbCmd represents the base command for database operations
var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Interact with the download database",
	Long:  `Perform operations like viewing, verifying, or managing entries in the download database.`,
	// No Run function for the base db command itself
}

// dbViewCmd represents the command to view database entries
var dbViewCmd = &cobra.Command{
	Use:   "view",
	Short: "View entries stored in the database",
	Long:  `Lists the models and files that have been recorded in the database.`,
	Run:   runDbView,
}

// dbVerifyCmd represents the command to verify database entries against the filesystem
var dbVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify database entries against the filesystem and optionally prompt for redownload",
	Long: `Checks if the files listed in the database exist at their expected locations,
optionally verifies their hashes, and prompts to redownload missing or mismatched files.`,
	Run: runDbVerify,
}

// dbRedownloadCmd represents the command to redownload a file based on its DB key
var dbRedownloadCmd = &cobra.Command{
	Use:   "redownload [MODEL_VERSION_ID]",
	Short: "Redownload a file stored in the database based on Model Version ID",
	Long: `Attempts to redownload a specific file using the information stored
in the database entry identified by the provided Model Version ID (used as the database key).`,
	Args: cobra.ExactArgs(1), // Requires exactly one argument (the version ID)
	Run:  runDbRedownload,
}

// dbSearchCmd represents the command to search database entries by model name
var dbSearchCmd = &cobra.Command{
	Use:   "search [MODEL_NAME_QUERY]",
	Short: "Search database entries by model name",
	Long: `Searches database entries for models whose names contain the provided query text (case-insensitive).
Prints matching entries.`,
	Args: cobra.ExactArgs(1), // Requires exactly one argument
	Run:  runDbSearch,
}

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbViewCmd)
	dbCmd.AddCommand(dbVerifyCmd)
	dbCmd.AddCommand(dbRedownloadCmd) // Add the redownload command
	dbCmd.AddCommand(dbSearchCmd)     // Add the search command

	// Add flags specific to db view if needed (e.g., filtering)
	// dbViewCmd.Flags().StringP("filter", "f", "", "Filter results (e.g., by model name)")

	// Add flags specific to db verify
	// These flags will be used by config.Initialize to populate globalConfig.DB.Verify
	dbVerifyCmd.Flags().BoolVar(&DbVerifyCheckHashFlag, "check-hash", true, "Perform hash check for existing files")
	dbVerifyCmd.Flags().BoolVarP(&DbVerifyYesFlag, "yes", "y", false, "Automatically attempt to redownload missing/mismatched files without prompting")

	// Add flags specific to db redownload if needed (e.g., force overwrite without hash check?)
	// dbRedownloadCmd.Flags().Bool("force", false, "Force redownload even if file exists and hash matches")
}

func runDbView(cmd *cobra.Command, args []string) {
	log.Info("Viewing database entries...")

	// Use globalConfig loaded by PersistentPreRunE
	if globalConfig.DatabasePath == "" {
		log.Fatal("Database path is not set in the configuration. Please check config file or path.")
	}

	// Open Database using globalConfig
	db, err := database.Open(globalConfig.DatabasePath)
	if err != nil {
		log.WithError(err).Fatalf("Failed to open database at %s", globalConfig.DatabasePath)
	}
	defer db.Close()

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0) // Adjust padding and alignment
	fmt.Fprintln(tw, "Model Name\tVersion Name\tFilename\tFolder\tType\tBase Model\tCreator\tStatus\tDB Key (VersionID)")
	fmt.Fprintln(tw, "----------\t------------\t--------\t------\t----\t----------\t-------\t------\t------------------")

	count := 0
	// Use Fold to iterate over key-value pairs
	errFold := db.Fold(func(key []byte, value []byte) error {
		keyStr := string(key)
		// Skip internal keys like page state
		if !strings.HasPrefix(keyStr, "v_") { // Only process keys starting with "v_"
			return nil
		}

		// Value is already provided by Fold, no need for db.Get
		var entry models.DatabaseEntry
		err := json.Unmarshal(value, &entry)
		if err != nil {
			log.WithError(err).Warnf("Failed to unmarshal JSON for key %s: %s", keyStr, string(value))
			return nil // Continue folding over other keys
		}

		// Print table row using the added fields, including Status
		// Extract version ID from key for display
		versionIDStr := strings.TrimPrefix(keyStr, "v_")
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			entry.ModelName, // Use added ModelName
			entry.Version.Name,
			entry.Filename,
			entry.Folder,
			entry.ModelType, // Use added ModelType
			entry.Version.BaseModel,
			entry.Creator.Username, // Print the username from the Creator struct
			entry.Status,           // Added Status field
			versionIDStr,           // Display the version ID
		)
		count++
		return nil
	})

	if errFold != nil {
		log.WithError(errFold).Error("Error occurred during database scan (Fold)")
	}

	if err := tw.Flush(); err != nil {
		log.WithError(err).Error("Error flushing table writer for db view")
	}
	log.Infof("Displayed %d entries.", count)
}

type verificationProblem struct {
	Reason string
	DbKey  string
	Entry  models.DatabaseEntry
}

func runDbVerify(cmd *cobra.Command, args []string) {
	log.Info("Verifying database entries against filesystem...")

	// Validate configuration and open database
	db, err := initializeVerificationDatabase()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Scan database and verify files
	stats, problemsToAddress := scanDatabaseEntries(db)
	logInitialScanSummary(stats)

	// Handle redownloads if problems found
	if len(problemsToAddress) > 0 {
		handleRedownloads(db, problemsToAddress, stats)
	} else {
		log.Info("No missing or mismatched files found requiring redownload.")
	}

	log.Info("Verification process completed.")
}

// VerificationStats holds statistics from the verification scan
type VerificationStats struct {
	TotalEntries      int
	FoundOk           int
	FoundHashMismatch int
	Missing           int
}

// initializeVerificationDatabase validates config and opens the database
func initializeVerificationDatabase() (*database.DB, error) {
	if globalConfig.DatabasePath == "" {
		return nil, fmt.Errorf("database path is not set in the configuration. Please check config file or path")
	}

	if globalConfig.SavePath == "" {
		if globalConfig.DatabasePath != "" {
			globalConfig.SavePath = filepath.Dir(globalConfig.DatabasePath)
			log.Warnf("SavePath is empty, inferring base directory from DatabasePath: %s", globalConfig.SavePath)
		} else {
			return nil, fmt.Errorf("save path is not set (and cannot be inferred from DatabasePath). Please check config file or path")
		}
	}

	db, err := database.Open(globalConfig.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", globalConfig.DatabasePath, err)
	}

	return db, nil
}

// scanDatabaseEntries scans all database entries and verifies files
func scanDatabaseEntries(db *database.DB) (VerificationStats, []verificationProblem) {
	var stats VerificationStats
	var problemsToAddress []verificationProblem

	log.Info("Scanning database entries...")

	errFold := db.Fold(func(key []byte, value []byte) error {
		keyStr := string(key)
		if !strings.HasPrefix(keyStr, "v_") {
			return nil // Skip non-version keys
		}

		stats.TotalEntries++

		var entry models.DatabaseEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			log.WithError(err).Warnf("Failed to unmarshal JSON for key %s, skipping verification for this entry.", keyStr)
			return nil // Continue folding
		}

		expectedPath := filepath.Join(globalConfig.SavePath, entry.Folder, entry.Filename)
		mainFileFound, hashOK, problemReason := verifyMainFile(expectedPath, entry)

		updateVerificationStats(&stats, mainFileFound, hashOK, problemReason)

		if problemReason != "" {
			problemsToAddress = append(problemsToAddress, verificationProblem{
				Entry:  entry,
				Reason: problemReason,
				DbKey:  keyStr,
			})
		}

		// Handle metadata files if main file is OK
		if mainFileFound && hashOK {
			handleMetadataVerification(expectedPath, entry)
		}

		return nil // Continue folding
	})

	if errFold != nil {
		log.WithError(errFold).Error("Error occurred during database scan (Fold)")
	}

	return stats, problemsToAddress
}

// verifyMainFile checks if the main model file exists and has correct hash
func verifyMainFile(expectedPath string, entry models.DatabaseEntry) (bool, bool, string) {
	checkHashFlag := globalConfig.DB.Verify.CheckHash

	_, statErr := os.Stat(expectedPath)
	if statErr == nil {
		// File exists
		if checkHashFlag {
			if helpers.CheckHash(expectedPath, entry.File.Hashes) {
				log.WithFields(log.Fields{"path": expectedPath, "status": entry.Status}).Info("[OK] File exists and hash matches.")
				return true, true, ""
			} else {
				log.WithFields(log.Fields{"path": expectedPath, "status": entry.Status}).Warn("[MISMATCH] File exists but hash mismatch.")
				return true, false, "Hash Mismatch"
			}
		} else {
			log.WithFields(log.Fields{"path": expectedPath, "status": entry.Status}).Info("[FOUND] File exists (hash check skipped).")
			return true, true, ""
		}
	} else if os.IsNotExist(statErr) {
		log.WithFields(log.Fields{"path": expectedPath, "status": entry.Status}).Error("[MISSING] File not found.")
		return false, false, "Missing"
	} else {
		log.WithError(statErr).Errorf("[ERROR] Could not check file status for %s", expectedPath)
		return false, false, ""
	}
}

// updateVerificationStats updates the verification statistics
func updateVerificationStats(stats *VerificationStats, mainFileFound, hashOK bool, problemReason string) {
	if mainFileFound && hashOK {
		stats.FoundOk++
	} else if mainFileFound && !hashOK {
		stats.FoundHashMismatch++
	} else if problemReason == "Missing" {
		stats.Missing++
	}
}

// handleMetadataVerification handles verification and creation of metadata files
func handleMetadataVerification(expectedPath string, entry models.DatabaseEntry) {
	if !globalConfig.Download.SaveMetadata {
		return
	}

	metaFilename := strings.TrimSuffix(entry.Filename, filepath.Ext(entry.Filename)) + ".json"
	metaFilepath := filepath.Join(globalConfig.SavePath, entry.Folder, metaFilename)

	if _, metaStatErr := os.Stat(metaFilepath); metaStatErr != nil {
		if os.IsNotExist(metaStatErr) {
			createMetadataFile(metaFilepath, entry.Version)
		} else {
			log.WithError(metaStatErr).Errorf("[METADATA ERROR] Could not check metadata file status for %s", metaFilepath)
		}
	} else {
		log.WithField("path", metaFilepath).Info("[METADATA OK] Metadata file exists.")
	}
}

// createMetadataFile creates a metadata file for a model version
func createMetadataFile(metaFilepath string, version models.ModelVersion) {
	log.WithField("path", metaFilepath).Warn("[METADATA MISSING] Creating metadata file...")

	jsonData, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		log.WithError(err).Errorf("Failed to marshal metadata for %s", filepath.Base(metaFilepath))
		return
	}

	metaDir := filepath.Dir(metaFilepath)
	if err := os.MkdirAll(metaDir, 0700); err != nil {
		log.WithError(err).Errorf("Failed to create directory for metadata file %s", metaFilepath)
		return
	}

	if err := os.WriteFile(metaFilepath, jsonData, 0600); err != nil {
		log.WithError(err).Errorf("Failed to write metadata file %s", metaFilepath)
		return
	}

	log.WithField("path", metaFilepath).Info("[METADATA CREATED] Successfully wrote metadata file.")
}

// logInitialScanSummary logs the summary of the initial scan
func logInitialScanSummary(stats VerificationStats) {
	log.Infof("Initial Scan Summary: Total Entries=%d, OK=%d, Missing=%d, Mismatch=%d",
		stats.TotalEntries, stats.FoundOk, stats.Missing, stats.FoundHashMismatch)
}

// handleRedownloads processes files that need to be redownloaded
func handleRedownloads(db *database.DB, problemsToAddress []verificationProblem, stats VerificationStats) {
	autoRedownloadFlag := globalConfig.DB.Verify.AutoRedownload

	log.Infof("Found %d file(s) that are missing or have hash mismatches.", len(problemsToAddress))

	var fileDownloader *downloader.Downloader
	var reader *bufio.Reader

	if !autoRedownloadFlag {
		reader = bufio.NewReader(os.Stdin)
	}

	redownloadStats := processRedownloadRequests(db, problemsToAddress, &fileDownloader, reader, autoRedownloadFlag)
	logRedownloadSummary(redownloadStats)
}

// RedownloadStats holds statistics for redownload operations
type RedownloadStats struct {
	Attempts int
	Success  int
	Fail     int
}

// processRedownloadRequests processes each redownload request
func processRedownloadRequests(db *database.DB, problems []verificationProblem, fileDownloader **downloader.Downloader, reader *bufio.Reader, autoRedownload bool) RedownloadStats {
	var stats RedownloadStats

	for _, problem := range problems {
		if shouldRedownload(problem, reader, autoRedownload) {
			stats.Attempts++

			if *fileDownloader == nil {
				*fileDownloader = initializeDownloader()
				if *fileDownloader == nil {
					stats.Fail++
					continue
				}
			}

			success := performRedownload(db, problem, *fileDownloader)
			if success {
				stats.Success++
			} else {
				stats.Fail++
			}
		} else {
			log.Infof("Skipping redownload for %s (%s).", problem.Entry.Filename, problem.Entry.Folder)
		}
	}

	return stats
}

// shouldRedownload determines if a file should be redownloaded
func shouldRedownload(problem verificationProblem, reader *bufio.Reader, autoRedownload bool) bool {
	if autoRedownload {
		log.Infof("Auto-attempting redownload for %s (%s) due to --yes flag.", problem.Entry.Filename, problem.Entry.Folder)
		return true
	}

	prompt := fmt.Sprintf("File '%s' (%s) - %s. Redownload? (y/N): ", problem.Entry.Filename, problem.Entry.Folder, problem.Reason)
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(input)) == "y"
}

// initializeDownloader initializes the file downloader
func initializeDownloader() *downloader.Downloader {
	log.Debug("Initializing downloader for redownload...")

	if globalHttpTransport == nil {
		log.Error("Global HTTP transport not initialized. Cannot perform redownload.")
		return nil
	}

	httpClient := &http.Client{
		Timeout:   0,
		Transport: globalHttpTransport,
	}

	log.Debug("Downloader initialized.")
	return downloader.NewDownloader(httpClient, globalConfig.APIKey, globalConfig.SessionCookie)
}

// performRedownload performs the actual redownload of a file
func performRedownload(db *database.DB, problem verificationProblem, fileDownloader *downloader.Downloader) bool {
	entry := problem.Entry
	targetPath := filepath.Join(globalConfig.SavePath, entry.Folder, entry.Filename)

	log.Infof("Attempting redownload: %s -> %s", entry.File.DownloadUrl, targetPath)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
		log.WithError(err).Errorf("Failed to create directory for redownload: %s", filepath.Dir(targetPath))
		updateDbEntryError(db, problem.DbKey, fmt.Sprintf("Mkdir failed: %v", err))
		return false
	}

	finalPath, downloadErr := fileDownloader.DownloadFile(targetPath, entry.File.DownloadUrl, entry.File.Hashes, entry.Version.ID)

	finalStatus := models.StatusError
	if downloadErr == nil {
		finalStatus = models.StatusDownloaded
		log.Infof("Redownload successful: %s", finalPath)
	} else {
		log.WithError(downloadErr).Errorf("Redownload failed for: %s", targetPath)
	}

	updateDbEntryAfterRedownload(db, problem.DbKey, finalStatus, finalPath, downloadErr)
	return downloadErr == nil
}

// updateDbEntryError updates database entry with error status
func updateDbEntryError(db *database.DB, dbKey, errorMsg string) {
	updateErr := updateDbEntry(db, dbKey, models.StatusError, func(e *models.DatabaseEntry) {
		e.ErrorDetails = errorMsg
	})
	if updateErr != nil {
		log.WithError(updateErr).Errorf("Failed to update DB status to Error for %s", dbKey)
	}
}

// updateDbEntryAfterRedownload updates database entry after redownload attempt
func updateDbEntryAfterRedownload(db *database.DB, dbKey, finalStatus, finalPath string, downloadErr error) {
	updateErr := updateDbEntry(db, dbKey, finalStatus, func(e *models.DatabaseEntry) {
		if downloadErr != nil {
			e.ErrorDetails = downloadErr.Error()
		} else {
			e.ErrorDetails = ""
			e.Filename = filepath.Base(finalPath)
		}
	})
	if updateErr != nil {
		log.Errorf("Failed to update DB status after redownload attempt for %s: %v", dbKey, updateErr)
	}
}

// logRedownloadSummary logs the summary of redownload operations
func logRedownloadSummary(stats RedownloadStats) {
	if stats.Attempts > 0 {
		log.Infof("Redownload Phase Summary: Attempts=%d, Success=%d, Failed=%d",
			stats.Attempts, stats.Success, stats.Fail)
	}
}

func runDbRedownload(cmd *cobra.Command, args []string) {
	versionIDStr := args[0]
	log.Infof("Attempting to redownload file with Model Version ID: %s", versionIDStr)

	// Use globalConfig loaded by PersistentPreRunE
	if globalConfig.DatabasePath == "" {
		log.Fatal("Database path is not set in the configuration. Please check config file or path.")
	}
	if globalConfig.SavePath == "" {
		log.Fatal("Save path is not set in the configuration. Please check config file or path.")
	}

	// Open Database using globalConfig
	db, err := database.Open(globalConfig.DatabasePath)
	if err != nil {
		log.WithError(err).Fatalf("Failed to open database at %s", globalConfig.DatabasePath)
	}
	defer db.Close()

	// Construct the database key
	dbKey := fmt.Sprintf("v_%s", versionIDStr)

	// Get the database entry
	value, err := db.Get([]byte(dbKey))
	if errors.Is(err, database.ErrNotFound) {
		log.Fatalf("No database entry found for Model Version ID %s (Key: %s)", versionIDStr, dbKey)
	} else if err != nil {
		log.WithError(err).Fatalf("Failed to retrieve database entry for key %s", dbKey)
	}

	var entry models.DatabaseEntry
	err = json.Unmarshal(value, &entry)
	if err != nil {
		log.WithError(err).Fatalf("Failed to unmarshal database entry for key %s", dbKey)
	}

	// Reconstruct the expected full path using globalConfig
	expectedPath := filepath.Join(globalConfig.SavePath, entry.Folder, entry.Filename)
	log.Infof("Target path for redownload: %s", expectedPath)
	log.Infof("Download URL from DB: %s", entry.File.DownloadUrl)

	// Ensure target directory exists
	if !helpers.CheckAndMakeDir(filepath.Dir(expectedPath)) {
		log.Fatalf("Failed to ensure directory exists: %s", filepath.Dir(expectedPath))
	}

	// Initialize downloader using the helper function from download.go
	log.Debug("Initializing HTTP client for redownload...")
	// createDownloaderClient is in download.go, cannot be called directly here.
	// Create a new client instance for this command.
	// TODO: Refactor client creation/sharing?
	downloaderHttpClient := &http.Client{Timeout: 30 * time.Minute} // Longer timeout for downloads
	// Use correct case for APIKey
	fileDownloader := downloader.NewDownloader(downloaderHttpClient, globalConfig.APIKey, globalConfig.SessionCookie)

	// Perform the download, checking the error
	// Pass the Model Version ID from the database entry
	finalPath, err := fileDownloader.DownloadFile(expectedPath, entry.File.DownloadUrl, entry.File.Hashes, entry.Version.ID)

	if err == nil {
		log.Infof("Successfully redownloaded and verified: %s", finalPath)
	} else {
		// Log specific errors
		logEntry := log.WithFields(log.Fields{
			"key": dbKey,
			"url": entry.File.DownloadUrl,
		})
		if errors.Is(err, downloader.ErrHashMismatch) {
			logEntry.WithError(err).Error("Redownload failed: Hash mismatch after download.")
		} else if errors.Is(err, downloader.ErrHttpStatus) {
			logEntry.WithError(err).Error("Redownload failed: Unexpected HTTP status.")
		} else if errors.Is(err, downloader.ErrFileSystem) {
			logEntry.WithError(err).Error("Redownload failed: Filesystem error.")
		} else if errors.Is(err, downloader.ErrHttpRequest) {
			logEntry.WithError(err).Error("Redownload failed: HTTP request error.")
		} else {
			logEntry.WithError(err).Errorf("Redownload failed for an unknown reason.")
		}
		// Consider exiting with non-zero status code on failure
		os.Exit(1)
	}
}

func runDbSearch(cmd *cobra.Command, args []string) {
	searchTerm := strings.ToLower(args[0]) // Case-insensitive search
	log.Infof("Searching database entries for model name containing: '%s'", searchTerm)

	// Use globalConfig loaded by PersistentPreRunE
	if globalConfig.DatabasePath == "" {
		log.Fatal("Database path is not set in the configuration.")
	}

	db, err := database.Open(globalConfig.DatabasePath)
	if err != nil {
		log.WithError(err).Fatalf("Failed to open database at %s", globalConfig.DatabasePath)
	}
	defer db.Close()

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Model Name\tVersion Name\tFilename\tFolder\tType\tBase Model\tCreator\tStatus\tDB Key (VersionID)")
	fmt.Fprintln(tw, "----------\t------------\t--------\t------\t----\t----------\t-------\t------\t------------------")

	matchCount := 0
	errFold := db.Fold(func(key []byte, value []byte) error {
		keyStr := string(key)
		// Skip non-version keys
		if !strings.HasPrefix(keyStr, "v_") {
			return nil
		}

		var entry models.DatabaseEntry
		err := json.Unmarshal(value, &entry)
		if err != nil {
			log.WithError(err).Warnf("Failed to unmarshal JSON for key %s, skipping search check.", keyStr)
			return nil
		}

		// Perform case-insensitive substring search
		if strings.Contains(strings.ToLower(entry.ModelName), searchTerm) {
			matchCount++
			// Extract version ID from key for display
			versionIDStr := strings.TrimPrefix(keyStr, "v_")
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				entry.ModelName,
				entry.Version.Name,
				entry.Filename,
				entry.Folder,
				entry.ModelType,
				entry.Version.BaseModel,
				entry.Creator.Username,
				entry.Status, // Added Status field
				versionIDStr, // Display the version ID
			)
		}
		return nil
	})

	if errFold != nil {
		log.WithError(errFold).Error("Error occurred during database scan (Fold)")
	}

	if err := tw.Flush(); err != nil {
		log.WithError(err).Error("Error flushing table writer for db search")
	}
	log.Infof("Found %d matching entries for query '%s'.", matchCount, searchTerm)
}
