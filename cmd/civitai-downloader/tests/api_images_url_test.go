package main_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImages_DebugPrintAPIURL checks API URL generation for the images command
func TestImages_DebugPrintAPIURL(t *testing.T) {
	tempCfgPath := createTempConfig(t, "") // Use empty config

	stdout, _, err := runCommand(t, "--config", tempCfgPath, "--model-id", "123", "--limit", "50", "--sort", "Most Reactions", "--nsfw", "None", "debug", "print-api-url", "images")

	require.NoError(t, err, "Command exited with error")

	expectedBase := "https://civitai.com/api/v1/images?"
	assert.Contains(t, stdout, expectedBase, "Output should contain base images API URL")
	assert.Contains(t, stdout, "modelId=123", "Output URL should contain modelId param from flag")
	assert.Contains(t, stdout, "limit=50", "Output URL should contain limit param from flag")
	assert.Contains(t, stdout, "sort=Most+Reactions", "Output URL should contain sort param from flag (URL encoded)")
	assert.Contains(t, stdout, "nsfw=false", "Output URL should contain nsfw=false param for --nsfw=None flag")
}

// TestImages_APIURL_PostID checks the --post-id flag for the images command URL
func TestImages_APIURL_PostID(t *testing.T) {
	tempCfgPath := createTempConfig(t, "")
	stdout, _, err := runCommand(t, "--config", tempCfgPath, "--post-id", "987", "debug", "print-api-url", "images")
	require.NoError(t, err)
	assert.Contains(t, stdout, "postId=987", "URL should contain postId param from flag")
	assert.Contains(t, stdout, "https://civitai.com/api/v1/images?", "URL should contain base images URL")
	assert.NotContains(t, stdout, "modelId=", "URL should not contain modelId when postId is used")
	assert.NotContains(t, stdout, "modelVersionId=", "URL should not contain modelVersionId when postId is used")
	assert.NotContains(t, stdout, "username=", "URL should not contain username when postId is used")
}

// TestImages_APIURL_Period checks the --period flag for the images command URL
func TestImages_APIURL_Period(t *testing.T) {
	tempCfgPath := createTempConfig(t, "")
	stdout, _, err := runCommand(t, "--config", tempCfgPath, "--model-id", "111", "--period", "Week", "debug", "print-api-url", "images")
	require.NoError(t, err)
	assert.Contains(t, stdout, "period=Week", "URL should contain period param from flag")
}

// TestImages_APIURL_Username checks the --username flag for the images command URL
func TestImages_APIURL_Username(t *testing.T) {
	tempCfgPath := createTempConfig(t, "")
	stdout, _, err := runCommand(t, "--config", tempCfgPath, "--username", "testuser", "debug", "print-api-url", "images")
	require.NoError(t, err)
	assert.Contains(t, stdout, "username=testuser", "URL should contain username param from flag")
	assert.NotContains(t, stdout, "modelId=", "URL should not contain modelId")
	assert.NotContains(t, stdout, "modelVersionId=", "URL should not contain modelVersionId")
}

// TestImages_APIURL_Nsfw checks the --nsfw flag for the images command URL
func TestImages_APIURL_Nsfw(t *testing.T) {
	tempCfgPath := createTempConfig(t, "")
	tests := []struct {
		name          string
		nsfwFlag      string
		expectedParam string
		shouldOmit    bool
	}{
		{"None", "None", "nsfw=false", false},
		{"Soft", "Soft", "nsfw=true", false},
		{"Mature", "Mature", "nsfw=true", false},
		{"X", "X", "nsfw=true", false},
		{"Empty (All)", "", "nsfw=false", false},
		{"TrueString", "true", "nsfw=true", false},
		{"FalseString", "false", "nsfw=false", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--config", tempCfgPath, "--model-id", "999"}
			if tc.nsfwFlag != "" {
				args = append(args, "--nsfw", tc.nsfwFlag)
			}
			args = append(args, "debug", "print-api-url", "images")

			stdout, _, err := runCommand(t, args...)
			require.NoError(t, err)
			if tc.shouldOmit {
				assert.NotContains(t, stdout, "nsfw=", "URL should omit nsfw param")
			} else {
				assert.Contains(t, stdout, tc.expectedParam, "URL should contain expected nsfw param")
			}
		})
	}
}

// TestImages_APIURL_Combined checks a combination of flags for the images command URL
func TestImages_APIURL_Combined(t *testing.T) {
	tempCfgPath := createTempConfig(t, "")
	args := []string{
		"--config", tempCfgPath,
		"--model-id", "777",
		"--limit", "42",
		"--sort", "Most Comments",
		"--period", "Year",
		"--nsfw", "Mature",
		"debug", "print-api-url", "images",
	}
	stdout, _, err := runCommand(t, args...)
	require.NoError(t, err)

	assert.Contains(t, stdout, "limit=42")
	assert.Contains(t, stdout, "sort=Most+Comments")
	assert.Contains(t, stdout, "period=Year")
	assert.Contains(t, stdout, "nsfw=true")
}
