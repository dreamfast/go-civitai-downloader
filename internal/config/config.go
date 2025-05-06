package config

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"go-civitai-download/internal/api"
	"go-civitai-download/internal/models"

	"dario.cat/mergo"
	"github.com/BurntSushi/toml"
	log "github.com/sirupsen/logrus"
)

// CliFlags holds pointers to values received from command-line flags.
// Nil fields indicate the flag was not provided by the user.
// Mirrors the structure of models.Config where possible for easier application.
type CliFlags struct {
	// Global/Persistent Flags
	ConfigFilePath      *string
	LogLevel            *string // --log-level
	LogFormat           *string // --log-format
	LogApiRequests      *bool   // --log-api
	SavePath            *string // --save-path
	APIDelayMs          *int    // --api-delay
	APIClientTimeoutSec *int    // --api-timeout
	BleveIndexPath      *string // --bleve-index-path
	APIKey              *string // --api-key (download command, but promote to global?)
	// Flags for potentially new config options:
	MaxRetries          *int // Needs new flag e.g. --max-retries
	InitialRetryDelayMs *int // Needs new flag e.g. --retry-delay

	// Command-specific flags nested
	Download *CliDownloadFlags
	Images   *CliImagesFlags
	Torrent  *CliTorrentFlags
	DB       *CliDBFlags
	Clean    *CliCleanFlags
	Search   *CliSearchFlags
}

type CliDownloadFlags struct {
	Concurrency           *int      // -c
	Tag                   *string   // -t
	Query                 *string   // -q
	ModelTypes            *[]string // -m
	BaseModels            *[]string // -b
	Username              *string   // -u (Single string flag)
	Nsfw                  *bool     // --nsfw
	Limit                 *int      // -l
	MaxPages              *int      // -p
	Sort                  *string   // --sort
	Period                *string   // --period
	ModelID               *int      // --model-id
	ModelVersionID        *int      // --model-version-id
	PrimaryOnly           *bool     // --primary-only
	Pruned                *bool     // --pruned
	Fp16                  *bool     // --fp16
	AllVersions           *bool     // --all-versions
	IgnoreBaseModels      *[]string // --ignore-base-models
	IgnoreFileNameStrings *[]string // --ignore-filename-strings
	SkipConfirmation      *bool     // --yes
	SaveMetadata          *bool     // --metadata
	SaveModelInfo         *bool     // --model-info
	SaveVersionImages     *bool     // --version-images
	SaveModelImages       *bool     // --model-images
	DownloadMetaOnly      *bool     // --meta-only
}

type CliImagesFlags struct {
	Limit          *int    // --limit
	PostID         *int    // --post-id
	ModelID        *int    // --model-id
	ModelVersionID *int    // --model-version-id
	Username       *string // -u
	Nsfw           *string // --nsfw
	Sort           *string // -s
	Period         *string // -p
	Page           *int    // --page
	MaxPages       *int    // --max-pages
	OutputDir      *string // -o
	Concurrency    *int    // -c
	SaveMetadata   *bool   // --metadata
}

type CliTorrentFlags struct {
	AnnounceURLs *[]string // --announce (Flag only)
	ModelIDs     *[]int    // --model-id (Flag only)
	OutputDir    *string   // -o
	Overwrite    *bool     // -f
	MagnetLinks  *bool     // --magnet-links
	Concurrency  *int      // -c
}

type CliDBFlags struct {
	Verify *CliDBVerifyFlags
}

type CliDBVerifyFlags struct {
	CheckHash      *bool // --check-hash
	AutoRedownload *bool // --yes
}

type CliCleanFlags struct { // Flags only
	Torrents *bool // -t
	Magnets  *bool // -m
}

type CliSearchFlags struct { // Flags only (covers models/images)
	Query *string // -q
}

// Initialize loads configuration based on defaults, config file, and flags.
// Precedence: Flags > Config File > Defaults.
func Initialize(flags CliFlags) (models.Config, http.RoundTripper, error) {
	// --- 1. Establish Defaults ---
	finalCfg := models.Config{
		// Set sensible defaults for all fields in models.Config
		SavePath:            "downloads",
		DatabasePath:        "", // Default derived from SavePath later
		BleveIndexPath:      "", // Default derived from SavePath later
		LogApiRequests:      false,
		APIDelayMs:          200,
		APIClientTimeoutSec: 120,
		MaxRetries:          3,    // Default retry count
		InitialRetryDelayMs: 1000, // Default retry delay

		Download: models.DownloadConfig{
			Concurrency:       4,
			Nsfw:              true, // Default to allowing NSFW content
			Limit:             0,    // Default to 0 (unlimited) for total downloads
			MaxPages:          0,
			Sort:              "Most Downloaded",
			Period:            "AllTime",
			SaveMetadata:      true,
			SaveModelInfo:     true,
			SaveVersionImages: false, // Default to false unless flag is provided
			// Initialize slices to avoid nil checks later, though merge should handle it
			ModelTypes:            []string{},
			BaseModels:            []string{},
			Usernames:             []string{},
			IgnoreBaseModels:      []string{},
			IgnoreFileNameStrings: []string{},
		},
		Images: models.ImagesConfig{
			Limit:       100,
			Sort:        "Newest",
			Period:      "AllTime",
			Page:        1,
			Concurrency: 4,
		},
		Torrent: models.TorrentConfig{
			Concurrency: 4,
		},
		DB: models.DBConfig{
			Verify: models.DBVerifyConfig{
				CheckHash: true,
			},
		},
	}

	// --- 2. Load Config File ---
	configFilePath := "config.toml" // Default path
	if flags.ConfigFilePath != nil && *flags.ConfigFilePath != "" {
		configFilePath = *flags.ConfigFilePath
	}

	// Create a temporary struct to load file into
	fileCfg := models.Config{}
	meta, err := toml.DecodeFile(configFilePath, &fileCfg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Infof("Configuration file (%s) not found. Using defaults and flags.", configFilePath)
		} else {
			// Log a warning but continue, allowing flags to override potentially incomplete defaults
			log.WithError(err).Warnf("Error reading config file %s", configFilePath)
		}
	} else {
		log.Infof("Using configuration file: %s", configFilePath)
		// Deep merge file config onto defaults. File values take precedence.
		if err := mergo.Merge(&finalCfg, fileCfg, mergo.WithOverride); err != nil {
			log.WithError(err).Warnf("Error merging config file values onto defaults")
			// Continue with potentially partially merged config
		}
		// --- Add Log Start ---
		log.Debugf("[Config Init] After TOML Load: fileCfg.Download.SaveVersionImages = %v", fileCfg.Download.SaveVersionImages)
		log.Debugf("[Config Init] After TOML Merge: finalCfg.Download.SaveVersionImages = %v", finalCfg.Download.SaveVersionImages)
		// --- Add Log End ---

		// Log undecoded keys as warnings
		if len(meta.Undecoded()) > 0 {
			log.Warnf("Unknown keys found in config file %s: %v", configFilePath, meta.Undecoded())
		}
	}

	// --- Capture SavePath before potential flag override ---
	savePathBeforeFlags := finalCfg.SavePath

	// --- 3. Apply Flag Overrides ---
	// Directly modify finalCfg based on non-nil flags.
	if flags.APIKey != nil {
		log.Debugf("[Config Init] Overriding APIKey from flag.")
		finalCfg.APIKey = *flags.APIKey
	}
	if flags.SavePath != nil {
		log.Debugf("[Config Init] Overriding SavePath from flag: '%s'", *flags.SavePath)
		finalCfg.SavePath = *flags.SavePath
	}
	if flags.BleveIndexPath != nil {
		log.Debugf("[Config Init] Overriding BleveIndexPath from flag: '%s'", *flags.BleveIndexPath)
		finalCfg.BleveIndexPath = *flags.BleveIndexPath
	}
	if flags.LogApiRequests != nil {
		log.Debugf("[Config Init] Overriding LogApiRequests from flag: %v", *flags.LogApiRequests)
		finalCfg.LogApiRequests = *flags.LogApiRequests
	}
	if flags.APIDelayMs != nil {
		log.Debugf("[Config Init] Overriding APIDelayMs from flag: %d", *flags.APIDelayMs)
		finalCfg.APIDelayMs = *flags.APIDelayMs
	}
	if flags.APIClientTimeoutSec != nil {
		log.Debugf("[Config Init] Overriding APIClientTimeoutSec from flag: %d", *flags.APIClientTimeoutSec)
		finalCfg.APIClientTimeoutSec = *flags.APIClientTimeoutSec
	}
	if flags.MaxRetries != nil {
		log.Debugf("[Config Init] Overriding MaxRetries from flag: %d", *flags.MaxRetries)
		finalCfg.MaxRetries = *flags.MaxRetries
	}
	if flags.InitialRetryDelayMs != nil {
		log.Debugf("[Config Init] Overriding InitialRetryDelayMs from flag: %d", *flags.InitialRetryDelayMs)
		finalCfg.InitialRetryDelayMs = *flags.InitialRetryDelayMs
	}
	if flags.LogLevel != nil {
		log.Debugf("[Config Init] Overriding LogLevel from flag: '%s'", *flags.LogLevel)
		finalCfg.LogLevel = *flags.LogLevel
	}
	if flags.LogFormat != nil {
		log.Debugf("[Config Init] Overriding LogFormat from flag: '%s'", *flags.LogFormat)
		finalCfg.LogFormat = *flags.LogFormat
	}

	// Apply nested Download flags directly
	if flags.Download != nil {
		log.Debug("[Config Init] Applying Download flags...")
		if flags.Download.Concurrency != nil {
			finalCfg.Download.Concurrency = *flags.Download.Concurrency
		}
		if flags.Download.Tag != nil {
			finalCfg.Download.Tag = *flags.Download.Tag
		}
		if flags.Download.Query != nil {
			finalCfg.Download.Query = *flags.Download.Query
		}
		if flags.Download.ModelTypes != nil {
			finalCfg.Download.ModelTypes = *flags.Download.ModelTypes
		}
		if flags.Download.BaseModels != nil {
			finalCfg.Download.BaseModels = *flags.Download.BaseModels
		}
		if flags.Download.Username != nil {
			finalCfg.Download.Usernames = []string{*flags.Download.Username}
		} // Wrap single username in slice
		if flags.Download.Nsfw != nil {
			finalCfg.Download.Nsfw = *flags.Download.Nsfw
		}
		if flags.Download.Limit != nil {
			finalCfg.Download.Limit = *flags.Download.Limit
		}
		if flags.Download.MaxPages != nil {
			finalCfg.Download.MaxPages = *flags.Download.MaxPages
		}
		if flags.Download.Sort != nil {
			finalCfg.Download.Sort = *flags.Download.Sort
		}
		if flags.Download.Period != nil {
			finalCfg.Download.Period = *flags.Download.Period
		}
		if flags.Download.ModelID != nil {
			finalCfg.Download.ModelID = *flags.Download.ModelID
		}
		if flags.Download.ModelVersionID != nil {
			finalCfg.Download.ModelVersionID = *flags.Download.ModelVersionID
		}
		if flags.Download.PrimaryOnly != nil {
			finalCfg.Download.PrimaryOnly = *flags.Download.PrimaryOnly
		}
		if flags.Download.Pruned != nil {
			finalCfg.Download.Pruned = *flags.Download.Pruned
		}
		if flags.Download.Fp16 != nil {
			finalCfg.Download.Fp16 = *flags.Download.Fp16
		}
		if flags.Download.AllVersions != nil {
			finalCfg.Download.AllVersions = *flags.Download.AllVersions
		}
		if flags.Download.IgnoreBaseModels != nil {
			finalCfg.Download.IgnoreBaseModels = *flags.Download.IgnoreBaseModels
		}
		if flags.Download.IgnoreFileNameStrings != nil {
			finalCfg.Download.IgnoreFileNameStrings = *flags.Download.IgnoreFileNameStrings
		}
		if flags.Download.SkipConfirmation != nil {
			finalCfg.Download.SkipConfirmation = *flags.Download.SkipConfirmation
		}
		if flags.Download.SaveMetadata != nil {
			finalCfg.Download.SaveMetadata = *flags.Download.SaveMetadata
		}
		if flags.Download.SaveModelInfo != nil {
			finalCfg.Download.SaveModelInfo = *flags.Download.SaveModelInfo
		}
		if flags.Download.SaveVersionImages != nil {
			finalCfg.Download.SaveVersionImages = *flags.Download.SaveVersionImages
		}
		if flags.Download.SaveModelImages != nil {
			finalCfg.Download.SaveModelImages = *flags.Download.SaveModelImages
		}
		if flags.Download.DownloadMetaOnly != nil {
			finalCfg.Download.DownloadMetaOnly = *flags.Download.DownloadMetaOnly
		}
	}

	// Apply nested Images flags directly
	if flags.Images != nil {
		log.Debug("[Config Init] Applying Images flags...")
		if flags.Images.Limit != nil {
			finalCfg.Images.Limit = *flags.Images.Limit
		}
		if flags.Images.PostID != nil {
			finalCfg.Images.PostID = *flags.Images.PostID
		}
		if flags.Images.ModelID != nil {
			finalCfg.Images.ModelID = *flags.Images.ModelID
		}
		if flags.Images.ModelVersionID != nil {
			finalCfg.Images.ModelVersionID = *flags.Images.ModelVersionID
		}
		if flags.Images.Username != nil {
			finalCfg.Images.Username = *flags.Images.Username
		}
		// Note: flags.Images.Nsfw is *string, models.ImagesConfig.Nsfw is bool. Conversion needed if used.
		if flags.Images.Nsfw != nil {
			log.Warnf("[Config Init] Skipping Images.Nsfw flag override due to type mismatch (string vs bool). Flag value: %s", *flags.Images.Nsfw)
		}
		if flags.Images.Sort != nil {
			finalCfg.Images.Sort = *flags.Images.Sort
		}
		if flags.Images.Period != nil {
			finalCfg.Images.Period = *flags.Images.Period
		}
		if flags.Images.Page != nil {
			finalCfg.Images.Page = *flags.Images.Page
		}
		// Note: flags.Images.MaxPages ignored (no config field)
		if flags.Images.MaxPages != nil {
			log.Warnf("[Config Init] Images flag --max-pages ignored, no corresponding config field.")
		}
		if flags.Images.OutputDir != nil {
			finalCfg.Images.OutputDir = *flags.Images.OutputDir
		}
		if flags.Images.Concurrency != nil {
			finalCfg.Images.Concurrency = *flags.Images.Concurrency
		}
		if flags.Images.SaveMetadata != nil {
			finalCfg.Images.SaveMetadata = *flags.Images.SaveMetadata
		}
	}

	// Apply nested Torrent flags directly
	if flags.Torrent != nil {
		log.Debug("[Config Init] Applying Torrent flags...")
		// Note: AnnounceURLs and ModelIDs are flags only, not stored in config struct.
		if flags.Torrent.OutputDir != nil {
			finalCfg.Torrent.OutputDir = *flags.Torrent.OutputDir
		}
		if flags.Torrent.Overwrite != nil {
			finalCfg.Torrent.Overwrite = *flags.Torrent.Overwrite
		}
		if flags.Torrent.MagnetLinks != nil {
			finalCfg.Torrent.MagnetLinks = *flags.Torrent.MagnetLinks
		}
		if flags.Torrent.Concurrency != nil {
			finalCfg.Torrent.Concurrency = *flags.Torrent.Concurrency
		}
	}

	// Apply nested DB flags directly
	if flags.DB != nil && flags.DB.Verify != nil {
		log.Debug("[Config Init] Applying DB Verify flags...")
		if flags.DB.Verify.CheckHash != nil {
			finalCfg.DB.Verify.CheckHash = *flags.DB.Verify.CheckHash
		}
		if flags.DB.Verify.AutoRedownload != nil {
			finalCfg.DB.Verify.AutoRedownload = *flags.DB.Verify.AutoRedownload
		}
	}

	// --- 4. Derive Default Paths if Empty ---
	// Default paths only if they are empty OR if they were previously defaulted based on the save path *before* flag overrides.
	defaultDbPath := filepath.Join(savePathBeforeFlags, "civitai.db")
	if finalCfg.DatabasePath == "" || finalCfg.DatabasePath == defaultDbPath {
		finalCfg.DatabasePath = filepath.Join(finalCfg.SavePath, "civitai.db") // Use final (potentially overridden) SavePath
		log.Debugf("[Config Init] DatabasePath defaulted based on final SavePath: %s", finalCfg.DatabasePath)
	} else {
		log.Debugf("[Config Init] DatabasePath ('%s') was explicitly set or derived from a non-default SavePath, not changing.", finalCfg.DatabasePath)
	}

	defaultBlevePath := filepath.Join(savePathBeforeFlags, "civitai.bleve")
	if finalCfg.BleveIndexPath == "" || finalCfg.BleveIndexPath == defaultBlevePath {
		finalCfg.BleveIndexPath = filepath.Join(finalCfg.SavePath, "civitai.bleve") // Use final (potentially overridden) SavePath
		log.Debugf("[Config Init] BleveIndexPath defaulted based on final SavePath: %s", finalCfg.BleveIndexPath)
	} else {
		log.Debugf("[Config Init] BleveIndexPath ('%s') was explicitly set or derived from a non-default SavePath, not changing.", finalCfg.BleveIndexPath)
	}

	// --- 5. Validation ---
	if finalCfg.SavePath == "" {
		return models.Config{}, nil, fmt.Errorf("SavePath cannot be empty (set via --save-path flag or SavePath in config)")
	}
	// DatabasePath is derived, so should not be empty here.
	// Add more validation as needed (e.g., for API key if required by command)

	// --- 6. Setup HTTP Transport ---
	baseTransport := http.DefaultTransport
	var finalTransport http.RoundTripper = baseTransport

	if finalCfg.LogApiRequests {
		log.Debug("API request logging enabled.")
		logFilePath := "api.log" // Default filename
		if finalCfg.SavePath != "" {
			// Ensure SavePath exists before trying to join
			if _, statErr := os.Stat(finalCfg.SavePath); statErr == nil {
				logFilePath = filepath.Join(finalCfg.SavePath, logFilePath)
			} else {
				log.Warnf("SavePath '%s' not found, saving api.log to current directory.", finalCfg.SavePath)
			}
		} else {
			log.Warnf("SavePath is empty, saving api.log to current directory.")
		}
		log.Infof("API logging to file: %s", logFilePath)

		loggingTransport, err := api.NewLoggingTransport(baseTransport, logFilePath)
		if err != nil {
			log.WithError(err).Error("Failed to initialize API logging transport, logging disabled.")
			// Keep finalTransport as baseTransport
		} else {
			finalTransport = loggingTransport // Use the wrapped transport
		}
	}

	log.Debug("Configuration initialized successfully.")
	return finalCfg, finalTransport, nil
}
