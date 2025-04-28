package models

import (
	"net/url"
	"strconv"
)

type (
	// Config holds the application's configuration settings.
	Config struct {
		// Global settings
		APIKey              string `toml:"ApiKey" json:"ApiKey"`
		SavePath            string `toml:"SavePath" json:"SavePath"`
		DatabasePath        string `toml:"DatabasePath" json:"DatabasePath"`
		BleveIndexPath      string `toml:"BleveIndexPath" json:"BleveIndexPath"`
		LogApiRequests      bool   `toml:"LogApiRequests" json:"LogApiRequests"`
		LogLevel            string `toml:"LogLevel" json:"LogLevel"`
		LogFormat           string `toml:"LogFormat" json:"LogFormat"`
		APIDelayMs          int    `toml:"ApiDelayMs" json:"ApiDelayMs"`
		APIClientTimeoutSec int    `toml:"ApiClientTimeoutSec" json:"ApiClientTimeoutSec"`
		MaxRetries          int    `toml:"MaxRetries" json:"MaxRetries"`
		InitialRetryDelayMs int    `toml:"InitialRetryDelayMs" json:"InitialRetryDelayMs"`

		Download DownloadConfig `toml:"Download" json:"Download"`
		Images   ImagesConfig   `toml:"Images" json:"Images"`
		Torrent  TorrentConfig  `toml:"Torrent" json:"Torrent"`
		DB       DBConfig       `toml:"DB" json:"DB"`
	}

	// DownloadConfig holds settings specific to the 'download' command.
	DownloadConfig struct {
		Concurrency           int      `toml:"Concurrency"`
		Tag                   string   `toml:"Tag"` // Using single tag based on current flag/API
		Query                 string   `toml:"Query"`
		ModelTypes            []string `toml:"ModelTypes"`
		BaseModels            []string `toml:"BaseModels"`
		Usernames             []string `toml:"Usernames"` // Mismatch with single --username flag noted in analysis
		Nsfw                  bool     `toml:"Nsfw"`
		Limit                 int      `toml:"Limit"`
		MaxPages              int      `toml:"MaxPages"`
		Sort                  string   `toml:"Sort"`
		Period                string   `toml:"Period"`
		ModelVersionID        int      `toml:"ModelVersionID"`
		PrimaryOnly           bool     `toml:"PrimaryOnly"`
		Pruned                bool     `toml:"Pruned"`
		Fp16                  bool     `toml:"Fp16"`
		AllVersions           bool     `toml:"AllVersions"`
		IgnoreBaseModels      []string `toml:"IgnoreBaseModels"`
		IgnoreFileNameStrings []string `toml:"IgnoreFileNameStrings"`
		SkipConfirmation      bool     `toml:"SkipConfirmation"`
		SaveMetadata          bool     `toml:"SaveMetadata"`
		SaveModelInfo         bool     `toml:"ModelInfo"`
		SaveVersionImages     bool     `toml:"VersionImages"`
		SaveModelImages       bool     `toml:"ModelImages"`
		DownloadMetaOnly      bool     `toml:"MetaOnly"`

		// Fields corresponding to flags without direct config.toml entries
		ModelID int `toml:"-"` // Flag only (`--model-id`)
	}

	// ImagesConfig holds settings specific to the 'images' command.
	// Added to config for potential future use, primarily driven by flags now.
	ImagesConfig struct {
		Limit          int    `toml:"Limit"`
		PostID         int    `toml:"PostID"`
		ModelID        int    `toml:"ModelID"`
		ModelVersionID int    `toml:"ModelVersionID"`
		Username       string `toml:"Username"`
		Nsfw           string `toml:"Nsfw"`
		Sort           string `toml:"Sort"`
		Period         string `toml:"Period"`
		Page           int    `toml:"Page"`
		MaxPages       int    `toml:"MaxPages"`
		OutputDir      string `toml:"OutputDir"`
		Concurrency    int    `toml:"Concurrency"`
		SaveMetadata   bool   `toml:"Metadata"`
	}

	// TorrentConfig holds settings specific to the 'torrent' command.
	// Added to config for potential future use, primarily driven by flags now.
	TorrentConfig struct {
		OutputDir   string `toml:"OutputDir"`
		Overwrite   bool   `toml:"Overwrite"`
		MagnetLinks bool   `toml:"MagnetLinks"`
		Concurrency int    `toml:"Concurrency"` // Separate from Download.Concurrency
	}

	// DBConfig holds settings specific to the 'db' command group.
	DBConfig struct {
		Verify DBVerifyConfig `toml:"Verify"`
	}

	// DBVerifyConfig holds settings for the 'db verify' subcommand.
	// Added to config for potential future use, primarily driven by flags now.
	DBVerifyConfig struct {
		CheckHash      bool `toml:"CheckHash"`
		AutoRedownload bool `toml:"AutoRedownload"` // Corresponds to --yes flag
	}

	// Api Calls and Responses
	QueryParameters struct {
		Limit                  int      `json:"limit"`
		Page                   int      `json:"page,omitempty"`
		Query                  string   `json:"query,omitempty"`
		Tag                    string   `json:"tag,omitempty"`
		Username               string   `json:"username,omitempty"`
		Types                  []string `json:"types,omitempty"`
		Sort                   string   `json:"sort"`
		Period                 string   `json:"period"`
		PrimaryFileOnly        bool     `json:"primaryFileOnly,omitempty"`
		AllowNoCredit          bool     `json:"allowNoCredit,omitempty"`
		AllowDerivatives       bool     `json:"allowDerivatives,omitempty"`
		AllowDifferentLicenses bool     `json:"allowDifferentLicenses,omitempty"`
		AllowCommercialUse     string   `json:"allowCommercialUse,omitempty"`
		Nsfw                   bool     `json:"nsfw"`
		BaseModels             []string `json:"baseModels,omitempty"`
		Cursor                 string   `json:"cursor,omitempty"`
	}

	Model struct {
		ID                    int            `json:"id"`
		Name                  string         `json:"name"`
		Description           string         `json:"description"`
		Type                  string         `json:"type"`
		Poi                   bool           `json:"poi"`
		Nsfw                  bool           `json:"nsfw"`
		AllowNoCredit         bool           `json:"allowNoCredit"`
		AllowCommercialUse    []string       `json:"allowCommercialUse"`
		AllowDerivatives      bool           `json:"allowDerivatives"`
		AllowDifferentLicense bool           `json:"allowDifferentLicense"`
		Stats                 Stats          `json:"stats"`
		Creator               Creator        `json:"creator"`
		Tags                  []string       `json:"tags"`
		ModelVersions         []ModelVersion `json:"modelVersions"`
		Meta                  interface{}    `json:"meta"` // Meta can be null or an object, so we use interface{}
	}

	Stats struct {
		DownloadCount int     `json:"downloadCount"`
		FavoriteCount int     `json:"favoriteCount"`
		CommentCount  int     `json:"commentCount"`
		RatingCount   int     `json:"ratingCount"`
		Rating        float64 `json:"rating"`
	}

	Creator struct {
		Username string `json:"username"`
		Image    string `json:"image"`
	}

	// --- NEW: Struct for nested 'model' field in /model-versions/{id} response ---
	BaseModelInfo struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Nsfw bool   `json:"nsfw"`
		Poi  bool   `json:"poi"`
		Mode string `json:"mode"` // Can be null, "Archived", "TakenDown"
	}

	ModelVersion struct {
		ID                   int          `json:"id"`
		ModelId              int          `json:"modelId"`
		Name                 string       `json:"name"`
		PublishedAt          string       `json:"publishedAt"`
		UpdatedAt            string       `json:"updatedAt"`
		TrainedWords         []string     `json:"trainedWords"`
		BaseModel            string       `json:"baseModel"`
		EarlyAccessTimeFrame int          `json:"earlyAccessTimeFrame"`
		Description          string       `json:"description"`
		Stats                Stats        `json:"stats"`
		Files                []File       `json:"files"`
		Images               []ModelImage `json:"images"`
		DownloadUrl          string       `json:"downloadUrl"`
		// --- ADDED: Nested model info from /model-versions/{id} endpoint ---
		Model BaseModelInfo `json:"model"`
	}

	File struct {
		Name              string   `json:"name"`
		ID                int      `json:"id"`
		SizeKB            float64  `json:"sizeKB"`
		Type              string   `json:"type"`
		Metadata          Metadata `json:"metadata"`
		PickleScanResult  string   `json:"pickleScanResult"`
		PickleScanMessage string   `json:"pickleScanMessage"`
		VirusScanResult   string   `json:"virusScanResult"`
		ScannedAt         string   `json:"scannedAt"`
		Hashes            Hashes   `json:"hashes"`
		DownloadUrl       string   `json:"downloadUrl"`
		Primary           bool     `json:"primary"`
	}

	Metadata struct {
		Fp     string `json:"fp"`
		Size   string `json:"size"`
		Format string `json:"format"`
	}

	Hashes struct {
		AutoV2 string `json:"AutoV2"`
		SHA256 string `json:"SHA256"`
		CRC32  string `json:"CRC32"`
		BLAKE3 string `json:"BLAKE3"`
	}

	ModelImage struct {
		ID        int         `json:"id"`
		URL       string      `json:"url"`
		Hash      string      `json:"hash"` // Blurhash
		Width     int         `json:"width"`
		Height    int         `json:"height"`
		Nsfw      bool        `json:"nsfw"`      // Keep boolean for simplicity, align with Model struct Nsfw
		NsfwLevel interface{} `json:"nsfwLevel"` // Changed to interface{} to handle number OR string from API
		CreatedAt string      `json:"createdAt"` // Consider parsing to time.Time if needed
		PostID    *int        `json:"postId"`    // Use pointer for optional field
		Stats     ImageStats  `json:"stats"`
		Meta      interface{} `json:"meta"` // Often unstructured JSON, use interface{}
		Username  string      `json:"username"`
	}

	ImageStats struct {
		CryCount     int `json:"cryCount"`
		LaughCount   int `json:"laughCount"`
		LikeCount    int `json:"likeCount"`
		HeartCount   int `json:"heartCount"`
		CommentCount int `json:"commentCount"`
	}

	ApiResponse struct { // Renamed from Response
		Items    []Model            `json:"items"`
		Metadata PaginationMetadata `json:"metadata"` // Added field for pagination
	}

	// Added struct for pagination metadata based on API docs
	PaginationMetadata struct {
		TotalItems  int    `json:"totalItems"`
		CurrentPage int    `json:"currentPage"`
		PageSize    int    `json:"pageSize"`
		TotalPages  int    `json:"totalPages"`
		NextPage    string `json:"nextPage"`
		PrevPage    string `json:"prevPage"`   // Added based on API docs
		NextCursor  string `json:"nextCursor"` // Added based on API docs (for images endpoint mainly)
	}

	// Internal file db entry for each model
	DatabaseEntry struct {
		ModelID      int          `json:"modelId"`
		ModelName    string       `json:"modelName"`
		ModelType    string       `json:"modelType"`
		Version      ModelVersion `json:"version"`
		File         File         `json:"file"`
		Timestamp    int64        `json:"timestamp"`
		Creator      Creator      `json:"creator"`
		Filename     string       `json:"filename"`
		Folder       string       `json:"folder"`
		Status       string       `json:"status"`
		ErrorDetails string       `json:"errorDetails,omitempty"`
	}

	// --- Start: /api/v1/images Endpoint Structures ---

	// ImageApiResponse represents the structure of the response from the /api/v1/images endpoint.
	ImageApiResponse struct {
		Items    []ImageApiItem     `json:"items"` // Renamed Image -> ImageApiItem to avoid conflict
		Metadata PaginationMetadata `json:"metadata"`
	}

	// ImageApiItem represents a single image item specifically from the /api/v1/images response.
	ImageApiItem struct {
		ID     int    `json:"id"`
		URL    string `json:"url"`
		Hash   string `json:"hash"` // Blurhash
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
)

// Database Status Constants
const (
	StatusPending    = "Pending"
	StatusDownloaded = "Downloaded"
	StatusError      = "Error"
)

// ConstructApiUrl builds the Civitai API URL from query parameters.
func ConstructApiUrl(params QueryParameters) string {
	base := "https://civitai.com/api/v1/models"
	values := url.Values{}

	// Add parameters if they have non-default values
	if params.Limit > 0 && params.Limit <= 100 { // Use API default if not set or invalid
		values.Set("limit", strconv.Itoa(params.Limit))
	} else {
		// Let the API use its default limit (usually 100)
	}

	if params.Page > 0 { // Page is only relevant for non-cursor pagination (less common now)
		// values.Set("page", strconv.Itoa(params.Page))
		// Generally, Cursor should be preferred over Page.
	}

	if params.Query != "" {
		values.Set("query", params.Query)
	}

	if params.Tag != "" {
		values.Set("tag", params.Tag)
	}

	if params.Username != "" {
		values.Set("username", params.Username)
	}

	for _, t := range params.Types {
		values.Add("types", t)
	}

	if params.Sort != "" {
		values.Set("sort", params.Sort)
	}

	if params.Period != "" {
		values.Set("period", params.Period)
	}

	if !params.AllowNoCredit { // Default is true, so only add if false
		values.Set("allowNoCredit", "false")
	}

	if !params.AllowDerivatives { // Default is true
		values.Set("allowDerivatives", "false")
	}

	if !params.AllowDifferentLicenses { // Default is true
		values.Set("allowDifferentLicense", "false") // API uses singular 'License'
	}

	if params.AllowCommercialUse != "Any" && params.AllowCommercialUse != "" { // Default is Any
		values.Set("allowCommercialUse", params.AllowCommercialUse)
	}

	// Only add nsfw param if true
	if params.Nsfw {
		values.Set("nsfw", "true")
	}

	for _, bm := range params.BaseModels {
		values.Add("baseModels", bm) // API uses camelCase
	}

	if params.Cursor != "" {
		values.Set("cursor", params.Cursor)
	}

	queryString := values.Encode()
	if queryString != "" {
		return base + "?" + queryString
	}
	return base
}
