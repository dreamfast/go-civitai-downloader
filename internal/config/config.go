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
			Limit:             100,
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

	// --- 3. Apply Flag Overrides ---
	// Use mergo again to overlay non-nil flag values onto the current finalCfg.
	// Create a temporary config struct populated ONLY from non-nil flags.
	flagOverrides := models.Config{}
	flagOverridesSet := false // Track if any flags were actually set

	if flags.APIKey != nil {
		flagOverrides.APIKey = *flags.APIKey
		flagOverridesSet = true
	}
	if flags.SavePath != nil {
		flagOverrides.SavePath = *flags.SavePath
		flagOverridesSet = true
	}
	if flags.BleveIndexPath != nil {
		flagOverrides.BleveIndexPath = *flags.BleveIndexPath
		flagOverridesSet = true
	}
	if flags.LogApiRequests != nil {
		flagOverrides.LogApiRequests = *flags.LogApiRequests
		flagOverridesSet = true
	}
	if flags.APIDelayMs != nil {
		flagOverrides.APIDelayMs = *flags.APIDelayMs
		flagOverridesSet = true
	}
	if flags.APIClientTimeoutSec != nil {
		flagOverrides.APIClientTimeoutSec = *flags.APIClientTimeoutSec
		flagOverridesSet = true
	}
	if flags.MaxRetries != nil {
		flagOverrides.MaxRetries = *flags.MaxRetries
		flagOverridesSet = true
	}
	if flags.InitialRetryDelayMs != nil {
		flagOverrides.InitialRetryDelayMs = *flags.InitialRetryDelayMs
		flagOverridesSet = true
	}

	if flags.LogLevel != nil {
		flagOverrides.LogLevel = *flags.LogLevel
		flagOverridesSet = true
	}
	if flags.LogFormat != nil {
		flagOverrides.LogFormat = *flags.LogFormat
		flagOverridesSet = true
	}

	if flags.Download != nil {
		flagOverrides.Download = models.DownloadConfig{} // Initialize nested struct
		if flags.Download.Concurrency != nil {
			flagOverrides.Download.Concurrency = *flags.Download.Concurrency
			flagOverridesSet = true
		}
		if flags.Download.Tag != nil {
			flagOverrides.Download.Tag = *flags.Download.Tag
			flagOverridesSet = true
		}
		if flags.Download.Query != nil {
			flagOverrides.Download.Query = *flags.Download.Query
			flagOverridesSet = true
		}
		if flags.Download.ModelTypes != nil {
			flagOverrides.Download.ModelTypes = *flags.Download.ModelTypes
			flagOverridesSet = true
		}
		if flags.Download.BaseModels != nil {
			flagOverrides.Download.BaseModels = *flags.Download.BaseModels
			flagOverridesSet = true
		}
		if flags.Download.Username != nil {
			flagOverrides.Download.Usernames = []string{*flags.Download.Username}
			flagOverridesSet = true
		} // Convert single flag string to slice
		if flags.Download.Nsfw != nil {
			flagOverrides.Download.Nsfw = *flags.Download.Nsfw
			flagOverridesSet = true
		}
		if flags.Download.Limit != nil {
			flagOverrides.Download.Limit = *flags.Download.Limit
			flagOverridesSet = true
		}
		if flags.Download.MaxPages != nil {
			flagOverrides.Download.MaxPages = *flags.Download.MaxPages
			flagOverridesSet = true
		}
		if flags.Download.Sort != nil {
			flagOverrides.Download.Sort = *flags.Download.Sort
			flagOverridesSet = true
		}
		if flags.Download.Period != nil {
			flagOverrides.Download.Period = *flags.Download.Period
			flagOverridesSet = true
		}
		if flags.Download.ModelID != nil {
			flagOverrides.Download.ModelID = *flags.Download.ModelID
			flagOverridesSet = true
		}
		if flags.Download.ModelVersionID != nil {
			flagOverrides.Download.ModelVersionID = *flags.Download.ModelVersionID
			flagOverridesSet = true
		}
		if flags.Download.PrimaryOnly != nil {
			flagOverrides.Download.PrimaryOnly = *flags.Download.PrimaryOnly
			flagOverridesSet = true
		}
		if flags.Download.Pruned != nil {
			flagOverrides.Download.Pruned = *flags.Download.Pruned
			flagOverridesSet = true
		}
		if flags.Download.Fp16 != nil {
			flagOverrides.Download.Fp16 = *flags.Download.Fp16
			flagOverridesSet = true
		}
		if flags.Download.AllVersions != nil {
			flagOverrides.Download.AllVersions = *flags.Download.AllVersions
			flagOverridesSet = true
		}
		if flags.Download.IgnoreBaseModels != nil {
			flagOverrides.Download.IgnoreBaseModels = *flags.Download.IgnoreBaseModels
			flagOverridesSet = true
		}
		if flags.Download.IgnoreFileNameStrings != nil {
			flagOverrides.Download.IgnoreFileNameStrings = *flags.Download.IgnoreFileNameStrings
			flagOverridesSet = true
		}
		if flags.Download.SkipConfirmation != nil {
			flagOverrides.Download.SkipConfirmation = *flags.Download.SkipConfirmation
			flagOverridesSet = true
		}
		if flags.Download.SaveMetadata != nil {
			flagOverrides.Download.SaveMetadata = *flags.Download.SaveMetadata
			flagOverridesSet = true
		}
		if flags.Download.SaveModelInfo != nil {
			flagOverrides.Download.SaveModelInfo = *flags.Download.SaveModelInfo
			flagOverridesSet = true
		}
		if flags.Download.SaveVersionImages != nil {
			flagOverrides.Download.SaveVersionImages = *flags.Download.SaveVersionImages
			flagOverridesSet = true
		}
		if flags.Download.SaveModelImages != nil {
			flagOverrides.Download.SaveModelImages = *flags.Download.SaveModelImages
			flagOverridesSet = true
		}
		if flags.Download.DownloadMetaOnly != nil {
			flagOverrides.Download.DownloadMetaOnly = *flags.Download.DownloadMetaOnly
			flagOverridesSet = true
		}
	}

	if flags.Images != nil {
		flagOverrides.Images = models.ImagesConfig{}
		if flags.Images.Limit != nil {
			flagOverrides.Images.Limit = *flags.Images.Limit
			flagOverridesSet = true
		}
		if flags.Images.PostID != nil {
			flagOverrides.Images.PostID = *flags.Images.PostID
			flagOverridesSet = true
		}
		if flags.Images.ModelID != nil {
			flagOverrides.Images.ModelID = *flags.Images.ModelID
			flagOverridesSet = true
		}
		if flags.Images.ModelVersionID != nil {
			flagOverrides.Images.ModelVersionID = *flags.Images.ModelVersionID
			flagOverridesSet = true
		}
		if flags.Images.Username != nil {
			flagOverrides.Images.Username = *flags.Images.Username
			flagOverridesSet = true
		}
		if flags.Images.Nsfw != nil {
			flagOverrides.Images.Nsfw = *flags.Images.Nsfw
			flagOverridesSet = true
		}
		if flags.Images.Sort != nil {
			flagOverrides.Images.Sort = *flags.Images.Sort
			flagOverridesSet = true
		}
		if flags.Images.Period != nil {
			flagOverrides.Images.Period = *flags.Images.Period
			flagOverridesSet = true
		}
		if flags.Images.Page != nil {
			flagOverrides.Images.Page = *flags.Images.Page
			flagOverridesSet = true
		}
		if flags.Images.MaxPages != nil {
			flagOverrides.Images.MaxPages = *flags.Images.MaxPages
			flagOverridesSet = true
		}
		if flags.Images.OutputDir != nil {
			flagOverrides.Images.OutputDir = *flags.Images.OutputDir
			flagOverridesSet = true
		}
		if flags.Images.Concurrency != nil {
			flagOverrides.Images.Concurrency = *flags.Images.Concurrency
			flagOverridesSet = true
		}
		if flags.Images.SaveMetadata != nil {
			flagOverrides.Images.SaveMetadata = *flags.Images.SaveMetadata
			flagOverridesSet = true
		}
	}

	if flags.Torrent != nil {
		flagOverrides.Torrent = models.TorrentConfig{}
		if flags.Torrent.OutputDir != nil {
			flagOverrides.Torrent.OutputDir = *flags.Torrent.OutputDir
			flagOverridesSet = true
		}
		if flags.Torrent.Overwrite != nil {
			flagOverrides.Torrent.Overwrite = *flags.Torrent.Overwrite
			flagOverridesSet = true
		}
		if flags.Torrent.MagnetLinks != nil {
			flagOverrides.Torrent.MagnetLinks = *flags.Torrent.MagnetLinks
			flagOverridesSet = true
		}
		if flags.Torrent.Concurrency != nil {
			flagOverrides.Torrent.Concurrency = *flags.Torrent.Concurrency
			flagOverridesSet = true
		}
		// Note: AnnounceURLs and ModelIDs from flags are handled directly in the command logic, not merged here.
	}

	if flags.DB != nil && flags.DB.Verify != nil {
		flagOverrides.DB.Verify = models.DBVerifyConfig{}
		if flags.DB.Verify.CheckHash != nil {
			flagOverrides.DB.Verify.CheckHash = *flags.DB.Verify.CheckHash
			flagOverridesSet = true
		}
		if flags.DB.Verify.AutoRedownload != nil {
			flagOverrides.DB.Verify.AutoRedownload = *flags.DB.Verify.AutoRedownload
			flagOverridesSet = true
		}
	}

	if flagOverridesSet {
		if err := mergo.Merge(&finalCfg, flagOverrides, mergo.WithOverride); err != nil {
			log.WithError(err).Warnf("Error merging flag values onto config")
			// Continue with potentially partially merged config
		}
	}

	// --- 4. Derive Default Paths if Empty ---
	if finalCfg.DatabasePath == "" {
		finalCfg.DatabasePath = filepath.Join(finalCfg.SavePath, "civitai.db")
	}
	// Handle default Bleve path if necessary (could also be command specific)
	// if finalCfg.BleveIndexPath == "" { ... }

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
