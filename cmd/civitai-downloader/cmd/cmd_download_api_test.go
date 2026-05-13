package cmd

import (
	"testing"

	"go-civitai-download/internal/models"
)

func TestPassesBaseModelsFilter(t *testing.T) {
	tests := []struct {
		name       string
		version    models.ModelVersion
		baseModels []string
		want       bool
	}{
		{
			name:       "no filter set - passes all",
			version:    models.ModelVersion{ID: 1, Name: "v1", BaseModel: "SD 1.5"},
			baseModels: []string{},
			want:       true,
		},
		{
			name:       "matching base model - passes",
			version:    models.ModelVersion{ID: 1, Name: "v1", BaseModel: "SD 1.5"},
			baseModels: []string{"SD 1.5"},
			want:       true,
		},
		{
			name:       "case-insensitive match - passes",
			version:    models.ModelVersion{ID: 1, Name: "v1", BaseModel: "SDXL 1.0"},
			baseModels: []string{"sdxl 1.0"},
			want:       true,
		},
		{
			name:       "non-matching base model - fails",
			version:    models.ModelVersion{ID: 1, Name: "v1", BaseModel: "Pony"},
			baseModels: []string{"SD 1.5"},
			want:       false,
		},
		{
			name:       "empty base model with filter active - fails",
			version:    models.ModelVersion{ID: 1, Name: "v1", BaseModel: ""},
			baseModels: []string{"SD 1.5"},
			want:       false,
		},
		{
			name:       "matches one of multiple filters - passes",
			version:    models.ModelVersion{ID: 1, Name: "v1", BaseModel: "SDXL 1.0"},
			baseModels: []string{"SD 1.5", "SDXL 1.0", "Pony"},
			want:       true,
		},
		{
			name:       "no match in multiple filters - fails",
			version:    models.ModelVersion{ID: 1, Name: "v1", BaseModel: "Flux"},
			baseModels: []string{"SD 1.5", "SDXL 1.0", "Pony"},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := models.Config{
				Download: models.DownloadConfig{
					BaseModels: tt.baseModels,
				},
			}
			got := passesBaseModelsFilter(tt.version, &cfg)
			if got != tt.want {
				t.Errorf("passesBaseModelsFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldSkipModelForTags(t *testing.T) {
	tests := []struct {
		name       string
		model      models.Model
		ignoreTags []string
		want       bool
	}{
		{
			name:       "no ignore tags configured - never skip",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{"anime", "style"}},
			ignoreTags: []string{},
			want:       false,
		},
		{
			name:       "model has no tags - never skip",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{}},
			ignoreTags: []string{"anime"},
			want:       false,
		},
		{
			name:       "model has nil tags - never skip",
			model:      models.Model{ID: 1, Name: "TestModel"},
			ignoreTags: []string{"anime"},
			want:       false,
		},
		{
			name:       "exact match - skip",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{"anime", "style"}},
			ignoreTags: []string{"anime"},
			want:       true,
		},
		{
			name:       "case-insensitive match - skip",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{"Anime", "Style"}},
			ignoreTags: []string{"anime"},
			want:       true,
		},
		{
			name:       "no matching tags - do not skip",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{"photorealistic", "portrait"}},
			ignoreTags: []string{"anime"},
			want:       false,
		},
		{
			name:       "matches one of multiple ignore tags - skip",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{"anime", "style"}},
			ignoreTags: []string{"nsfw", "anime", "deprecated"},
			want:       true,
		},
		{
			name:       "matches one of multiple model tags - skip",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{"photorealistic", "nsfw", "portrait"}},
			ignoreTags: []string{"nsfw"},
			want:       true,
		},
		{
			name:       "no match in multiple tags vs multiple ignores - do not skip",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{"photorealistic", "portrait", "landscape"}},
			ignoreTags: []string{"anime", "nsfw", "deprecated"},
			want:       false,
		},
		{
			name:       "empty string in ignore tags is ignored",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{"anime"}},
			ignoreTags: []string{""},
			want:       false,
		},
		{
			name:       "both empty - do not skip",
			model:      models.Model{ID: 1, Name: "TestModel", Tags: []string{}},
			ignoreTags: []string{},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := models.Config{
				Download: models.DownloadConfig{
					IgnoreTags: tt.ignoreTags,
				},
			}
			got := shouldSkipModelForTags(tt.model, &cfg)
			if got != tt.want {
				t.Errorf("shouldSkipModelForTags() = %v, want %v", got, tt.want)
			}
		})
	}
}
