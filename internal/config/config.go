package config

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go-civitai-download/internal/api"
	"go-civitai-download/internal/models"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Default values for configuration
const (
	DefaultSavePath            = "models"
	DefaultDatabasePath        = "civitai.db" // Relative to SavePath if not absolute
	DefaultLogApiRequests      = false
	DefaultAPIDelayMs          = 500 // milliseconds
	DefaultAPIClientTimeoutSec = 60  // seconds
	DefaultMaxRetries          = 3
	DefaultInitialRetryDelayMs = 1000 // milliseconds
	DefaultLogLevel            = "info"
	DefaultLogFormat           = "text"
	DefaultConfigFilePath      = "config.toml" // Added constant

	// Download specific defaults
	DefaultConfigDownloadConcurrency = 5
	DefaultConfigDownloadTag         = ""
	DefaultConfigDownloadQuery       = ""
	// DefaultConfigDownloadModelTypes (empty slice by default)
	// DefaultConfigDownloadBaseModels (empty slice by default)
	// DefaultConfigDownloadUsernames (empty slice by default)
	DefaultConfigDownloadNsfw           = true
	DefaultConfigDownloadLimit          = 100
	DefaultConfigDownloadMaxPages       = 10
	DefaultConfigDownloadSort           = "Most Downloaded"
	DefaultConfigDownloadPeriod         = "AllTime"
	DefaultConfigDownloadModelID        = 0
	DefaultConfigDownloadModelVersionID = 0
	DefaultConfigDownloadPrimaryOnly    = false
	DefaultConfigDownloadPruned         = false
	DefaultConfigDownloadFp16           = false
	DefaultConfigDownloadAllVersions    = false
	// DefaultConfigDownloadIgnoreBaseModels (empty slice by default)
	// DefaultConfigDownloadIgnoreFileNameStrings (empty slice by default)
	DefaultConfigDownloadSkipConfirmation        = false
	DefaultConfigDownloadSaveMetadata            = true
	DefaultConfigDownloadSaveModelInfo           = false
	DefaultConfigDownloadSaveVersionImages       = false
	DefaultConfigDownloadSaveModelImages         = false
	DefaultConfigDownloadDownloadMetaOnly        = false
	DefaultConfigDownloadPathPattern             = "{{.CreatorName}}/{{.ModelName}}/{{.VersionName}}/{{.Filename}}"
	DefaultConfigDownloadModelInfoPathPattern    = "{{.CreatorName}}/{{.ModelName}}/model.info.json"
	DefaultConfigDownloadTrainedWordsPathPattern = "{{.CreatorName}}/{{.ModelName}}/{{.VersionName}}/{{.TrainedWordsFilename}}"

	// Images specific defaults
	DefaultConfigImagesLimit            = 100
	DefaultConfigImagesPostID           = 0
	DefaultConfigImagesModelID          = 0
	DefaultConfigImagesModelVersionID   = 0
	DefaultConfigImagesUsername         = ""
	DefaultConfigImagesNsfw             = "None" // API values: "None", "Soft", "Mature", "X"
	DefaultConfigImagesSort             = "Newest"
	DefaultConfigImagesPeriod           = "AllTime"
	DefaultConfigImagesPage             = 1
	DefaultConfigImagesMaxPages         = 10
	DefaultConfigImagesOutputDir        = "" // Empty means SavePath/images
	DefaultConfigImagesConcurrency      = 5
	DefaultConfigImagesSaveMetadata     = true
	DefaultConfigImagesPathPattern      = "{{.Username}}/{{.ImageID}}_{{.Filename}}"
	DefaultConfigImagesSubfolderPattern = "{{.ModelName}}/{{.ModelVersionName}}" // if model context known

	// Torrent specific defaults
	DefaultConfigTorrentOutputDir         = "torrents"
	DefaultConfigTorrentDHTNodes          = "router.bittorrent.com:6881,router.utorrent.com:6881,dht.transmissionbt.com:6881"
	DefaultConfigTorrentTrackers          = "udp://tracker.openbittorrent.com:80,udp://tracker.opentrackr.org:1337/announce"
	DefaultConfigTorrentOverwrite         = false
	DefaultConfigTorrentMagnetLinks       = false
	DefaultConfigTorrentConcurrency       = 2
	DefaultConfigTorrentPieceLengthKB     = 256
	DefaultConfigTorrentExcludeFileTypes  = ".json,.txt,.info,.yaml,.md,.html"
	DefaultConfigTorrentIncludeExtensions = ".ckpt,.safetensors,.pt,.bin,.pth,.onnx,.zip,.gguf,.ggml"
	DefaultConfigTorrentSourceTag         = "civitai.com"

	// DB specific defaults
	DefaultConfigDBVerifyCheckHash      = true
	DefaultConfigDBVerifyAutoRedownload = false

	// Clean specific defaults
	DefaultConfigCleanTorrents = false
	DefaultConfigCleanMagnets  = false
)

// setViperDefaults configures Viper with the application's default values.
func setViperDefaults(v *viper.Viper) {
	v.SetDefault("apikey", "")
	v.SetDefault("savepath", DefaultSavePath)
	v.SetDefault("databasepath", DefaultDatabasePath) // Will be made absolute later if relative
	v.SetDefault("logapirequests", DefaultLogApiRequests)
	v.SetDefault("apidelayms", DefaultAPIDelayMs)
	v.SetDefault("apiclienttimeoutsec", DefaultAPIClientTimeoutSec)
	v.SetDefault("maxretries", DefaultMaxRetries)
	v.SetDefault("initialretrydelayms", DefaultInitialRetryDelayMs)
	v.SetDefault("loglevel", DefaultLogLevel)
	v.SetDefault("logformat", DefaultLogFormat)

	// Download defaults
	v.SetDefault("download.concurrency", DefaultConfigDownloadConcurrency)
	v.SetDefault("download.tag", DefaultConfigDownloadTag)
	v.SetDefault("download.query", DefaultConfigDownloadQuery)
	v.SetDefault("download.modeltypes", []string{}) // Default empty slice
	v.SetDefault("download.basemodels", []string{}) // Default empty slice
	v.SetDefault("download.usernames", []string{})  // Default empty slice
	v.SetDefault("download.nsfw", DefaultConfigDownloadNsfw)
	v.SetDefault("download.limit", DefaultConfigDownloadLimit)
	v.SetDefault("download.maxpages", DefaultConfigDownloadMaxPages)
	v.SetDefault("download.sort", DefaultConfigDownloadSort)
	v.SetDefault("download.period", DefaultConfigDownloadPeriod)
	v.SetDefault("download.modelid", DefaultConfigDownloadModelID)
	v.SetDefault("download.modelversionid", DefaultConfigDownloadModelVersionID)
	v.SetDefault("download.primaryonly", DefaultConfigDownloadPrimaryOnly)
	v.SetDefault("download.pruned", DefaultConfigDownloadPruned)
	v.SetDefault("download.fp16", DefaultConfigDownloadFp16)
	v.SetDefault("download.allversions", DefaultConfigDownloadAllVersions)
	v.SetDefault("download.ignorebasemodels", []string{})      // Default empty slice
	v.SetDefault("download.ignorefilenamestrings", []string{}) // Default empty slice
	v.SetDefault("download.skipconfirmation", DefaultConfigDownloadSkipConfirmation)
	v.SetDefault("download.savemetadata", DefaultConfigDownloadSaveMetadata)
	v.SetDefault("download.savemodelinfo", DefaultConfigDownloadSaveModelInfo)
	v.SetDefault("download.saveversionimages", DefaultConfigDownloadSaveVersionImages)
	v.SetDefault("download.savemodelimages", DefaultConfigDownloadSaveModelImages)
	v.SetDefault("download.downloadmetaonly", DefaultConfigDownloadDownloadMetaOnly)
	v.SetDefault("download.pathpattern", DefaultConfigDownloadPathPattern)
	v.SetDefault("download.modelinfopathpattern", DefaultConfigDownloadModelInfoPathPattern)
	v.SetDefault("download.trainedwordspathpattern", DefaultConfigDownloadTrainedWordsPathPattern)

	// Images defaults
	v.SetDefault("images.limit", DefaultConfigImagesLimit)
	v.SetDefault("images.postid", DefaultConfigImagesPostID)
	v.SetDefault("images.modelid", DefaultConfigImagesModelID)
	v.SetDefault("images.modelversionid", DefaultConfigImagesModelVersionID)
	v.SetDefault("images.username", DefaultConfigImagesUsername)
	v.SetDefault("images.nsfw", DefaultConfigImagesNsfw)
	v.SetDefault("images.sort", DefaultConfigImagesSort)
	v.SetDefault("images.period", DefaultConfigImagesPeriod)
	v.SetDefault("images.page", DefaultConfigImagesPage)
	v.SetDefault("images.maxpages", DefaultConfigImagesMaxPages)
	v.SetDefault("images.outputdir", DefaultConfigImagesOutputDir)
	v.SetDefault("images.concurrency", DefaultConfigImagesConcurrency)
	v.SetDefault("images.savemetadata", DefaultConfigImagesSaveMetadata)
	v.SetDefault("images.pathpattern", DefaultConfigImagesPathPattern)
	v.SetDefault("images.subfolderpattern", DefaultConfigImagesSubfolderPattern)

	// Torrent defaults
	v.SetDefault("torrent.outputdir", DefaultConfigTorrentOutputDir)
	v.SetDefault("torrent.dhtnodes", DefaultConfigTorrentDHTNodes)
	v.SetDefault("torrent.trackers", DefaultConfigTorrentTrackers)
	v.SetDefault("torrent.overwrite", DefaultConfigTorrentOverwrite)
	v.SetDefault("torrent.magnetlinks", DefaultConfigTorrentMagnetLinks)
	v.SetDefault("torrent.concurrency", DefaultConfigTorrentConcurrency)
	v.SetDefault("torrent.piecelengthkb", DefaultConfigTorrentPieceLengthKB)
	v.SetDefault("torrent.excludefiletypes", DefaultConfigTorrentExcludeFileTypes)
	v.SetDefault("torrent.includeextensions", DefaultConfigTorrentIncludeExtensions)
	v.SetDefault("torrent.sourcetag", DefaultConfigTorrentSourceTag)

	// DB defaults
	v.SetDefault("db.verify.checkhash", DefaultConfigDBVerifyCheckHash)
	v.SetDefault("db.verify.autoredownload", DefaultConfigDBVerifyAutoRedownload)

	// Clean defaults
	v.SetDefault("clean.torrents", DefaultConfigCleanTorrents)
	v.SetDefault("clean.magnets", DefaultConfigCleanMagnets)
}

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

// Initialize loads configuration based on defaults, config file, and flags.
// Precedence: Flags > Config File > Defaults.
func Initialize(flags CliFlags) (models.Config, http.RoundTripper, error) {
	// --- 1. Establish Defaults ---
	finalCfg := models.Config{
		// Set sensible defaults for all fields in models.Config
		SavePath:            "downloads",
		DatabasePath:        "", // Default derived from SavePath later
		LogApiRequests:      false,
		APIDelayMs:          200,
		APIClientTimeoutSec: 120,
		MaxRetries:          3,    // Default retry count
		InitialRetryDelayMs: 1000, // Default retry delay

		Download: models.DownloadConfig{
			Concurrency:          4,
			Nsfw:                 true, // Default to allowing NSFW content
			Limit:                0,    // Default to 0 (unlimited) for total downloads
			MaxPages:             0,
			Sort:                 "Most Downloaded",
			Period:               "AllTime",
			SaveMetadata:         true,
			SaveModelInfo:        true,
			SaveVersionImages:    false,                                                           // Default to false unless flag is provided
			VersionPathPattern:   "{modelType}/{modelName}/{baseModel}/{versionId}-{versionName}", // Default version path
			ModelInfoPathPattern: "{modelType}/{modelName}",                                       // Default model info path
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

	log.Debugf("[Initialize] Applying default values. Current cfg.Download: %+v", finalCfg.Download)

	// Initialize Viper
	v := viper.New()
	v.SetEnvPrefix("CIVITAI")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set defaults using Viper as well, so they are part of the hierarchy
	setViperDefaults(v)
	log.Debugf("[Initialize] Viper defaults set. cfg.Download after Viper defaults (should be same as above): %+v", finalCfg.Download) // Should be same as initial defaults

	// Determine config file path
	actualConfigFilePath := DefaultConfigFilePath
	if flags.ConfigFilePath != nil {
		actualConfigFilePath = *flags.ConfigFilePath
		log.Debugf("[Initialize] Using config file path from CLI flag: %s", actualConfigFilePath)
	} else {
		log.Debugf("[Initialize] Using default config file path: %s", actualConfigFilePath)
	}
	v.SetConfigFile(actualConfigFilePath)

	// Attempt to read the config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Warnf("[Initialize] Config file '%s' not found. Using defaults and CLI flags only.", actualConfigFilePath)
		} else {
			log.Warnf("[Initialize] Error reading config file '%s': %v. Using defaults and CLI flags only.", actualConfigFilePath, err)
		}
		// Even if file read fails, proceed to unmarshal. Viper will use defaults for missing keys/file.
	} else {
		log.Infof("[Initialize] Successfully read config file: %s", v.ConfigFileUsed())
	}

	// Unmarshal Viper data (defaults + file if read) into the config struct.
	// This MUST happen regardless of whether ReadInConfig succeeded or not, to apply Viper's defaults.
	if err := v.Unmarshal(&finalCfg); err != nil {
		log.Errorf("[Initialize] Failed to unmarshal config from Viper: %v", err)
		return models.Config{}, nil, fmt.Errorf("failed to unmarshal config from viper: %w", err)
	}
	// This log will now reflect either (defaults) or (defaults overridden by file)
	log.Debugf("[Initialize] After attempting file read and unmarshalling. cfg.Download: %+v", finalCfg.Download)
	log.Debugf("[Initialize] Specifically, after unmarshal: Query='%s', ModelTypes=%v, Limit=%d, Nsfw=%t, Sort='%s', Period='%s'", finalCfg.Download.Query, finalCfg.Download.ModelTypes, finalCfg.Download.Limit, finalCfg.Download.Nsfw, finalCfg.Download.Sort, finalCfg.Download.Period)

	// --- Override with CLI Flags ---
	log.Debugf("[Initialize] About to override with CLI flags. Current cfg.Download (after Viper unmarshal): %+v", finalCfg.Download)
	if flags.APIKey != nil {
		log.Debugf("[Initialize] Overriding APIKey from flag.")
		finalCfg.APIKey = *flags.APIKey
	}
	if flags.SavePath != nil {
		log.Debugf("[Initialize] Overriding SavePath from flag: '%s'", *flags.SavePath)
		finalCfg.SavePath = *flags.SavePath
	}
	if flags.LogApiRequests != nil {
		log.Debugf("[Initialize] Overriding LogApiRequests from flag: %v", *flags.LogApiRequests)
		finalCfg.LogApiRequests = *flags.LogApiRequests
	}
	if flags.APIDelayMs != nil {
		log.Debugf("[Initialize] Overriding APIDelayMs from flag: %d", *flags.APIDelayMs)
		finalCfg.APIDelayMs = *flags.APIDelayMs
	}
	if flags.APIClientTimeoutSec != nil {
		log.Debugf("[Initialize] Overriding APIClientTimeoutSec from flag: %d", *flags.APIClientTimeoutSec)
		finalCfg.APIClientTimeoutSec = *flags.APIClientTimeoutSec
	}
	if flags.MaxRetries != nil {
		log.Debugf("[Initialize] Overriding MaxRetries from flag: %d", *flags.MaxRetries)
		finalCfg.MaxRetries = *flags.MaxRetries
	}
	if flags.InitialRetryDelayMs != nil {
		log.Debugf("[Initialize] Overriding InitialRetryDelayMs from flag: %d", *flags.InitialRetryDelayMs)
		finalCfg.InitialRetryDelayMs = *flags.InitialRetryDelayMs
	}
	if flags.LogLevel != nil {
		log.Debugf("[Initialize] Overriding LogLevel from flag: '%s'", *flags.LogLevel)
		finalCfg.LogLevel = *flags.LogLevel
	}
	if flags.LogFormat != nil {
		log.Debugf("[Initialize] Overriding LogFormat from flag: '%s'", *flags.LogFormat)
		finalCfg.LogFormat = *flags.LogFormat
	}

	// Override Download settings
	if flags.Download != nil {
		log.Debugf("[Initialize] Processing Download CLI flags: %+v", *flags.Download)
		if flags.Download.Concurrency != nil {
			finalCfg.Download.Concurrency = *flags.Download.Concurrency
			log.Debugf("[Initialize] CLI Override: Download.Concurrency = %d", finalCfg.Download.Concurrency)
		}
		if flags.Download.Tag != nil {
			finalCfg.Download.Tag = *flags.Download.Tag
			log.Debugf("[Initialize] CLI Override: Download.Tag = '%s'", finalCfg.Download.Tag)
		}
		if flags.Download.Query != nil {
			finalCfg.Download.Query = *flags.Download.Query
			log.Debugf("[Initialize] CLI Override: Download.Query = '%s'", finalCfg.Download.Query)
		}
		if flags.Download.ModelTypes != nil && len(*flags.Download.ModelTypes) > 0 { // Check if slice is not empty
			finalCfg.Download.ModelTypes = *flags.Download.ModelTypes
			log.Debugf("[Initialize] CLI Override: Download.ModelTypes = %v", finalCfg.Download.ModelTypes)
		}
		if flags.Download.BaseModels != nil && len(*flags.Download.BaseModels) > 0 { // Check if slice is not empty
			finalCfg.Download.BaseModels = *flags.Download.BaseModels
			log.Debugf("[Initialize] CLI Override: Download.BaseModels = %v", finalCfg.Download.BaseModels)
		}
		if flags.Download.Username != nil {
			// API expects single username, config stores list. Flag provides single.
			// For consistency, if flag is set, it becomes the first (and only relevant for API) username.
			finalCfg.Download.Usernames = []string{*flags.Download.Username}
			log.Debugf("[Initialize] CLI Override: Download.Usernames = %v (from single username flag)", finalCfg.Download.Usernames)
		}
		if flags.Download.Nsfw != nil {
			finalCfg.Download.Nsfw = *flags.Download.Nsfw
			log.Debugf("[Initialize] CLI Override: Download.Nsfw = %t", finalCfg.Download.Nsfw)
		}
		if flags.Download.Limit != nil {
			finalCfg.Download.Limit = *flags.Download.Limit
			log.Debugf("[Initialize] CLI Override: Download.Limit = %d", finalCfg.Download.Limit)
		}
		if flags.Download.MaxPages != nil {
			finalCfg.Download.MaxPages = *flags.Download.MaxPages
			log.Debugf("[Initialize] CLI Override: Download.MaxPages = %d", finalCfg.Download.MaxPages)
		}
		if flags.Download.Sort != nil {
			finalCfg.Download.Sort = *flags.Download.Sort
			log.Debugf("[Initialize] CLI Override: Download.Sort = '%s'", finalCfg.Download.Sort)
		}
		if flags.Download.Period != nil {
			finalCfg.Download.Period = *flags.Download.Period
			log.Debugf("[Initialize] CLI Override: Download.Period = '%s'", finalCfg.Download.Period)
		}
		if flags.Download.ModelID != nil {
			finalCfg.Download.ModelID = *flags.Download.ModelID
			log.Debugf("[Initialize] CLI Override: Download.ModelID = %d", finalCfg.Download.ModelID)
		}
		if flags.Download.ModelVersionID != nil {
			finalCfg.Download.ModelVersionID = *flags.Download.ModelVersionID
			log.Debugf("[Initialize] CLI Override: Download.ModelVersionID = %d", finalCfg.Download.ModelVersionID)
		}
		if flags.Download.PrimaryOnly != nil {
			finalCfg.Download.PrimaryOnly = *flags.Download.PrimaryOnly
			log.Debugf("[Initialize] CLI Override: Download.PrimaryOnly = %t", finalCfg.Download.PrimaryOnly)
		}
		if flags.Download.Pruned != nil {
			finalCfg.Download.Pruned = *flags.Download.Pruned
			log.Debugf("[Initialize] CLI Override: Download.Pruned = %t", finalCfg.Download.Pruned)
		}
		if flags.Download.Fp16 != nil {
			finalCfg.Download.Fp16 = *flags.Download.Fp16
			log.Debugf("[Initialize] CLI Override: Download.Fp16 = %t", finalCfg.Download.Fp16)
		}
		if flags.Download.AllVersions != nil {
			finalCfg.Download.AllVersions = *flags.Download.AllVersions
			log.Debugf("[Initialize] CLI Override: Download.AllVersions = %t", finalCfg.Download.AllVersions)
		}
		if flags.Download.IgnoreBaseModels != nil {
			finalCfg.Download.IgnoreBaseModels = *flags.Download.IgnoreBaseModels
			log.Debugf("[Initialize] CLI Override: Download.IgnoreBaseModels = %v", finalCfg.Download.IgnoreBaseModels)
		}
		if flags.Download.IgnoreFileNameStrings != nil {
			finalCfg.Download.IgnoreFileNameStrings = *flags.Download.IgnoreFileNameStrings
			log.Debugf("[Initialize] CLI Override: Download.IgnoreFileNameStrings = %v", finalCfg.Download.IgnoreFileNameStrings)
		}
		if flags.Download.SkipConfirmation != nil {
			finalCfg.Download.SkipConfirmation = *flags.Download.SkipConfirmation
			log.Debugf("[Initialize] CLI Override: Download.SkipConfirmation = %t", finalCfg.Download.SkipConfirmation)
		}
		if flags.Download.SaveMetadata != nil {
			finalCfg.Download.SaveMetadata = *flags.Download.SaveMetadata
			log.Debugf("[Initialize] CLI Override: Download.SaveMetadata = %t", finalCfg.Download.SaveMetadata)
		}
		if flags.Download.SaveModelInfo != nil {
			finalCfg.Download.SaveModelInfo = *flags.Download.SaveModelInfo
			log.Debugf("[Initialize] CLI Override: Download.SaveModelInfo = %t", finalCfg.Download.SaveModelInfo)
		}
		if flags.Download.SaveVersionImages != nil {
			finalCfg.Download.SaveVersionImages = *flags.Download.SaveVersionImages
			log.Debugf("[Initialize] CLI Override: Download.SaveVersionImages = %t", finalCfg.Download.SaveVersionImages)
		}
		if flags.Download.SaveModelImages != nil {
			finalCfg.Download.SaveModelImages = *flags.Download.SaveModelImages
			log.Debugf("[Initialize] CLI Override: Download.SaveModelImages = %t", finalCfg.Download.SaveModelImages)
		}
		if flags.Download.DownloadMetaOnly != nil {
			finalCfg.Download.DownloadMetaOnly = *flags.Download.DownloadMetaOnly
			log.Debugf("[Initialize] CLI Override: Download.DownloadMetaOnly = %t", finalCfg.Download.DownloadMetaOnly)
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
	defaultDbPath := filepath.Join(finalCfg.SavePath, "civitai.db")
	if finalCfg.DatabasePath == "" || finalCfg.DatabasePath == defaultDbPath {
		finalCfg.DatabasePath = filepath.Join(finalCfg.SavePath, "civitai.db") // Use final (potentially overridden) SavePath
		log.Debugf("[Config Init] DatabasePath defaulted based on final SavePath: %s", finalCfg.DatabasePath)
	} else {
		log.Debugf("[Config Init] DatabasePath ('%s') was explicitly set or derived from a non-default SavePath, not changing.", finalCfg.DatabasePath)
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

	log.Debugf("[Initialize] Final merged cfg.Download before returning: %+v", finalCfg.Download)
	log.Debugf("[Initialize] Specifically, final: Query='%s', ModelTypes=%v, Limit=%d, Nsfw=%t, Sort='%s', Period='%s'", finalCfg.Download.Query, finalCfg.Download.ModelTypes, finalCfg.Download.Limit, finalCfg.Download.Nsfw, finalCfg.Download.Sort, finalCfg.Download.Period)

	if err := performPathPatternValidation(&finalCfg); err != nil {
		// Log the error and potentially return it, or handle it as a fatal error
		// For now, we'll log it and continue, but in a stricter setup, you might exit.
		log.Errorf("Path pattern validation failed: %v", err)
		// Depending on desired behavior, you might want to:
		// return nil, nil, fmt.Errorf("path pattern validation failed: %w", err)
		// or log.Fatalf("Path pattern validation failed: %v", err)
	}

	log.Debug("Configuration initialized successfully.")
	return finalCfg, finalTransport, nil
}

// --- Path Pattern Validation --- START ---
var pathPatternTagRegex = regexp.MustCompile(`\{([^}]+)\}`)

// modelLevelAllowedTags are placeholders valid in ModelInfoPathPattern
var modelLevelAllowedTags = map[string]struct{}{
	"modelId":     {},
	"modelName":   {},
	"modelType":   {},
	"creatorName": {},
	// {baseModel} is intentionally omitted as it leads to 'unknown_baseModel'
}

// versionLevelAllowedTags are placeholders valid in VersionPathPattern
var versionLevelAllowedTags = map[string]struct{}{
	"modelId":     {},
	"modelName":   {},
	"modelType":   {},
	"creatorName": {},
	"versionId":   {},
	"versionName": {},
	"baseModel":   {},
}

// validatePathPattern checks a given pattern string against a map of allowed tags.
// It returns a list of disallowed tags found in the pattern.
func validatePathPattern(pattern string, allowedTags map[string]struct{}, patternName string) []string {
	matches := pathPatternTagRegex.FindAllStringSubmatch(pattern, -1)
	var disallowedTagsFound []string

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		tagName := match[1] // e.g., "modelName"
		// Check if this tag is in the allowed map for the given pattern context
		if _, isAllowed := allowedTags[tagName]; !isAllowed {
			disallowedTagsFound = append(disallowedTagsFound, tagName)
		}
	}
	return disallowedTagsFound
}

// performPathPatternValidation checks all relevant path patterns in the configuration.
func performPathPatternValidation(cfg *models.Config) error {
	// Validate ModelInfoPathPattern
	disallowedInModelInfo := validatePathPattern(cfg.Download.ModelInfoPathPattern, modelLevelAllowedTags, "ModelInfoPathPattern")
	if len(disallowedInModelInfo) > 0 {
		for _, tag := range disallowedInModelInfo {
			if tag == "baseModel" {
				log.Warnf("[Config Validation] ModelInfoPathPattern contains '{%s}'. This placeholder is ambiguous at the model level and will resolve to 'unknown_basemodel'. Consider removing it for clarity unless this is intended.", tag)
			} else if _, isVersionTag := versionLevelAllowedTags[tag]; isVersionTag && tag != "baseModel" {
				// It's a version-specific tag other than baseModel
				log.Warnf("[Config Validation] ModelInfoPathPattern contains version-specific tag '{%s}'. This tag will likely resolve to an 'empty_%s' or 'unknown_%s' segment as version context is not available for this pattern. Consider removing it.", tag, tag, tag)
			} else {
				log.Warnf("[Config Validation] ModelInfoPathPattern contains potentially problematic tag '{%s}'. This tag may not resolve as expected in a model-level context.", tag)
			}
		}
	}

	// Validate VersionPathPattern
	// All tags in versionLevelAllowedTags are generally fine here. This check is more for unknown/mistyped tags.
	disallowedInVersionPath := validatePathPattern(cfg.Download.VersionPathPattern, versionLevelAllowedTags, "VersionPathPattern")
	if len(disallowedInVersionPath) > 0 {
		log.Warnf("[Config Validation] VersionPathPattern contains unexpected or disallowed tags: %v. Please review your pattern. Allowed version-level tags are: modelId, modelName, modelType, creatorName, versionId, versionName, baseModel.", disallowedInVersionPath)
	}

	// TODO: Add validation for TrainedWordsPathPattern if it also has specific context needs
	// TODO: Add validation for Images.PathPattern and Images.SubfolderPattern

	return nil
}

// --- Path Pattern Validation --- END ---
