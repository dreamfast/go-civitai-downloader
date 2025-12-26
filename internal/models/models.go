package models

import (
	"encoding/json"
	"net/url"
	"strconv"
)

// StringOrStringSlice is a custom type that can unmarshal from either
// a JSON string or a JSON array of strings. This handles API responses
// where a field may return either format.
type StringOrStringSlice []string

// UnmarshalJSON implements json.Unmarshaler for StringOrStringSlice
func (s *StringOrStringSlice) UnmarshalJSON(data []byte) error {
	// First try to unmarshal as a string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = []string{str}
		return nil
	}

	// If that fails, try to unmarshal as an array of strings
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	*s = arr
	return nil
}

type (
	// Config holds the application's configuration settings.
	Config struct {
		SavePath            string         `toml:"SavePath" json:"SavePath"`
		DatabasePath        string         `toml:"DatabasePath" json:"DatabasePath"`
		BleveIndexPath      string         `toml:"BleveIndexPath" json:"BleveIndexPath"`
		LogLevel            string         `toml:"LogLevel" json:"LogLevel"`
		LogFormat           string         `toml:"LogFormat" json:"LogFormat"`
		APIKey              string         `toml:"ApiKey" json:"ApiKey"`
		SessionCookie       string         `toml:"SessionCookie" json:"SessionCookie"` // Browser session cookie for login-required downloads
		Torrent             TorrentConfig  `toml:"Torrent" json:"Torrent"`
		Download            DownloadConfig `toml:"Download" json:"Download"`
		Images              ImagesConfig   `toml:"Images" json:"Images"`
		APIDelayMs          int            `toml:"ApiDelayMs" json:"ApiDelayMs"`
		APIClientTimeoutSec int            `toml:"ApiClientTimeoutSec" json:"ApiClientTimeoutSec"`
		MaxRetries          int            `toml:"MaxRetries" json:"MaxRetries"`
		InitialRetryDelayMs int            `toml:"InitialRetryDelayMs" json:"InitialRetryDelayMs"`
		DB                  DBConfig       `toml:"DB" json:"DB"`
		LogApiRequests      bool           `toml:"LogApiRequests" json:"LogApiRequests"`
	}

	// DownloadConfig holds settings specific to the 'download' command.
	DownloadConfig struct {
		// Strings first
		Tag                  string `toml:"Tag"`
		Query                string `toml:"Query"`
		Sort                 string `toml:"Sort"`
		Period               string `toml:"Period"`
		VersionPathPattern   string `toml:"VersionPathPattern"`
		ModelInfoPathPattern string `toml:"ModelInfoPathPattern"`
		// Slices (largest items)
		ModelTypes            []string `toml:"ModelTypes"`
		BaseModels            []string `toml:"BaseModels"`
		Usernames             []string `toml:"Usernames"`
		IgnoreBaseModels      []string `toml:"IgnoreBaseModels"`
		IgnoreFileNameStrings []string `toml:"IgnoreFileNameStrings"`
		// Integers
		Concurrency    int `toml:"Concurrency"`
		Limit          int `toml:"Limit"`
		MaxPages       int `toml:"MaxPages"`
		ModelVersionID int `toml:"ModelVersionID"`
		ModelID        int `toml:"-"` // Flag only (`--model-id`)
		// Bools (smallest)
		Nsfw              bool `toml:"Nsfw"`
		PrimaryOnly       bool `toml:"PrimaryOnly"`
		Pruned            bool `toml:"Pruned"`
		Fp16              bool `toml:"Fp16"`
		AllVersions       bool `toml:"AllVersions"`
		SkipConfirmation  bool `toml:"SkipConfirmation"`
		SaveMetadata      bool `toml:"SaveMetadata"`
		SaveModelInfo     bool `toml:"ModelInfo"`
		SaveVersionImages bool `toml:"VersionImages"`
		SaveModelImages   bool `toml:"ModelImages"`
		DownloadMetaOnly  bool `toml:"MetaOnly"`
	}

	// ImagesConfig holds settings specific to the 'images' command.
	// Added to config for potential future use, primarily driven by flags now.
	ImagesConfig struct {
		// Strings first
		Username    string `toml:"Username"`
		Nsfw        string `toml:"Nsfw"`
		Sort        string `toml:"Sort"`
		Period      string `toml:"Period"`
		OutputDir   string `toml:"OutputDir"`
		PathPattern string `toml:"PathPattern"`
		// Integers
		Limit          int `toml:"Limit"`
		PostID         int `toml:"PostID"`
		ModelID        int `toml:"ModelID"`
		ModelVersionID int `toml:"ModelVersionID"`
		Page           int `toml:"Page"`
		MaxPages       int `toml:"MaxPages"`
		Concurrency    int `toml:"Concurrency"`
		// Bools
		SaveMetadata bool `toml:"Metadata"`
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
		Cursor                 string   `json:"cursor,omitempty"`
		Tag                    string   `json:"tag,omitempty"`
		Username               string   `json:"username,omitempty"`
		Sort                   string   `json:"sort"`
		Period                 string   `json:"period"`
		AllowCommercialUse     string   `json:"allowCommercialUse,omitempty"`
		Query                  string   `json:"query,omitempty"`
		Types                  []string `json:"types,omitempty"`
		BaseModels             []string `json:"baseModels,omitempty"`
		Limit                  int      `json:"limit"`
		Page                   int      `json:"page,omitempty"`
		PrimaryFileOnly        bool     `json:"primaryFileOnly,omitempty"`
		AllowNoCredit          bool     `json:"allowNoCredit,omitempty"`
		AllowDerivatives       bool     `json:"allowDerivatives,omitempty"`
		AllowDifferentLicenses bool     `json:"allowDifferentLicenses,omitempty"`
		Nsfw                   bool     `json:"nsfw"`
	}

	Model struct {
		Meta                  interface{}         `json:"meta"`
		Creator               Creator             `json:"creator"`
		Description           string              `json:"description"`
		Type                  string              `json:"type"`
		Name                  string              `json:"name"`
		Mode                  *string             `json:"mode"` // Can be null, "Archived", or "TakenDown"
		AllowCommercialUse    StringOrStringSlice `json:"allowCommercialUse"`
		Tags                  []string            `json:"tags"`
		ModelVersions         []ModelVersion      `json:"modelVersions"`
		Stats                 Stats               `json:"stats"`
		ID                    int                 `json:"id"`
		Poi                   bool                `json:"poi"`
		Nsfw                  bool                `json:"nsfw"`
		AllowNoCredit         bool                `json:"allowNoCredit"`
		AllowDerivatives      bool                `json:"allowDerivatives"`
		AllowDifferentLicense bool                `json:"allowDifferentLicense"`
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
		// Strings first
		Name string `json:"name"`
		Type string `json:"type"`
		Mode string `json:"mode"` // Can be null, "Archived", "TakenDown"
		// Bools
		Nsfw bool `json:"nsfw"`
		Poi  bool `json:"poi"`
	}

	ModelVersion struct {
		CreatedAt            string        `json:"createdAt"`
		PublishedAt          string        `json:"publishedAt"`
		UpdatedAt            string        `json:"updatedAt"`
		BaseModel            string        `json:"baseModel"`
		Description          string        `json:"description"`
		DownloadUrl          string        `json:"downloadUrl"`
		Name                 string        `json:"name"`
		Model                BaseModelInfo `json:"model"`
		TrainedWords         []string      `json:"trainedWords"`
		Images               []ModelImage  `json:"images"`
		Files                []File        `json:"files"`
		Stats                Stats         `json:"stats"`
		ID                   int           `json:"id"`
		ModelId              int           `json:"modelId"`
		EarlyAccessTimeFrame int           `json:"earlyAccessTimeFrame"`
	}

	File struct {
		// Strings first
		Name              string `json:"name"`
		Type              string `json:"type"`
		PickleScanResult  string `json:"pickleScanResult"`
		PickleScanMessage string `json:"pickleScanMessage"`
		VirusScanResult   string `json:"virusScanResult"`
		ScannedAt         string `json:"scannedAt"`
		DownloadUrl       string `json:"downloadUrl"`
		// Structs
		Metadata Metadata `json:"metadata"`
		Hashes   Hashes   `json:"hashes"`
		// Float64
		SizeKB float64 `json:"sizeKB"`
		// Integer
		ID int `json:"id"`
		// Bool
		Primary bool `json:"primary"`
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
		NsfwLevel interface{} `json:"nsfwLevel"`
		Meta      interface{} `json:"meta"`
		PostID    *int        `json:"postId"`
		URL       string      `json:"url"`
		Hash      string      `json:"hash"`
		CreatedAt string      `json:"createdAt"`
		Username  string      `json:"username"`
		Stats     ImageStats  `json:"stats"`
		ID        int         `json:"id"`
		Width     int         `json:"width"`
		Height    int         `json:"height"`
		Nsfw      bool        `json:"nsfw"`
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
		// Strings first
		NextPage   string `json:"nextPage"`
		PrevPage   string `json:"prevPage"`   // Added based on API docs
		NextCursor string `json:"nextCursor"` // Added based on API docs (for images endpoint mainly)
		// Integers
		TotalItems  int `json:"totalItems"`
		CurrentPage int `json:"currentPage"`
		PageSize    int `json:"pageSize"`
		TotalPages  int `json:"totalPages"`
	}

	// Internal file db entry for each model
	DatabaseEntry struct {
		Creator      Creator      `json:"creator"`
		ModelName    string       `json:"modelName"`
		ModelType    string       `json:"modelType"`
		Filename     string       `json:"filename"`
		Folder       string       `json:"folder"`
		Status       string       `json:"status"`
		ErrorDetails string       `json:"errorDetails,omitempty"`
		File         File         `json:"file"`
		Version      ModelVersion `json:"version"`
		Timestamp    int64        `json:"timestamp"`
		ModelID      int          `json:"modelId"`
	}

	// --- Start: /api/v1/images Endpoint Structures ---

	// ImageApiResponse represents the structure of the response from the /api/v1/images endpoint.
	ImageApiResponse struct {
		// Slices first
		Items []ImageApiItem `json:"items"` // Renamed Image -> ImageApiItem to avoid conflict
		// Structs
		Metadata PaginationMetadata `json:"metadata"`
	}

	// ImageApiItem represents a single image item specifically from the /api/v1/images response.
	ImageApiItem struct {
		Nsfw           interface{} `json:"nsfw,omitempty"`
		NsfwLevel      interface{} `json:"nsfwLevel,omitempty"`
		PostID         *int        `json:"postId,omitempty"`
		URL            string      `json:"url"`
		Hash           string      `json:"hash"`
		Username       string      `json:"username,omitempty"`
		BaseModel      string      `json:"baseModel,omitempty"`
		ID             int         `json:"id"`
		Width          int         `json:"width"`
		Height         int         `json:"height"`
		ModelID        int         `json:"modelId,omitempty"`
		ModelVersionID int         `json:"modelVersionId,omitempty"`
	}

	// ImageAPIParameters defines the query parameters specific to the /api/v1/images endpoint.
	ImageAPIParameters struct {
		// Strings first
		Username string `json:"username,omitempty"`
		Sort     string `json:"sort,omitempty"`   // e.g., "Newest", "Most Reactions"
		Period   string `json:"period,omitempty"` // e.g., "AllTime", "Day"
		Nsfw     string `json:"nsfw,omitempty"`   // API values: "None", "Soft", "Mature", "X", "true", "false". Empty means omit.
		Cursor   string `json:"cursor,omitempty"`
		// Integers
		ModelID        int `json:"modelId,omitempty"`
		ModelVersionID int `json:"modelVersionId,omitempty"`
		PostID         int `json:"postId,omitempty"`
		Limit          int `json:"limit,omitempty"` // API default is 100, max 200 for images. 0 could mean API default.
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
