package main_test

import (
	"testing"

	// Assuming models.Config is used by parseShowConfigOutput

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDownloadShowConfig_Defaults verifies default values when using an empty temp config
func TestDownloadShowConfig_Defaults(t *testing.T) {
	tempCfgPath := createTempConfig(t, "") // Empty config

	stdout, _, err := runCommand(t, "--config", tempCfgPath, "debug", "show-config")
	require.NoError(t, err, "Command execution failed")

	parsed := parseShowConfigOutput(t, stdout)

	assert.Equal(t, "Most Downloaded", parsed.Download.Sort, "Default Sort mismatch")
	assert.Equal(t, 0, parsed.Download.Limit, "Default Limit mismatch")
	assert.Equal(t, false, parsed.Download.AllVersions, "Default DownloadAllVersions mismatch")
}

// TestDownloadShowConfig_ConfigLoad verifies loading values from config
func TestDownloadShowConfig_ConfigLoad(t *testing.T) {
	configContent := `
[Download]
Limit = 55
Sort = "Newest"
AllVersions = true
`
	tempCfgPath := createTempConfig(t, configContent)

	stdout, _, err := runCommand(t, "--log-level", "debug", "--config", tempCfgPath, "debug", "show-config")
	require.NoError(t, err, "Command execution failed")

	parsed := parseShowConfigOutput(t, stdout)

	assert.Equal(t, 55, parsed.Download.Limit, "Config Limit mismatch")
	assert.Equal(t, "Newest", parsed.Download.Sort, "Config Sort mismatch")
	assert.Equal(t, true, parsed.Download.AllVersions, "AllVersions=true in config should now be reflected")
}

// TestDownloadShowConfig_FlagOverride verifies command flags override config
func TestDownloadShowConfig_FlagOverride(t *testing.T) {
	configContent := `
[Download]
Limit = 55
Sort = "Newest"
Period = "Day"
`
	tempCfgPath := createTempConfig(t, configContent)

	// Pass overriding flags AFTER the full command path
	stdout, _, err := runCommand(t, "--config", tempCfgPath, "debug", "show-config", "--limit", "66", "--sort", "Highest Rated")
	require.NoError(t, err, "Command execution failed")

	parsed := parseShowConfigOutput(t, stdout)

	assert.Equal(t, 66, parsed.Download.Limit, "Config Limit should be overridden by flag")
	assert.Equal(t, "Highest Rated", parsed.Download.Sort, "Config Sort should be overridden by flag")
	assert.Equal(t, "Day", parsed.Download.Period, "Config Period should be from file (not overridden)")
}

// TestDownloadShowConfig_BooleanLoadIssue specifically tests boolean loading
func TestDownloadShowConfig_BooleanLoadIssue(t *testing.T) {
	configContent := `
[Download]
AllVersions = true
MetaOnly = true
ModelImages = true
SkipConfirmation = true
PrimaryOnly = true
Pruned = true
Fp16 = true
SaveMetadata = true
SaveModelInfo = true
SaveVersionImages = true
`
	tempCfgPath := createTempConfig(t, configContent)

	stdout, _, err := runCommand(t, "--config", tempCfgPath, "debug", "show-config")
	require.NoError(t, err, "Command execution failed")

	parsed := parseShowConfigOutput(t, stdout)

	assert.Equal(t, true, parsed.Download.AllVersions, "AllVersions mismatch")
	assert.Equal(t, true, parsed.Download.DownloadMetaOnly, "MetaOnly mismatch")
	assert.Equal(t, true, parsed.Download.SaveModelImages, "ModelImages mismatch")
	assert.Equal(t, true, parsed.Download.SkipConfirmation, "SkipConfirmation mismatch")
	assert.Equal(t, true, parsed.Download.PrimaryOnly, "PrimaryOnly mismatch")
	assert.Equal(t, true, parsed.Download.Pruned, "Pruned mismatch")
	assert.Equal(t, true, parsed.Download.Fp16, "Fp16 mismatch")
	assert.Equal(t, true, parsed.Download.SaveMetadata, "SaveMetadata mismatch")
	assert.Equal(t, true, parsed.Download.SaveModelInfo, "SaveModelInfo mismatch")
	assert.Equal(t, true, parsed.Download.SaveVersionImages, "SaveVersionImages mismatch")
}

// TestDownloadShowConfig_ListFlags verifies list flags from config and override
func TestDownloadShowConfig_ListFlags(t *testing.T) {
	baseConfigContent := `
[Download]
ModelTypes = ["LORA", "Checkpoint"]
BaseModels = ["SD 1.5"]
Usernames = ["config_user_from_config"]
`

	t.Run("ListFlags_FlagOnly_ModelTypes", func(t *testing.T) {
		tempEmptyCfgPath := createTempConfig(t, "")
		stdout, _, err := runCommand(t, "--config", tempEmptyCfgPath,
			"--model-types", "VAE", "--model-types", "Hypernetwork",
			"debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.ElementsMatch(t, []string{"VAE", "Hypernetwork"}, parsed.Download.ModelTypes)
		assert.Empty(t, parsed.Download.BaseModels)
		assert.Empty(t, parsed.Download.Usernames)
	})

	t.Run("ListFlags_ConfigOnly_ModelTypes", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.ElementsMatch(t, []string{"LORA", "Checkpoint"}, parsed.Download.ModelTypes)
		assert.ElementsMatch(t, []string{"SD 1.5"}, parsed.Download.BaseModels)
		assert.ElementsMatch(t, []string{"config_user_from_config"}, parsed.Download.Usernames)
	})

	t.Run("ListFlags_FlagOverridesConfig_ModelTypes", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath,
			"--model-types", "TextualInversion",
			"--base-models", "SDXL 1.0", "--base-models", "Pony",
			"--username", "flag_user",
			"debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.ElementsMatch(t, []string{"TextualInversion"}, parsed.Download.ModelTypes)
		assert.ElementsMatch(t, []string{"SDXL 1.0", "Pony"}, parsed.Download.BaseModels)
		assert.ElementsMatch(t, []string{"flag_user"}, parsed.Download.Usernames)
	})

	t.Run("ListFlags_Default_Empty_ModelTypes", func(t *testing.T) {
		tempEmptyCfgPath := createTempConfig(t, "")
		stdout, _, err := runCommand(t, "--config", tempEmptyCfgPath, "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.Empty(t, parsed.Download.ModelTypes)
		assert.Empty(t, parsed.Download.BaseModels)
		assert.Empty(t, parsed.Download.Usernames)
	})

	t.Run("ListFlags_UsernameFlagOnly", func(t *testing.T) {
		tempEmptyCfgPath := createTempConfig(t, "")
		stdout, _, err := runCommand(t, "--config", tempEmptyCfgPath, "--username", "user_by_flag_only", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.ElementsMatch(t, []string{"user_by_flag_only"}, parsed.Download.Usernames)
	})
}

// TestDownloadShowConfig_SaveFlags verifies boolean flags related to saving extra data.
func TestDownloadShowConfig_SaveFlags(t *testing.T) {
	tempCfgPath := createTempConfig(t, "") // Empty config

	stdoutMeta, _, errMeta := runCommand(t, "--config", tempCfgPath, "--metadata", "debug", "show-config")
	require.NoError(t, errMeta, "Command failed for --metadata")
	parsedMeta := parseShowConfigOutput(t, stdoutMeta)
	assert.Equal(t, true, parsedMeta.Download.SaveMetadata, "--metadata flag should set SaveMetadata true")

	stdoutInfo, _, errInfo := runCommand(t, "--config", tempCfgPath, "--model-info", "debug", "show-config")
	require.NoError(t, errInfo, "Command failed for --model-info")
	parsedInfo := parseShowConfigOutput(t, stdoutInfo)
	assert.Equal(t, true, parsedInfo.Download.SaveModelInfo, "--model-info flag should set SaveModelInfo true")

	stdoutVImg, _, errVImg := runCommand(t, "--config", tempCfgPath, "--version-images", "debug", "show-config")
	require.NoError(t, errVImg, "Command failed for --version-images")
	parsedVImg := parseShowConfigOutput(t, stdoutVImg)
	assert.Equal(t, true, parsedVImg.Download.SaveVersionImages, "--version-images flag should set SaveVersionImages true")

	stdoutMImg, _, errMImg := runCommand(t, "--config", tempCfgPath, "--model-images", "debug", "show-config")
	require.NoError(t, errMImg, "Command failed for --model-images")
	parsedMImg := parseShowConfigOutput(t, stdoutMImg)
	assert.Equal(t, true, parsedMImg.Download.SaveModelImages, "--model-images flag should set SaveModelImages true")

	baseConfigContent := `
[Download]
SaveMetadata = false
SaveModelInfo = false
SaveVersionImages = false
SaveModelImages = false
`

	t.Run("SaveMetadata_FlagTrue_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--metadata", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.True(t, parsed.Download.SaveMetadata, "Expected SaveMetadata to be true due to flag")
	})

	t.Run("SaveMetadata_FlagFalse_OverridesConfigTrue", func(t *testing.T) {
		configWithTrue := `[Download]
SaveMetadata = true`
		tempCfgPath := createTempConfig(t, configWithTrue)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--metadata=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SaveMetadata, "Expected SaveMetadata to be false due to --metadata=false flag overriding config=true")
	})

	t.Run("SaveMetadata_FlagFalse_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--metadata=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SaveMetadata, "Expected SaveMetadata to be false due to --metadata=false flag (config also false)")
	})

	t.Run("ModelInfo_FlagTrue_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--model-info", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.True(t, parsed.Download.SaveModelInfo, "Expected SaveModelInfo to be true due to flag")
	})

	t.Run("ModelInfo_FlagFalse_OverridesConfigTrue", func(t *testing.T) {
		configWithTrue := `[Download]
SaveModelInfo = true`
		tempCfgPath := createTempConfig(t, configWithTrue)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--model-info=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SaveModelInfo, "Expected SaveModelInfo to be false due to --model-info=false flag overriding config=true")
	})

	t.Run("ModelInfo_FlagFalse_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--model-info=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SaveModelInfo, "Expected SaveModelInfo to be false due to --model-info=false flag (config also false)")
	})

	t.Run("SaveVersionImages_FlagTrue_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--version-images", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.True(t, parsed.Download.SaveVersionImages, "Expected SaveVersionImages to be true due to flag")
	})

	t.Run("SaveVersionImages_FlagFalse_OverridesConfigTrue", func(t *testing.T) {
		configWithTrue := `[Download]
SaveVersionImages = true`
		tempCfgPath := createTempConfig(t, configWithTrue)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--version-images=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SaveVersionImages, "Expected SaveVersionImages to be false due to --version-images=false flag overriding config=true")
	})

	t.Run("SaveVersionImages_FlagFalse_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--version-images=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SaveVersionImages, "Expected SaveVersionImages to be false due to --version-images=false flag (config also false)")
	})

	t.Run("SaveModelImages_FlagTrue_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--model-images", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.True(t, parsed.Download.SaveModelImages, "Expected SaveModelImages to be true due to flag")
	})

	t.Run("SaveModelImages_FlagFalse_OverridesConfigTrue", func(t *testing.T) {
		configWithTrue := `[Download]
SaveModelImages = true`
		tempCfgPath := createTempConfig(t, configWithTrue)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--model-images=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SaveModelImages, "Expected SaveModelImages to be false due to --model-images=false flag overriding config=true")
	})

	t.Run("SaveModelImages_FlagFalse_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--model-images=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SaveModelImages, "Expected SaveModelImages to be false due to --model-images=false flag (config also false)")
	})
}

func TestDownloadShowConfig_BehaviorFlags(t *testing.T) {
	tempCfgPath := createTempConfig(t, "")
	stdoutMeta, _, errMeta := runCommand(t, "--config", tempCfgPath, "--meta-only", "debug", "show-config")
	require.NoError(t, errMeta, "Command failed for --meta-only")
	parsedMeta := parseShowConfigOutput(t, stdoutMeta)
	assert.Equal(t, true, parsedMeta.Download.DownloadMetaOnly, "--meta-only flag should set DownloadMetaOnly true")

	stdoutYes, _, errYes := runCommand(t, "--config", tempCfgPath, "--yes", "debug", "show-config")
	require.NoError(t, errYes, "Command failed for --yes")
	parsedYes := parseShowConfigOutput(t, stdoutYes)
	assert.Equal(t, true, parsedYes.Download.SkipConfirmation, "--yes flag should set SkipConfirmation true")

	stdoutAll, _, errAll := runCommand(t, "--config", tempCfgPath, "--all-versions", "debug", "show-config")
	require.NoError(t, errAll, "Command failed for --all-versions")
	parsedAll := parseShowConfigOutput(t, stdoutAll)
	assert.Equal(t, true, parsedAll.Download.AllVersions, "--all-versions flag should set DownloadAllVersions true")

	baseConfigContent := `
[Download]
AllVersions = false
MetaOnly = false
SkipConfirmation = false
`
	t.Run("AllVersions_FlagTrue_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--all-versions", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.True(t, parsed.Download.AllVersions, "Expected AllVersions to be true due to flag overriding config")
	})
	t.Run("AllVersions_FlagFalse_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--all-versions=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.AllVersions, "Expected AllVersions to be false due to --all-versions=false flag")
	})
	t.Run("AllVersions_FlagFalse_OverridesConfigTrue", func(t *testing.T) {
		configWithTrue := `[Download]
AllVersions = true`
		tempCfgPath := createTempConfig(t, configWithTrue)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--all-versions=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.AllVersions, "Expected AllVersions to be false due to flag --all-versions=false overriding config=true")
	})
	t.Run("MetaOnly_FlagTrue_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--meta-only", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.True(t, parsed.Download.DownloadMetaOnly, "Expected DownloadMetaOnly to be true due to flag overriding config")
	})
	t.Run("MetaOnly_FlagFalse_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--meta-only=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.DownloadMetaOnly, "Expected DownloadMetaOnly to be false due to --meta-only=false flag")
	})
	t.Run("MetaOnly_FlagFalse_OverridesConfigTrue", func(t *testing.T) {
		configWithTrue := `[Download]
MetaOnly = true`
		tempCfgPath := createTempConfig(t, configWithTrue)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--meta-only=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.DownloadMetaOnly, "Expected DownloadMetaOnly to be false due to flag --meta-only=false overriding config=true")
	})
	t.Run("SkipConfirmation_FlagTrue_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--yes", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.True(t, parsed.Download.SkipConfirmation, "Expected SkipConfirmation to be true due to flag overriding config")
	})
	t.Run("SkipConfirmation_FlagFalse_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContent)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--yes=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SkipConfirmation, "Expected SkipConfirmation to be false due to --yes=false flag")
	})
	t.Run("SkipConfirmation_FlagFalse_OverridesConfigTrue", func(t *testing.T) {
		configWithTrue := `[Download]
SkipConfirmation = true`
		tempCfgPath := createTempConfig(t, configWithTrue)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--yes=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.SkipConfirmation, "Expected SkipConfirmation to be false due to flag --yes=false overriding config=true")
	})
}

func TestDownloadShowConfig_FilterFlags(t *testing.T) {
	tempCfgPath := createTempConfig(t, "")
	stdoutPruned, _, errPruned := runCommand(t, "--config", tempCfgPath, "--pruned", "debug", "show-config")
	require.NoError(t, errPruned, "Command failed for --pruned")
	parsedPruned := parseShowConfigOutput(t, stdoutPruned)
	assert.Equal(t, true, parsedPruned.Download.Pruned, "--pruned flag should set Pruned true")

	stdoutFp16, _, errFp16 := runCommand(t, "--config", tempCfgPath, "--fp16", "debug", "show-config")
	require.NoError(t, errFp16, "Command failed for --fp16")
	parsedFp16 := parseShowConfigOutput(t, stdoutFp16)
	assert.Equal(t, true, parsedFp16.Download.Fp16, "--fp16 flag should set Fp16 true")

	baseConfigContentF := `
[Download]
AllVersions = true
MetaOnly = true
ModelImages = true
SkipConfirmation = true
PrimaryOnly = false
Pruned = false
Fp16 = false
Nsfw = true
`
	t.Run("Pruned_FlagTrue_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContentF)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--pruned", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.True(t, parsed.Download.Pruned, "Expected Pruned to be true due to flag")
	})
	t.Run("Pruned_FlagFalse_OverridesConfigTrue", func(t *testing.T) {
		configWithTrue := `[Download]
Pruned = true`
		tempCfgPath := createTempConfig(t, configWithTrue)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--pruned=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.Pruned, "Expected Pruned to be false due to --pruned=false flag overriding config=true")
	})
	t.Run("Pruned_FlagFalse_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContentF)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--pruned=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.Pruned, "Expected Pruned to be false due to --pruned=false flag (config also false)")
	})
	t.Run("Fp16_FlagTrue_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContentF)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--fp16", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.True(t, parsed.Download.Fp16, "Expected Fp16 to be true due to flag")
	})
	t.Run("Fp16_FlagFalse_OverridesConfigTrue", func(t *testing.T) {
		configWithTrue := `[Download]
Fp16 = true`
		tempCfgPath := createTempConfig(t, configWithTrue)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--fp16=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.Fp16, "Expected Fp16 to be false due to --fp16=false flag overriding config=true")
	})
	t.Run("Fp16_FlagFalse_OverridesConfigFalse", func(t *testing.T) {
		tempCfgPath := createTempConfig(t, baseConfigContentF)
		stdout, _, err := runCommand(t, "--config", tempCfgPath, "--fp16=false", "debug", "show-config")
		require.NoError(t, err)
		parsed := parseShowConfigOutput(t, stdout)
		assert.False(t, parsed.Download.Fp16, "Expected Fp16 to be false due to --fp16=false flag (config also false)")
	})
}
