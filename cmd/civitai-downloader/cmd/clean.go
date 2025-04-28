package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	// Use correct relative paths for internal packages

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Package-level variables for clean flags
var (
	cleanTorrentsFlag bool
	cleanMagnetsFlag  bool
)

func init() {
	// Assumes rootCmd is defined in root.go within the same package
	rootCmd.AddCommand(cleanCmd)

	// Link flags to package-level variables
	cleanCmd.Flags().BoolVarP(&cleanTorrentsFlag, "torrents", "t", false, "Also remove *.torrent files")
	cleanCmd.Flags().BoolVarP(&cleanMagnetsFlag, "magnets", "m", false, "Also remove *-magnet.txt files")
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove temporary (.tmp) files from the download directory",
	Long: `Recursively scans the configured SavePath and removes any files ending with the .tmp extension.
Optionally removes *.torrent and *-magnet.txt files as well.`,
	Run: runClean,
}

func runClean(cmd *cobra.Command, args []string) {
	// Access the globally loaded config from root.go's PersistentPreRunE
	cfg := globalConfig // Use the globalConfig variable
	savePath := cfg.SavePath

	// Get flag values - These are now accessible directly via the package variables,
	// but using cmd.Flags().GetBool() is also fine as Cobra handles it.
	// Let's keep using cmd.Flags().GetBool() for consistency within Run functions.
	cleanTorrents, _ := cmd.Flags().GetBool("torrents")
	cleanMagnets, _ := cmd.Flags().GetBool("magnets")

	// --- Path Validation --- (Moved up slightly)
	if savePath == "" {
		if cfg.DatabasePath != "" {
			savePath = filepath.Dir(cfg.DatabasePath)
			log.Warnf("SavePath is empty, inferring base directory from DatabasePath: %s", savePath)
		} else {
			log.Error("SavePath is not configured (and cannot be inferred from DatabasePath). Cannot determine where to clean.")
			os.Exit(1)
		}
	}
	info, err := os.Stat(savePath)
	if os.IsNotExist(err) {
		log.Errorf("SavePath directory does not exist: %s", savePath)
		os.Exit(1)
	}
	if err != nil {
		log.Errorf("Error accessing SavePath %q: %v", savePath, err)
		os.Exit(1)
	}
	if !info.IsDir() {
		log.Errorf("SavePath is not a directory: %s", savePath)
		os.Exit(1)
	}
	// --- End Path Validation ---

	logLine := fmt.Sprintf("Scanning for .tmp files in %s", savePath)
	if cleanTorrents {
		logLine += " (and *.torrent files)"
	}
	if cleanMagnets {
		logLine += " (and *-magnet.txt files)"
	}
	log.Info(logLine + "...")

	var tmpRemoved, torrentRemoved, magnetRemoved int64
	var filesFailed int64

	walkErr := filepath.Walk(savePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Warnf("Error accessing path %q during scan: %v", path, err)
			return nil
		}
		if info.IsDir() {
			return nil // Skip directories
		}

		lowerName := strings.ToLower(info.Name())
		shouldRemove := false
		fileType := ""

		// Check file types based on flags
		if strings.HasSuffix(lowerName, ".tmp") {
			shouldRemove = true
			fileType = ".tmp"
		} else if cleanTorrents && strings.HasSuffix(lowerName, ".torrent") {
			shouldRemove = true
			fileType = ".torrent"
		} else if cleanMagnets && strings.HasSuffix(lowerName, "-magnet.txt") {
			shouldRemove = true
			fileType = "-magnet.txt"
		}

		if shouldRemove {
			log.Debugf("Found %s file: %s", fileType, path)
			err := os.Remove(path)
			if err != nil {
				if os.IsNotExist(err) {
					log.Warnf("Attempted to remove %s file %q, but it was already gone.", fileType, path)
				} else {
					log.Errorf("Failed to remove %s file %q: %v", fileType, path, err)
					filesFailed++
				}
			} else {
				log.Infof("Removed %s file: %s", fileType, path)
				// Increment specific counter
				switch fileType {
				case ".tmp":
					tmpRemoved++
				case ".torrent":
					torrentRemoved++
				case "-magnet.txt":
					magnetRemoved++
				}
			}
		}
		return nil // Continue walking
	})

	if walkErr != nil {
		log.Errorf("Error during directory walk of %q: %v", savePath, walkErr)
	}

	// Build summary string
	var summaryParts []string
	if tmpRemoved > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d .tmp file(s)", tmpRemoved))
	}
	if torrentRemoved > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d .torrent file(s)", torrentRemoved))
	}
	if magnetRemoved > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d -magnet.txt file(s)", magnetRemoved))
	}

	summary := "Clean complete. Removed: "
	if len(summaryParts) > 0 {
		summary += strings.Join(summaryParts, ", ")
	} else {
		summary += "0 files"
	}

	if filesFailed > 0 {
		summary += fmt.Sprintf(". Failed to remove %d file(s).", filesFailed)
	}
	log.Info(summary)

	if filesFailed > 0 || walkErr != nil {
		os.Exit(1)
	}
}
