package main_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compareURL (moved from main_integration_test.go)
func compareURL(t *testing.T, command string, paramKeyURL, expectedValue string, flags []string, configContent string) {
	t.Helper()
	tempCfgPath := createTempConfig(t, configContent)
	// Construct args: persistent flags first, then full debug command path, then command-specific local flags
	args := []string{"--config", tempCfgPath, "debug", "print-api-url", command}
	args = append(args, flags...) // Append command-specific local flags AFTER the full command path

	stdoutDebugURL, _, errDebugURL := runCommand(t, args...)
	require.NoError(t, errDebugURL)
	require.Contains(t, stdoutDebugURL, "?", "URL output missing query string")
	urlQueryPart := stdoutDebugURL[strings.Index(stdoutDebugURL, "?")+1:]
	urlQueryPart = strings.TrimSpace(urlQueryPart)
	parsedURLQuery, errParseQuery := url.ParseQuery(urlQueryPart)
	require.NoError(t, errParseQuery)

	if expectedValue != "<OMIT>" {
		require.Contains(t, parsedURLQuery, paramKeyURL, fmt.Sprintf("URL missing %s param", paramKeyURL))
		if strings.HasPrefix(expectedValue, "[") && strings.HasSuffix(expectedValue, "]") {
			expectedItems := strings.Split(strings.Trim(expectedValue, "[]"), ", ")
			actualItems := parsedURLQuery[paramKeyURL]
			assert.ElementsMatch(t, expectedItems, actualItems, fmt.Sprintf("URL %s param list mismatch", paramKeyURL))
		} else {
			assert.Equal(t, expectedValue, parsedURLQuery.Get(paramKeyURL), fmt.Sprintf("URL %s param value mismatch", paramKeyURL))
		}
	} else {
		assert.NotContains(t, parsedURLQuery, paramKeyURL, fmt.Sprintf("URL should not contain %s param", paramKeyURL))
	}
}

// TestDownload_DebugPrintAPIURL checks if the debug command prints the URL
func TestDownload_DebugPrintAPIURL(t *testing.T) {
	configContent := `
[Download]
Query = "test query"
ModelTypes = ["LORA"]
Limit = 25
`
	tempCfgPath := createTempConfig(t, configContent)

	stdout, _, err := runCommand(t, "--config", tempCfgPath, "--sort", "Newest", "--period", "Week", "debug", "print-api-url", "download")
	require.NoError(t, err, "Command exited with error")

	expectedBase := "https://civitai.com/api/v1/models?"
	assert.Contains(t, stdout, expectedBase, "Output should contain base API URL")
	assert.Contains(t, stdout, "query=test+query", "Output URL should contain query param from config")
	assert.Contains(t, stdout, "types=LORA", "Output URL should contain types param from config")
	assert.Contains(t, stdout, "limit=25", "Output URL should contain limit param from config")
	assert.Contains(t, stdout, "sort=Newest", "Output URL should contain sort param from flag")
	assert.Contains(t, stdout, "period=Week", "Output URL should contain period param from flag")
}

// TestDownload_APIURL_Tags verifies the --tag flag populates the tag= query parameter.
func TestDownload_APIURL_Tags(t *testing.T) {
	tempCfgPath := createTempConfig(t, "") // Empty config

	stdout, _, err := runCommand(t, "--config", tempCfgPath, "--tag", "testtag", "debug", "print-api-url", "download")
	require.NoError(t, err, "Command execution failed for single tag")
	assert.Contains(t, stdout, "tag=testtag", "URL should contain tag parameter from flag")
}

// TestDownload_APIURL_Username verifies the --username flag populates the username= query parameter.
func TestDownload_APIURL_Username(t *testing.T) {
	tempCfgPath := createTempConfig(t, "") // Empty config

	stdout, _, err := runCommand(t, "--config", tempCfgPath, "--username", "testuser", "debug", "print-api-url", "download")
	require.NoError(t, err, "Command execution failed for username flag")
	assert.Contains(t, stdout, "username=testuser", "URL should contain username parameter from flag")
}

// TestDownload_APIURL_NoUser verifies username is not added to URL when flag is omitted.
func TestDownload_APIURL_NoUser(t *testing.T) {
	tempCfgPath := createTempConfig(t, "") // Empty config

	stdout, _, err := runCommand(t, "--config", tempCfgPath, "--limit", "10", "debug", "print-api-url", "download") // Add another flag for baseline
	require.NoError(t, err, "Command execution failed")
	assert.NotContains(t, stdout, "username=", "URL should not contain username parameter when --username flag is omitted")
	assert.Contains(t, stdout, "limit=10", "URL should still contain other parameters like limit")
}
