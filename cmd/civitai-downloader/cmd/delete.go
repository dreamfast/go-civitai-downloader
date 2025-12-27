package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"go-civitai-download/internal/database"
	"go-civitai-download/internal/models"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Package-level variables for delete flags
var (
	deleteModelIDs   []int
	deleteVersionIDs []int
	deleteUsername   string
	deleteSearch     string
	deleteForce      bool
	deleteDryRun     bool
	deleteKeepFiles  bool
)

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().IntSliceVarP(&deleteModelIDs, "model-id", "m", []int{}, "Model ID(s) to delete (all versions)")
	deleteCmd.Flags().IntSliceVarP(&deleteVersionIDs, "version-id", "v", []int{}, "Specific version ID(s) to delete")
	deleteCmd.Flags().StringVarP(&deleteUsername, "username", "u", "", "Delete all models from this creator")
	deleteCmd.Flags().StringVarP(&deleteSearch, "search", "s", "", "Search by model name and select entries to delete")
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation prompt")
	deleteCmd.Flags().BoolVarP(&deleteDryRun, "dry-run", "n", false, "Show what would be deleted without deleting")
	deleteCmd.Flags().BoolVar(&deleteKeepFiles, "keep-files", false, "Only remove database entries, keep files on disk")
}

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete downloaded models from database and disk",
	Long: `Delete downloaded models by model ID, version ID, username, or search query.

Examples:
  # Delete all versions of a model by model ID
  civitai-downloader delete --model-id 12345

  # Delete specific version(s) by version ID
  civitai-downloader delete --version-id 67890

  # Delete all models from a specific creator
  civitai-downloader delete --username "CreatorName"

  # Search and interactively select entries to delete
  civitai-downloader delete --search "anime style"

  # Skip confirmation prompt
  civitai-downloader delete --model-id 12345 --force

  # Preview what would be deleted (dry run)
  civitai-downloader delete --model-id 12345 --dry-run

  # Only remove from database, keep files on disk
  civitai-downloader delete --model-id 12345 --keep-files`,
	Run: runDelete,
}

func runDelete(cmd *cobra.Command, args []string) {
	cfg := globalConfig

	// Validate that at least one selection method is provided
	if len(deleteModelIDs) == 0 && len(deleteVersionIDs) == 0 && deleteUsername == "" && deleteSearch == "" {
		log.Error("At least one selection method is required: --model-id, --version-id, --username, or --search")
		_ = cmd.Usage()
		os.Exit(1)
	}

	// Validate paths
	if cfg.DatabasePath == "" {
		log.Fatal("Database path is not set in the configuration.")
	}

	savePath := cfg.SavePath
	if savePath == "" {
		savePath = filepath.Dir(cfg.DatabasePath)
		log.Warnf("SavePath is empty, using database directory: %s", savePath)
	}

	// Open database
	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		log.WithError(err).Fatalf("Error opening database at %s", cfg.DatabasePath)
	}
	defer db.Close()

	// Find entries based on provided criteria
	entries := findEntriesToDelete(db)

	if len(entries) == 0 {
		log.Info("No entries found matching the specified criteria.")
		return
	}

	// For search mode, display numbered list and allow selection
	if deleteSearch != "" {
		entries = interactiveSelectEntries(entries)
		if len(entries) == 0 {
			log.Info("No entries selected for deletion.")
			return
		}
	}

	// Display what will be deleted
	displayDeletionTable(entries, savePath)

	// Confirm deletion
	if !confirmDeletion(entries, deleteForce, deleteDryRun) {
		log.Info("Deletion canceled.")
		return
	}

	// Perform deletion
	if deleteDryRun {
		log.Info("Dry run complete. No changes were made.")
		return
	}

	deleted, skipped, errs := deleteEntries(db, entries, savePath, deleteKeepFiles)

	// Report results
	if len(errs) > 0 {
		for _, e := range errs {
			log.Error(e)
		}
	}

	summary := fmt.Sprintf("Deletion complete: %d deleted", deleted)
	if skipped > 0 {
		summary += fmt.Sprintf(", %d skipped (no file)", skipped)
	}
	if len(errs) > 0 {
		summary += fmt.Sprintf(", %d errors", len(errs))
	}
	log.Info(summary)
}

// findEntriesToDelete finds all database entries matching the provided criteria
func findEntriesToDelete(db *database.DB) []models.DatabaseEntry {
	var allEntries []models.DatabaseEntry

	// Create sets for fast lookup
	modelIDSet := make(map[int]struct{})
	for _, id := range deleteModelIDs {
		modelIDSet[id] = struct{}{}
	}

	versionIDSet := make(map[int]struct{})
	for _, id := range deleteVersionIDs {
		versionIDSet[id] = struct{}{}
	}

	searchLower := strings.ToLower(deleteSearch)
	usernameLower := strings.ToLower(deleteUsername)

	// Iterate through all entries and filter
	err := db.Fold(func(key []byte, value []byte) error {
		keyStr := string(key)
		if !strings.HasPrefix(keyStr, "v_") {
			return nil
		}

		var entry models.DatabaseEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			log.WithError(err).Warnf("Failed to unmarshal entry for key %s", keyStr)
			return nil
		}

		// Check if entry matches any criteria
		matches := false

		// Check model ID
		if len(modelIDSet) > 0 {
			if _, ok := modelIDSet[entry.ModelID]; ok {
				matches = true
			}
		}

		// Check version ID
		if len(versionIDSet) > 0 {
			if _, ok := versionIDSet[entry.Version.ID]; ok {
				matches = true
			}
		}

		// Check username (case-insensitive)
		if usernameLower != "" {
			if strings.ToLower(entry.Creator.Username) == usernameLower {
				matches = true
			}
		}

		// Check search query (case-insensitive substring match)
		if searchLower != "" {
			if strings.Contains(strings.ToLower(entry.ModelName), searchLower) {
				matches = true
			}
		}

		if matches {
			allEntries = append(allEntries, entry)
		}

		return nil
	})

	if err != nil {
		log.WithError(err).Error("Error scanning database")
	}

	// Sort entries by model name, then version name
	sort.Slice(allEntries, func(i, j int) bool {
		if allEntries[i].ModelName != allEntries[j].ModelName {
			return allEntries[i].ModelName < allEntries[j].ModelName
		}
		return allEntries[i].Version.Name < allEntries[j].Version.Name
	})

	return allEntries
}

// interactiveSelectEntries displays numbered entries and lets user select which to delete
func interactiveSelectEntries(entries []models.DatabaseEntry) []models.DatabaseEntry {
	fmt.Printf("\nFound %d entries matching \"%s\":\n\n", len(entries), deleteSearch)

	// Display numbered table
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  #\tModel Name\tVersion\tCreator\tType\tStatus\tVersion ID")
	fmt.Fprintln(tw, "  -\t----------\t-------\t-------\t----\t------\t----------")

	for i, entry := range entries {
		fmt.Fprintf(tw, "  %d\t%s\t%s\t%s\t%s\t%s\t%d\n",
			i+1,
			truncateString(entry.ModelName, 30),
			truncateString(entry.Version.Name, 15),
			truncateString(entry.Creator.Username, 15),
			entry.ModelType,
			entry.Status,
			entry.Version.ID,
		)
	}
	tw.Flush()

	fmt.Println()
	fmt.Print("Enter numbers to delete (e.g., 1,3,5 or 1-3 or 'all', or 'q' to cancel): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		log.WithError(err).Error("Error reading input")
		return nil
	}

	input = strings.TrimSpace(strings.ToLower(input))

	if input == "q" || input == "quit" || input == "cancel" || input == "" {
		return nil
	}

	// Parse selection
	selectedIndices := parseSelection(input, len(entries))
	if len(selectedIndices) == 0 {
		fmt.Println("No valid selection made.")
		return nil
	}

	// Build selected entries list
	selected := make([]models.DatabaseEntry, 0, len(selectedIndices))
	for _, idx := range selectedIndices {
		selected = append(selected, entries[idx])
	}

	return selected
}

// parseSelection parses user input like "1,3,5" or "1-3" or "all" into indices
func parseSelection(input string, max int) []int {
	if input == "all" {
		indices := make([]int, max)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}

	indexSet := make(map[int]struct{})

	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for range (e.g., "1-3")
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
				if err1 == nil && err2 == nil && start >= 1 && end <= max && start <= end {
					for i := start; i <= end; i++ {
						indexSet[i-1] = struct{}{} // Convert to 0-based index
					}
				}
			}
		} else {
			// Single number
			num, err := strconv.Atoi(part)
			if err == nil && num >= 1 && num <= max {
				indexSet[num-1] = struct{}{} // Convert to 0-based index
			}
		}
	}

	// Convert set to sorted slice
	indices := make([]int, 0, len(indexSet))
	for idx := range indexSet {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	return indices
}

// displayDeletionTable shows a formatted table of entries to be deleted
func displayDeletionTable(entries []models.DatabaseEntry, savePath string) {
	fmt.Printf("\nEntries to be deleted (%d total):\n\n", len(entries))

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Model Name\tVersion\tCreator\tType\tStatus\tFolder\tVersion ID")
	fmt.Fprintln(tw, "----------\t-------\t-------\t----\t------\t------\t----------")

	var totalSizeKB uint64
	for _, entry := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
			truncateString(entry.ModelName, 25),
			truncateString(entry.Version.Name, 12),
			truncateString(entry.Creator.Username, 12),
			entry.ModelType,
			entry.Status,
			truncateString(entry.Folder, 30),
			entry.Version.ID,
		)
		if entry.File.SizeKB > 0 {
			totalSizeKB += uint64(entry.File.SizeKB)
		}
	}
	tw.Flush()

	// Show size estimate
	if totalSizeKB > 0 {
		sizeMB := float64(totalSizeKB) / 1024
		sizeGB := sizeMB / 1024
		if sizeGB >= 1.0 {
			fmt.Printf("\nEstimated disk space: %.2f GB\n", sizeGB)
		} else {
			fmt.Printf("\nEstimated disk space: %.2f MB\n", sizeMB)
		}
	}
}

// confirmDeletion prompts the user to confirm deletion
func confirmDeletion(entries []models.DatabaseEntry, force bool, dryRun bool) bool {
	if dryRun {
		fmt.Println("\n[DRY RUN] The above entries would be deleted. No changes will be made.")
		return true
	}

	if force {
		log.Info("Skipping confirmation due to --force flag.")
		return true
	}

	fmt.Printf("\nDelete %d entries? This cannot be undone. (y/N): ", len(entries))

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		log.WithError(err).Error("Error reading input")
		return false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == confirmYes
}

const confirmYes = "yes"

// deleteEntries removes entries from database and optionally deletes files
func deleteEntries(db *database.DB, entries []models.DatabaseEntry, savePath string, keepFiles bool) (deleted int, skipped int, errors []error) {
	for _, entry := range entries {
		versionID := entry.Version.ID
		dbKey := []byte(fmt.Sprintf("v_%d", versionID))

		// Delete files unless --keep-files is set
		if !keepFiles && entry.Status == models.StatusDownloaded {
			filePath := filepath.Join(savePath, entry.Folder, entry.Filename)

			// Check if file exists before attempting deletion
			if _, err := os.Stat(filePath); err == nil {
				if err := os.Remove(filePath); err != nil {
					errors = append(errors, fmt.Errorf("failed to delete file %s: %w", filePath, err))
				} else {
					log.Infof("Deleted file: %s", filePath)
				}
			} else if os.IsNotExist(err) {
				log.Warnf("File already missing: %s", filePath)
				skipped++
			}

			// Also try to delete images directory if it exists
			imagesDir := filepath.Join(savePath, entry.Folder, "images")
			if info, err := os.Stat(imagesDir); err == nil && info.IsDir() {
				if err := os.RemoveAll(imagesDir); err != nil {
					log.Warnf("Failed to remove images directory %s: %v", imagesDir, err)
				} else {
					log.Debugf("Removed images directory: %s", imagesDir)
				}
			}

			// Try to clean up empty parent directories
			cleanupEmptyDirs(filepath.Join(savePath, entry.Folder), savePath)
		} else if !keepFiles && entry.Status != models.StatusDownloaded {
			// Entry is Pending or Error, no file to delete
			skipped++
		}

		// Delete database entry
		if err := db.Delete(dbKey); err != nil {
			errors = append(errors, fmt.Errorf("failed to delete database entry for version %d: %w", versionID, err))
		} else {
			deleted++
			log.Debugf("Deleted database entry: v_%d (%s - %s)", versionID, entry.ModelName, entry.Version.Name)
		}
	}

	return deleted, skipped, errors
}

// cleanupEmptyDirs removes empty directories from dir up to (but not including) stopAt
func cleanupEmptyDirs(dir string, stopAt string) {
	for dir != stopAt && dir != "." && dir != "/" {
		// Check if directory is empty
		entries, err := os.ReadDir(dir)
		if err != nil {
			return // Can't read directory, stop
		}

		if len(entries) > 0 {
			return // Directory not empty, stop
		}

		// Remove empty directory
		if err := os.Remove(dir); err != nil {
			log.Debugf("Could not remove empty directory %s: %v", dir, err)
			return
		}
		log.Debugf("Removed empty directory: %s", dir)

		// Move up to parent
		dir = filepath.Dir(dir)
	}
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
