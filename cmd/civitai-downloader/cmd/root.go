package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus" // Import logrus for config loading message
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"go-civitai-download/internal/api"
	"go-civitai-download/internal/models"
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

// logLevel and logFormat are declared elsewhere (e.g., cmd_download_setup.go)
// var logLevel string
// var logFormat string

// globalConfig holds the loaded configuration
var globalConfig models.Config

// globalHttpTransport holds the globally configured HTTP transport (base or logging-wrapped)
var globalHttpTransport http.RoundTripper

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "civitai-downloader",
	Short: "A tool to download models from Civitai",
	Long: `Civitai Downloader allows you to fetch and manage models 
from Civitai.com based on specified criteria.`,
	PersistentPreRunE: loadGlobalConfig, // Load config before any command runs
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// cobra.OnInitialize(initConfig) // We use PersistentPreRunE now
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Initialize Viper before adding flags so we can bind them
	viper.SetConfigName("config") // name of config file (without extension)
	viper.SetConfigType("toml")   // REQUIRED if the config file does not have the extension in the name

	// Add persistent flags that apply to all commands
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Configuration file path (default is ./config.toml or ~/.config/civitai-downloader/config.toml)")

	// Add persistent flags for logging
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Logging level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "Logging format (text, json)")
	// NOTE: Viper binding for log level/format is not strictly necessary
	// as they are handled directly in initLogging() before Viper might be fully ready,
	// but we can add them for consistency if needed elsewhere.
	// viper.BindPFlag("loglevel", rootCmd.PersistentFlags().Lookup("log-level"))
	// viper.BindPFlag("logformat", rootCmd.PersistentFlags().Lookup("log-format"))

	// Add persistent flag for API logging
	rootCmd.PersistentFlags().BoolVar(&logApiFlag, "log-api", false, "Log API requests/responses to api.log (overrides config)")
	viper.BindPFlag("logapirequests", rootCmd.PersistentFlags().Lookup("log-api"))

	// Add persistent flag for save path
	rootCmd.PersistentFlags().StringVar(&savePathFlag, "save-path", "", "Directory to save models (overrides config)")
	viper.BindPFlag("savepath", rootCmd.PersistentFlags().Lookup("save-path"))

	// Add persistent flag for API delay
	// Default value 0 or negative means "use config or viper default"
	rootCmd.PersistentFlags().IntVar(&apiDelayFlag, "api-delay", -1, "Delay between API calls in ms (overrides config, -1 uses config default)")
	viper.BindPFlag("apidelayms", rootCmd.PersistentFlags().Lookup("api-delay"))

	// Add persistent flag for API timeout
	// Default value 0 or negative means "use config or viper default"
	rootCmd.PersistentFlags().IntVar(&apiTimeoutFlag, "api-timeout", -1, "Timeout for API HTTP client in seconds (overrides config, -1 uses config default)")
	viper.BindPFlag("apiclienttimeoutsec", rootCmd.PersistentFlags().Lookup("api-timeout"))

	// Set Viper defaults (these are applied only if not set in config file or by flag)
	viper.SetDefault("apidelayms", 200)         // Default polite delay
	viper.SetDefault("apiclienttimeoutsec", 60) // Default timeout

	// Bind persistent flags defined above
	_ = viper.BindPFlag("logapirequests", rootCmd.PersistentFlags().Lookup("log-api"))
	_ = viper.BindPFlag("savepath", rootCmd.PersistentFlags().Lookup("save-path"))
	_ = viper.BindPFlag("apidelayms", rootCmd.PersistentFlags().Lookup("api-delay"))
	_ = viper.BindPFlag("apiclienttimeoutsec", rootCmd.PersistentFlags().Lookup("api-timeout"))
	_ = viper.BindPFlag("bleveindexpath", rootCmd.PersistentFlags().Lookup("bleve-index-path"))

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// loadGlobalConfig attempts to load the configuration and applies flag overrides.
// It also sets up the global HTTP transport based on logging settings.
func loadGlobalConfig(cmd *cobra.Command, args []string) error {
	// --- Configure Viper to read the config file ---
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".go-civitai-downloader" (without extension).
		viper.AddConfigPath(home)
		// Add current directory path
		viper.AddConfigPath(".")
		viper.SetConfigName("config") // Name of config file (without extension)
		viper.SetConfigType("toml")   // REQUIRED if the config file does not have the extension in the name
	}

	viper.AutomaticEnv() // read in environment variables that match
	viper.SetEnvPrefix("CIVITAI") // Set prefix for env vars
	// Normalize keys (e.g., from config like BaseModels to BASMODELS)
	// Might help resolve precedence issues with bound flags
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Check environment variable for API key if not set in config
	if viper.GetString("apikey") == "" {
		if envKey := os.Getenv("CIVITAI_DOWNLOADER_APIKEY"); envKey != "" {
			viper.Set("apikey", envKey)
			log.Debug("Using API key from CIVITAI_DOWNLOADER_APIKEY environment variable")
		}
	}

	// Set config search paths
	if cfgFile != "" {
		// Use explicit config file path from flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Search in standard locations
		viper.AddConfigPath(".") // Current directory
		if home, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(filepath.Join(home, ".config", "civitai-downloader"))
		}
	}

	// Read in config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Info("No config file found, using defaults and flags")
		} else {
			// Config file was found but another error occurred
			log.Errorf("Error reading config file: %v", err)
			os.Exit(1)
		}
	} else {
		log.Infof("Using config file: %s", viper.ConfigFileUsed())
	}
	// --- Try merging config AFTER reading and AutomaticEnv ---
	if viper.ConfigFileUsed() != "" { // Only merge if a config file was actually used
		if err := viper.MergeInConfig(); err != nil {
			log.WithError(err).Warnf("Error explicitly merging config file: %s", viper.ConfigFileUsed())
		}
	}
	// --- End Viper config file reading ---

	// --- Unmarshal directly from the global viper instance AFTER ReadInConfig/Merge ---
	if err := viper.Unmarshal(&globalConfig); err != nil {
		log.WithError(err).Warnf("Error unmarshalling config into globalConfig struct: %v", err)
		// Don't make it fatal, allow commands to proceed with defaults/flags if possible.
	}

	// --- REMOVED: Manual merge of loaded config values into Viper ---

	log.Debug("Config loaded (or attempted). Viper will manage value precedence.")

	baseTransport := http.DefaultTransport

	// Check if API logging is enabled using Viper
	globalHttpTransport = baseTransport // Default to base transport
	log.Debugf("Initial globalHttpTransport type: %T", globalHttpTransport)

	if viper.GetBool("logapirequests") {
		log.Debug("API request logging enabled (via Viper), wrapping global HTTP transport.")
		// Define log file path
		logFilePath := "api.log"
		// Attempt to resolve relative to SavePath if possible, otherwise use current dir
		// Get SavePath using Viper
		savePath := viper.GetString("savepath")
		if savePath != "" {
			// Ensure SavePath exists (it might not if config loading failed partially)
			if _, statErr := os.Stat(savePath); statErr == nil {
				logFilePath = filepath.Join(savePath, logFilePath)
			} else {
				log.Warnf("SavePath '%s' (from Viper) not found, saving api.log to current directory.", savePath)
			}
		}
		log.Infof("API logging to file: %s", logFilePath)

		// Initialize the logging transport
		loggingTransport, err := api.NewLoggingTransport(baseTransport, logFilePath)
		if err != nil {
			log.WithError(err).Error("Failed to initialize API logging transport, logging disabled.")
			// Keep globalHttpTransport as baseTransport
		} else {
			globalHttpTransport = loggingTransport // Use the wrapped transport
		}
	}
	// --- End Setup Global HTTP Transport ---

	// If successful or partially successful, globalConfig is populated for use by commands.
	// BUT: Rely on viper.Get*() for values potentially overridden by flags.
	return nil
}
