package main_test

import (
	"testing"
	// No testify needed if all assertions are within compareURL, which is in api_download_url_test.go
	// However, individual test functions might use t.Run which is fine.
	// models.Config is not directly used by these tests, only by compareURL's call to parseShowConfigOutput.
)

// Note: compareURL is defined in api_download_url_test.go and is accessible
// because both files are part of the main_test package.

func TestQueryParam_Query(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "download", "query", "flag_query", []string{"--query", "flag_query"}, "")
	})
	t.Run("ConfigOnly", func(t *testing.T) {
		compareURL(t, "download", "query", "config_query", []string{}, `[Download]
Query = "config_query"`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "download", "query", "flag_query", []string{"--query", "flag_query"}, `[Download]
Query = "config_query"`)
	})
	t.Run("Default", func(t *testing.T) {
		compareURL(t, "download", "query", "<OMIT>", []string{}, "")
	})
}

func TestQueryParam_Username(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "download", "username", "flag_user", []string{"--username", "flag_user"}, "")
	})
	t.Run("ConfigOnly (Uses First from List)", func(t *testing.T) {
		compareURL(t, "download", "username", "config_user1", []string{}, `[Download]
Usernames = ["config_user1", "config_user2"]`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "download", "username", "flag_user", []string{"--username", "flag_user"}, `[Download]
Usernames = ["config_user1"]`)
	})
	t.Run("Default", func(t *testing.T) {
		compareURL(t, "download", "username", "<OMIT>", []string{}, "")
	})
}

func TestQueryParam_PrimaryOnly(t *testing.T) {
	t.Run("FlagTrue", func(t *testing.T) {
		compareURL(t, "download", "primaryFileOnly", "true", []string{"--primary-only"}, "")
	})
	t.Run("FlagFalse", func(t *testing.T) {
		compareURL(t, "download", "primaryFileOnly", "<OMIT>", []string{"--primary-only=false"}, "")
	})
	t.Run("ConfigTrue", func(t *testing.T) {
		compareURL(t, "download", "primaryFileOnly", "true", []string{}, `[Download]
PrimaryOnly = true`)
	})
	t.Run("ConfigFalse", func(t *testing.T) {
		compareURL(t, "download", "primaryFileOnly", "<OMIT>", []string{}, `[Download]
PrimaryOnly = false`)
	})
	t.Run("Default (False)", func(t *testing.T) {
		compareURL(t, "download", "primaryFileOnly", "<OMIT>", []string{}, "")
	})
}

func TestQueryParam_Limit(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "download", "limit", "77", []string{"--limit", "77"}, "")
	})
	t.Run("ConfigOnly", func(t *testing.T) {
		compareURL(t, "download", "limit", "88", []string{}, `[Download]
Limit = 88`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "download", "limit", "77", []string{"--limit", "77"}, `[Download]
Limit = 88`)
	})
	t.Run("Default", func(t *testing.T) {
		compareURL(t, "download", "limit", "100", []string{}, "")
	})
}

func TestQueryParam_Sort(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "download", "sort", "Highest Rated", []string{"--sort", "Highest Rated"}, "")
	})
	t.Run("ConfigOnly", func(t *testing.T) {
		compareURL(t, "download", "sort", "Newest", []string{}, `[Download]
Sort = "Newest"`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "download", "sort", "Highest Rated", []string{"--sort", "Highest Rated"}, `[Download]
Sort = "Newest"`)
	})
	t.Run("Default", func(t *testing.T) {
		compareURL(t, "download", "sort", "Most Downloaded", []string{}, "")
	})
}

func TestQueryParam_Period(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "download", "period", "Week", []string{"--period", "Week"}, "")
	})
	t.Run("ConfigOnly", func(t *testing.T) {
		compareURL(t, "download", "period", "Month", []string{}, `[Download]
Period = "Month"`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "download", "period", "Week", []string{"--period", "Week"}, `[Download]
Period = "Month"`)
	})
	t.Run("Default", func(t *testing.T) {
		compareURL(t, "download", "period", "AllTime", []string{}, "")
	})
}

func TestQueryParam_Tag(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "download", "tag", "flag_tag", []string{"--tag", "flag_tag"}, "")
	})
	t.Run("ConfigOnly", func(t *testing.T) {
		compareURL(t, "download", "tag", "config_tag", []string{}, `[Download]
Tag = "config_tag"`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "download", "tag", "flag_tag", []string{"--tag", "flag_tag"}, `[Download]
Tag = "config_tag"`)
	})
	t.Run("Default", func(t *testing.T) {
		compareURL(t, "download", "tag", "<OMIT>", []string{}, "")
	})
}

func TestQueryParam_Types(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "download", "types", "[LORA, VAE]", []string{"--model-types", "LORA", "--model-types", "VAE"}, "")
	})
	t.Run("ConfigOnly", func(t *testing.T) {
		compareURL(t, "download", "types", "[Checkpoint]", []string{}, `[Download]
ModelTypes = ["Checkpoint"]`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "download", "types", "[LORA, VAE]", []string{"--model-types", "LORA", "--model-types", "VAE"}, `[Download]
ModelTypes = ["Checkpoint"]`)
	})
	t.Run("Default", func(t *testing.T) {
		compareURL(t, "download", "types", "<OMIT>", []string{}, "")
	})
}

func TestQueryParam_BaseModels(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "download", "baseModels", "[SDXL 1.0, Pony]", []string{"--base-models", "SDXL 1.0", "--base-models", "Pony"}, "")
	})
	t.Run("ConfigOnly", func(t *testing.T) {
		compareURL(t, "download", "baseModels", "[SD 1.5]", []string{}, `[Download]
BaseModels = ["SD 1.5"]`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "download", "baseModels", "[SDXL 1.0, Pony]", []string{"--base-models", "SDXL 1.0", "--base-models", "Pony"}, `[Download]
BaseModels = ["SD 1.5"]`)
	})
	t.Run("Default", func(t *testing.T) {
		compareURL(t, "download", "baseModels", "<OMIT>", []string{}, "")
	})
}

func TestQueryParam_Nsfw(t *testing.T) {
	t.Run("FlagTrue", func(t *testing.T) {
		compareURL(t, "download", "nsfw", "true", []string{"--nsfw"}, "")
	})
	t.Run("DefaultNsfwIsTrue", func(t *testing.T) {
		compareURL(t, "download", "nsfw", "true", []string{}, "")
	})
	t.Run("ConfigTrue", func(t *testing.T) {
		compareURL(t, "download", "nsfw", "true", []string{}, `[Download]
Nsfw = true`)
	})
	t.Run("ConfigFalse", func(t *testing.T) {
		compareURL(t, "download", "nsfw", "false", []string{}, `[Download]
Nsfw = false`)
	})
	t.Run("FlagTrueOverridesConfigFalse", func(t *testing.T) {
		compareURL(t, "download", "nsfw", "true", []string{"--nsfw"}, `[Download]
Nsfw = false`)
	})
	t.Run("FlagFalseOverridesConfigTrue", func(t *testing.T) {
		compareURL(t, "download", "nsfw", "false", []string{"--nsfw=false"}, `[Download]
Nsfw = true`)
	})
}

// --- Tests for 'images' command URL parameters (These belong in api_images_url_test.go) ---

func TestQueryParam_Images_ModelID(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "images", "modelId", "123", []string{"--model-id", "123"}, "")
	})
	t.Run("ConfigOnly", func(t *testing.T) {
		compareURL(t, "images", "modelId", "555", []string{}, `[Images]
ModelID = 555`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "images", "modelId", "123", []string{"--model-id", "123"}, `[Images]
ModelID = 555`)
	})
	t.Run("Default_Omitted", func(t *testing.T) {
		compareURL(t, "images", "modelId", "<OMIT>", []string{}, "")
	})
	t.Run("FlagIsZero_OverridesConfig_Omitted", func(t *testing.T) {
		compareURL(t, "images", "modelId", "<OMIT>", []string{"--model-id", "0"}, `[Images]
ModelID = 555`)
	})
}

func TestQueryParam_Images_Sort(t *testing.T) {
	t.Run("FlagOnly", func(t *testing.T) {
		compareURL(t, "images", "sort", "Most Reactions", []string{"--sort", "Most Reactions"}, "")
	})
	t.Run("ConfigOnly", func(t *testing.T) {
		compareURL(t, "images", "sort", "Oldest", []string{}, `[Images]
Sort = "Oldest"`)
	})
	t.Run("FlagOverridesConfig", func(t *testing.T) {
		compareURL(t, "images", "sort", "Most Reactions", []string{"--sort", "Most Reactions"}, `[Images]
Sort = "Oldest"`)
	})
	t.Run("Default_Newest", func(t *testing.T) {
		compareURL(t, "images", "sort", "Newest", []string{}, "")
	})
	t.Run("ConfigEmpty_Omitted", func(t *testing.T) {
		compareURL(t, "images", "sort", "<OMIT>", []string{}, `[Images]
Sort = ""`)
	})
	t.Run("FlagIsEmptyString_Omitted", func(t *testing.T) {
		compareURL(t, "images", "sort", "<OMIT>", []string{"--sort", ""}, "")
	})
}
