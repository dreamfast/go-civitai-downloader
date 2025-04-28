package cmd

import "go-civitai-download/internal/models"

// potentialDownload holds information about a model file version that might be downloaded.
type potentialDownload struct {
	ModelID           int                 // Added: ID of the parent model
	FullModel         models.Model        // Added: Full model details (used for Model Images)
	ModelName         string              // Name of the parent model
	ModelType         string              // Type of the parent model (Checkpoint, LORA, etc.)
	Creator           models.Creator      // Creator info
	FullVersion       models.ModelVersion // Full details of this specific version
	ModelVersionID    int                 // ID of this specific version
	File              models.File         // Details of the specific file to download
	TargetFilepath    string              // Calculated final path for the file (incl. version ID prefix)
	FinalBaseFilename string              // Base filename including version ID prefix
	OriginalImages    []models.ModelImage // Images associated with this version
	BaseModel         string              // Base model string (e.g., "SD 1.5")
	Slug              string              // Model name slug
	VersionName       string              // Name of the model version
	CleanedVersion    models.ModelVersion // Store cleaned version for DB entry
}

// downloadJob represents a download task passed to workers.
type downloadJob struct {
	PotentialDownload potentialDownload // The download details
	DatabaseKey       string            // Key for database operations (e.g., "v_12345")
}
