package cmd

import (
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	// No Bleve/index import needed here, logic is in runSearchLogic
)

// searchModelsCmd represents the command to search the models index
var searchModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Search the Bleve index for downloaded models",
	Long: `Performs a search against the Bleve index for downloaded models.
This typically searches the index located at '[SavePath]/civitai.bleve' unless 
'BleveIndexPath' is set in the configuration.

Supports Bleve's query string syntax. 

The following fields (using their lowercase JSON tag names) are typically relevant for models:
  - id (string): Unique ID (e.g., v_12345)
  - type (string): Should be "model_file"
  - name (string): File name of the model
  - description (string): Model or version description text
  - filePath (string): Full path to the downloaded file
  - directoryPath (string): Directory containing the file
  - baseModelPath (string): Path up to the base model slug
  - modelPath (string): Path up to the model name slug
  - modelName (string): Name of the parent model
  - versionName (string): Name of the model version
  - baseModel (string): Base model name (e.g., "SDXL 1.0")
  - creatorName (string): Username of the creator
  - tags ([]string): Associated model or version tags
  - publishedAt (time): Version publication timestamp (e.g., +publishedAt:>[2024-01-01])
  - versionDownloadCount (numeric): Version download count
  - versionRating (numeric): Version rating
  - versionRatingCount (numeric): Version rating count
  - fileSizeKB (numeric): File size in KB
  - fileFormat (string): File format (e.g., "SafeTensor")
  - filePrecision (string): File precision (e.g., "fp16")
  - fileSizeType (string): File size type (e.g., "pruned")
  - torrentPath (string): Path to the downloaded .torrent file (if any)
  - magnetLink (string): Magnet link for the torrent (if any)

Examples:
  civitai-downloader search models -q "lora"
  civitai-downloader search models -q "+modelName:MyModel +baseModel:sdxl*"
  civitai-downloader search models -q "+tags:style"`,
	Run: runSearchModels,
}

func init() {
	searchCmd.AddCommand(searchModelsCmd) // Add to parent search command

	// Use PersistentFlags if you want flags to be available to potential sub-subcommands
	// Use Flags for flags specific to this command
	searchModelsCmd.Flags().StringVarP(&searchQuery, "query", "q", "", "Search query (uses Bleve query string syntax)")
	_ = searchModelsCmd.MarkFlagRequired("query")
}

// runSearchModels determines the model index path and calls the shared search logic.
func runSearchModels(cmd *cobra.Command, args []string) {
	// initLogging() // Removed - Logging initialized by PersistentPreRunE
	log.Info("Starting Search Models Command")

	// Determine the index path for models
	indexPath := globalConfig.BleveIndexPath // Use path from config if set
	if indexPath == "" {
		if globalConfig.SavePath == "" {
			log.Fatal("Cannot determine default Bleve index path: SavePath and BleveIndexPath are not set in config.")
		}
		indexPath = filepath.Join(globalConfig.SavePath, "civitai.bleve")
		log.Infof("BleveIndexPath not set, using default model index: %s", indexPath)
	}

	// Call the shared search logic
	runSearchLogic(indexPath, searchQuery)
}
