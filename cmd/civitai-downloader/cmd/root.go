package cmd

import (
	"fmt"
	"net/http"
	"os"

	"go-civitai-download/internal/config" // Import new config package
	"go-civitai-download/internal/models"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// cfgFile holds the path to the config file specified by the user
var cfgFile string

// logApiFlag holds the value of the --log-api flag
var logApiFlag bool

// savePathFlag holds the value of the --save-path flag
var savePathFlag string

// apiDelayFlag holds the value of the --api-delay flag
var apiDelayFlag int

// apiTimeoutFlag holds the value of the --api-timeout flag
var apiTimeoutFlag int

// logLevelFlagValue holds the value of the --log-level flag, bound by Cobra
var logLevelFlagValue string

// logFormatFlagValue holds the value of the --log-format flag, bound by Cobra
var logFormatFlagValue string


// globalConfig holds the loaded configuration from config.Initialize
var globalConfig models.Config

// globalHttpTransport holds the globally configured HTTP transport
var globalHttpTransport http.RoundTripper

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "civitai-downloader",
	Short: "A tool to download models from Civitai",
	Long: `Civitai Downloader allows you to fetch and manage models 
from Civitai.com based on specified criteria.`,
	PersistentPreRunE: loadGlobalConfig,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Define persistent flags, binding them to global variables.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "config.toml", "Configuration file path")
	rootCmd.PersistentFlags().StringVar(&logLevelFlagValue, "log-level", "info", "Logging level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().StringVar(&logFormatFlagValue, "log-format", "text", "Logging format (text, json)")
	rootCmd.PersistentFlags().BoolVar(&logApiFlag, "log-api", false, "Log API requests/responses to api.log (overrides config)")
	rootCmd.PersistentFlags().StringVar(&savePathFlag, "save-path", "", "Directory to save models (overrides config)")                                        // Default empty string
	rootCmd.PersistentFlags().IntVar(&apiDelayFlag, "api-delay", -1, "Delay between API calls in ms (overrides config, -1 uses config default)")              // Default -1
	rootCmd.PersistentFlags().IntVar(&apiTimeoutFlag, "api-timeout", -1, "Timeout for API HTTP client in seconds (overrides config, -1 uses config default)") // Default -1

	// Removed viper.BindPFlag calls
	// Removed viper.SetDefault calls
}

// applyDownloadFlags applies download command flags to the CliFlags structure
func applyDownloadFlags(cmd *cobra.Command, flags *config.CliFlags) {
	if flags.Download == nil {
		flags.Download = &config.CliDownloadFlags{}
	}

	// Apply each flag if it was changed
	if cmd.Flags().Changed("concurrency") {
		flags.Download.Concurrency = &downloadConcurrencyFlag
	}
	if cmd.Flags().Changed("tag") {
		flags.Download.Tag = &downloadTagFlag
	}
	if cmd.Flags().Changed("query") {
		flags.Download.Query = &downloadQueryFlag
	}
	if cmd.Flags().Changed("model-types") {
		flags.Download.ModelTypes = &downloadModelTypesFlag
	}
	if cmd.Flags().Changed("base-models") {
		flags.Download.BaseModels = &downloadBaseModelsFlag
	}
	if cmd.Flags().Changed("username") {
		flags.Download.Username = &downloadUsernameFlag
	}
	if cmd.Flags().Changed("nsfw") {
		flags.Download.Nsfw = &downloadNsfwFlag
	}
	if cmd.Flags().Changed("limit") {
		flags.Download.Limit = &downloadLimitFlag
	}
	if cmd.Flags().Changed("max-pages") {
		flags.Download.MaxPages = &downloadMaxPagesFlag
	}
	if cmd.Flags().Changed("sort") {
		flags.Download.Sort = &downloadSortFlag
	}
	if cmd.Flags().Changed("period") {
		flags.Download.Period = &downloadPeriodFlag
	}
	if cmd.Flags().Changed("model-id") {
		flags.Download.ModelID = &downloadModelIDFlag
	}
	if cmd.Flags().Changed("model-version-id") {
		flags.Download.ModelVersionID = &downloadModelVersionIDFlag
	}
	if cmd.Flags().Changed("primary-only") {
		flags.Download.PrimaryOnly = &downloadPrimaryOnlyFlag
	}
	if cmd.Flags().Changed("pruned") {
		flags.Download.Pruned = &downloadPrunedFlag
	}
	if cmd.Flags().Changed("fp16") {
		flags.Download.Fp16 = &downloadFp16Flag
	}
	if cmd.Flags().Changed("all-versions") {
		flags.Download.AllVersions = &downloadAllVersionsFlag
	}
	if cmd.Flags().Changed("ignore-base-models") {
		flags.Download.IgnoreBaseModels = &downloadIgnoreBaseModelsFlag
	}
	if cmd.Flags().Changed("ignore-filename-strings") {
		flags.Download.IgnoreFileNameStrings = &downloadIgnoreFileNameStringsFlag
	}
	if cmd.Flags().Changed("yes") {
		flags.Download.SkipConfirmation = &downloadYesFlag
	}
	if cmd.Flags().Changed("metadata") {
		flags.Download.SaveMetadata = &downloadMetadataFlag
	}
	if cmd.Flags().Changed("model-info") {
		flags.Download.SaveModelInfo = &downloadModelInfoFlag
	}
	if cmd.Flags().Changed("version-images") {
		flags.Download.SaveVersionImages = &downloadVersionImagesFlag
	}
	if cmd.Flags().Changed("model-images") {
		flags.Download.SaveModelImages = &downloadModelImagesFlag
	}
	if cmd.Flags().Changed("meta-only") {
		flags.Download.DownloadMetaOnly = &downloadMetaOnlyFlag
	}
}

// applyImagesFlags applies images command flags to the CliFlags structure
func applyImagesFlags(cmd *cobra.Command, flags *config.CliFlags) {
	if flags.Images == nil {
		flags.Images = &config.CliImagesFlags{}
	}

	// Apply each flag if it was changed
	if cmd.Flags().Changed("limit") {
		flags.Images.Limit = &imagesLimitFlag
	}
	if cmd.Flags().Changed("post-id") {
		flags.Images.PostID = &imagesPostIDFlag
	}
	if cmd.Flags().Changed("model-id") {
		flags.Images.ModelID = &imagesModelIDFlag
	}
	if cmd.Flags().Changed("model-version-id") {
		flags.Images.ModelVersionID = &imagesModelVersionIDFlag
	}
	if cmd.Flags().Changed("username") {
		flags.Images.Username = &imagesUsernameFlag
	}
	if cmd.Flags().Changed("nsfw") {
		flags.Images.Nsfw = &imagesNsfwFlag
	}
	if cmd.Flags().Changed("sort") {
		flags.Images.Sort = &imagesSortFlag
	}
	if cmd.Flags().Changed("period") {
		flags.Images.Period = &imagesPeriodFlag
	}
	if cmd.Flags().Changed("max-pages") {
		flags.Images.MaxPages = &imagesMaxPagesFlag
	}
	if cmd.Flags().Changed("output-dir") {
		flags.Images.OutputDir = &imagesOutputDirFlag
	}
	if cmd.Flags().Changed("concurrency") {
		flags.Images.Concurrency = &imagesConcurrencyFlag
	}
	if cmd.Flags().Changed("metadata") {
		flags.Images.SaveMetadata = &imagesMetadataFlag
	}
}

// applyDownloadFlagsFromGlobals applies download flags by checking global variables against their defaults
func applyDownloadFlagsFromGlobals(flags *config.CliFlags) {
	if flags.Download == nil {
		flags.Download = &config.CliDownloadFlags{}
	}

	// Helper function to avoid duplication in default checking
	if downloadConcurrencyFlag != -1 {
		flags.Download.Concurrency = &downloadConcurrencyFlag
	}
	if downloadTagFlag != "" {
		flags.Download.Tag = &downloadTagFlag
	}
	if downloadQueryFlag != "" {
		flags.Download.Query = &downloadQueryFlag
	}
	if len(downloadModelTypesFlag) > 0 {
		flags.Download.ModelTypes = &downloadModelTypesFlag
	}
	if len(downloadBaseModelsFlag) > 0 {
		flags.Download.BaseModels = &downloadBaseModelsFlag
	}
	if downloadUsernameFlag != "" {
		flags.Download.Username = &downloadUsernameFlag
	}
	if downloadNsfwFlag {
		flags.Download.Nsfw = &downloadNsfwFlag
	}
	if downloadLimitFlag != -1 {
		flags.Download.Limit = &downloadLimitFlag
	}
	if downloadMaxPagesFlag != -1 {
		flags.Download.MaxPages = &downloadMaxPagesFlag
	}
	if downloadSortFlag != "" {
		flags.Download.Sort = &downloadSortFlag
	}
	if downloadPeriodFlag != "" {
		flags.Download.Period = &downloadPeriodFlag
	}
	if downloadModelIDFlag != 0 {
		flags.Download.ModelID = &downloadModelIDFlag
	}
	if downloadModelVersionIDFlag != 0 {
		flags.Download.ModelVersionID = &downloadModelVersionIDFlag
	}
	if downloadPrimaryOnlyFlag {
		flags.Download.PrimaryOnly = &downloadPrimaryOnlyFlag
	}
	if downloadPrunedFlag {
		flags.Download.Pruned = &downloadPrunedFlag
	}
	if downloadFp16Flag {
		flags.Download.Fp16 = &downloadFp16Flag
	}
	if downloadAllVersionsFlag {
		flags.Download.AllVersions = &downloadAllVersionsFlag
	}
	if len(downloadIgnoreBaseModelsFlag) > 0 {
		flags.Download.IgnoreBaseModels = &downloadIgnoreBaseModelsFlag
	}
	if len(downloadIgnoreFileNameStringsFlag) > 0 {
		flags.Download.IgnoreFileNameStrings = &downloadIgnoreFileNameStringsFlag
	}
	if downloadYesFlag {
		flags.Download.SkipConfirmation = &downloadYesFlag
	}
	if downloadMetadataFlag {
		flags.Download.SaveMetadata = &downloadMetadataFlag
	}
	if downloadModelInfoFlag {
		flags.Download.SaveModelInfo = &downloadModelInfoFlag
	}
	if downloadVersionImagesFlag {
		flags.Download.SaveVersionImages = &downloadVersionImagesFlag
	}
	if downloadModelImagesFlag {
		flags.Download.SaveModelImages = &downloadModelImagesFlag
	}
	if downloadMetaOnlyFlag {
		flags.Download.DownloadMetaOnly = &downloadMetaOnlyFlag
	}
}

// applyImagesFlagsFromGlobals applies images flags by checking global variables against their defaults
func applyImagesFlagsFromGlobals(flags *config.CliFlags) {
	if flags.Images == nil {
		flags.Images = &config.CliImagesFlags{}
	}

	if imagesLimitFlag != -1 {
		flags.Images.Limit = &imagesLimitFlag
	}
	if imagesPostIDFlag != 0 {
		flags.Images.PostID = &imagesPostIDFlag
	}
	if imagesModelIDFlag != 0 {
		flags.Images.ModelID = &imagesModelIDFlag
	}
	if imagesModelVersionIDFlag != 0 {
		flags.Images.ModelVersionID = &imagesModelVersionIDFlag
	}
	if imagesUsernameFlag != "" {
		flags.Images.Username = &imagesUsernameFlag
	}
	if imagesNsfwFlag != "" {
		flags.Images.Nsfw = &imagesNsfwFlag
	}
	if imagesSortFlag != "" {
		flags.Images.Sort = &imagesSortFlag
	}
	if imagesPeriodFlag != "" {
		flags.Images.Period = &imagesPeriodFlag
	}
	if imagesMaxPagesFlag != -1 {
		flags.Images.MaxPages = &imagesMaxPagesFlag
	}
	if imagesOutputDirFlag != "" {
		flags.Images.OutputDir = &imagesOutputDirFlag
	}
	if imagesConcurrencyFlag != -1 {
		flags.Images.Concurrency = &imagesConcurrencyFlag
	}
	if imagesMetadataFlag {
		flags.Images.SaveMetadata = &imagesMetadataFlag
	}
}

// applyPersistentFlags applies persistent flags to the CliFlags structure
func applyPersistentFlags(cmd *cobra.Command, flags *config.CliFlags) {
	if cmd.PersistentFlags().Changed("config") {
		flags.ConfigFilePath = &cfgFile
	}

	if logLevelFlagValue != "info" {
		flags.LogLevel = &logLevelFlagValue
	}

	if logFormatFlagValue != "text" {
		flags.LogFormat = &logFormatFlagValue
	}

	if logApiFlag {
		log.Debugf("[loadGlobalConfig] --log-api flag detected as true.")
		flags.LogApiRequests = &logApiFlag
	} else {
		log.Debugf("[loadGlobalConfig] --log-api flag not detected or is false.")
	}

	if savePathFlag != "" {
		log.Debugf("[loadGlobalConfig] --save-path flag detected, value: '%s'", savePathFlag)
		flags.SavePath = &savePathFlag
	} else {
		log.Debugf("[loadGlobalConfig] --save-path flag not detected or is default empty string.")
	}

	if apiDelayFlag != -1 {
		log.Debugf("[loadGlobalConfig] --api-delay flag detected, value: %d", apiDelayFlag)
		flags.APIDelayMs = &apiDelayFlag
	} else {
		log.Debugf("[loadGlobalConfig] --api-delay flag not detected or is default -1.")
	}

	if apiTimeoutFlag != -1 {
		log.Debugf("[loadGlobalConfig] --api-timeout flag detected, value: %d", apiTimeoutFlag)
		flags.APIClientTimeoutSec = &apiTimeoutFlag
	} else {
		log.Debugf("[loadGlobalConfig] --api-timeout flag not detected or is default -1.")
	}
}

// applyCommandSpecificFlags applies flags specific to the current command
func applyCommandSpecificFlags(cmd *cobra.Command, flags *config.CliFlags) {
	switch cmd.Name() {
	case "download":
		applyDownloadFlags(cmd, flags)
	case "images":
		applyImagesFlags(cmd, flags)
	case "show-config":
		if cmd.Parent() != nil && cmd.Parent().Name() == "debug" {
			log.Debug("[loadGlobalConfig] Detected 'debug show-config'. Populating Download and Images flags from global vars.")
			applyDownloadFlagsFromGlobals(flags)
			applyImagesFlagsFromGlobals(flags)
		}
	case "torrent":
		flags.Torrent = &config.CliTorrentFlags{}
		if cmd.Flags().Changed("output-dir") {
			flags.Torrent.OutputDir = &torrentOutputDir
		}
		if cmd.Flags().Changed("overwrite") {
			flags.Torrent.Overwrite = &overwriteTorrents
		}
		if cmd.Flags().Changed("magnet-links") {
			flags.Torrent.MagnetLinks = &generateMagnetLinks
		}
		if cmd.Flags().Changed("concurrency") {
			flags.Torrent.Concurrency = &torrentConcurrencyFlag
		}
	case "verify":
		if cmd.Parent() != nil && cmd.Parent().Name() == "db" {
			if flags.DB == nil {
				flags.DB = &config.CliDBFlags{}
			}
			if flags.DB.Verify == nil {
				flags.DB.Verify = &config.CliDBVerifyFlags{}
			}
			if cmd.Flags().Changed("check-hash") {
				flags.DB.Verify.CheckHash = &DbVerifyCheckHashFlag
			}
			if cmd.Flags().Changed("yes") {
				flags.DB.Verify.AutoRedownload = &DbVerifyYesFlag
			}
		}
	case "clean":
		flags.Clean = &config.CliCleanFlags{}
		if cmd.Flags().Changed("torrents") {
			flags.Clean.Torrents = &cleanTorrentsFlag
		}
		if cmd.Flags().Changed("magnets") {
			flags.Clean.Magnets = &cleanMagnetsFlag
		}
	}
}

// loadGlobalConfig populates the config.CliFlags struct and initializes the global configuration
func loadGlobalConfig(cmd *cobra.Command, args []string) error {
	log.Debug("Attempting to load global configuration...")
	flags := config.CliFlags{}

	// Apply persistent flags
	applyPersistentFlags(cmd, &flags)

	// Configure early logging
	configureLoggingFromFlags(logLevelFlagValue, logFormatFlagValue)
	log.Debug("Initial logging configured from flags (before config file load)")

	// Apply command-specific flags
	applyCommandSpecificFlags(cmd, &flags)

	log.Debugf("[loadGlobalConfig] Initializing config with CliFlags: %+v", flags)
	if flags.Download != nil {
		log.Debugf("[loadGlobalconfig] CliFlags.Download: %+v", *flags.Download)
	}
	if flags.Images != nil {
		log.Debugf("[loadGlobalconfig] CliFlags.Images: %+v", *flags.Images)
	}

	var err error
	globalConfig, globalHttpTransport, err = config.Initialize(flags)
	if err != nil {
		log.Errorf("Failed to initialize configuration: %v", err)
		return err
	}

	// Reconfigure logging with final config
	log.Debug("Re-configuring logging based on final loaded configuration...")
	configureLogging(&globalConfig)

	log.Debugf("Global configuration loaded: %+v", globalConfig)
	log.Debugf("Global HTTP transport configured: type %T", globalHttpTransport)

	return nil
}

// configureLoggingFromFlags sets up initial logging based *only* on flag values.
// This is used before the full config is loaded to see early debug messages.
func configureLoggingFromFlags(levelStr, formatStr string) {
	level, err := log.ParseLevel(levelStr)
	if err != nil {
		log.Warnf("Invalid log level '%s' from flag, using default 'info'. Error: %v", levelStr, err)
		level = log.InfoLevel
	}

	log.SetLevel(level)

	switch formatStr {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	case "text":
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
		})
	default:
		log.Warnf("Invalid log format '%s' from flag, using default 'text'.", formatStr)
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	}
}

// configureLogging sets up logrus based on the final loaded configuration.
func configureLogging(cfg *models.Config) {
	if cfg == nil {
		log.Error("configureLogging called with nil config")
		return
	}
	log.Debugf("[configureLogging] Received config LogLevel: '%s'", cfg.LogLevel)

	level, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Warnf("Invalid log level '%s' in config, using default 'info'. Error: %v", cfg.LogLevel, err)
		level = log.InfoLevel
	}

	log.SetLevel(level)

	switch cfg.LogFormat {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	case "text":
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
			// ForceColors:   true, // Optional: Force colors even without TTY
		})
	default:
		log.Warnf("Invalid log format '%s' in config, using default 'text'.", cfg.LogFormat)
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	}
	log.Infof("Logging configured: Level=%s, Format=%s", level.String(), cfg.LogFormat)
}
