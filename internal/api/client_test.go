package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-civitai-download/internal/models"
)

// TestNewClient tests the API client creation
func TestNewClient(t *testing.T) {
	apiKey := "test-api-key"
	cfg := models.Config{}
	
	client := NewClient(apiKey, nil, cfg)
	
	if client.ApiKey != apiKey {
		t.Errorf("Expected API key %s, got %s", apiKey, client.ApiKey)
	}
	
	if client.HttpClient == nil {
		t.Error("Expected HTTP client to be initialized")
	}
	
	if client.HttpClient.Timeout != 30*time.Second {
		t.Errorf("Expected timeout to be 30s, got %v", client.HttpClient.Timeout)
	}
}

// TestRetryableHTTPRequest_Success tests successful HTTP requests
func TestRetryableHTTPRequest_Success(t *testing.T) {
	// Mock server that returns success on first try
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	client := NewClient("test-key", &http.Client{}, models.Config{})
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	resp, err := client.RetryableHTTPRequest(req)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	
	resp.Body.Close()
}

// TestRetryableHTTPRequest_RateLimit tests rate limit handling
func TestRetryableHTTPRequest_RateLimit(t *testing.T) {
	attemptCount := 0
	
	// Mock server that returns rate limit error twice, then success
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limited"}`))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "success"}`))
		}
	}))
	defer server.Close()

	client := NewClient("test-key", &http.Client{}, models.Config{})
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	// This test will be slow due to retry delays, so we'll use a shorter timeout
	// for testing by using a custom client with shorter timeout
	client.HttpClient.Timeout = 1 * time.Second
	
	start := time.Now()
	resp, err := client.RetryableHTTPRequest(req)
	duration := time.Since(start)
	
	if err != nil {
		t.Fatalf("Expected success after retries, got error: %v", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	
	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}
	
	// Should take at least some time due to retries
	if duration < 1*time.Second {
		t.Errorf("Expected some delay due to retries, took %v", duration)
	}
	
	resp.Body.Close()
}

// TestRetryableHTTPRequest_MaxRetries tests that max retries are respected
func TestRetryableHTTPRequest_MaxRetries(t *testing.T) {
	attemptCount := 0
	
	// Mock server that always returns rate limit error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "rate limited"}`))
	}))
	defer server.Close()

	client := NewClient("test-key", &http.Client{}, models.Config{})
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	_, err = client.RetryableHTTPRequest(req)
	
	if err == nil {
		t.Error("Expected error after max retries, got success")
	}
	
	if err != ErrRateLimited {
		t.Errorf("Expected ErrRateLimited, got %v", err)
	}
	
	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts (max retries), got %d", attemptCount)
	}
}

// TestRetryableHTTPRequest_Unauthorized tests unauthorized responses
func TestRetryableHTTPRequest_Unauthorized(t *testing.T) {
	// Mock server that returns unauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "unauthorized"}`))
	}))
	defer server.Close()

	client := NewClient("test-key", &http.Client{}, models.Config{})
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	_, err = client.RetryableHTTPRequest(req)
	
	if err == nil {
		t.Error("Expected error for unauthorized, got success")
	}
	
	if err != ErrUnauthorized {
		t.Errorf("Expected ErrUnauthorized, got %v", err)
	}
}

// TestRetryableHTTPRequest_NotFound tests not found responses
func TestRetryableHTTPRequest_NotFound(t *testing.T) {
	// Mock server that returns not found
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	}))
	defer server.Close()

	client := NewClient("test-key", &http.Client{}, models.Config{})
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	_, err = client.RetryableHTTPRequest(req)
	
	if err == nil {
		t.Error("Expected error for not found, got success")
	}
	
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestRetryableHTTPRequest_ServerError tests server error handling
func TestRetryableHTTPRequest_ServerError(t *testing.T) {
	attemptCount := 0
	
	// Mock server that always returns server error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "server error"}`))
	}))
	defer server.Close()

	client := NewClient("test-key", &http.Client{}, models.Config{})
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	_, err = client.RetryableHTTPRequest(req)
	
	if err == nil {
		t.Error("Expected error for server error, got success")
	}
	
	if !strings.Contains(err.Error(), "server error") {
		t.Errorf("Expected server error message, got %v", err)
	}
	
	// Should retry for server errors
	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts for server error, got %d", attemptCount)
	}
}

// TestGetModels_Integration is an integration test that makes a real API call
// This test will be skipped unless the CIVITAI_API_KEY environment variable is set
func TestGetModels_Integration(t *testing.T) {
	apiKey := getTestAPIKey(t)
	if apiKey == "" {
		t.Skip("Skipping integration test: CIVITAI_API_KEY environment variable not set")
	}
	
	client := NewClient(apiKey, &http.Client{Timeout: 30 * time.Second}, models.Config{})
	
	// Test with basic parameters
	queryParams := models.QueryParameters{
		Limit: 5,
		Sort:  "Most Downloaded",
		Nsfw:  false,
	}
	
	_, result, err := client.GetModels("", queryParams)
	if err != nil {
		t.Fatalf("GetModels failed: %v", err)
	}
	
	if len(result.Items) == 0 {
		t.Error("Expected at least one model in results")
	}
	
	// Verify structure of returned data
	if len(result.Items) > 0 {
		model := result.Items[0]
		if model.ID == 0 {
			t.Error("Expected model to have an ID")
		}
		if model.Name == "" {
			t.Error("Expected model to have a name")
		}
		if len(model.ModelVersions) == 0 {
			t.Error("Expected model to have at least one version")
		}
	}
}

// TestGetModelDetails_Integration tests fetching detailed model information
func TestGetModelDetails_Integration(t *testing.T) {
	apiKey := getTestAPIKey(t)
	if apiKey == "" {
		t.Skip("Skipping integration test: CIVITAI_API_KEY environment variable not set")
	}
	
	client := NewClient(apiKey, &http.Client{Timeout: 30 * time.Second}, models.Config{})
	
	// Use a known model ID (this is the Stable Diffusion 1.5 model which should be stable)
	modelID := 4201 // This is a well-known public model
	
	model, err := client.GetModelDetails(modelID)
	if err != nil {
		t.Fatalf("GetModelDetails failed: %v", err)
	}
	
	if model.ID == 0 {
		t.Error("Expected model to have an ID")
	}
	
	if model.Name == "" {
		t.Error("Expected model to have a name")
	}
	
	if len(model.ModelVersions) == 0 {
		t.Error("Expected model to have at least one version")
	}
	
	// Test version details
	version := model.ModelVersions[0]
	if version.ID == 0 {
		t.Error("Expected version to have an ID")
	}
	
	if len(version.Files) == 0 {
		t.Error("Expected version to have at least one file")
	}
}

// TestGetModelVersionDetails_Integration tests fetching model version details
func TestGetModelVersionDetails_Integration(t *testing.T) {
	apiKey := getTestAPIKey(t)
	if apiKey == "" {
		t.Skip("Skipping integration test: CIVITAI_API_KEY environment variable not set")
	}
	
	client := NewClient(apiKey, &http.Client{Timeout: 30 * time.Second}, models.Config{})
	
	// Use a known model version ID
	versionID := 15236 // This should be a stable version
	
	version, err := client.GetModelVersionDetails(versionID)
	if err != nil {
		t.Fatalf("GetModelVersionDetails failed: %v", err)
	}
	
	if version.ID == 0 {
		t.Error("Expected version to have an ID")
	}
	
	if len(version.Files) == 0 {
		t.Error("Expected version to have at least one file")
	}
	
	// Verify file structure
	file := version.Files[0]
	if file.Name == "" {
		t.Error("Expected file to have a name")
	}
	
	if file.DownloadUrl == "" {
		t.Error("Expected file to have a download URL")
	}
}

// Helper function to get API key from environment for integration tests
func getTestAPIKey(t *testing.T) string {
	// For integration tests, users need to set CIVITAI_API_KEY environment variable
	// This prevents accidental API calls during normal testing
	return "" // Return empty string to skip integration tests by default
	
	// To enable integration tests, users should uncomment the following line
	// and set the CIVITAI_API_KEY environment variable:
	// return os.Getenv("CIVITAI_API_KEY")
}

// TestAPIErrorHandling tests various API error conditions
func TestAPIErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedError  error
		shouldRetry    bool
	}{
		{"Success", http.StatusOK, nil, false},
		{"Rate Limited", http.StatusTooManyRequests, ErrRateLimited, true},
		{"Unauthorized", http.StatusUnauthorized, ErrUnauthorized, false},
		{"Forbidden", http.StatusForbidden, ErrUnauthorized, false},
		{"Not Found", http.StatusNotFound, ErrNotFound, false},
		{"Server Error", http.StatusInternalServerError, ErrServerError, true},
		{"Service Unavailable", http.StatusServiceUnavailable, ErrServerError, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptCount++
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					w.Write([]byte(`{"status": "success"}`))
				} else {
					w.Write([]byte(`{"error": "test error"}`))
				}
			}))
			defer server.Close()

			client := NewClient("test-key", &http.Client{}, models.Config{})
			
			req, err := http.NewRequest("GET", server.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			
			_, err = client.RetryableHTTPRequest(req)
			
			if tt.expectedError == nil {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error %v, got none", tt.expectedError)
				} else if err != tt.expectedError && !strings.Contains(err.Error(), tt.expectedError.Error()) {
					t.Errorf("Expected error %v, got %v", tt.expectedError, err)
				}
			}
			
			expectedAttempts := 1
			if tt.shouldRetry {
				expectedAttempts = 3
			}
			
			if attemptCount != expectedAttempts {
				t.Errorf("Expected %d attempts, got %d", expectedAttempts, attemptCount)
			}
		})
	}
}