package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"go-civitai-download/internal/models"

	log "github.com/sirupsen/logrus"
)

// Custom Error Types
var (
	ErrRateLimited  = errors.New("API rate limit exceeded")
	ErrUnauthorized = errors.New("API request unauthorized (check API key)")
	ErrNotFound     = errors.New("API resource not found")
	ErrServerError  = errors.New("API server error")
)

const CivitaiApiBaseUrl = "https://civitai.com/api/v1"

// apiLogger is a dedicated logger for api.log
var apiLogger = log.New()
var apiLogFile *os.File

// configureApiLogger sets up the apiLogger based on config.
// This should be called once, perhaps from the main command setup or PersistentPreRun.
// For simplicity now, we'll call it within NewClient, though not ideal.
func configureApiLogger(shouldLog bool) {
	log.Debugf("configureApiLogger called with shouldLog=%t", shouldLog) // Log entry
	if !shouldLog {
		apiLogger.SetOutput(io.Discard)
		log.Debug("API logging disabled by config/flag.")
		return
	}

	if apiLogFile == nil {
		log.Debug("apiLogFile is nil, attempting to open...")
		var err error
		apiLogFile, err = os.OpenFile("api.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			log.WithError(err).Error("Failed to open api.log, API logging disabled.")
			apiLogger.SetOutput(io.Discard)
			return
		}
		log.Debug("api.log opened successfully.")
		apiLogger.SetOutput(apiLogFile)
		// Use a simple text formatter for the log file
		apiLogger.SetFormatter(&log.TextFormatter{
			DisableColors:    true,
			FullTimestamp:    true,
			DisableQuote:     true,
			QuoteEmptyFields: true,
		})
		apiLogger.SetLevel(log.DebugLevel) // Log everything to the file if enabled
		apiLogger.Info("API Logger Initialized")
	} else {
		log.Debug("apiLogFile already open, reusing existing handle.")
	}
}

// CleanupApiLog closes the api.log file handle. Should be called on application exit.
func CleanupApiLog() {
	if apiLogFile != nil {
		apiLogger.Info("Closing API log file.")
		if err := apiLogFile.Close(); err != nil {
			apiLogger.WithError(err).Error("Error closing API log file")
		}
	}
}

// Client struct for interacting with the Civitai API
// TODO: Add http.Client field for reuse
type Client struct {
	ApiKey         string
	HttpClient     *http.Client // Use a shared client
	logApiRequests bool         // Store the config setting
}

// NewClient creates a new API client
// TODO: Initialize and pass a shared http.Client
func NewClient(apiKey string, httpClient *http.Client, cfg models.Config) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	// Enhanced API key logging
	if apiKey != "" {
		log.Infof("Initializing API client with key: %s", apiKey)
	} else {
		log.Warn("Initializing API client without authentication key - some endpoints may be restricted")
	}

	log.Debugf("NewClient called, cfg.LogApiRequests value: %t", cfg.LogApiRequests)
	// Configure the logger based on the *global* config setting
	configureApiLogger(cfg.LogApiRequests)

	return &Client{
		ApiKey:         apiKey,
		HttpClient:     httpClient,
		logApiRequests: cfg.LogApiRequests, // Store flag for use in methods
	}
}

// GetModels fetches models based on query parameters, using cursor pagination.
// Accepts the cursor for the next page. Returns the next cursor and the response.
func (c *Client) GetModels(cursor string, queryParams models.QueryParameters) (string, models.ApiResponse, error) {
	// Use the helper function to build base query parameters
	values := ConvertQueryParamsToURLValues(queryParams)

	// Add cursor *only if* it's provided (not empty)
	if cursor != "" {
		values.Add("cursor", cursor)
	} else {
		// For the first request (empty cursor), do not add 'page' either.
		// The API defaults to the first page/results without page/cursor.
	}

	reqURL := fmt.Sprintf("%s/models?%s", CivitaiApiBaseUrl, values.Encode())
	// No change to main logger here
	// log.Debugf("Requesting URL: %s", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		log.WithError(err).Errorf("Error creating request for %s", reqURL)
		// Wrap the underlying error
		return "", models.ApiResponse{}, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.ApiKey != "" {
		log.Debugf("Adding Authorization header with API key to request for %s", reqURL)
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	} else {
		log.Debugf("Making unauthenticated request to %s", reqURL)
	}

	// --- Log API Request ---
	if c.logApiRequests {
		reqDump, dumpErr := httputil.DumpRequestOut(req, true) // Dump outgoing request
		if dumpErr != nil {
			apiLogger.WithError(dumpErr).Error("Failed to dump API request")
		} else {
			apiLogger.Debugf("\n--- API Request ---\n%s\n--------------------", string(reqDump))
		}
	}
	// --- End Log API Request ---

	var resp *http.Response
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err = c.HttpClient.Do(req)

		// --- Log API Response (Attempt) ---
		if c.logApiRequests && resp != nil { // Log even on non-200 responses
			// Read body first for logging, then replace it for potential retries/final processing
			bodyBytes, readErr := io.ReadAll(resp.Body)
			if closeErr := resp.Body.Close(); closeErr != nil { // Close original body and check error
				apiLogger.WithError(closeErr).Warn("Error closing response body after reading for logging")
			}
			if readErr != nil {
				apiLogger.WithError(readErr).Errorf("Attempt %d: Failed to read response body for logging", attempt+1)
			} else {
				// Create a new io.ReadCloser from the read bytes
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

				// Try to dump response (might fail on huge bodies)
				respDump, dumpErr := httputil.DumpResponse(resp, false) // false = don't dump body here
				if dumpErr != nil {
					apiLogger.WithError(dumpErr).Errorf("Attempt %d: Failed to dump API response headers", attempt+1)
				} else {
					apiLogger.Debugf("\n--- API Response (Attempt %d) ---\n%s\n--- Body (%d bytes) ---\n%s\n----------------------------- \n",
						attempt+1, string(respDump), len(bodyBytes), string(bodyBytes))
				}
			}
		} else if c.logApiRequests && err != nil {
			// Log if the Do() call itself failed
			apiLogger.WithError(err).Errorf("Attempt %d: HTTP Client Do() failed", attempt+1)
		}
		// --- End Log API Response (Attempt) ---

		if err != nil {
			lastErr = fmt.Errorf("http request failed (attempt %d/%d): %w", attempt+1, maxRetries, err)
			if attempt < maxRetries-1 { // Only log retry warning if not the last attempt
				log.WithError(err).Warnf("Retrying (%d/%d)...", attempt+1, maxRetries)
				time.Sleep(time.Duration(attempt+1) * 2 * time.Second) // Exponential backoff
				continue
			}
			break // Max retries reached on HTTP error
		}

		switch resp.StatusCode {
		case http.StatusOK:
			lastErr = nil        // Success
			goto ProcessResponse // Use goto to break out of switch and loop
		case http.StatusTooManyRequests:
			lastErr = ErrRateLimited
		case http.StatusUnauthorized, http.StatusForbidden:
			if c.ApiKey != "" {
				log.Errorf("API request unauthorized with key (status %d) - check if key is valid", resp.StatusCode)
			} else {
				log.Errorf("API request unauthorized (status %d) - endpoint may require authentication", resp.StatusCode)
			}
			lastErr = ErrUnauthorized
			goto RequestFailed // Non-retryable auth error
		case http.StatusNotFound:
			lastErr = ErrNotFound
			goto RequestFailed // Non-retryable not found error
		case http.StatusServiceUnavailable:
			lastErr = fmt.Errorf("%w (status code 503)", ErrServerError)
		default:
			if resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("%w (status code %d)", ErrServerError, resp.StatusCode)
			} else {
				// Other client-side errors (4xx) are likely not retryable
				lastErr = fmt.Errorf("API request failed with status %d", resp.StatusCode)
				goto RequestFailed
			}
		}

		// If we are here, it's a retryable error (Rate Limit or 5xx)
		// resp.Body was already closed and replaced during logging
		if attempt < maxRetries-1 {
			var sleepDuration time.Duration
			if resp.StatusCode == http.StatusTooManyRequests {
				// Longer backoff for rate limits
				sleepDuration = time.Duration(attempt+1) * 5 * time.Second
				log.WithError(lastErr).Warnf("Rate limited. Retrying (%d/%d) after %s...", attempt+1, maxRetries, sleepDuration)
			} else { // Server errors (5xx)
				sleepDuration = time.Duration(attempt+1) * 3 * time.Second
				log.WithError(lastErr).Warnf("Server error. Retrying (%d/%d) after %s...", attempt+1, maxRetries, sleepDuration)
			}
			time.Sleep(sleepDuration)
		} else {
			log.WithError(lastErr).Errorf("Request failed after %d attempts with status %d", maxRetries, resp.StatusCode)
		}
	}

RequestFailed:
	if lastErr != nil {
		// Don't close body here, it should have been closed during logging or by defer on success path
		// if resp != nil { resp.Body.Close() }
		return "", models.ApiResponse{}, lastErr
	}

ProcessResponse:
	// Body should already be replaced with a readable version from logging step
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body) // Read the replaced body
	if err != nil {
		log.WithError(err).Error("Error reading final response body")
		return "", models.ApiResponse{}, fmt.Errorf("error reading response body: %w", err)
	}

	var response models.ApiResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.WithError(err).Errorf("Error unmarshalling response JSON")
		// Log the body that caused the error (already logged to api.log if enabled)
		log.Debugf("Response body causing unmarshal error: %s", string(body))
		return "", models.ApiResponse{}, fmt.Errorf("error unmarshalling response JSON: %w", err)
	}

	// Return the next cursor provided by the API
	return response.Metadata.NextCursor, response, nil
}

// ConvertQueryParamsToURLValues converts the QueryParameters struct into url.Values for API requests.
// This is used for constructing the request URL.
func ConvertQueryParamsToURLValues(queryParams models.QueryParameters) url.Values {
	values := url.Values{}
	values.Add("sort", queryParams.Sort)
	values.Add("period", queryParams.Period)
	// Always include the nsfw parameter, converting the boolean to string "true" or "false"
	values.Add("nsfw", fmt.Sprintf("%t", queryParams.Nsfw))
	values.Add("limit", fmt.Sprintf("%d", queryParams.Limit))
	for _, t := range queryParams.Types {
		values.Add("types", t)
	}
	for _, t := range queryParams.BaseModels {
		values.Add("baseModels", t)
	}
	if queryParams.PrimaryFileOnly {
		values.Add("primaryFileOnly", fmt.Sprintf("%t", queryParams.PrimaryFileOnly))
	}
	if queryParams.Query != "" {
		values.Add("query", queryParams.Query)
	}
	if queryParams.Tag != "" {
		values.Add("tag", queryParams.Tag)
	}
	if queryParams.Username != "" {
		values.Add("username", queryParams.Username)
	}

	// Note: Cursor/Page parameters are typically added separately based on pagination logic.
	return values
}

// TODO: Add methods for other API endpoints (e.g., GetModelByID, GetModelVersionByID)
