package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	// "github.com/spf13/viper" // Removed Viper import

	"go-civitai-download/internal/database"
	"go-civitai-download/internal/helpers"
	"go-civitai-download/internal/models"
)

// Struct to hold job parameters for torrent workers
type torrentJob struct {
	LogFields      log.Fields
	SourcePath     string
	OutputDir      string
	ModelName      string
	ModelType      string
	Trackers       []string
	ModelID        int
	Overwrite      bool
	GenerateMagnet bool
}

// torrentWorker function - Uses helper for indexing
func torrentWorker(id int, jobs <-chan torrentJob, wg *sync.WaitGroup, successCounter *atomic.Int64, failureCounter *atomic.Int64) {
	defer wg.Done()
	log.Debugf("Torrent Worker %d starting", id)
	for job := range jobs {
		log.WithFields(job.LogFields).Infof("Worker %d: Processing torrent job for model directory %s", id, job.SourcePath)
		// Generate torrent for the entire model directory
		_, _, _, err := generateTorrentFile(job.SourcePath, job.Trackers, job.OutputDir, job.Overwrite, job.GenerateMagnet)
		if err != nil {
			log.WithFields(job.LogFields).WithError(err).Errorf("Worker %d: Failed to generate torrent for %s", id, job.SourcePath)
			failureCounter.Add(1)
			continue // Skip indexing if torrent failed
		}

		log.WithFields(job.LogFields).Infof("Worker %d: Successfully generated torrent for %s", id, job.SourcePath)
		successCounter.Add(1)
	} // end for job := range jobs
	log.Debugf("Torrent Worker %d finished", id)
}

var (
	torrentModelIDs        []int
	announceURLs           []string
	torrentOutputDir       string
	overwriteTorrents      bool
	generateMagnetLinks    bool
	torrentConcurrencyFlag int // Added package-level var for concurrency flag
)

var torrentCmd = &cobra.Command{
	Use:   "torrent",
	Short: "Generate .torrent files for downloaded models (one per model directory)",
	Long: `Generates a single BitTorrent metainfo (.torrent) file for each downloaded model's main directory,
encompassing all its downloaded versions and files. Requires access to the download history database
and the downloaded files themselves. You must specify tracker announce URLs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(announceURLs) == 0 {
			return errors.New("at least one --announce URL is required")
		}

		// Retrieve settings using globalConfig
		cfg := globalConfig                    // Use the global config
		concurrency := cfg.Torrent.Concurrency // Use Torrent specific concurrency
		if concurrency <= 0 {
			log.Warnf("Invalid concurrency value %d from config, defaulting to 4", concurrency)
			concurrency = 4
		}

		savePath := cfg.SavePath // Use global config
		if savePath == "" {
			log.Error("Save path is not configured (--save-path or config file)")
			return errors.New("save path is not configured (--save-path or config file)")
		}

		dbPath := cfg.DatabasePath // Use global config
		if dbPath == "" {
			// This should be handled by Initialize setting a default based on SavePath
			log.Error("Database path is not configured (and could not be defaulted)")
			return errors.New("database path is not configured")
		}
		db, err := database.Open(dbPath)
		if err != nil {
			log.WithError(err).Errorf("Error opening database at %s", dbPath)
			return fmt.Errorf("error opening database: %w", err)
		}
		defer db.Close()

		// Retrieve bound flag values using Viper
		torrentOutputDirEffective := viper.GetString("torrent.outputdir")
		overwriteTorrentsEffective := viper.GetBool("torrent.overwrite")
		generateMagnetLinksEffective := viper.GetBool("torrent.magnetlinks")

		// Map to store model directory paths and associated info (to avoid duplicate jobs)
		modelDirsToProcess := make(map[string]torrentJob)
		modelIDSet := make(map[int]struct{})
		if len(torrentModelIDs) > 0 {
			for _, id := range torrentModelIDs {
				modelIDSet[id] = struct{}{}
			}
		}

		log.Info("Scanning database to identify model directories...")
		errFold := db.Fold(func(key []byte, value []byte) error {
			keyStr := string(key)
			// Process only version entries ('v_*') as they contain path info
			if !strings.HasPrefix(keyStr, "v_") {
				return nil
			}

			var entry models.DatabaseEntry
			if err := json.Unmarshal(value, &entry); err != nil {
				log.WithError(err).Warnf("Failed to unmarshal JSON for key %s, skipping", keyStr)
				return nil
			}

			// Filter by specific model IDs if provided
			if len(torrentModelIDs) > 0 {
				if _, exists := modelIDSet[entry.Version.ModelId]; !exists {
					return nil // Skip if not in the target model ID list
				}
			}

			if entry.Folder == "" {
				log.WithFields(log.Fields{
					"modelID":   entry.Version.ModelId,
					"versionID": entry.Version.ID,
					"key":       keyStr,
				}).Warn("Skipping entry due to missing Folder path.")
				return nil
			}

			// --- Derive the MODEL directory path ---
			// Assumes Folder structure is like: type/modelName/baseModel/versionSlug
			// We want: savePath/type/modelName
			// Need to handle potential variations in depth, e.g. if Base Model isn't used as a dir level
			// Let's assume the first component of entry.Folder is the type, and the second is the model name slug.
			folderParts := strings.Split(entry.Folder, string(filepath.Separator))
			if len(folderParts) < 2 {
				log.WithFields(log.Fields{
					"modelID":   entry.Version.ModelId,
					"versionID": entry.Version.ID,
					"folder":    entry.Folder,
				}).Warn("Could not reliably determine model directory from Folder path (not enough parts), skipping entry.")
				return nil
			}
			modelTypePart := folderParts[0]
			modelNamePart := folderParts[1]
			modelDir := filepath.Join(savePath, modelTypePart, modelNamePart)

			// Check if this model directory is already marked for processing
			if _, exists := modelDirsToProcess[modelDir]; !exists {
				log.Debugf("Identified model directory to process: %s (from version %d)", modelDir, entry.Version.ID)

				// Determine Model Type from version info (use first part of folder as fallback)
				modelType := "unknown_type"
				if entry.ModelType != "" { // Check DbEntry.ModelType first
					modelType = entry.ModelType
				} else if entry.Version.Model.Type != "" { // Then check embedded Model Type
					modelType = entry.Version.Model.Type
				} else if modelTypePart != "" {
					modelType = modelTypePart // Fallback to path component
					log.Warnf("Could not determine Model Type directly for model ID %d, using path component '%s'.", entry.Version.ModelId, modelType)
				} else {
					log.Warnf("Could not determine Model Type for model ID %d, using fallback 'unknown_type'.", entry.Version.ModelId)
				}

				job := torrentJob{
					SourcePath:     modelDir, // Target the model directory
					Trackers:       announceURLs,
					OutputDir:      torrentOutputDirEffective,    // Use viper value
					Overwrite:      overwriteTorrentsEffective,   // Use viper value
					GenerateMagnet: generateMagnetLinksEffective, // Use viper value
					LogFields: log.Fields{ // Context for the model directory
						"modelID":   entry.ModelID,
						"modelName": entry.ModelName, // Use ModelName from entry
						"directory": modelDir,
					},
					ModelID:   entry.ModelID,
					ModelName: entry.ModelName,
					ModelType: modelType, // Store the determined model type
				}
				modelDirsToProcess[modelDir] = job
			}

			return nil
		})

		if errFold != nil {
			log.WithError(errFold).Error("Error scanning database")
			return fmt.Errorf("error scanning database: %w", errFold)
		}

		if len(modelDirsToProcess) == 0 {
			if len(torrentModelIDs) > 0 {
				log.Warnf("No downloaded models found matching specified IDs: %v", torrentModelIDs)
			} else {
				log.Info("No processable model download entries found in the database.")
			}
			return nil
		}

		log.Infof("Generating torrents for %d unique model directories using %d workers...", len(modelDirsToProcess), concurrency)

		// --- Worker Pool Setup ---
		jobs := make(chan torrentJob, concurrency) // Buffered channel
		var wg sync.WaitGroup
		var successCounter atomic.Int64
		var failureCounter atomic.Int64

		// Start workers
		for i := 1; i <= concurrency; i++ {
			wg.Add(1)
			go torrentWorker(i, jobs, &wg, &successCounter, &failureCounter)
		}

		// --- Queue Jobs ---
		queuedJobs := 0
		for _, job := range modelDirsToProcess {
			jobs <- job
			queuedJobs++
		}

		close(jobs) // Signal no more jobs
		log.Infof("Queued %d model directory jobs for torrent generation. Waiting for workers...", queuedJobs)

		// --- Wait for Workers ---
		wg.Wait()

		// --- Final Summary ---
		successCount := successCounter.Load()
		failCount := failureCounter.Load()

		log.Infof("Torrent generation complete. Success: %d, Failed: %d", successCount, failCount)
		if failCount > 0 {
			log.Errorf("%d torrents failed to generate", failCount)
			return fmt.Errorf("%d torrents failed to generate", failCount)
		}
		return nil
	},
}

// generateTorrentFile creates a .torrent file for the given sourcePath (directory).
// It can optionally also create a text file containing the magnet link.
// It returns the path to the generated .torrent file, the magnet link file (if created),
// the magnet URI string itself, or an error.
func generateTorrentFile(sourcePath string, trackers []string, outputDir string, overwrite bool, generateMagnetLinks bool) (torrentFilePath string, magnetFilePath string, magnetURI string, err error) {
	// Validate source path
	if err := validateSourcePath(sourcePath); err != nil {
		return "", "", "", err
	}

	// Determine output path
	outPath, err := determineOutputPath(sourcePath, outputDir)
	if err != nil {
		return "", "", "", err
	}
	torrentFilePath = outPath

	// Check for existing files
	existingMagnetPath, skipGeneration := checkExistingFiles(outPath, overwrite, generateMagnetLinks)
	if skipGeneration {
		return torrentFilePath, existingMagnetPath, "", nil
	}

	// Create torrent metainfo
	mi, info, err := createTorrentMetainfo(sourcePath, trackers)
	if err != nil {
		return "", "", "", err
	}

	// Write torrent file
	if err := writeTorrentFile(outPath, mi); err != nil {
		return torrentFilePath, magnetFilePath, "", err
	}

	log.WithField("path", outPath).Info("Successfully generated torrent file")

	// Generate magnet URI
	magnetURI = generateMagnetURI(mi, info)

	// Write magnet file if requested
	if generateMagnetLinks {
		magnetFilePath = handleMagnetFileGeneration(outPath, magnetURI, overwrite)
	}

	return torrentFilePath, magnetFilePath, magnetURI, nil
}

// validateSourcePath checks if the source path exists and is a directory
func validateSourcePath(sourcePath string) error {
	stat, err := os.Stat(sourcePath)
	if os.IsNotExist(err) {
		log.WithField("path", sourcePath).Error("Source path not found for torrent generation")
		return fmt.Errorf("source path does not exist: %s", sourcePath)
	}
	if err != nil {
		log.WithError(err).WithField("path", sourcePath).Error("Error stating source path")
		return fmt.Errorf("error stating source path %s: %w", sourcePath, err)
	}
	if !stat.IsDir() {
		log.WithField("path", sourcePath).Error("Source path is not a directory")
		return fmt.Errorf("source path is not a directory: %s", sourcePath)
	}
	return nil
}

// determineOutputPath determines where the torrent file should be written
func determineOutputPath(sourcePath, outputDir string) (string, error) {
	torrentFileName := fmt.Sprintf("%s.torrent", filepath.Base(sourcePath))

	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0750); err != nil {
			log.WithError(err).WithField("dir", outputDir).Error("Error creating output directory")
			return "", fmt.Errorf("error creating output directory %s: %w", outputDir, err)
		}
		return filepath.Join(outputDir, torrentFileName), nil
	}

	return filepath.Join(sourcePath, torrentFileName), nil
}

// checkExistingFiles checks if files already exist and handles overwrite logic
func checkExistingFiles(outPath string, overwrite, generateMagnetLinks bool) (string, bool) {
	var magnetFilePath string

	if !overwrite {
		if _, err := os.Stat(outPath); err == nil {
			log.WithField("path", outPath).Info("Skipping existing torrent file (use --overwrite to replace)")

			if generateMagnetLinks {
				magnetFileName := fmt.Sprintf("%s-magnet.txt", strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath)))
				magnetOutPath := filepath.Join(filepath.Dir(outPath), magnetFileName)
				if _, magnetErr := os.Stat(magnetOutPath); magnetErr == nil {
					magnetFilePath = magnetOutPath
					log.WithField("path", magnetOutPath).Info("Found existing magnet link file.")
				}
			}
			return magnetFilePath, true
		} else if !os.IsNotExist(err) {
			log.WithError(err).WithField("path", outPath).Warn("Could not check status of potential existing torrent file, attempting to create/overwrite")
		}
	} else {
		if _, err := os.Stat(outPath); err == nil {
			log.WithField("path", outPath).Warn("Overwriting existing torrent file")
		}
	}

	return "", false
}

// createTorrentMetainfo creates the torrent metainfo and info structures
func createTorrentMetainfo(sourcePath string, trackers []string) (*metainfo.MetaInfo, metainfo.Info, error) {
	mi := metainfo.MetaInfo{}

	// Validate and set trackers
	validTrackers := validateTrackers(trackers)
	if len(validTrackers) > 0 {
		mi.Announce = validTrackers[0]
		mi.AnnounceList = make([][]string, 1)
		mi.AnnounceList[0] = validTrackers
	} else {
		log.Error("No valid tracker URLs could be added to the torrent.")
	}

	mi.CreatedBy = "go-civitai-download"
	mi.CreationDate = time.Now().Unix()

	// Create info structure
	const pieceLength = 512 * 1024 // 512 KiB
	info := metainfo.Info{
		PieceLength: pieceLength,
		Name:        filepath.Base(sourcePath),
	}

	log.WithField("directory", sourcePath).Debug("Building torrent info...")
	if err := info.BuildFromFilePath(sourcePath); err != nil {
		log.WithError(err).WithField("path", sourcePath).Error("Error building torrent info from path")
		return nil, metainfo.Info{}, fmt.Errorf("error building torrent info from path %s: %w", sourcePath, err)
	}

	// Validate that files were added
	if err := validateTorrentInfo(sourcePath, info); err != nil {
		return nil, metainfo.Info{}, err
	}

	// Marshal the info dictionary
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		log.WithError(err).Error("Error marshaling torrent info dictionary")
		return nil, metainfo.Info{}, fmt.Errorf("error marshaling torrent info: %w", err)
	}
	mi.InfoBytes = infoBytes

	return &mi, info, nil
}

// validateTrackers validates tracker URLs and returns only valid ones
func validateTrackers(trackers []string) []string {
	validTrackers := make([]string, 0, len(trackers))
	for _, tracker := range trackers {
		parsedURL, err := url.Parse(tracker)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https" && parsedURL.Scheme != "udp") {
			log.WithError(err).WithField("tracker", tracker).Warn("Invalid or unsupported tracker URL provided, skipping.")
			continue
		}
		validTrackers = append(validTrackers, tracker)
	}
	return validTrackers
}

// validateTorrentInfo validates that the torrent info contains files
func validateTorrentInfo(sourcePath string, info metainfo.Info) error {
	if len(info.Files) == 0 && info.Length == 0 {
		stat, _ := os.Stat(sourcePath)
		if !stat.IsDir() {
			log.WithField("path", sourcePath).Error("Source path is not a directory after check.")
			return fmt.Errorf("source path %s is not a directory", sourcePath)
		}

		dirEntries, readDirErr := os.ReadDir(sourcePath)
		if readDirErr != nil {
			log.WithError(readDirErr).WithField("path", sourcePath).Warn("Could not read directory contents to check for emptiness.")
		} else if len(dirEntries) == 0 {
			log.WithField("path", sourcePath).Warn("Source directory is empty. Torrent will be generated but contain no files.")
		} else {
			log.WithField("path", sourcePath).Error("No files added to torrent info despite directory not being empty.")
			return fmt.Errorf("failed to add files from path %s to torrent info", sourcePath)
		}
	}
	return nil
}

// writeTorrentFile writes the torrent metainfo to a file
func writeTorrentFile(outPath string, mi *metainfo.MetaInfo) error {
	f, err := os.Create(helpers.SanitizePath(outPath))
	if err != nil {
		log.WithError(err).WithField("path", outPath).Error("Error creating torrent file")
		return fmt.Errorf("error creating torrent file %s: %w", outPath, err)
	}

	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.WithError(closeErr).Errorf("Error closing torrent file %s", outPath)
			if err == nil {
				err = fmt.Errorf("error closing torrent file %s: %w", outPath, closeErr)
				if removeErr := os.Remove(outPath); removeErr != nil && !os.IsNotExist(removeErr) {
					log.WithError(removeErr).Errorf("Failed to clean up partially written torrent file %s after close error", outPath)
				}
			}
		}
	}()

	if err := mi.Write(f); err != nil {
		log.WithError(err).WithField("path", outPath).Error("Error writing torrent file")
		if removeErr := os.Remove(outPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.WithError(removeErr).Warnf("Failed to remove partially written torrent file %s after write error", outPath)
		}
		return fmt.Errorf("error writing torrent file %s: %w", outPath, err)
	}

	return nil
}

// generateMagnetURI generates a magnet URI from torrent metainfo and info
func generateMagnetURI(mi *metainfo.MetaInfo, info metainfo.Info) string {
	infoHash := mi.HashInfoBytes()
	magnetParts := []string{
		fmt.Sprintf("magnet:?xt=urn:btih:%s", infoHash.HexString()),
		fmt.Sprintf("dn=%s", url.QueryEscape(info.Name)),
	}

	uniqueTrackers := make(map[string]struct{})
	if mi.Announce != "" {
		magnetParts = append(magnetParts, fmt.Sprintf("tr=%s", url.QueryEscape(mi.Announce)))
		uniqueTrackers[mi.Announce] = struct{}{}
	}

	for _, tier := range mi.AnnounceList {
		for _, tracker := range tier {
			if _, exists := uniqueTrackers[tracker]; !exists {
				parsedURL, err := url.Parse(tracker)
				if err == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https" || parsedURL.Scheme == "udp") {
					magnetParts = append(magnetParts, fmt.Sprintf("tr=%s", url.QueryEscape(tracker)))
					uniqueTrackers[tracker] = struct{}{}
				}
			}
		}
	}

	return strings.Join(magnetParts, "&")
}

// handleMagnetFileGeneration handles the creation of magnet link files
func handleMagnetFileGeneration(outPath, magnetURI string, overwrite bool) string {
	magnetFileName := fmt.Sprintf("%s-magnet.txt", strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath)))
	magnetOutPath := filepath.Join(filepath.Dir(outPath), magnetFileName)

	writeMagnet := true
	if !overwrite {
		if _, err := os.Stat(magnetOutPath); err == nil {
			log.WithField("path", magnetOutPath).Info("Skipping existing magnet link file (use --overwrite to replace)")
			return magnetOutPath
		} else if !os.IsNotExist(err) {
			log.WithError(err).WithField("path", magnetOutPath).Warn("Could not check status of potential existing magnet file, attempting to create/overwrite")
		}
	} else {
		if _, err := os.Stat(magnetOutPath); err == nil {
			log.WithField("path", magnetOutPath).Warn("Overwriting existing magnet link file")
		}
	}

	if writeMagnet {
		if err := writeMagnetFile(magnetOutPath, magnetURI); err != nil {
			log.WithError(err).WithField("path", magnetOutPath).Error("Failed to write magnet link file")
			return ""
		}
		log.WithField("path", magnetOutPath).Info("Successfully generated magnet link file")
		return magnetOutPath
	}

	return ""
}

// writeMagnetFile writes the magnet URI string to the specified file path.
func writeMagnetFile(filePath string, magnetURI string) error {
	f, err := os.Create(helpers.SanitizePath(filePath))
	if err != nil {
		return fmt.Errorf("error creating magnet file %s: %w", filePath, err)
	}
	// Use defer with a closure to check close error
	defer func() {
		closeErr := f.Close()
		if err == nil && closeErr != nil { // Only assign closeErr if no previous error occurred
			err = fmt.Errorf("error closing magnet file %s: %w", filePath, closeErr)
			// Attempt cleanup if close fails after successful write
			if removeErr := os.Remove(filePath); removeErr != nil && !os.IsNotExist(removeErr) {
				log.WithError(removeErr).Errorf("Failed to clean up partially written magnet file %s after close error", filePath)
			}
		}
	}()

	_, err = f.WriteString(magnetURI)
	if err != nil {
		// Attempt to remove partially written file on error
		if removeErr := os.Remove(filePath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.WithError(removeErr).Warnf("Failed to remove partially written magnet file %s after write error", filePath)
		}
		return fmt.Errorf("error writing magnet file %s: %w", filePath, err)
	}
	return err // err will be nil on success, or the potential f.Close() error
}

func init() {
	rootCmd.AddCommand(torrentCmd)

	// Flags definition using Viper binding where appropriate
	torrentCmd.Flags().StringSliceVar(&announceURLs, "announce", []string{}, "Tracker announce URL (repeatable)")
	torrentCmd.Flags().IntSliceVar(&torrentModelIDs, "model-id", []int{}, "Specific model ID(s) to generate torrents for (comma-separated or repeated). Default: all downloaded models.")
	torrentCmd.Flags().StringVarP(&torrentOutputDir, "output-dir", "o", "", "Directory to save generated .torrent files (default: place inside each model's directory)")
	torrentCmd.Flags().BoolVarP(&overwriteTorrents, "overwrite", "f", false, "Overwrite existing .torrent files")
	torrentCmd.Flags().BoolVar(&generateMagnetLinks, "magnet-links", false, "Generate a .txt file containing the magnet link alongside each .torrent file")

	// Concurrency is often a command-line only setting, but could be bound too
	// Link to package-level variable
	torrentCmd.Flags().IntVarP(&torrentConcurrencyFlag, "concurrency", "c", 4, "Number of concurrent torrent generation workers")
}
