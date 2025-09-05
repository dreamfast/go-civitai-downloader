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

// initializeDefaults creates a Config with sensible default values
func initializeDefaults() models.Config {
	return models.Config{
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
}

// setupViper initializes Viper with environment variable settings and defaults
func setupViper(flags CliFlags) *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix("CIVITAI")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set defaults using Viper as well, so they are part of the hierarchy
	setViperDefaults(v)
	log.Debugf("[setupViper] Viper defaults set")

	// Determine config file path
	actualConfigFilePath := DefaultConfigFilePath
	if flags.ConfigFilePath != nil {
		actualConfigFilePath = *flags.ConfigFilePath
		log.Debugf("[setupViper] Using config file path from CLI flag: %s", actualConfigFilePath)
	} else {
		log.Debugf("[setupViper] Using default config file path: %s", actualConfigFilePath)
	}
	v.SetConfigFile(actualConfigFilePath)
	return v
}

// readConfigFile reads the configuration file and unmarshals it into the provided config
func readConfigFile(v *viper.Viper, finalCfg *models.Config) error {
	// Attempt to read the config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Warnf("[readConfigFile] Config file not found. Using defaults and CLI flags only.")
		} else {
			log.Warnf("[readConfigFile] Error reading config file: %v. Using defaults and CLI flags only.", err)
		}
		// Even if file read fails, proceed to unmarshal. Viper will use defaults for missing keys/file.
	} else {
		log.Infof("[readConfigFile] Successfully read config file: %s", v.ConfigFileUsed())
	}

	// Unmarshal Viper data (defaults + file if read) into the config struct.
	// This MUST happen regardless of whether ReadInConfig succeeded or not, to apply Viper's defaults.
	if err := v.Unmarshal(finalCfg); err != nil {
		log.Errorf("[readConfigFile] Failed to unmarshal config from Viper: %v", err)
		return fmt.Errorf("failed to unmarshal config from viper: %w", err)
	}
	log.Debugf("[readConfigFile] After attempting file read and unmarshalling. cfg.Download: %+v", finalCfg.Download)
	return nil
}

// Initialize loads configuration based on defaults, config file, and flags.
// Precedence: Flags > Config File > Defaults.
func Initialize(flags CliFlags) (models.Config, http.RoundTripper, error) {
	// --- 1. Establish Defaults ---
	finalCfg := initializeDefaults()

	log.Debugf("[Initialize] Applying default values. Current cfg.Download: %+v", finalCfg.Download)

	// --- 2. Setup and read configuration file ---
	v := setupViper(flags)
	if err := readConfigFile(v, &finalCfg); err != nil {
		return models.Config{}, nil, err
	}

	log.Debugf("[Initialize] After attempting file read and unmarshalling. cfg.Download: %+v", finalCfg.Download)
	log.Debugf("[Initialize] Specifically, after unmarshal: Query='%s', ModelTypes=%v, Limit=%d, Nsfw=%t, Sort='%s', Period='%s'", finalCfg.Download.Query, finalCfg.Download.ModelTypes, finalCfg.Download.Limit, finalCfg.Download.Nsfw, finalCfg.Download.Sort, finalCfg.Download.Period)

	// --- 3. Override with CLI Flags ---
	log.Debugf("[Initialize] About to override with CLI flags. Current cfg.Download (after Viper unmarshal): %+v", finalCfg.Download)

	applyGlobalFlags(&finalCfg, flags)
	applyDownloadFlags(&finalCfg, flags)
	applyImagesFlags(&finalCfg, flags)
	applyTorrentFlags(&finalCfg, flags)
	applyDBFlags(&finalCfg, flags)

	// --- 4. Derive Default Paths if Empty ---
	deriveDefaultPaths(&finalCfg)

	// --- 5. Validation ---
	if err := validateConfig(&finalCfg); err != nil {
		return models.Config{}, nil, err
	}

	// --- 6. Setup HTTP Transport ---
	finalTransport, err := setupHTTPTransport(&finalCfg)
	if err != nil {
		return models.Config{}, nil, err
	}

	log.Debugf("[Initialize] Final merged cfg.Download before returning: %+v", finalCfg.Download)
	log.Debugf("[Initialize] Specifically, final: Query='%s', ModelTypes=%v, Limit=%d, Nsfw=%t, Sort='%s', Period='%s'", finalCfg.Download.Query, finalCfg.Download.ModelTypes, finalCfg.Download.Limit, finalCfg.Download.Nsfw, finalCfg.Download.Sort, finalCfg.Download.Period)

	if err := performPathPatternValidation(&finalCfg); err != nil {
		log.Errorf("Path pattern validation failed: %v", err)
	}

	log.Debug("Configuration initialized successfully.")
	return finalCfg, finalTransport, nil
}

// applyGlobalFlags applies global-level CLI flags to the configuration
func applyGlobalFlags(cfg *models.Config, flags CliFlags) {
	if flags.APIKey != nil {
		log.Debugf("[Initialize] Overriding APIKey from flag.")
		cfg.APIKey = *flags.APIKey
	}
	if flags.SavePath != nil {
		log.Debugf("[Initialize] Overriding SavePath from flag: '%s'", *flags.SavePath)
		cfg.SavePath = *flags.SavePath
	}
	if flags.LogApiRequests != nil {
		log.Debugf("[Initialize] Overriding LogApiRequests from flag: %v", *flags.LogApiRequests)
		cfg.LogApiRequests = *flags.LogApiRequests
	}
	if flags.APIDelayMs != nil {
		log.Debugf("[Initialize] Overriding APIDelayMs from flag: %d", *flags.APIDelayMs)
		cfg.APIDelayMs = *flags.APIDelayMs
	}
	if flags.APIClientTimeoutSec != nil {
		log.Debugf("[Initialize] Overriding APIClientTimeoutSec from flag: %d", *flags.APIClientTimeoutSec)
		cfg.APIClientTimeoutSec = *flags.APIClientTimeoutSec
	}
	if flags.MaxRetries != nil {
		log.Debugf("[Initialize] Overriding MaxRetries from flag: %d", *flags.MaxRetries)
		cfg.MaxRetries = *flags.MaxRetries
	}
	if flags.InitialRetryDelayMs != nil {
		log.Debugf("[Initialize] Overriding InitialRetryDelayMs from flag: %d", *flags.InitialRetryDelayMs)
		cfg.InitialRetryDelayMs = *flags.InitialRetryDelayMs
	}
	if flags.LogLevel != nil {
		log.Debugf("[Initialize] Overriding LogLevel from flag: '%s'", *flags.LogLevel)
		cfg.LogLevel = *flags.LogLevel
	}
	if flags.LogFormat != nil {
		log.Debugf("[Initialize] Overriding LogFormat from flag: '%s'", *flags.LogFormat)
		cfg.LogFormat = *flags.LogFormat
	}
}

// applyDownloadFlags applies download-specific CLI flags to the configuration
func applyDownloadFlags(cfg *models.Config, flags CliFlags) {
	if flags.Download == nil {
		return
	}

	log.Debugf("[Initialize] Processing Download CLI flags: %+v", *flags.Download)

	if flags.Download.Concurrency != nil {
		cfg.Download.Concurrency = *flags.Download.Concurrency
		log.Debugf("[Initialize] CLI Override: Download.Concurrency = %d", cfg.Download.Concurrency)
	}
	if flags.Download.Tag != nil {
		cfg.Download.Tag = *flags.Download.Tag
		log.Debugf("[Initialize] CLI Override: Download.Tag = '%s'", cfg.Download.Tag)
	}
	if flags.Download.Query != nil {
		cfg.Download.Query = *flags.Download.Query
		log.Debugf("[Initialize] CLI Override: Download.Query = '%s'", cfg.Download.Query)
	}
	if flags.Download.ModelTypes != nil && len(*flags.Download.ModelTypes) > 0 {
		cfg.Download.ModelTypes = *flags.Download.ModelTypes
		log.Debugf("[Initialize] CLI Override: Download.ModelTypes = %v", cfg.Download.ModelTypes)
	}
	if flags.Download.BaseModels != nil && len(*flags.Download.BaseModels) > 0 {
		cfg.Download.BaseModels = *flags.Download.BaseModels
		log.Debugf("[Initialize] CLI Override: Download.BaseModels = %v", cfg.Download.BaseModels)
	}
	if flags.Download.Username != nil {
		cfg.Download.Usernames = []string{*flags.Download.Username}
		log.Debugf("[Initialize] CLI Override: Download.Usernames = %v (from single username flag)", cfg.Download.Usernames)
	}
	if flags.Download.Nsfw != nil {
		cfg.Download.Nsfw = *flags.Download.Nsfw
		log.Debugf("[Initialize] CLI Override: Download.Nsfw = %t", cfg.Download.Nsfw)
	}
	if flags.Download.Limit != nil {
		cfg.Download.Limit = *flags.Download.Limit
		log.Debugf("[Initialize] CLI Override: Download.Limit = %d", cfg.Download.Limit)
	}
	if flags.Download.MaxPages != nil {
		cfg.Download.MaxPages = *flags.Download.MaxPages
		log.Debugf("[Initialize] CLI Override: Download.MaxPages = %d", cfg.Download.MaxPages)
	}
	if flags.Download.Sort != nil {
		cfg.Download.Sort = *flags.Download.Sort
		log.Debugf("[Initialize] CLI Override: Download.Sort = '%s'", cfg.Download.Sort)
	}
	if flags.Download.Period != nil {
		cfg.Download.Period = *flags.Download.Period
		log.Debugf("[Initialize] CLI Override: Download.Period = '%s'", cfg.Download.Period)
	}
	if flags.Download.ModelID != nil {
		cfg.Download.ModelID = *flags.Download.ModelID
		log.Debugf("[Initialize] CLI Override: Download.ModelID = %d", cfg.Download.ModelID)
	}
	if flags.Download.ModelVersionID != nil {
		cfg.Download.ModelVersionID = *flags.Download.ModelVersionID
		log.Debugf("[Initialize] CLI Override: Download.ModelVersionID = %d", cfg.Download.ModelVersionID)
	}
	if flags.Download.PrimaryOnly != nil {
		cfg.Download.PrimaryOnly = *flags.Download.PrimaryOnly
		log.Debugf("[Initialize] CLI Override: Download.PrimaryOnly = %t", cfg.Download.PrimaryOnly)
	}
	if flags.Download.Pruned != nil {
		cfg.Download.Pruned = *flags.Download.Pruned
		log.Debugf("[Initialize] CLI Override: Download.Pruned = %t", cfg.Download.Pruned)
	}
	if flags.Download.Fp16 != nil {
		cfg.Download.Fp16 = *flags.Download.Fp16
		log.Debugf("[Initialize] CLI Override: Download.Fp16 = %t", cfg.Download.Fp16)
	}
	if flags.Download.AllVersions != nil {
		cfg.Download.AllVersions = *flags.Download.AllVersions
		log.Debugf("[Initialize] CLI Override: Download.AllVersions = %t", cfg.Download.AllVersions)
	}
	if flags.Download.IgnoreBaseModels != nil {
		cfg.Download.IgnoreBaseModels = *flags.Download.IgnoreBaseModels
		log.Debugf("[Initialize] CLI Override: Download.IgnoreBaseModels = %v", cfg.Download.IgnoreBaseModels)
	}
	if flags.Download.IgnoreFileNameStrings != nil {
		cfg.Download.IgnoreFileNameStrings = *flags.Download.IgnoreFileNameStrings
		log.Debugf("[Initialize] CLI Override: Download.IgnoreFileNameStrings = %v", cfg.Download.IgnoreFileNameStrings)
	}
	if flags.Download.SkipConfirmation != nil {
		cfg.Download.SkipConfirmation = *flags.Download.SkipConfirmation
		log.Debugf("[Initialize] CLI Override: Download.SkipConfirmation = %t", cfg.Download.SkipConfirmation)
	}
	if flags.Download.SaveMetadata != nil {
		cfg.Download.SaveMetadata = *flags.Download.SaveMetadata
		log.Debugf("[Initialize] CLI Override: Download.SaveMetadata = %t", cfg.Download.SaveMetadata)
	}
	if flags.Download.SaveModelInfo != nil {
		cfg.Download.SaveModelInfo = *flags.Download.SaveModelInfo
		log.Debugf("[Initialize] CLI Override: Download.SaveModelInfo = %t", cfg.Download.SaveModelInfo)
	}
	if flags.Download.SaveVersionImages != nil {
		cfg.Download.SaveVersionImages = *flags.Download.SaveVersionImages
		log.Debugf("[Initialize] CLI Override: Download.SaveVersionImages = %t", cfg.Download.SaveVersionImages)
	}
	if flags.Download.SaveModelImages != nil {
		cfg.Download.SaveModelImages = *flags.Download.SaveModelImages
		log.Debugf("[Initialize] CLI Override: Download.SaveModelImages = %t", cfg.Download.SaveModelImages)
	}
	if flags.Download.DownloadMetaOnly != nil {
		cfg.Download.DownloadMetaOnly = *flags.Download.DownloadMetaOnly
		log.Debugf("[Initialize] CLI Override: Download.DownloadMetaOnly = %t", cfg.Download.DownloadMetaOnly)
	}
}

// applyImagesFlags applies images-specific CLI flags to the configuration
func applyImagesFlags(cfg *models.Config, flags CliFlags) {
	if flags.Images == nil {
		return
	}

	log.Debug("[Config Init] Applying Images flags...")

	if flags.Images.Limit != nil {
		cfg.Images.Limit = *flags.Images.Limit
	}
	if flags.Images.PostID != nil {
		cfg.Images.PostID = *flags.Images.PostID
	}
	if flags.Images.ModelID != nil {
		cfg.Images.ModelID = *flags.Images.ModelID
	}
	if flags.Images.ModelVersionID != nil {
		cfg.Images.ModelVersionID = *flags.Images.ModelVersionID
	}
	if flags.Images.Username != nil {
		cfg.Images.Username = *flags.Images.Username
	}
	if flags.Images.Nsfw != nil {
		log.Warnf("[Config Init] Skipping Images.Nsfw flag override due to type mismatch (string vs bool). Flag value: %s", *flags.Images.Nsfw)
	}
	if flags.Images.Sort != nil {
		cfg.Images.Sort = *flags.Images.Sort
	}
	if flags.Images.Period != nil {
		cfg.Images.Period = *flags.Images.Period
	}
	if flags.Images.Page != nil {
		cfg.Images.Page = *flags.Images.Page
	}
	if flags.Images.MaxPages != nil {
		log.Warnf("[Config Init] Images flag --max-pages ignored, no corresponding config field.")
	}
	if flags.Images.OutputDir != nil {
		cfg.Images.OutputDir = *flags.Images.OutputDir
	}
	if flags.Images.Concurrency != nil {
		cfg.Images.Concurrency = *flags.Images.Concurrency
	}
	if flags.Images.SaveMetadata != nil {
		cfg.Images.SaveMetadata = *flags.Images.SaveMetadata
	}
}

// applyTorrentFlags applies torrent-specific CLI flags to the configuration
func applyTorrentFlags(cfg *models.Config, flags CliFlags) {
	if flags.Torrent == nil {
		return
	}

	log.Debug("[Config Init] Applying Torrent flags...")

	if flags.Torrent.OutputDir != nil {
		cfg.Torrent.OutputDir = *flags.Torrent.OutputDir
	}
	if flags.Torrent.Overwrite != nil {
		cfg.Torrent.Overwrite = *flags.Torrent.Overwrite
	}
	if flags.Torrent.MagnetLinks != nil {
		cfg.Torrent.MagnetLinks = *flags.Torrent.MagnetLinks
	}
	if flags.Torrent.Concurrency != nil {
		cfg.Torrent.Concurrency = *flags.Torrent.Concurrency
	}
}

// applyDBFlags applies database-specific CLI flags to the configuration
func applyDBFlags(cfg *models.Config, flags CliFlags) {
	if flags.DB == nil || flags.DB.Verify == nil {
		return
	}

	log.Debug("[Config Init] Applying DB Verify flags...")

	if flags.DB.Verify.CheckHash != nil {
		cfg.DB.Verify.CheckHash = *flags.DB.Verify.CheckHash
	}
	if flags.DB.Verify.AutoRedownload != nil {
		cfg.DB.Verify.AutoRedownload = *flags.DB.Verify.AutoRedownload
	}
}

// deriveDefaultPaths derives default paths based on the SavePath
func deriveDefaultPaths(cfg *models.Config) {
	defaultDbPath := filepath.Join(cfg.SavePath, "civitai.db")
	if cfg.DatabasePath == "" || cfg.DatabasePath == defaultDbPath {
		cfg.DatabasePath = filepath.Join(cfg.SavePath, "civitai.db")
		log.Debugf("[Config Init] DatabasePath defaulted based on final SavePath: %s", cfg.DatabasePath)
	} else {
		log.Debugf("[Config Init] DatabasePath ('%s') was explicitly set or derived from a non-default SavePath, not changing.", cfg.DatabasePath)
	}
}

// validateConfig validates the final configuration
func validateConfig(cfg *models.Config) error {
	if cfg.SavePath == "" {
		return fmt.Errorf("SavePath cannot be empty (set via --save-path flag or SavePath in config)")
	}
	return nil
}

// setupHTTPTransport sets up the HTTP transport with optional logging
func setupHTTPTransport(cfg *models.Config) (http.RoundTripper, error) {
	baseTransport := http.DefaultTransport
	var finalTransport http.RoundTripper = baseTransport

	if cfg.LogApiRequests {
		log.Debug("API request logging enabled.")
		logFilePath := "api.log"
		if cfg.SavePath != "" {
			if _, statErr := os.Stat(cfg.SavePath); statErr == nil {
				logFilePath = filepath.Join(cfg.SavePath, logFilePath)
			} else {
				log.Warnf("SavePath '%s' not found, saving api.log to current directory.", cfg.SavePath)
			}
		} else {
			log.Warnf("SavePath is empty, saving api.log to current directory.")
		}
		log.Infof("API logging to file: %s", logFilePath)

		loggingTransport, err := api.NewLoggingTransport(baseTransport, logFilePath)
		if err != nil {
			log.WithError(err).Error("Failed to initialize API logging transport, logging disabled.")
		} else {
			finalTransport = loggingTransport
		}
	}

	return finalTransport, nil
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
