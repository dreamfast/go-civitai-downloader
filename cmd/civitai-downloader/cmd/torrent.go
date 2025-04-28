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
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query" // Import for query types
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	// "github.com/spf13/viper" // Removed Viper import

	index "go-civitai-download/index"
	"go-civitai-download/internal/database"
	"go-civitai-download/internal/models"
)

// Struct to hold job parameters for torrent workers
type torrentJob struct {
	SourcePath     string
	Trackers       []string
	OutputDir      string
	Overwrite      bool
	GenerateMagnet bool
	LogFields      log.Fields // For context in worker logs
	ModelID        int        // ID of the parent model
	ModelName      string     // Name of the model
	ModelType      string     // Type of the model (e.g., LORA, Checkpoint) - Keep for potential use if Item struct changes
	BleveIndex     bleve.Index
}

// Helper to update or create the index item for a model torrent
func updateModelTorrentIndex(job torrentJob, torrentPath, magnetURI string) error {
	modelItemID := fmt.Sprintf("m_%d", job.ModelID) // Index key for the model
	var itemToUpdate index.Item

	// --- Search for existing model item ---
	termQuery := query.NewTermQuery(modelItemID)
	termQuery.SetField("_id") // Search by document ID
	searchRequest := bleve.NewSearchRequest(termQuery)
	searchRequest.Size = 1 // We only need to know if it exists

	searchResult, err := job.BleveIndex.Search(searchRequest)
	if err != nil {
		log.WithFields(job.LogFields).WithError(err).Errorf("Error searching for existing index item %s", modelItemID)
		return fmt.Errorf("error searching index for %s: %w", modelItemID, err)
	}

	if searchResult.Total > 0 {
		// --- Item exists, fetch it to update ---
		docID := searchResult.Hits[0].ID
		_, err := job.BleveIndex.Document(docID) // Use blank identifier as doc is not used
		if err != nil {
			log.WithFields(job.LogFields).WithError(err).Warnf("Found index item %s in search but failed to retrieve document. Will attempt to create new.", modelItemID)
			// Proceed to create new item below
		} else {
			// Found existing document, try to unmarshal its fields
			// This assumes the document fields match the index.Item struct
			// A more robust way involves iterating fields: doc.VisitFields(...)
			// But let's try direct unmarshal for simplicity first.
			// We need the raw bytes stored under a known field or use Bleve's internal storage retrieval if possible.
			// Simplification: Let's recreate the item with known fields + new torrent info.
			// This might lose some fields if the original index item had more, but ensures core info is present.
			log.WithFields(job.LogFields).Debugf("Found existing index item %s, preparing update.", modelItemID)
			itemToUpdate = index.Item{
				ID:            modelItemID,
				Type:          "model",
				ModelName:     job.ModelName,
				DirectoryPath: job.SourcePath,
				// Preserve other fields if fetched, otherwise they get defaults
			}
			// If we successfully fetched and unmarshalled the *full* existing item,
			// we would just modify itemToUpdate.TorrentPath and itemToUpdate.MagnetLink here.
		}
	}

	// --- If item didn't exist or fetch failed, create a new one ---
	if itemToUpdate.ID == "" { // Check if we need to create
		log.WithFields(job.LogFields).Debugf("Index item %s not found or fetch failed, creating new item.", modelItemID)
		itemToUpdate = index.Item{
			ID:        modelItemID,
			Type:      "model", // Indicate this is a model-level item
			ModelName: job.ModelName,
			// ModelType:     job.ModelType, // REMOVED - Field does not exist in index.Item
			DirectoryPath: job.SourcePath, // Path to the main model directory
			// Add other essential fields if needed/available, otherwise leave blank
		}
	}

	// Add/Update torrent info
	itemToUpdate.TorrentPath = torrentPath
	itemToUpdate.MagnetLink = magnetURI // Store the actual magnet URI

	// Update the index
	if err := index.IndexItem(job.BleveIndex, itemToUpdate); err != nil { // Pass by value ok here
		log.WithFields(job.LogFields).WithError(err).Errorf("Failed to update/create index for model item %s", modelItemID)
		return fmt.Errorf("failed to index model item %s: %w", modelItemID, err)
	}

	log.WithFields(job.LogFields).Debugf("Successfully updated/created index for model item %s with torrent info", modelItemID)
	return nil
}

// torrentWorker function - Uses helper for indexing
func torrentWorker(id int, jobs <-chan torrentJob, wg *sync.WaitGroup, successCounter *atomic.Int64, failureCounter *atomic.Int64) {
	defer wg.Done()
	log.Debugf("Torrent Worker %d starting", id)
	for job := range jobs {
		log.WithFields(job.LogFields).Infof("Worker %d: Processing torrent job for model directory %s", id, job.SourcePath)
		// Generate torrent for the entire model directory
		// Capture magnetPath (_), as we don't need it for indexing anymore, but need the magnetURI
		torrentPath, _, magnetURI, err := generateTorrentFile(job.SourcePath, job.Trackers, job.OutputDir, job.Overwrite, job.GenerateMagnet)
		if err != nil {
			log.WithFields(job.LogFields).WithError(err).Errorf("Worker %d: Failed to generate torrent for %s", id, job.SourcePath)
			failureCounter.Add(1)
			continue // Skip indexing if torrent failed
		}

		log.WithFields(job.LogFields).Infof("Worker %d: Successfully generated torrent for %s", id, job.SourcePath)
		successCounter.Add(1)

		// Update the index with model-level torrent information using the helper
		if job.BleveIndex != nil {
			// Pass the actual magnetURI string
			if err := updateModelTorrentIndex(job, torrentPath, magnetURI); err != nil {
				// Log the error from the helper, but don't count as torrent generation failure
				log.WithFields(job.LogFields).WithError(err).Errorf("Worker %d: Index update failed after successful torrent generation.", id)
			}
		}
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

		indexPath := cfg.BleveIndexPath // Use global config
		if indexPath == "" {
			// This should be handled by Initialize setting a default
			log.Warnf("BleveIndexPath not set in config, using default path derived from SavePath: %s", filepath.Join(savePath, "civitai.bleve"))
			indexPath = filepath.Join(savePath, "civitai.bleve") // Default if not set
		}
		log.Infof("Opening/Creating Bleve index at: %s", indexPath)
		bleveIndex, err := index.OpenOrCreateIndex(indexPath)
		if err != nil {
			log.WithError(err).Error("Failed to open or create Bleve index")
			// Attempt to close index even if opening failed (might be partially open)
			if bleveIndex != nil {
				_ = bleveIndex.Close() // Ignore error on close attempt here
			}
			return fmt.Errorf("failed to open or create Bleve index: %w", err)
		}
		defer func() {
			log.Info("Closing Bleve index")
			if err := bleveIndex.Close(); err != nil {
				log.WithError(err).Error("Error closing Bleve index")
			}
		}()

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
			// versionDir := filepath.Join(savePath, entry.Folder) // Removed unused variable
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
					ModelID:    entry.ModelID,
					ModelName:  entry.ModelName,
					ModelType:  modelType, // Store the determined model type
					BleveIndex: bleveIndex,
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
	stat, err := os.Stat(sourcePath)
	if os.IsNotExist(err) {
		log.WithField("path", sourcePath).Error("Source path not found for torrent generation")
		return "", "", "", fmt.Errorf("source path does not exist: %s", sourcePath)
	} else if err != nil {
		log.WithError(err).WithField("path", sourcePath).Error("Error stating source path")
		return "", "", "", fmt.Errorf("error stating source path %s: %w", sourcePath, err)
	} else if !stat.IsDir() {
		log.WithField("path", sourcePath).Error("Source path is not a directory")
		return "", "", "", fmt.Errorf("source path is not a directory: %s", sourcePath)
	}

	// Use the directory name (which should be the model name slug) for the torrent file
	torrentFileName := fmt.Sprintf("%s.torrent", filepath.Base(sourcePath))
	var outPath string
	if outputDir != "" {
		// Ensure output directory exists
		if err := os.MkdirAll(outputDir, 0750); err != nil {
			log.WithError(err).WithField("dir", outputDir).Error("Error creating output directory")
			return "", "", "", fmt.Errorf("error creating output directory %s: %w", outputDir, err)
		}
		outPath = filepath.Join(outputDir, torrentFileName)
	} else {
		// Place the torrent file *inside* the source (model) directory
		outPath = filepath.Join(sourcePath, torrentFileName)
	}
	torrentFilePath = outPath // Assign to return variable

	if !overwrite {
		if _, err := os.Stat(outPath); err == nil {
			log.WithField("path", outPath).Info("Skipping existing torrent file (use --overwrite to replace)")
			// If magnet generation is enabled, check if it also exists
			if generateMagnetLinks {
				magnetFileName := fmt.Sprintf("%s-magnet.txt", strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath)))
				magnetOutPath := filepath.Join(filepath.Dir(outPath), magnetFileName)
				if _, magnetErr := os.Stat(magnetOutPath); magnetErr == nil {
					magnetFilePath = magnetOutPath // Existing magnet file found
					log.WithField("path", magnetOutPath).Info("Found existing magnet link file.")
				}
			}
			// Return existing paths if found and overwrite is false
			return torrentFilePath, magnetFilePath, "", nil
		} else if !os.IsNotExist(err) {
			// Log if we couldn't stat the file for reasons other than not existing
			log.WithError(err).WithField("path", outPath).Warn("Could not check status of potential existing torrent file, attempting to create/overwrite")
		}
	} else {
		// Overwrite is true, log if the file already exists
		if _, err := os.Stat(outPath); err == nil {
			log.WithField("path", outPath).Warn("Overwriting existing torrent file")
		}
	}

	// --- Torrent Metainfo Creation ---
	mi := metainfo.MetaInfo{}
	// Initialize AnnounceList properly
	validTrackers := []string{}
	for _, tracker := range trackers {
		// Ensure tracker URL is valid before adding
		parsedURL, urlErr := url.Parse(tracker)
		if urlErr != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https" && parsedURL.Scheme != "udp") { // Basic validation
			log.WithError(urlErr).WithField("tracker", tracker).Warn("Invalid or unsupported tracker URL provided, skipping.")
			continue
		}
		validTrackers = append(validTrackers, tracker)
	}

	if len(validTrackers) > 0 {
		mi.Announce = validTrackers[0] // Set primary announce
		mi.AnnounceList = make([][]string, 1)
		mi.AnnounceList[0] = validTrackers // Add all valid trackers to the first tier
	} else {
		log.Error("No valid tracker URLs could be added to the torrent.")
		// Consider returning an error if trackers are essential
		// return "", "", "", errors.New("no valid tracker URLs provided or parsed")
	}

	mi.CreatedBy = "go-civitai-download"
	mi.CreationDate = time.Now().Unix() // Add creation date

	// Use a reasonable piece length (adjust if needed for very large/small files)
	const pieceLength = 512 * 1024 // 512 KiB
	info := metainfo.Info{
		PieceLength: pieceLength,
		Name:        filepath.Base(sourcePath), // Set the base name in the info dict
	}

	log.WithField("directory", sourcePath).Debug("Building torrent info...")
	// BuildFromFilePath expects the path to the root of the torrent content
	err = info.BuildFromFilePath(sourcePath)
	if err != nil {
		log.WithError(err).WithField("path", sourcePath).Error("Error building torrent info from path")
		return "", "", "", fmt.Errorf("error building torrent info from path %s: %w", sourcePath, err)
	}

	// Check if any files were actually added
	if len(info.Files) == 0 && info.Length == 0 {
		// This might happen for an empty directory, check if it's intentional
		if !stat.IsDir() { // Should not happen due to earlier check, but safety first
			log.WithField("path", sourcePath).Error("Source path is not a directory after check.")
			return "", "", "", fmt.Errorf("source path %s is not a directory", sourcePath)
		}
		// Check if directory is empty
		dirEntries, readDirErr := os.ReadDir(sourcePath)
		if readDirErr != nil {
			log.WithError(readDirErr).WithField("path", sourcePath).Warn("Could not read directory contents to check for emptiness.")
			// Proceed cautiously, might create an empty torrent
		} else if len(dirEntries) == 0 {
			log.WithField("path", sourcePath).Warn("Source directory is empty. Torrent will be generated but contain no files.")
			// Allow creating torrent for empty dir? Or return error? Let's allow for now.
		} else {
			// Directory not empty, but info.BuildFromFilePath failed to add files?
			log.WithField("path", sourcePath).Error("No files added to torrent info despite directory not being empty.")
			return "", "", "", fmt.Errorf("failed to add files from path %s to torrent info", sourcePath)
		}
	}

	// Marshal the info dictionary
	mi.InfoBytes, err = bencode.Marshal(info)
	if err != nil {
		log.WithError(err).Error("Error marshaling torrent info dictionary")
		return "", "", "", fmt.Errorf("error marshaling torrent info: %w", err)
	}

	// --- Write Torrent File ---
	f, err := os.Create(outPath)
	if err != nil {
		log.WithError(err).WithField("path", outPath).Error("Error creating torrent file")
		return "", "", "", fmt.Errorf("error creating torrent file %s: %w", outPath, err)
	}
	// Use defer with a closure to check close error
	defer func() {
		closeErr := f.Close()
		if err == nil && closeErr != nil { // Only assign closeErr if no previous error occurred
			err = fmt.Errorf("error closing torrent file %s: %w", outPath, closeErr)
			// Attempt cleanup if close fails after successful write
			if removeErr := os.Remove(outPath); removeErr != nil && !os.IsNotExist(removeErr) {
				log.WithError(removeErr).Errorf("Failed to clean up partially written torrent file %s after close error", outPath)
			}
		}
	}()

	err = mi.Write(f)
	if err != nil {
		log.WithError(err).WithField("path", outPath).Error("Error writing torrent file")
		// Attempt to remove partially written file on error
		if removeErr := os.Remove(outPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.WithError(removeErr).Warnf("Failed to remove partially written torrent file %s after write error", outPath)
		}
		// Return paths but with error, magnet URI is not generated yet or irrelevant due to error
		return torrentFilePath, magnetFilePath, "", fmt.Errorf("error writing torrent file %s: %w", outPath, err)
	}

	log.WithField("path", outPath).Info("Successfully generated torrent file")

	// --- Generate Magnet Link String (always generated for return value) ---
	infoHash := mi.HashInfoBytes()
	magnetParts := []string{
		fmt.Sprintf("magnet:?xt=urn:btih:%s", infoHash.HexString()),
		// Use the Name field from the info dict for dn (more reliable than stat.Name())
		fmt.Sprintf("dn=%s", url.QueryEscape(info.Name)),
	}
	// Add trackers from the AnnounceList (which should contain only valid ones)
	uniqueTrackers := make(map[string]struct{})
	if mi.Announce != "" { // Add primary announce first if it exists
		magnetParts = append(magnetParts, fmt.Sprintf("tr=%s", url.QueryEscape(mi.Announce)))
		uniqueTrackers[mi.Announce] = struct{}{}
	}
	for _, tier := range mi.AnnounceList {
		for _, tracker := range tier {
			if _, exists := uniqueTrackers[tracker]; !exists {
				// Double check validity just in case
				parsedURL, urlErr := url.Parse(tracker)
				if urlErr == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https" || parsedURL.Scheme == "udp") {
					magnetParts = append(magnetParts, fmt.Sprintf("tr=%s", url.QueryEscape(tracker)))
					uniqueTrackers[tracker] = struct{}{}
				}
			}
		}
	}
	// Assign the generated magnet URI to the return variable
	magnetURI = strings.Join(magnetParts, "&")

	// --- Write Magnet Link File (if requested) ---
	if generateMagnetLinks {
		// Create magnet file name based on torrent file name
		magnetFileName := fmt.Sprintf("%s-magnet.txt", strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath)))
		// Place magnet file next to the torrent file
		magnetOutPath := filepath.Join(filepath.Dir(outPath), magnetFileName)

		// Handle overwrite for magnet file similar to torrent file
		writeMagnet := true
		if !overwrite {
			if _, statErr := os.Stat(magnetOutPath); statErr == nil {
				log.WithField("path", magnetOutPath).Info("Skipping existing magnet link file (use --overwrite to replace)")
				magnetFilePath = magnetOutPath // Assign existing path
				writeMagnet = false
			} else if !os.IsNotExist(statErr) {
				log.WithError(statErr).WithField("path", magnetOutPath).Warn("Could not check status of potential existing magnet file, attempting to create/overwrite")
			}
		} else {
			if _, statErr := os.Stat(magnetOutPath); statErr == nil {
				log.WithField("path", magnetOutPath).Warn("Overwriting existing magnet link file")
			}
		}

		if writeMagnet {
			writeErr := writeMagnetFile(magnetOutPath, magnetURI)
			if writeErr != nil {
				// Log error but don't fail the whole torrent generation just for the magnet link
				log.WithError(writeErr).WithField("path", magnetOutPath).Error("Failed to write magnet link file")
				// Don't return error, but magnetFilePath will remain empty
			} else {
				log.WithField("path", magnetOutPath).Info("Successfully generated magnet link file")
				magnetFilePath = magnetOutPath // Assign path of newly created file
			}
		}
	} // End if generateMagnetLinks

	// If we reached here without err being set by defer, it's success
	return torrentFilePath, magnetFilePath, magnetURI, err // err will be nil on success, or the potential f.Close() error
}

// writeMagnetFile writes the magnet URI string to the specified file path.
func writeMagnetFile(filePath string, magnetURI string) error {
	f, err := os.Create(filePath)
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
