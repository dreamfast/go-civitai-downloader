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
