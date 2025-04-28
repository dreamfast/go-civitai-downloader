package cmd

import (
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	// No Bleve/index import needed here, logic is in runSearchLogic
)

// searchImagesCmd represents the command to search the images index
var searchImagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Search the Bleve index for downloaded images",
	Long: `Performs a search against the Bleve index for downloaded images.
This typically searches the index located at '[SavePath]/images/civitai_images.bleve' 
unless 'BleveIndexPath' is set in the configuration (in which case it searches that path).

Supports Bleve's query string syntax. 

The following fields (using their lowercase JSON tag names) are typically relevant for images:
  - id (string): Unique ID (e.g., img_67890)
  - type (string): Should be "image"
  - name (string): Image file name
  - filePath (string): Full path to the downloaded image file
  - directoryPath (string): Directory containing the file (often the same as filePath for images)
  - modelName (string): Name of the parent model (if image is from a model)
  - versionName (string): Name of the model version (if image is from a version)
  - baseModel (string): Base model associated with the image
  - creatorName (string): Username of the image creator
  - tags ([]string): Associated image tags (often from generation data)
  - prompt (string): Image generation prompt
  - nsfwLevel (string): Image NSFW level (e.g., "None", "Soft", "Mature", "X")

Examples:
  civitai-downloader search images -q "cat"
  civitai-downloader search images -q "+creatorName:some_creator +prompt:landscape"
  civitai-downloader search images -q "+tags:photorealistic"`,
	Run: runSearchImages,
}

func init() {
	searchCmd.AddCommand(searchImagesCmd) // Add to parent search command

	// Share the searchQuery variable with the models command
	searchImagesCmd.Flags().StringVarP(&searchQuery, "query", "q", "", "Search query (uses Bleve query string syntax)")
	_ = searchImagesCmd.MarkFlagRequired("query")
}

// runSearchImages determines the image index path and calls the shared search logic.
func runSearchImages(cmd *cobra.Command, args []string) {
	// initLogging() // Removed - Logging initialized by PersistentPreRunE
	log.Info("Starting Search Images Command")

	// Determine the index path for images
	indexPath := globalConfig.BleveIndexPath // Use path from config if set
	if indexPath == "" {
		// Default path determination for images
		if globalConfig.SavePath == "" {
			log.Fatal("Cannot determine default Bleve index path: SavePath and BleveIndexPath are not set in config.")
		}
		// Images default index is inside the default images directory
		defaultImageDir := filepath.Join(globalConfig.SavePath, "images")
		indexPath = filepath.Join(defaultImageDir, "civitai_images.bleve")
		log.Infof("BleveIndexPath not set, using default image index: %s", indexPath)
	}

	// Call the shared search logic
	runSearchLogic(indexPath, searchQuery)
}
