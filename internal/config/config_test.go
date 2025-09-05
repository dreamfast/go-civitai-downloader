package config

import (
	"testing"
)

// TestConfigInitialization tests basic configuration initialization
func TestConfigInitialization(t *testing.T) {
	flags := CliFlags{}
	cfg, _, err := Initialize(flags)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	// Verify default values were set
	if cfg.SavePath == "" {
		t.Error("Expected save path to be set to default")
	}

	if cfg.Download.Concurrency <= 0 {
		t.Error("Expected download concurrency to be positive")
	}

	if cfg.Download.Sort == "" {
		t.Error("Expected download sort to be set to default")
	}

	if cfg.Images.Limit <= 0 {
		t.Error("Expected images limit to be positive")
	}
}

// TestFlagOverrides tests that CLI flags override default values
func TestFlagOverrides(t *testing.T) {
	// Create flags that should override defaults
	concurrency := 8
	limit := 100
	flags := CliFlags{
		Download: &CliDownloadFlags{
			Concurrency: &concurrency,
			Limit:       &limit,
		},
	}

	cfg, _, err := Initialize(flags)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	// Verify flags overrode default values
	if cfg.Download.Concurrency != 8 {
		t.Errorf("Expected download concurrency 8 (from flags), got %d", cfg.Download.Concurrency)
	}

	if cfg.Download.Limit != 100 {
		t.Errorf("Expected download limit 100 (from flags), got %d", cfg.Download.Limit)
	}
}

// TestConfigValidation tests configuration validation for critical values
func TestConfigValidation(t *testing.T) {
	// Test that initialization produces a valid config
	flags := CliFlags{}
	cfg, _, err := Initialize(flags)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	// Basic validation checks
	if cfg.Download.Concurrency <= 0 {
		t.Error("Download concurrency should be positive")
	}

	if cfg.Download.Concurrency > 100 {
		t.Error("Download concurrency should be reasonable (<=100)")
	}

	if cfg.Images.Concurrency <= 0 {
		t.Error("Images concurrency should be positive")
	}

	if cfg.APIClientTimeoutSec <= 0 {
		t.Error("API client timeout should be positive")
	}

	if cfg.APIDelayMs < 0 {
		t.Error("API delay should not be negative")
	}
}

// TestEmptyFlags tests initialization with empty flags
func TestEmptyFlags(t *testing.T) {
	flags := CliFlags{}
	_, _, err := Initialize(flags)
	if err != nil {
		t.Errorf("Initialize should not fail with empty flags: %v", err)
	}
}

// TestNilFlagPointers tests that nil flag pointers are handled gracefully
func TestNilFlagPointers(t *testing.T) {
	flags := CliFlags{
		Download: nil, // Explicitly nil
		Images:   nil, // Explicitly nil
	}

	cfg, _, err := Initialize(flags)
	if err != nil {
		t.Errorf("Initialize should handle nil flag pointers: %v", err)
	}

	// Should still have valid defaults
	if cfg.Download.Concurrency <= 0 {
		t.Error("Should have valid default concurrency even with nil flags")
	}
}

// TestFlagsStructure tests the flags structure itself
func TestFlagsStructure(t *testing.T) {
	// Test that we can create and use the flags structure
	concurrency := 5
	limit := 50

	flags := CliFlags{
		Download: &CliDownloadFlags{
			Concurrency: &concurrency,
			Limit:       &limit,
		},
		Images: &CliImagesFlags{
			Concurrency: &concurrency,
		},
	}

	if flags.Download == nil {
		t.Error("Download flags should not be nil")
	}

	if flags.Images == nil {
		t.Error("Images flags should not be nil")
	}

	if *flags.Download.Concurrency != 5 {
		t.Error("Download concurrency flag not set correctly")
	}

	if *flags.Images.Concurrency != 5 {
		t.Error("Images concurrency flag not set correctly")
	}
}

// TestHTTPTransportCreation tests that HTTP transport is created
func TestHTTPTransportCreation(t *testing.T) {
	flags := CliFlags{}
	_, transport, err := Initialize(flags)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	if transport == nil {
		t.Error("HTTP transport should be created")
	}
}

// TestConfigDefaults tests that reasonable defaults are set
func TestConfigDefaults(t *testing.T) {
	flags := CliFlags{}
	cfg, _, err := Initialize(flags)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	// Test that defaults are reasonable
	if cfg.Download.Concurrency < 1 || cfg.Download.Concurrency > 20 {
		t.Errorf("Download concurrency default should be reasonable (1-20), got %d", cfg.Download.Concurrency)
	}

	if cfg.Images.Concurrency < 1 || cfg.Images.Concurrency > 20 {
		t.Errorf("Images concurrency default should be reasonable (1-20), got %d", cfg.Images.Concurrency)
	}

	if cfg.APIClientTimeoutSec < 10 || cfg.APIClientTimeoutSec > 300 {
		t.Errorf("API timeout default should be reasonable (10-300s), got %d", cfg.APIClientTimeoutSec)
	}

	if cfg.APIDelayMs < 0 || cfg.APIDelayMs > 10000 {
		t.Errorf("API delay default should be reasonable (0-10000ms), got %d", cfg.APIDelayMs)
	}

	if cfg.SavePath == "" {
		t.Error("SavePath should have a default value")
	}

	if cfg.Download.Sort == "" {
		t.Error("Download sort should have a default value")
	}
}

// TestMultipleFlagTypes tests different flag types
func TestMultipleFlagTypes(t *testing.T) {
	// Test string flags
	tag := "test-tag"
	query := "test query"

	// Test slice flags
	modelTypes := []string{"Checkpoint", "LORA"}
	baseModels := []string{"SD 1.5", "SDXL 1.0"}

	// Test bool flags
	nsfw := true
	metadata := false

	flags := CliFlags{
		Download: &CliDownloadFlags{
			Tag:          &tag,
			Query:        &query,
			ModelTypes:   &modelTypes,
			BaseModels:   &baseModels,
			Nsfw:         &nsfw,
			SaveMetadata: &metadata,
		},
	}

	cfg, _, err := Initialize(flags)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	if cfg.Download.Tag != "test-tag" {
		t.Errorf("Expected tag 'test-tag', got '%s'", cfg.Download.Tag)
	}

	if cfg.Download.Query != "test query" {
		t.Errorf("Expected query 'test query', got '%s'", cfg.Download.Query)
	}

	if len(cfg.Download.ModelTypes) != 2 {
		t.Errorf("Expected 2 model types, got %d", len(cfg.Download.ModelTypes))
	}

	if cfg.Download.Nsfw != true {
		t.Error("Expected NSFW flag to be true")
	}

	if cfg.Download.SaveMetadata != false {
		t.Error("Expected SaveMetadata flag to be false")
	}
}
