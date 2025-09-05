package paths

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"go-civitai-download/internal/helpers"
)

// Define allowed tags using a map for easy lookup
var allowedTags = map[string]struct{}{
	"modelId":     {},
	"modelName":   {},
	"modelType":   {},
	"creatorName": {},
	"username":    {}, // For images API compatibility
	"versionId":   {},
	"versionName": {},
	"baseModel":   {},
	"imageId":     {}, // For images API compatibility
	// Add more tags here if needed in the future
}

// Regex to find tags like {tagName}
var tagRegex = regexp.MustCompile(`\{([^}]+)\}`)

// GeneratePath substitutes placeholders in a pattern string with sanitized values from the data map.
// It returns the generated relative path string or an error if substitution fails.
func GeneratePath(pattern string, data map[string]string) (string, error) {
	generatedPath := pattern

	// Find all tags in the pattern
	matches := tagRegex.FindAllStringSubmatch(pattern, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue // Should not happen with the regex, but safety check
		}
		tagName := match[1]       // e.g., "modelName"
		tagWithBraces := match[0] // e.g., "{modelName}"

		// Check if it's a known tag
		if _, allowed := allowedTags[tagName]; !allowed {
			return "", fmt.Errorf("unknown tag found in path pattern: %s", tagWithBraces)
		}

		// Get the value from the data map
		value, ok := data[tagName]

		// Sanitize the value for use in a file path.
		// If the original value from data map is missing or empty, slug will also be empty.
		var sanitizedValue string
		if ok {
			sanitizedValue = helpers.ConvertToSlug(value)
		} else {
			// Value was not in data map, treat as if it were an empty string for slugging purposes.
			// helpers.ConvertToSlug("") returns ""
			sanitizedValue = helpers.ConvertToSlug("")
		}

		if sanitizedValue == "" {
			// If slug is empty (either from missing data, empty data, or data that slugs to empty),
			// use "empty_<tagName>" as the fallback.
			// This ensures that an empty version.BaseModel results in "empty_basemodel".
			sanitizedValue = "empty_" + tagName
		}

		// Replace the tag in the path string
		generatedPath = strings.ReplaceAll(generatedPath, tagWithBraces, sanitizedValue)
	}

	// Final cleanup
	cleanedPath := filepath.Clean(generatedPath)
	if cleanedPath == "." || cleanedPath == "" {
		// If the pattern itself is invalid leading to an empty path, return an error
		// Consider returning a default path like "_invalid_pattern_" instead of erroring?
		// For now, error out.
		return "", fmt.Errorf("generated path pattern resulted in an empty or invalid path: '%s'", pattern)
	}
	// Ensure it's a relative path
	cleanedPath = strings.TrimPrefix(cleanedPath, string(filepath.Separator))

	// Security check: Prevent path traversal
	if strings.Contains(cleanedPath, "..") {
		return "", fmt.Errorf("generated path contains invalid sequence '..': %s", cleanedPath)
	}

	return cleanedPath, nil
}
