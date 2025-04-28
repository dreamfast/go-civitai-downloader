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
	dbVerifyCheckHashFlag bool
	dbVerifyYesFlag       bool
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
	// Link to package-level variables
	dbVerifyCmd.Flags().BoolVar(&dbVerifyCheckHashFlag, "check-hash", true, "Perform hash check for existing files")
	dbVerifyCmd.Flags().BoolVarP(&dbVerifyYesFlag, "yes", "y", false, "Automatically attempt to redownload missing/mismatched files without prompting")

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
	Entry  models.DatabaseEntry
	Reason string // e.g., "Missing", "Hash Mismatch"
	DbKey  string
}

func runDbVerify(cmd *cobra.Command, args []string) {
	log.Info("Verifying database entries against filesystem...")
	// Read flags directly from globalConfig
	checkHashFlag := globalConfig.DB.Verify.CheckHash           // Use config struct
	autoRedownloadFlag := globalConfig.DB.Verify.AutoRedownload // Use config struct

	// --- Basic Config Checks ---
	if globalConfig.DatabasePath == "" {
		log.Fatal("Database path is not set in the configuration. Please check config file or path.")
	}
	if globalConfig.SavePath == "" {
		// Try to infer from DB path if possible (mirroring clean command logic)
		if globalConfig.DatabasePath != "" {
			globalConfig.SavePath = filepath.Dir(globalConfig.DatabasePath)
			log.Warnf("SavePath is empty, inferring base directory from DatabasePath: %s", globalConfig.SavePath)
		} else {
			log.Fatal("Save path is not set (and cannot be inferred from DatabasePath). Please check config file or path.")
		}
	}

	// --- Open Database --- (moved up)
	db, err := database.Open(globalConfig.DatabasePath)
	if err != nil {
		log.WithError(err).Fatalf("Failed to open database at %s", globalConfig.DatabasePath)
	}
	defer db.Close()

	var totalEntries, foundOk, foundHashMismatch, missing int
	var problemsToAddress []verificationProblem // List to store entries needing attention

	log.Info("Scanning database entries...")
	// Use Fold for potentially better efficiency than Keys()
	errFold := db.Fold(func(key []byte, value []byte) error {
		keyStr := string(key)
		if !strings.HasPrefix(keyStr, "v_") {
			return nil // Skip non-version keys
		}

		totalEntries++

		var entry models.DatabaseEntry
		err := json.Unmarshal(value, &entry)
		if err != nil {
			log.WithError(err).Warnf("Failed to unmarshal JSON for key %s, skipping verification for this entry.", keyStr)
			return nil // Continue folding
		}

		// Construct the expected full path using globalConfig and entry data
		// Ensure the path uses the stored Filename and Folder
		expectedPath := filepath.Join(globalConfig.SavePath, entry.Folder, entry.Filename)

		// --- Check Main Model File --- (Simplified logic)
		mainFileFound := false
		hashOK := false
		problemReason := ""

		_, statErr := os.Stat(expectedPath)
		if statErr == nil {
			// File exists
			mainFileFound = true
			if checkHashFlag {
				if helpers.CheckHash(expectedPath, entry.File.Hashes) {
					hashOK = true
					foundOk++
					log.WithFields(log.Fields{"path": expectedPath, "status": entry.Status}).Info("[OK] File exists and hash matches.")
				} else {
					foundHashMismatch++
					problemReason = "Hash Mismatch"
					log.WithFields(log.Fields{"path": expectedPath, "status": entry.Status}).Warn("[MISMATCH] File exists but hash mismatch.")
				}
			} else {
				hashOK = true // Assume OK if not checking hash
				foundOk++
				log.WithFields(log.Fields{"path": expectedPath, "status": entry.Status}).Info("[FOUND] File exists (hash check skipped).")
			}
		} else if os.IsNotExist(statErr) {
			// File does not exist
			missing++
			problemReason = "Missing"
			log.WithFields(log.Fields{"path": expectedPath, "status": entry.Status}).Error("[MISSING] File not found.")
		} else {
			// Other error stating the file
			log.WithError(statErr).Errorf("[ERROR] Could not check file status for %s", expectedPath)
			// Optionally treat this as a problem? For now, just log error.
		}
		// --- End Check Main Model File ---

		// --- Add to problems list if missing or mismatch --- (moved down)
		if problemReason != "" {
			problemsToAddress = append(problemsToAddress, verificationProblem{
				Entry:  entry,
				Reason: problemReason,
				DbKey:  keyStr,
			})
		}

		// --- Check/Create Metadata File if Enabled --- (moved down, only if main file is OK)
		if mainFileFound && hashOK && globalConfig.Download.SaveMetadata {
			// Construct metadata filepath based on expectedPath (which already has the final filename)
			metaFilename := strings.TrimSuffix(entry.Filename, filepath.Ext(entry.Filename)) + ".json"
			metaFilepath := filepath.Join(globalConfig.SavePath, entry.Folder, metaFilename)

			if _, metaStatErr := os.Stat(metaFilepath); metaStatErr != nil {
				if os.IsNotExist(metaStatErr) {
					// Metadata file missing, attempt to create it
					log.WithField("path", metaFilepath).Warn("[METADATA MISSING] Creating metadata file...")
					// Marshal the version info stored in the entry
					versionToSave := entry.Version // Use the version stored in the DB entry
					jsonData, jsonErr := json.MarshalIndent(versionToSave, "", "  ")
					if jsonErr != nil {
						log.WithError(jsonErr).Errorf("Failed to marshal metadata for %s", metaFilename)
					} else {
						// Ensure directory exists BEFORE writing
						metaDir := filepath.Dir(metaFilepath)
						if mkdirErr := os.MkdirAll(metaDir, 0700); mkdirErr != nil {
							log.WithError(mkdirErr).Errorf("Failed to create directory for metadata file %s", metaFilepath)
						} else if writeErr := os.WriteFile(metaFilepath, jsonData, 0600); writeErr != nil {
							log.WithError(writeErr).Errorf("Failed to write metadata file %s", metaFilepath)
						} else {
							log.WithField("path", metaFilepath).Info("[METADATA CREATED] Successfully wrote metadata file.")
						}
					}
				} else {
					// Other error stating the metadata file
					log.WithError(metaStatErr).Errorf("[METADATA ERROR] Could not check metadata file status for %s", metaFilepath)
				}
			} else {
				// Metadata file exists
				log.WithField("path", metaFilepath).Info("[METADATA OK] Metadata file exists.")
			}
		} else if globalConfig.Download.SaveMetadata && (!mainFileFound || !hashOK) {
			// Log skipping metadata check because main file is missing or hash mismatch
			metaFilename := strings.TrimSuffix(entry.Filename, filepath.Ext(entry.Filename)) + ".json"
			metaFilepath := filepath.Join(globalConfig.SavePath, entry.Folder, metaFilename)
			log.WithField("path", metaFilepath).Debug("[METADATA SKIP] Skipping metadata check/creation because main file is missing or has hash mismatch.")
		}
		// --- End Check/Create Metadata File ---

		return nil // Continue folding
	})

	if errFold != nil {
		log.WithError(errFold).Error("Error occurred during database scan (Fold)")
	}

	log.Infof("Initial Scan Summary: Total Entries=%d, OK=%d, Missing=%d, Mismatch=%d",
		totalEntries, foundOk, missing, foundHashMismatch)

	// --- Prompt for Redownloads --- (New Section)
	if len(problemsToAddress) > 0 {
		log.Infof("Found %d file(s) that are missing or have hash mismatches.", len(problemsToAddress))

		// --- Initialize Downloader (Lazy) ---
		var fileDownloader *downloader.Downloader
		var reader *bufio.Reader

		if !autoRedownloadFlag {
			reader = bufio.NewReader(os.Stdin)
		}

		var redownloadAttempts, redownloadSuccess, redownloadFail int

		for _, problem := range problemsToAddress {
			entry := problem.Entry
			prompt := fmt.Sprintf("File '%s' (%s) - %s. Redownload? (y/N): ", entry.Filename, entry.Folder, problem.Reason)
			confirm := false

			if autoRedownloadFlag {
				log.Infof("Auto-attempting redownload for %s (%s) due to --yes flag.", entry.Filename, entry.Folder)
				confirm = true
			} else {
				fmt.Print(prompt)
				input, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(input)) == "y" {
					confirm = true
				}
			}

			if confirm {
				redownloadAttempts++

				// Initialize downloader only if needed
				if fileDownloader == nil {
					log.Debug("Initializing downloader for redownload...")
					// Ensure globalHttpTransport is available (should be from root PersistentPreRunE)
					if globalHttpTransport == nil {
						log.Error("Global HTTP transport not initialized. Cannot perform redownload.")
						// Maybe set a flag to stop further attempts?
						redownloadFail++ // Count as fail if transport is missing
						continue
					}
					// Create a client instance for the downloader using the global transport
					httpClient := &http.Client{
						Timeout:   0, // Rely on transport timeouts
						Transport: globalHttpTransport,
					}
					fileDownloader = downloader.NewDownloader(httpClient, globalConfig.APIKey)
					log.Debug("Downloader initialized.")
				}

				// --- Perform Redownload using existing logic ---
				targetPath := filepath.Join(globalConfig.SavePath, entry.Folder, entry.Filename)
				downloadUrl := entry.File.DownloadUrl
				hashes := entry.File.Hashes
				versionID := entry.Version.ID // Use the version ID from the entry
				dbKey := problem.DbKey

				log.Infof("Attempting redownload: %s -> %s", downloadUrl, targetPath)
				// Ensure directory exists (important for redownload)
				if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
					log.WithError(err).Errorf("Failed to create directory for redownload: %s", filepath.Dir(targetPath))
					updateErr := updateDbEntry(db, dbKey, models.StatusError, func(e *models.DatabaseEntry) {
						e.ErrorDetails = fmt.Sprintf("Mkdir failed: %v", err)
					})
					if updateErr != nil {
						// Also log error if updating DB status failed
						log.WithError(updateErr).Errorf("Failed to update DB status to Error after mkdir failure for %s", dbKey)
					}
					redownloadFail++
					continue // Next problem
				}

				finalPath, downloadErr := fileDownloader.DownloadFile(targetPath, downloadUrl, hashes, versionID)

				// --- Update DB and Handle Metadata ---
				finalStatus := models.StatusError
				if downloadErr == nil {
					finalStatus = models.StatusDownloaded
					log.Infof("Redownload successful: %s", finalPath)
					redownloadSuccess++
				} else {
					log.WithError(downloadErr).Errorf("Redownload failed for: %s", targetPath)
					redownloadFail++
				}

				updateErr := updateDbEntry(db, dbKey, finalStatus, func(e *models.DatabaseEntry) {
					if downloadErr != nil {
						e.ErrorDetails = downloadErr.Error()
					} else {
						e.ErrorDetails = ""                   // Clear error on success
						e.Filename = filepath.Base(finalPath) // Update filename if ID was prepended
						// Update File and Version structs? Maybe not necessary here unless they changed upstream?
					}
				})
				if updateErr != nil {
					log.Errorf("Failed to update DB status after redownload attempt for %s: %v", dbKey, updateErr)
				}

				// Handle metadata saving only on successful redownload
				if finalStatus == models.StatusDownloaded {
					log.Debugf("Redownload successful for %s. Checking if metadata saving is enabled...", finalPath)
					// REMOVE call to handleMetadataSaving - Verification should not save metadata.
					/*
						// We need a potentialDownload struct for handleMetadataSaving.
						// Construct a simplified one from the entry.
						pdForMeta := potentialDownload{
							CleanedVersion: entry.Version, // Use the version from the DB entry
						}
						// Call handleMetadataSaving (pass nil for writer as we are not using uilive here)
						handleMetadataSaving("VerifyRedownload", pdForMeta, finalPath, finalStatus, nil)
					*/
				}
			} else {
				log.Infof("Skipping redownload for %s (%s).", entry.Filename, entry.Folder)
			}
		} // End loop through problems

		// --- Final Redownload Summary ---
		if redownloadAttempts > 0 {
			log.Infof("Redownload Phase Summary: Attempts=%d, Success=%d, Failed=%d",
				redownloadAttempts, redownloadSuccess, redownloadFail)
		}

	} else {
		log.Info("No missing or mismatched files found requiring redownload.")
	}

	log.Info("Verification process completed.")
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
	fileDownloader := downloader.NewDownloader(downloaderHttpClient, globalConfig.APIKey)

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
