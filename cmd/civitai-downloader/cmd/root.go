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

// bleveIndexPathFlag holds the value of the --bleve-index-path flag (needed for persistent flag)
var bleveIndexPathFlag string

// --- Additions Start ---
// logLevelFlagValue holds the value of the --log-level flag, bound by Cobra
var logLevelFlagValue string

// logFormatFlagValue holds the value of the --log-format flag, bound by Cobra
var logFormatFlagValue string

// --- Additions End ---

// apiKeyFlag holds the value of the --api-key flag (defined in download.go, but needs global access?)
// Consider moving the flag definition here if truly global.
// var apiKeyFlag string

// globalConfig holds the loaded configuration from config.Initialize
var globalConfig models.Config

// globalHttpTransport holds the globally configured HTTP transport from config.Initialize
var globalHttpTransport http.RoundTripper

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "civitai-downloader",
	Short: "A tool to download models from Civitai",
	Long: `Civitai Downloader allows you to fetch and manage models 
from Civitai.com based on specified criteria.`,
	// PersistentPreRunE ensures config is loaded before ANY command runs.
	PersistentPreRunE: loadGlobalConfig,
	// Run: func(cmd *cobra.Command, args []string) { },
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
	// Define persistent flags. Viper binding is removed.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "config.toml", "Configuration file path")
	// Bind log-level and log-format to global variables
	rootCmd.PersistentFlags().StringVar(&logLevelFlagValue, "log-level", "info", "Logging level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().StringVar(&logFormatFlagValue, "log-format", "text", "Logging format (text, json)")
	rootCmd.PersistentFlags().BoolVar(&logApiFlag, "log-api", false, "Log API requests/responses to api.log (overrides config)")
	rootCmd.PersistentFlags().StringVar(&savePathFlag, "save-path", "", "Directory to save models (overrides config)")
	rootCmd.PersistentFlags().IntVar(&apiDelayFlag, "api-delay", -1, "Delay between API calls in ms (overrides config, -1 uses config default)")
	rootCmd.PersistentFlags().IntVar(&apiTimeoutFlag, "api-timeout", -1, "Timeout for API HTTP client in seconds (overrides config, -1 uses config default)")
	rootCmd.PersistentFlags().StringVar(&bleveIndexPathFlag, "bleve-index-path", "", "Directory for the search index (overrides config)")

	// Removed viper.BindPFlag calls
	// Removed viper.SetDefault calls
}

// loadGlobalConfig populates the config.CliFlags struct based on the state of
// cobra flags and then calls config.Initialize to load the actual configuration.
func loadGlobalConfig(cmd *cobra.Command, args []string) error {
	log.Debug("Attempting to load global configuration...")
	flags := config.CliFlags{}

	// --- Populate CliFlags from Persistent Flags ---
	if cmd.PersistentFlags().Changed("config") {
		flags.ConfigFilePath = &cfgFile
	}

	// --- Use bound flag variables directly ---
	flags.LogLevel = &logLevelFlagValue   // Store pointer to bound variable
	flags.LogFormat = &logFormatFlagValue // Store pointer to bound variable

	// --- TEMPORARY DEBUG ---
	// fmt.Printf("!!! Flag --log-level value read: %s\n", logLevelFlagValue) // Use bound var // REMOVED
	// --- END TEMP DEBUG ---

	// --- Early Logging Configuration (using flag values) ---
	// --- TEMPORARY DEBUG ---
	// fmt.Printf("!!! Calling configureLoggingFromFlags with level: %s, format: %s\n", logLevelFlagValue, logFormatFlagValue) // Use bound vars // REMOVED
	// --- END TEMP DEBUG ---
	configureLoggingFromFlags(logLevelFlagValue, logFormatFlagValue) // Use bound vars
	log.Debug("Initial logging configured from flags (before config file load)")

	// Continue populating other flags...
	if logApiFlag { // If the flag was set to true via command line
		log.Debugf("[loadGlobalConfig] --log-api flag detected as true. Overriding config.")
		flags.LogApiRequests = &logApiFlag // Assign address of the true value
	} else {
		log.Debugf("[loadGlobalConfig] --log-api flag not detected or is false. Will rely on config file/defaults.")
		// Keep flags.LogApiRequests nil if flag wasn't explicitly set to true
	}

	if cmd.PersistentFlags().Changed("save-path") {
		flags.SavePath = &savePathFlag
	}
	if cmd.PersistentFlags().Changed("api-delay") {
		flags.APIDelayMs = &apiDelayFlag
	}
	if cmd.PersistentFlags().Changed("api-timeout") {
		flags.APIClientTimeoutSec = &apiTimeoutFlag
	}
	if cmd.PersistentFlags().Changed("bleve-index-path") {
		flags.BleveIndexPath = &bleveIndexPathFlag
	}

	// --- Populate CliFlags from relevant Local Flags of the current command ---
	// Check the command name and populate the corresponding nested CliFlags struct
	if cmd.Name() == "download" {
		flags.Download = &config.CliDownloadFlags{}
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
	} else if cmd.Name() == "images" {
		flags.Images = &config.CliImagesFlags{}
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
		if cmd.Flags().Changed("page") {
			flags.Images.Page = &imagesPageFlag
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
	} else if cmd.Name() == "torrent" {
		flags.Torrent = &config.CliTorrentFlags{}
		// Note: --announce and --model-id (torrent) are flags only, not in CliTorrentFlags
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
	} else if cmd.Name() == "verify" && cmd.Parent().Name() == "db" { // Handle nested db verify
		// Ensure the DB and Verify structs are initialized
		if flags.DB == nil {
			flags.DB = &config.CliDBFlags{}
		}
		if flags.DB.Verify == nil {
			flags.DB.Verify = &config.CliDBVerifyFlags{}
		}
		if cmd.Flags().Changed("check-hash") {
			flags.DB.Verify.CheckHash = &dbVerifyCheckHashFlag
		}
		if cmd.Flags().Changed("yes") {
			flags.DB.Verify.AutoRedownload = &dbVerifyYesFlag
		}
	} else if cmd.Name() == "clean" {
		flags.Clean = &config.CliCleanFlags{}
		if cmd.Flags().Changed("torrents") {
			flags.Clean.Torrents = &cleanTorrentsFlag
		}
		if cmd.Flags().Changed("magnets") {
			flags.Clean.Magnets = &cleanMagnetsFlag
		}
	} else if cmd.Name() == "models" && cmd.Parent().Name() == "search" {
		// Search model flags
		if flags.Search == nil {
			flags.Search = &config.CliSearchFlags{}
		}
		if cmd.Flags().Changed("query") {
			flags.Search.Query = &searchQuery
		}
	} else if cmd.Name() == "images" && cmd.Parent().Name() == "search" {
		// Search image flags (shares query variable)
		if flags.Search == nil {
			flags.Search = &config.CliSearchFlags{}
		}
		if cmd.Flags().Changed("query") {
			flags.Search.Query = &searchQuery
		}
	}

	// --- Initialize Configuration ---
	cfg, transport, err := config.Initialize(flags)
	if err != nil {
		// Log the error and return it to cobra for handling
		log.WithError(err).Errorf("Failed to initialize configuration")
		return fmt.Errorf("failed to initialize configuration: %w", err)
	}

	// --- Assign to Global Variables ---
	globalConfig = cfg
	globalHttpTransport = transport

	// --- Final Logging Configuration (Optional Re-config) ---
	// Re-configure logging using the *final* loaded configuration from file/defaults/flags
	// This ensures config file values for logging take precedence if flags weren't set.
	log.Debug("Re-configuring logging based on final loaded configuration...")
	configureLogging(&globalConfig) // Use the existing function that takes the config struct

	log.Debugf("Global configuration loaded: %+v", globalConfig) // Log loaded config for debugging
	log.Debugf("Global HTTP transport configured: type %T", globalHttpTransport)

	return nil
}

// configureLoggingFromFlags sets up logrus based on flag values.
// Used for initial setup before full config is loaded.
func configureLoggingFromFlags(levelStr, formatStr string) {
	level, err := log.ParseLevel(levelStr)
	if err != nil {
		log.WithError(err).Warnf("Invalid log level flag '%s', using default 'info' for initial logging", levelStr)
		level = log.InfoLevel
	}
	log.SetLevel(level)

	switch formatStr {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	case "text":
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	default:
		log.Warnf("Invalid log format flag '%s', using default 'text' for initial logging", formatStr)
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	}
	// No need for the Info message here, the later configureLogging call handles that.
}

// configureLogging sets up logrus based on the final config values.
func configureLogging(cfg *models.Config) {
	log.Debugf("[configureLogging] Received config LogLevel: '%s'", cfg.LogLevel)
	level, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.WithError(err).Warnf("Invalid log level '%s' in config, using default 'info'", cfg.LogLevel)
		level = log.InfoLevel
	}
	log.SetLevel(level)

	switch cfg.LogFormat {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	case "text":
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	default:
		log.Warnf("Invalid log format '%s' in config, using default 'text'", cfg.LogFormat)
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	}

	log.Infof("Logging configured: Level=%s, Format=%s", log.GetLevel(), cfg.LogFormat)
}
