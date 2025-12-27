package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// UserAgent is the browser User-Agent string used for HTTP requests to avoid 401 errors
const UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// Client struct for interacting with the Civitai API
type Client struct {
	// Pointer first
	HttpClient *http.Client // Use a shared client
	// String
	ApiKey string
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

// RetryableHTTPRequest executes an HTTP request with unified retry logic
func (c *Client) RetryableHTTPRequest(req *http.Request) (*http.Response, error) {
	const maxRetries = 3

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := c.HttpClient.Do(req)

		if err != nil {
			lastErr = fmt.Errorf("http request failed (attempt %d/%d): %w", attempt+1, maxRetries, err)
			if attempt < maxRetries-1 {
				log.WithError(err).Warnf("Retrying (%d/%d)...", attempt+1, maxRetries)
				time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
				continue
			}
			break
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return resp, nil
		case http.StatusTooManyRequests:
			lastErr = ErrRateLimited
			if attempt < maxRetries-1 {
				sleepDuration := time.Duration(attempt+1) * 5 * time.Second
				log.WithError(lastErr).Warnf("Rate limited. Retrying (%d/%d) after %s...", attempt+1, maxRetries, sleepDuration)
				c.closeResponseBody(resp)
				time.Sleep(sleepDuration)
				continue
			}
		case http.StatusUnauthorized, http.StatusForbidden:
			c.closeResponseBody(resp)
			return nil, ErrUnauthorized
		case http.StatusNotFound:
			c.closeResponseBody(resp)
			return nil, ErrNotFound
		case http.StatusServiceUnavailable:
			lastErr = fmt.Errorf("%w (status code 503)", ErrServerError)
		default:
			if resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("%w (status code %d)", ErrServerError, resp.StatusCode)
			} else {
				c.closeResponseBody(resp)
				return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
			}
		}

		// Retryable error - close body before retry
		c.closeResponseBody(resp)

		if attempt < maxRetries-1 {
			var sleepDuration time.Duration
			if resp.StatusCode == http.StatusTooManyRequests {
				sleepDuration = time.Duration(attempt+1) * 5 * time.Second
			} else {
				sleepDuration = time.Duration(attempt+1) * 3 * time.Second
			}
			log.WithError(lastErr).Warnf("Server error. Retrying (%d/%d) after %s...", attempt+1, maxRetries, sleepDuration)
			time.Sleep(sleepDuration)
		} else {
			log.WithError(lastErr).Errorf("Request failed after %d attempts with status %d", maxRetries, resp.StatusCode)
		}
	}

	return nil, lastErr
}

// closeResponseBody safely closes response body and drains it for connection reuse
func (c *Client) closeResponseBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

// GetModels fetches models based on query parameters, using cursor pagination.
// Accepts the cursor for the next page. Returns the next cursor and the response.
func (c *Client) GetModels(cursor string, queryParams models.QueryParameters) (string, models.ApiResponse, error) {
	values := ConvertQueryParamsToURLValues(queryParams)

	if cursor != "" {
		values.Add("cursor", cursor)
	}

	reqURL := fmt.Sprintf("%s/models?%s", CivitaiApiBaseUrl, values.Encode())

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		log.WithError(err).Errorf("Error creating request for %s", reqURL)
		return "", models.ApiResponse{}, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if c.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	}

	resp, err := c.RetryableHTTPRequest(req)
	if err != nil {
		return "", models.ApiResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
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

	return response.Metadata.NextCursor.String(), response, nil
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
	var modelDetails models.Model

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		log.WithError(err).Errorf("Error creating request for model details %d", modelID)
		return modelDetails, fmt.Errorf("error creating request for model %d: %w", modelID, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if c.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	}

	resp, err := c.RetryableHTTPRequest(req)
	if err != nil {
		return models.Model{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
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

// GetModelVersionDetails fetches details for a specific model version ID.
func (c *Client) GetModelVersionDetails(versionID int) (models.ModelVersion, error) {
	reqURL := fmt.Sprintf("%s/model-versions/%d", CivitaiApiBaseUrl, versionID)
	var versionDetails models.ModelVersion

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return versionDetails, fmt.Errorf("error creating request for model version %d: %w", versionID, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if c.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	}

	resp, err := c.RetryableHTTPRequest(req)
	if err != nil {
		return versionDetails, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return versionDetails, fmt.Errorf("error reading model version details response body: %w", err)
	}

	err = json.Unmarshal(body, &versionDetails)
	if err != nil {
		log.Debugf("Response body causing unmarshal error: %s", string(body))
		return versionDetails, fmt.Errorf("error unmarshalling model version details JSON: %w", err)
	}

	return versionDetails, nil
}

// GetImages fetches images based on query parameters, using cursor pagination.
func (c *Client) GetImages(cursor string, queryParams models.ImageAPIParameters) (string, models.ImageApiResponse, error) {
	values := ConvertImageAPIParamsToURLValues(queryParams)
	if cursor != "" {
		values.Add("cursor", cursor)
	}

	reqURL := fmt.Sprintf("%s/images?%s", CivitaiApiBaseUrl, values.Encode())
	var response models.ImageApiResponse

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", response, fmt.Errorf("error creating request for images: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if c.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	}

	resp, err := c.RetryableHTTPRequest(req)
	if err != nil {
		return "", response, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", response, fmt.Errorf("error reading image response body: %w", err)
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		log.Debugf("Response body causing unmarshal error: %s", string(body))
		return "", response, fmt.Errorf("error unmarshalling image response JSON: %w", err)
	}

	return response.Metadata.NextCursor.String(), response, nil
}

// ConvertImageAPIParamsToURLValues converts the ImageAPIParameters struct into url.Values
// suitable for Civitai /api/v1/images requests.
func ConvertImageAPIParamsToURLValues(queryParams models.ImageAPIParameters) url.Values {
	values := url.Values{}

	if queryParams.ImageID != 0 {
		values.Add("imageId", strconv.Itoa(queryParams.ImageID))
	}
	if queryParams.ModelID != 0 {
		values.Add("modelId", strconv.Itoa(queryParams.ModelID))
	}
	if queryParams.ModelVersionID != 0 {
		values.Add("modelVersionId", strconv.Itoa(queryParams.ModelVersionID))
	}
	if queryParams.PostID != 0 {
		values.Add("postId", strconv.Itoa(queryParams.PostID))
	}
	if queryParams.Username != "" {
		values.Add("username", queryParams.Username)
	}

	// Handle Limit: API default is 100, max 200.
	// If 0, it could mean let API use its default, or it's an invalid user input.
	// For now, only add if > 0. The calling function should ensure it's within API constraints if set.
	if queryParams.Limit > 0 {
		values.Add("limit", strconv.Itoa(queryParams.Limit))
	}

	if queryParams.Sort != "" {
		values.Add("sort", queryParams.Sort)
	}
	if queryParams.Period != "" {
		values.Add("period", queryParams.Period)
	}

	// Handle Nsfw: Can be "None", "Soft", "Mature", "X", "true", "false".
	// If "None", map to "false". Empty string means omit.
	// Otherwise, use the string value directly.
	if queryParams.Nsfw != "" {
		if strings.ToLower(queryParams.Nsfw) == "none" {
			values.Add("nsfw", "false")
		} else {
			values.Add("nsfw", queryParams.Nsfw) // Pass "true", "false", "Soft", "Mature", "X" directly
		}
	}

	// Cursor is added by the calling loop, similar to GetModels
	// if queryParams.Cursor != "" {
	// 	values.Add("cursor", queryParams.Cursor)
	// }

	return values
}

// TODO: Add methods for other API endpoints (e.g., GetModelVersionByID)
