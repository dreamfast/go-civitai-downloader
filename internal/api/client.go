package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// Client struct for interacting with the Civitai API
type Client struct {
	ApiKey     string
	HttpClient *http.Client // Use a shared client
}

// NewClient creates a new API client
func NewClient(apiKey string, httpClient *http.Client, cfg models.Config) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	log.Debugf("NewClient called (API logging handled by transport if enabled)")

	return &Client{
		ApiKey:     apiKey,
		HttpClient: httpClient,
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

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		log.WithError(err).Errorf("Error creating request for %s", reqURL)
		// Wrap the underlying error
		return "", models.ApiResponse{}, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	}

	var resp *http.Response
	var lastErr error
	maxRetries := 3 // TODO: Make this configurable via cfg?

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err = c.HttpClient.Do(req) // Transport will log if enabled

		if err != nil {
			lastErr = fmt.Errorf("http request failed (attempt %d/%d): %w", attempt+1, maxRetries, err)
			if attempt < maxRetries-1 { // Only log retry warning if not the last attempt
				log.WithError(err).Warnf("Retrying (%d/%d)...", attempt+1, maxRetries)
				time.Sleep(time.Duration(attempt+1) * 2 * time.Second) // Exponential backoff
				continue
			}
			break // Max retries reached on HTTP error
		}

		// Note: Response body is handled by the logging transport if logging is enabled.
		// The transport reads it, logs it, and replaces resp.Body with a readable buffer.
		// If logging is not enabled, the original resp.Body is passed through.

		switch resp.StatusCode {
		case http.StatusOK:
			lastErr = nil        // Success
			goto ProcessResponse // Use goto to break out of switch and loop
		case http.StatusTooManyRequests:
			lastErr = ErrRateLimited
		case http.StatusUnauthorized, http.StatusForbidden:
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
		// Body should be closed here before retry if it wasn't handled by transport/logging
		if resp != nil && resp.Body != nil {
			// Drain and close the body to allow connection reuse for retry
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		if attempt < maxRetries-1 {
			var sleepDuration time.Duration
			if resp.StatusCode == http.StatusTooManyRequests {
				// Longer backoff for rate limits
				sleepDuration = time.Duration(attempt+1) * 5 * time.Second
				log.WithError(lastErr).Warnf("Rate limited. Retrying (%d/%d) after %s...\"", attempt+1, maxRetries, sleepDuration)
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
		// Close body if response exists and we failed
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return "", models.ApiResponse{}, lastErr
	}

ProcessResponse:
	// Body should be readable here (either original or replaced by logging transport)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body) // Read the final body
	if err != nil {
		log.WithError(err).Error("Error reading final response body")
		return "", models.ApiResponse{}, fmt.Errorf("error reading response body: %w", err)
	}

	var response models.ApiResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.WithError(err).Errorf("Error unmarshalling response JSON")
		log.Debugf("Response body causing unmarshal error: %s", string(body))
		return "", models.ApiResponse{}, fmt.Errorf("error unmarshalling response JSON: %w", err)
	}

	// Return the next cursor provided by the API
	return response.Metadata.NextCursor, response, nil
}

// ConvertQueryParamsToURLValues converts the QueryParameters struct into url.Values
// suitable for Civitai API requests.
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

// GetModelDetails fetches details for a specific model ID.
func (c *Client) GetModelDetails(modelID int) (models.Model, error) {
	reqURL := fmt.Sprintf("%s/models/%d", CivitaiApiBaseUrl, modelID)
	var modelDetails models.Model // Assuming models.Model is the correct struct

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		log.WithError(err).Errorf("Error creating request for model details %d", modelID)
		return modelDetails, fmt.Errorf("error creating request for model %d: %w", modelID, err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	}

	var resp *http.Response
	var lastErr error
	maxRetries := 3 // Or get from config if needed here too

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err = c.HttpClient.Do(req) // Transport will log if enabled

		if err != nil {
			lastErr = fmt.Errorf("http request failed for model details (attempt %d/%d): %w", attempt+1, maxRetries, err)
			if attempt < maxRetries-1 {
				log.WithError(err).Warnf("Retrying model details (%d/%d)...", attempt+1, maxRetries)
				time.Sleep(time.Duration(attempt+1) * 1 * time.Second) // Shorter backoff maybe?
				continue
			}
			break
		}

		// Body handled by logging transport if enabled

		switch resp.StatusCode {
		case http.StatusOK:
			lastErr = nil
			goto ProcessModelDetailsResponse
		case http.StatusUnauthorized, http.StatusForbidden:
			lastErr = ErrUnauthorized
			goto RequestModelDetailsFailed
		case http.StatusNotFound:
			lastErr = ErrNotFound
			goto RequestModelDetailsFailed
		default:
			lastErr = fmt.Errorf("API request for model details failed with status %d", resp.StatusCode)
			if resp.StatusCode >= 500 && attempt < maxRetries-1 {
				log.WithError(lastErr).Warnf("Server error on model details. Retrying (%d/%d)...", attempt+1, maxRetries)
				// Close body before retry
				if resp != nil && resp.Body != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
				time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
				continue // Retry server errors
			}
			goto RequestModelDetailsFailed // Non-retryable or max retries reached
		}
	}

RequestModelDetailsFailed:
	if lastErr != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return models.Model{}, lastErr
	}

ProcessModelDetailsResponse:
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body) // Read final body
	if err != nil {
		log.WithError(err).Error("Error reading final model details response body")
		return models.Model{}, fmt.Errorf("error reading model details response body: %w", err)
	}

	err = json.Unmarshal(body, &modelDetails)
	if err != nil {
		log.WithError(err).Errorf("Error unmarshalling model details JSON for model ID %d", modelID)
		log.Debugf("Response body causing unmarshal error: %s", string(body))
		return models.Model{}, fmt.Errorf("error unmarshalling model details JSON: %w", err)
	}

	return modelDetails, nil
}

// TODO: Add methods for other API endpoints (e.g., GetModelVersionByID)
