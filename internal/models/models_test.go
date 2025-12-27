package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusConstants(t *testing.T) {
	// Verify status constants have expected values
	if StatusPending != "Pending" {
		t.Errorf("StatusPending = %q, want %q", StatusPending, "Pending")
	}
	if StatusDownloaded != "Downloaded" {
		t.Errorf("StatusDownloaded = %q, want %q", StatusDownloaded, "Downloaded")
	}
	if StatusError != "Error" {
		t.Errorf("StatusError = %q, want %q", StatusError, "Error")
	}
}

func TestStringOrStringSlice_UnmarshalString(t *testing.T) {
	// Test unmarshaling a single string
	jsonStr := `"single value"`

	var result StringOrStringSlice
	err := json.Unmarshal([]byte(jsonStr), &result)

	if err != nil {
		t.Fatalf("Unmarshal string failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 element, got %d", len(result))
	}
	if result[0] != "single value" {
		t.Errorf("Expected 'single value', got %q", result[0])
	}
}

func TestStringOrStringSlice_UnmarshalArray(t *testing.T) {
	// Test unmarshaling an array of strings
	jsonStr := `["value1", "value2", "value3"]`

	var result StringOrStringSlice
	err := json.Unmarshal([]byte(jsonStr), &result)

	if err != nil {
		t.Fatalf("Unmarshal array failed: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(result))
	}
	expected := []string{"value1", "value2", "value3"}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("Element %d: expected %q, got %q", i, v, result[i])
		}
	}
}

func TestStringOrStringSlice_EmptyArray(t *testing.T) {
	jsonStr := `[]`

	var result StringOrStringSlice
	err := json.Unmarshal([]byte(jsonStr), &result)

	if err != nil {
		t.Fatalf("Unmarshal empty array failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 elements, got %d", len(result))
	}
}

func TestStringOrStringSlice_EmptyString(t *testing.T) {
	jsonStr := `""`

	var result StringOrStringSlice
	err := json.Unmarshal([]byte(jsonStr), &result)

	if err != nil {
		t.Fatalf("Unmarshal empty string failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 element, got %d", len(result))
	}
	if result[0] != "" {
		t.Errorf("Expected empty string, got %q", result[0])
	}
}

func TestConstructApiUrl_BasicParams(t *testing.T) {
	params := QueryParameters{
		Limit:  50,
		Sort:   "Most Downloaded",
		Period: "AllTime",
		Nsfw:   true,
	}

	url := ConstructApiUrl(params)

	// Check base URL
	if !strings.HasPrefix(url, "https://civitai.com/api/v1/models") {
		t.Errorf("URL should start with API base, got: %s", url)
	}

	// Check parameters
	if !strings.Contains(url, "limit=50") {
		t.Errorf("URL should contain limit=50, got: %s", url)
	}
	if !strings.Contains(url, "nsfw=true") {
		t.Errorf("URL should contain nsfw=true, got: %s", url)
	}
}

func TestConstructApiUrl_WithQuery(t *testing.T) {
	params := QueryParameters{
		Query: "anime style",
		Limit: 100,
	}

	url := ConstructApiUrl(params)

	if !strings.Contains(url, "query=") {
		t.Errorf("URL should contain query parameter, got: %s", url)
	}
}

func TestConstructApiUrl_WithTypes(t *testing.T) {
	params := QueryParameters{
		Types: []string{"Checkpoint", "LORA"},
		Limit: 100,
	}

	url := ConstructApiUrl(params)

	// Should have multiple types parameters
	if strings.Count(url, "types=") != 2 {
		t.Errorf("URL should contain 2 types parameters, got: %s", url)
	}
}

func TestConstructApiUrl_WithBaseModels(t *testing.T) {
	params := QueryParameters{
		BaseModels: []string{"SD 1.5", "SDXL 1.0"},
		Limit:      100,
	}

	url := ConstructApiUrl(params)

	// Should have baseModels parameters
	if !strings.Contains(url, "baseModels=") {
		t.Errorf("URL should contain baseModels parameter, got: %s", url)
	}
}

func TestConstructApiUrl_WithCursor(t *testing.T) {
	params := QueryParameters{
		Cursor: "abc123cursor",
		Limit:  100,
	}

	url := ConstructApiUrl(params)

	if !strings.Contains(url, "cursor=abc123cursor") {
		t.Errorf("URL should contain cursor parameter, got: %s", url)
	}
}

func TestConstructApiUrl_NoParams(t *testing.T) {
	params := QueryParameters{}

	url := ConstructApiUrl(params)

	// With empty QueryParameters, the function adds defaults for boolean flags
	// that default to false (AllowNoCredit, AllowDerivatives, AllowDifferentLicenses)
	// So the URL will contain these parameters
	if !strings.HasPrefix(url, "https://civitai.com/api/v1/models") {
		t.Errorf("URL should start with base URL, got: %s", url)
	}

	// When booleans are false (zero value), these params are added
	if !strings.Contains(url, "allowNoCredit=false") {
		t.Errorf("URL should contain allowNoCredit=false (default), got: %s", url)
	}
}

func TestConstructApiUrl_InvalidLimit(t *testing.T) {
	params := QueryParameters{
		Limit: 150, // Over 100, should be ignored
	}

	url := ConstructApiUrl(params)

	// Should not contain limit since it's invalid
	if strings.Contains(url, "limit=") {
		t.Errorf("URL should not contain invalid limit, got: %s", url)
	}
}

func TestDatabaseEntry_JSON(t *testing.T) {
	entry := DatabaseEntry{
		ModelID:   12345,
		ModelName: "Test Model",
		ModelType: "Checkpoint",
		Status:    StatusPending,
		Version: ModelVersion{
			ID:        67890,
			ModelId:   12345,
			Name:      "v1.0",
			BaseModel: "SD 1.5",
		},
		Folder:    "Checkpoint/test-model/v1.0",
		Timestamp: 1704067200,
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal DatabaseEntry: %v", err)
	}

	// Unmarshal back
	var decoded DatabaseEntry
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal DatabaseEntry: %v", err)
	}

	// Verify key fields
	if decoded.ModelID != entry.ModelID {
		t.Errorf("ModelID mismatch: got %d, want %d", decoded.ModelID, entry.ModelID)
	}
	if decoded.ModelName != entry.ModelName {
		t.Errorf("ModelName mismatch: got %q, want %q", decoded.ModelName, entry.ModelName)
	}
	if decoded.Status != entry.Status {
		t.Errorf("Status mismatch: got %q, want %q", decoded.Status, entry.Status)
	}
	if decoded.Version.ID != entry.Version.ID {
		t.Errorf("Version.ID mismatch: got %d, want %d", decoded.Version.ID, entry.Version.ID)
	}
}

func TestModel_JSON(t *testing.T) {
	model := Model{
		ID:   12345,
		Name: "Test Model",
		Type: "Checkpoint",
		Creator: Creator{
			Username: "testuser",
		},
		Tags: []string{"anime", "realistic"},
		Stats: Stats{
			DownloadCount: 1000,
			FavoriteCount: 100,
			Rating:        4.5,
		},
		Nsfw: false,
	}

	// Marshal and unmarshal
	data, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("Failed to marshal Model: %v", err)
	}

	var decoded Model
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal Model: %v", err)
	}

	// Verify
	if decoded.ID != model.ID {
		t.Errorf("ID mismatch: got %d, want %d", decoded.ID, model.ID)
	}
	if decoded.Name != model.Name {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, model.Name)
	}
	if len(decoded.Tags) != len(model.Tags) {
		t.Errorf("Tags length mismatch: got %d, want %d", len(decoded.Tags), len(model.Tags))
	}
}

func TestHashes_AllFields(t *testing.T) {
	hashes := Hashes{
		AutoV2: "ABCD123456",
		SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		CRC32:  "D202EF8D",
		BLAKE3: "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262",
	}

	// Marshal and unmarshal
	data, err := json.Marshal(hashes)
	if err != nil {
		t.Fatalf("Failed to marshal Hashes: %v", err)
	}

	var decoded Hashes
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal Hashes: %v", err)
	}

	if decoded.AutoV2 != hashes.AutoV2 {
		t.Errorf("AutoV2 mismatch")
	}
	if decoded.SHA256 != hashes.SHA256 {
		t.Errorf("SHA256 mismatch")
	}
	if decoded.CRC32 != hashes.CRC32 {
		t.Errorf("CRC32 mismatch")
	}
	if decoded.BLAKE3 != hashes.BLAKE3 {
		t.Errorf("BLAKE3 mismatch")
	}
}

func TestFile_Metadata(t *testing.T) {
	file := File{
		ID:          111,
		Name:        "model.safetensors",
		SizeKB:      6700000,
		Type:        "Model",
		Primary:     true,
		DownloadUrl: "https://civitai.com/api/download/models/111",
		Metadata: Metadata{
			Fp:     "fp16",
			Size:   "full",
			Format: "SafeTensor",
		},
	}

	// Marshal and unmarshal
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("Failed to marshal File: %v", err)
	}

	var decoded File
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal File: %v", err)
	}

	if decoded.Metadata.Fp != "fp16" {
		t.Errorf("Metadata.Fp mismatch: got %q, want %q", decoded.Metadata.Fp, "fp16")
	}
	if decoded.Metadata.Format != "SafeTensor" {
		t.Errorf("Metadata.Format mismatch")
	}
}

func TestConfig_Defaults(t *testing.T) {
	// Test that a zero-value Config has expected defaults (all zero/empty)
	var cfg Config

	if cfg.APIDelayMs != 0 {
		t.Errorf("Default APIDelayMs should be 0, got %d", cfg.APIDelayMs)
	}
	if cfg.SavePath != "" {
		t.Errorf("Default SavePath should be empty, got %q", cfg.SavePath)
	}
	if cfg.Download.Concurrency != 0 {
		t.Errorf("Default Download.Concurrency should be 0, got %d", cfg.Download.Concurrency)
	}
}

func TestImageApiItem_JSON(t *testing.T) {
	item := ImageApiItem{
		ID:             12345,
		URL:            "https://image.civitai.com/test.jpg",
		Width:          1024,
		Height:         768,
		Username:       "artist",
		BaseModel:      "SDXL 1.0",
		ModelID:        111,
		ModelVersionID: 222,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Failed to marshal ImageApiItem: %v", err)
	}

	var decoded ImageApiItem
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal ImageApiItem: %v", err)
	}

	if decoded.ID != item.ID {
		t.Errorf("ID mismatch")
	}
	if decoded.URL != item.URL {
		t.Errorf("URL mismatch")
	}
	if decoded.Username != item.Username {
		t.Errorf("Username mismatch")
	}
}

func TestPaginationMetadata_JSON(t *testing.T) {
	meta := PaginationMetadata{
		TotalItems:  1000,
		CurrentPage: 5,
		PageSize:    100,
		TotalPages:  10,
		NextPage:    "https://api.civitai.com?page=6",
		NextCursor:  "cursor123",
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("Failed to marshal PaginationMetadata: %v", err)
	}

	var decoded PaginationMetadata
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal PaginationMetadata: %v", err)
	}

	if decoded.TotalItems != meta.TotalItems {
		t.Errorf("TotalItems mismatch")
	}
	if decoded.NextCursor != meta.NextCursor {
		t.Errorf("NextCursor mismatch")
	}
}

func TestFlexibleCursor_Unmarshal(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected string
	}{
		{
			name:     "string cursor",
			json:     `{"nextCursor": "abc123"}`,
			expected: "abc123",
		},
		{
			name:     "numeric cursor",
			json:     `{"nextCursor": 36386}`,
			expected: "36386",
		},
		{
			name:     "large numeric cursor",
			json:     `{"nextCursor": 1234567890}`,
			expected: "1234567890",
		},
		{
			name:     "empty string cursor",
			json:     `{"nextCursor": ""}`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result struct {
				NextCursor FlexibleCursor `json:"nextCursor"`
			}
			err := json.Unmarshal([]byte(tt.json), &result)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}
			if result.NextCursor.String() != tt.expected {
				t.Errorf("FlexibleCursor = %q, want %q", result.NextCursor.String(), tt.expected)
			}
		})
	}
}
