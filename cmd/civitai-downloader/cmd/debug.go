package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	// "strconv" // No longer needed here as strconv moved to CreateImageQueryParams

	"go-civitai-download/internal/api" // Use relative path based on assumed go.mod
	// Use relative path based on assumed go.mod
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.AddCommand(debugShowConfigCmd)
	debugCmd.AddCommand(debugPrintApiUrlCmd)

	// Add subcommands for print-api-url
	debugPrintApiUrlCmd.AddCommand(debugPrintApiUrlDownloadCmd)
	debugPrintApiUrlCmd.AddCommand(debugPrintApiUrlImagesCmd)

	// Add relevant flags from download/images commands to the debug versions
	// so loadGlobalConfig populates the flags struct correctly.
	// We reuse the *same* package-level flag variables.

	// Flags for 'debug print-api-url download' (mirroring download.go)
	addDownloadFlags(debugPrintApiUrlDownloadCmd)

	// Flags for 'debug print-api-url images' (mirroring images.go/cmd_images_setup.go)
	addImagesFlags(debugPrintApiUrlImagesCmd)

	// DO NOT add flags to debugShowConfigCmd here. This caused "flag redefined" panics.
	// Its flag processing for overrides will be handled by loadGlobalConfig logic
	// by checking the global flag variables if Cobra populates them based on args passed for 'debug show-config'.
	// addDownloadFlags(debugShowConfigCmd) // Keep this commented
	// addImagesFlags(debugShowConfigCmd)    // Keep this commented
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debugging utilities (not for general use)",
	Long:  `Contains helper commands for debugging application behavior, like inspecting configuration or API URLs.`,
	// PersistentPreRunE: loadGlobalConfig, // Relies on rootCmd's PersistentPreRunE
}

// --- debug show-config ---

var debugShowConfigCmd = &cobra.Command{
	Use:   "show-config",
	Short: "Print the fully loaded configuration object as JSON",
	Long: `Loads configuration via flags and config file (respecting precedence)
and prints the final resulting configuration struct to stdout as JSON.
Useful for verifying how settings are merged.`,
	// PersistentPreRunE: loadGlobalConfig, // Relies on rootCmd's PersistentPreRunE
	Run: func(cmd *cobra.Command, args []string) {
		// globalConfig is populated by PersistentPreRunE
		jsonBytes, err := json.MarshalIndent(globalConfig, "", "  ")
		if err != nil {
			log.Fatalf("Failed to marshal global config to JSON: %v", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonBytes))
	},
}

// --- debug print-api-url ---

var debugPrintApiUrlCmd = &cobra.Command{
	Use:   "print-api-url",
	Short: "Print the API URL that would be used by a command",
	Long: `Constructs and prints the API URL based on loaded configuration and flags,
similar to the old --debug-print-api-url flag. Choose a subcommand (download|images).`,
	// PersistentPreRunE: loadGlobalConfig, // Relies on rootCmd's PersistentPreRunE
	// No Run function needed here, subcommands handle it.
	// PersistentPreRunE is inherited.
}

var debugPrintApiUrlDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Print the API URL for the download command",
	// PersistentPreRunE: loadGlobalConfig, // Relies on rootCmd's PersistentPreRunE
	Run: func(cmd *cobra.Command, args []string) {
		// globalConfig is populated
		// Call the exported helper function from cmd_download_api.go
		queryParams := CreateDownloadQueryParams(&globalConfig)
		baseURL := api.CivitaiApiBaseUrl + "/models" // Use exported base URL + path

		// Construct the URL using the exported helper and Sprintf
		urlValues := api.ConvertQueryParamsToURLValues(queryParams)
		fullURL := fmt.Sprintf("%s?%s", baseURL, urlValues.Encode())

		fmt.Println(fullURL)
	},
}

var debugPrintApiUrlImagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Print the API URL for the images command",
	// PersistentPreRunE: loadGlobalConfig, // Relies on rootCmd's PersistentPreRunE
	Run: func(cmd *cobra.Command, args []string) {
		// globalConfig is populated
		// Call the exported helper function from cmd_images_run.go
		queryParams := CreateImageQueryParams(&globalConfig)
		baseURL := api.CivitaiApiBaseUrl + "/images" // Use exported base URL + path

		// Construct the URL using the exported helper and Sprintf
		urlValues := api.ConvertImageAPIParamsToURLValues(queryParams)
		fullURL := fmt.Sprintf("%s?%s", baseURL, urlValues.Encode())

		fmt.Println(fullURL)
	},
}

// Helper function to add download flags (to avoid duplication)
func addDownloadFlags(cmd *cobra.Command) {
	// Reuse flags from download.go
	cmd.Flags().IntVarP(&downloadConcurrencyFlag, "concurrency", "c", -1, "Number of concurrent download workers (-1 uses config)")
	cmd.Flags().StringVarP(&downloadTagFlag, "tag", "", "", "Filter by tag (API)")
	cmd.Flags().StringVarP(&downloadQueryFlag, "query", "q", "", "Filter by text query (API)")
	cmd.Flags().StringSliceVarP(&downloadModelTypesFlag, "model-types", "", []string{}, "Filter by model types (API, comma-separated or multiple flags)")
	cmd.Flags().StringSliceVarP(&downloadBaseModelsFlag, "base-models", "", []string{}, "Filter by base models (API, comma-separated or multiple flags)")
	cmd.Flags().StringVarP(&downloadUsernameFlag, "username", "", "", "Filter by username (API)")
	cmd.Flags().BoolVarP(&downloadNsfwFlag, "nsfw", "", false, "Include NSFW models (API)") // Note: Cobra bool defaults to false if flag not present
	cmd.Flags().IntVarP(&downloadLimitFlag, "limit", "l", -1, "Limit number of models per page (-1 uses config, API)")
	cmd.Flags().IntVarP(&downloadMaxPagesFlag, "max-pages", "p", -1, "Maximum number of pages to fetch (-1 uses config)")
	cmd.Flags().StringVarP(&downloadSortFlag, "sort", "s", "", "Sort order (API, overrides config)")
	cmd.Flags().StringVarP(&downloadPeriodFlag, "period", "", "", "Sort period (API, overrides config)")
	cmd.Flags().IntVarP(&downloadModelIDFlag, "model-id", "", 0, "Download a specific model ID (ignores API filters)")
	cmd.Flags().IntVarP(&downloadModelVersionIDFlag, "model-version-id", "", 0, "Download a specific model version ID (requires --model-id)")
	cmd.Flags().BoolVarP(&downloadPrimaryOnlyFlag, "primary-only", "", false, "Only consider primary model file (Client Filter)")
	cmd.Flags().BoolVarP(&downloadPrunedFlag, "pruned", "", false, "Prefer pruned models (Client Filter)")
	cmd.Flags().BoolVarP(&downloadFp16Flag, "fp16", "", false, "Prefer fp16 models (Client Filter)")
	cmd.Flags().BoolVarP(&downloadAllVersionsFlag, "all-versions", "a", false, "Download all versions of a model (requires --model-id)")
	cmd.Flags().StringSliceVar(&downloadIgnoreBaseModelsFlag, "ignore-base-models", []string{}, "Base models to ignore (Client Filter, comma-separated or multiple flags)")
	cmd.Flags().StringSliceVar(&downloadIgnoreFileNameStringsFlag, "ignore-filename-strings", []string{}, "Substrings in filenames to ignore (Client Filter, comma-separated or multiple flags)")
	cmd.Flags().BoolVarP(&downloadYesFlag, "yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().BoolVar(&downloadMetadataFlag, "metadata", false, "Save model metadata file")
	cmd.Flags().BoolVar(&downloadModelInfoFlag, "model-info", false, "Save full model info file")
	cmd.Flags().BoolVar(&downloadVersionImagesFlag, "version-images", false, "Save model version images")
	cmd.Flags().BoolVar(&downloadModelImagesFlag, "model-images", false, "Save all model gallery images")
	cmd.Flags().BoolVar(&downloadMetaOnlyFlag, "meta-only", false, "Only download metadata/images, skip model file")
}

// Helper function to add images flags (to avoid duplication)
func addImagesFlags(cmd *cobra.Command) {
	// Reuse flags from cmd_images_setup.go
	cmd.Flags().IntVarP(&imagesLimitFlag, "limit", "l", -1, "Limit number of images per page (-1 uses config, API)")
	cmd.Flags().IntVar(&imagesPostIDFlag, "post-id", 0, "Filter by specific post ID (API)")
	cmd.Flags().IntVar(&imagesModelIDFlag, "model-id", 0, "Filter by specific model ID (API)")
	cmd.Flags().IntVar(&imagesModelVersionIDFlag, "model-version-id", 0, "Filter by specific model version ID (API)")
	cmd.Flags().StringVarP(&imagesUsernameFlag, "username", "u", "", "Filter by username (API)")
	cmd.Flags().StringVar(&imagesNsfwFlag, "nsfw", "", "Filter by NSFW level (None, Soft, Mature, X, All - API, overrides config)")
	cmd.Flags().StringVarP(&imagesSortFlag, "sort", "s", "", "Sort order (API, overrides config)")
	cmd.Flags().StringVar(&imagesPeriodFlag, "period", "", "Sort period (API, overrides config)")
	cmd.Flags().IntVarP(&imagesPageFlag, "page", "p", -1, "API page to start fetching from (-1 uses config)")
	cmd.Flags().IntVar(&imagesMaxPagesFlag, "max-pages", -1, "Maximum number of pages to fetch (-1 uses config)")
	cmd.Flags().StringVarP(&imagesOutputDirFlag, "output-dir", "o", "", "Directory to save images (overrides config SavePath + /images)")
	cmd.Flags().IntVarP(&imagesConcurrencyFlag, "concurrency", "c", -1, "Number of concurrent image download workers (-1 uses config)")
	cmd.Flags().BoolVar(&imagesMetadataFlag, "metadata", false, "Save image metadata file")
}
