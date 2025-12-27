package paths

import (
	"strings"
	"testing"
)

func TestGeneratePath_BasicSubstitution(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		data     map[string]string
		expected string
		wantErr  bool
	}{
		{
			name:     "single placeholder",
			pattern:  "{modelName}",
			data:     map[string]string{"modelName": "Test Model"},
			expected: "test_model", // spaces become underscores
			wantErr:  false,
		},
		{
			name:     "multiple placeholders",
			pattern:  "{modelType}/{modelName}/{versionName}",
			data:     map[string]string{"modelType": "Checkpoint", "modelName": "My Model", "versionName": "v1.0"},
			expected: "checkpoint/my_model/v1.0", // spaces become underscores
			wantErr:  false,
		},
		{
			name:     "with creator name",
			pattern:  "{creatorName}/{modelName}",
			data:     map[string]string{"creatorName": "TestUser", "modelName": "Cool Model"},
			expected: "testuser/cool_model", // spaces become underscores
			wantErr:  false,
		},
		{
			name:     "with model and version IDs",
			pattern:  "{modelId}/{versionId}",
			data:     map[string]string{"modelId": "12345", "versionId": "67890"},
			expected: "12345/67890",
			wantErr:  false,
		},
		{
			name:     "image path pattern",
			pattern:  "{username}/{baseModel}/{imageId}",
			data:     map[string]string{"username": "artist", "baseModel": "SDXL 1.0", "imageId": "999"},
			expected: "artist/sdxl_1.0/999", // spaces become underscores
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GeneratePath(tt.pattern, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("GeneratePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("GeneratePath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGeneratePath_EmptyValues(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		data     map[string]string
		expected string
		wantErr  bool
	}{
		{
			name:     "missing value uses fallback",
			pattern:  "{modelName}/{baseModel}",
			data:     map[string]string{"modelName": "Test"},
			expected: "test/empty_baseModel",
			wantErr:  false,
		},
		{
			name:     "empty string value uses fallback",
			pattern:  "{modelName}/{baseModel}",
			data:     map[string]string{"modelName": "Test", "baseModel": ""},
			expected: "test/empty_baseModel",
			wantErr:  false,
		},
		{
			name:     "all empty values",
			pattern:  "{modelName}/{versionName}",
			data:     map[string]string{},
			expected: "empty_modelName/empty_versionName",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GeneratePath(tt.pattern, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("GeneratePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("GeneratePath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGeneratePath_UnknownTags(t *testing.T) {
	tests := []struct {
		pattern string
		data    map[string]string
		name    string
	}{
		{
			name:    "unknown tag",
			pattern: "{unknownTag}",
			data:    map[string]string{"unknownTag": "value"},
		},
		{
			name:    "mixed known and unknown tags",
			pattern: "{modelName}/{unknownTag}",
			data:    map[string]string{"modelName": "Test", "unknownTag": "value"},
		},
		{
			name:    "typo in tag name",
			pattern: "{modelNam}",
			data:    map[string]string{"modelName": "Test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GeneratePath(tt.pattern, tt.data)
			if err == nil {
				t.Errorf("GeneratePath() expected error for unknown tag, got nil")
			}
			if !strings.Contains(err.Error(), "unknown tag") {
				t.Errorf("GeneratePath() error should mention 'unknown tag', got: %v", err)
			}
		})
	}
}

func TestGeneratePath_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		data     map[string]string
		expected string
		wantErr  bool
	}{
		{
			name:     "special characters sanitized",
			pattern:  "{modelName}",
			data:     map[string]string{"modelName": "Test@Model#With$Special%Chars"},
			expected: "testmodelwithspecialchars",
			wantErr:  false,
		},
		{
			name:     "spaces converted to underscores",
			pattern:  "{modelName}",
			data:     map[string]string{"modelName": "Test Model With Spaces"},
			expected: "test_model_with_spaces",
			wantErr:  false,
		},
		{
			name:     "underscores preserved",
			pattern:  "{modelName}",
			data:     map[string]string{"modelName": "test_model_name"},
			expected: "test_model_name",
			wantErr:  false,
		},
		{
			name:     "dots preserved",
			pattern:  "{versionName}",
			data:     map[string]string{"versionName": "v1.0.0"},
			expected: "v1.0.0",
			wantErr:  false,
		},
		{
			name:     "dashes preserved",
			pattern:  "{modelName}",
			data:     map[string]string{"modelName": "my-cool-model"},
			expected: "my-cool-model",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GeneratePath(tt.pattern, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("GeneratePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("GeneratePath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGeneratePath_PathTraversal(t *testing.T) {
	// Path traversal attempts should be blocked
	tests := []struct {
		pattern string
		data    map[string]string
		name    string
	}{
		{
			name:    "direct path traversal in data",
			pattern: "{modelName}",
			data:    map[string]string{"modelName": "../../../etc/passwd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GeneratePath(tt.pattern, tt.data)
			// The sanitization should remove the dots, so it shouldn't error
			// but the result should NOT contain ".."
			if err != nil {
				// If it errors, that's also acceptable security behavior
				return
			}
			if strings.Contains(got, "..") {
				t.Errorf("GeneratePath() result contains path traversal: %v", got)
			}
		})
	}
}

func TestGeneratePath_NoPlaceholders(t *testing.T) {
	// Pattern without any placeholders
	got, err := GeneratePath("static/path/here", map[string]string{})
	if err != nil {
		t.Errorf("GeneratePath() unexpected error: %v", err)
	}
	if got != "static/path/here" {
		t.Errorf("GeneratePath() = %v, want static/path/here", got)
	}
}

func TestGeneratePath_AllAllowedTags(t *testing.T) {
	// Verify all allowed tags work
	allowedTagList := []string{
		"modelId", "modelName", "modelType", "creatorName",
		"username", "versionId", "versionName", "baseModel", "imageId",
	}

	for _, tag := range allowedTagList {
		t.Run(tag, func(t *testing.T) {
			pattern := "{" + tag + "}"
			data := map[string]string{tag: "test-value"}
			got, err := GeneratePath(pattern, data)
			if err != nil {
				t.Errorf("GeneratePath() with tag %s returned error: %v", tag, err)
			}
			if got != "test-value" {
				t.Errorf("GeneratePath() with tag %s = %v, want test-value", tag, got)
			}
		})
	}
}

func TestGeneratePath_ComplexPatterns(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		data     map[string]string
		expected string
		wantErr  bool
	}{
		{
			name:    "realistic download path pattern",
			pattern: "{creatorName}/{modelName}/{versionName}",
			data: map[string]string{
				"creatorName": "ArtistName",
				"modelName":   "Amazing Model v2",
				"versionName": "v2.1-Final",
			},
			expected: "artistname/amazing_model_v2/v2.1-final", // spaces become underscores
			wantErr:  false,
		},
		{
			name:    "realistic model info path pattern",
			pattern: "{creatorName}/{modelName}/model.info.json",
			data: map[string]string{
				"creatorName": "Creator",
				"modelName":   "Model",
			},
			expected: "creator/model/model.info.json",
			wantErr:  false,
		},
		{
			name:    "nested directory structure",
			pattern: "{modelType}/{baseModel}/{creatorName}/{modelName}/{versionName}",
			data: map[string]string{
				"modelType":   "LORA",
				"baseModel":   "SD 1.5",
				"creatorName": "User123",
				"modelName":   "Cool Lora",
				"versionName": "v1",
			},
			expected: "lora/sd_1.5/user123/cool_lora/v1", // spaces become underscores
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GeneratePath(tt.pattern, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("GeneratePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("GeneratePath() = %v, want %v", got, tt.expected)
			}
		})
	}
}
