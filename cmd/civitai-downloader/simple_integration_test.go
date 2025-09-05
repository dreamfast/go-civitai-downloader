package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-civitai-download/internal/database"
	"go-civitai-download/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSimpleIntegrationWithSQLite tests basic end-to-end functionality
func TestSimpleIntegrationWithSQLite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "simple_integration.db")

	t.Run("Complete Workflow Simulation", func(t *testing.T) {
		// 1. Open database
		db, err := database.Open(testDBPath)
		require.NoError(t, err, "Should open database")
		defer db.Close()

		// 2. Verify empty database
		initialCount := countDatabaseEntries(t, db)
		assert.Equal(t, 0, initialCount, "Database should start empty")

		// 3. Create and store a realistic entry
		entry := createRealisticDatabaseEntry()
		key := fmt.Sprintf("v_%d", entry.Version.ID)

		data, err := json.Marshal(entry)
		require.NoError(t, err, "Should marshal entry")

		err = db.Put([]byte(key), data)
		require.NoError(t, err, "Should store entry")

		t.Logf("✓ Stored entry: %s (v%d)", entry.ModelName, entry.Version.ID)

		// 4. Verify entry was stored
		exists := db.Has([]byte(key))
		assert.True(t, exists, "Entry should exist after storage")

		// 5. Retrieve and verify data integrity
		retrieved, err := db.Get([]byte(key))
		require.NoError(t, err, "Should retrieve stored entry")

		var retrievedEntry models.DatabaseEntry
		err = json.Unmarshal(retrieved, &retrievedEntry)
		require.NoError(t, err, "Should unmarshal retrieved entry")

		// Verify critical data
		assert.Equal(t, entry.ModelID, retrievedEntry.ModelID, "Model ID should match")
		assert.Equal(t, entry.ModelName, retrievedEntry.ModelName, "Model name should match")
		assert.Equal(t, entry.Version.ID, retrievedEntry.Version.ID, "Version ID should match")
		assert.Equal(t, entry.Status, retrievedEntry.Status, "Status should match")
		assert.Equal(t, len(entry.Version.Files), len(retrievedEntry.Version.Files), "File count should match")

		t.Logf("✓ Data integrity verified")

		// 6. Test status updates (simulating download progress)
		retrievedEntry.Status = models.StatusDownloaded
		retrievedEntry.Timestamp = time.Now().Unix()

		updatedData, err := json.Marshal(retrievedEntry)
		require.NoError(t, err, "Should marshal updated entry")

		err = db.Put([]byte(key), updatedData)
		require.NoError(t, err, "Should update entry")

		// 7. Verify status update
		final, err := db.Get([]byte(key))
		require.NoError(t, err, "Should retrieve updated entry")

		var finalEntry models.DatabaseEntry
		err = json.Unmarshal(final, &finalEntry)
		require.NoError(t, err, "Should unmarshal final entry")

		assert.Equal(t, models.StatusDownloaded, finalEntry.Status, "Status should be updated")
		t.Logf("✓ Status update successful: %s", finalEntry.Status)

		// 8. Test pagination state management
		queryHash := "test_integration_query"

		err = db.SetPageState(queryHash, 3)
		require.NoError(t, err, "Should set page state")

		page, err := db.GetPageState(queryHash)
		require.NoError(t, err, "Should get page state")
		assert.Equal(t, 3, page, "Page state should match")

		t.Logf("✓ Pagination state management working")

		// 9. Test database enumeration (like db view command)
		finalCount := countDatabaseEntries(t, db)
		assert.Equal(t, 1, finalCount, "Should have exactly one entry")

		entries := getAllDatabaseEntries(t, db)
		assert.Len(t, entries, 1, "Should retrieve one entry")

		for k, v := range entries {
			t.Logf("✓ Found entry: %s - %s (%s)", k, v.ModelName, v.Status)
		}

		// 10. Test search functionality (like db search command)
		searchResults := searchDatabaseEntries(t, db, "test")
		assert.Greater(t, len(searchResults), 0, "Should find search results")

		t.Logf("✓ Search functionality working (%d results)", len(searchResults))

		// 11. Test deletion
		err = db.Delete([]byte(key))
		require.NoError(t, err, "Should delete entry")

		exists = db.Has([]byte(key))
		assert.False(t, exists, "Entry should not exist after deletion")

		t.Logf("✓ Deletion successful")
	})

	t.Run("Multiple Entries Workflow", func(t *testing.T) {
		db, err := database.Open(testDBPath)
		require.NoError(t, err, "Should open database")
		defer db.Close()

		// Create multiple entries with different states
		entries := []struct {
			name   string
			status string
		}{
			{"Model Alpha", models.StatusPending},
			{"Model Beta", models.StatusDownloaded},
			{"Model Gamma", models.StatusError},
		}

		storedKeys := make([]string, 0, len(entries))

		// Store all entries
		for i, entryInfo := range entries {
			entry := createRealisticDatabaseEntry()
			entry.ModelName = entryInfo.name
			entry.Status = entryInfo.status
			entry.Version.ID = 50000 + i
			entry.ModelID = 40000 + i

			// Make file and image IDs unique
			for j := range entry.Version.Files {
				entry.Version.Files[j].ID = 200000 + i*100 + j
			}
			entry.File.ID = 200000 + i*100

			for j := range entry.Version.Images {
				entry.Version.Images[j].ID = 300000 + i*100 + j
			}

			if entryInfo.status == models.StatusError {
				entry.ErrorDetails = "Simulated error for testing"
			}

			key := fmt.Sprintf("v_%d", entry.Version.ID)
			storedKeys = append(storedKeys, key)

			data, err := json.Marshal(entry)
			require.NoError(t, err, "Should marshal entry %s", entryInfo.name)

			err = db.Put([]byte(key), data)
			require.NoError(t, err, "Should store entry %s", entryInfo.name)
		}

		t.Logf("✓ Stored %d entries with different statuses", len(entries))

		// Verify all entries exist
		for i, key := range storedKeys {
			exists := db.Has([]byte(key))
			assert.True(t, exists, "Entry %d should exist", i)
		}

		// Count entries by status
		allEntries := getAllDatabaseEntries(t, db)
		statusCounts := make(map[string]int)
		for _, entry := range allEntries {
			statusCounts[entry.Status]++
		}

		t.Logf("✓ Status distribution: Pending=%d, Downloaded=%d, Error=%d",
			statusCounts[models.StatusPending],
			statusCounts[models.StatusDownloaded],
			statusCounts[models.StatusError])

		assert.Equal(t, 1, statusCounts[models.StatusPending], "Should have 1 pending entry")
		assert.Equal(t, 1, statusCounts[models.StatusDownloaded], "Should have 1 downloaded entry")
		assert.Equal(t, 1, statusCounts[models.StatusError], "Should have 1 error entry")

		// Test cleanup
		for _, key := range storedKeys {
			err := db.Delete([]byte(key))
			assert.NoError(t, err, "Should delete entry %s", key)
		}

		t.Logf("✓ Cleanup successful")
	})
}

// Helper functions
func countDatabaseEntries(t *testing.T, db *database.DB) int {
	count := 0
	err := db.Fold(func(key []byte, value []byte) error {
		if strings.HasPrefix(string(key), "v_") {
			count++
		}
		return nil
	})
	require.NoError(t, err, "Should count entries")
	return count
}

func getAllDatabaseEntries(t *testing.T, db *database.DB) map[string]models.DatabaseEntry {
	entries := make(map[string]models.DatabaseEntry)
	err := db.Fold(func(key []byte, value []byte) error {
		keyStr := string(key)
		if !strings.HasPrefix(keyStr, "v_") {
			return nil
		}

		var entry models.DatabaseEntry
		err := json.Unmarshal(value, &entry)
		if err != nil {
			t.Logf("Warning: Failed to unmarshal entry %s: %v", keyStr, err)
			return nil
		}

		entries[keyStr] = entry
		return nil
	})
	require.NoError(t, err, "Should enumerate entries")
	return entries
}

func searchDatabaseEntries(t *testing.T, db *database.DB, searchTerm string) []models.DatabaseEntry {
	var results []models.DatabaseEntry
	searchLower := strings.ToLower(searchTerm)

	err := db.Fold(func(key []byte, value []byte) error {
		keyStr := string(key)
		if !strings.HasPrefix(keyStr, "v_") {
			return nil
		}

		var entry models.DatabaseEntry
		err := json.Unmarshal(value, &entry)
		if err != nil {
			return nil
		}

		if strings.Contains(strings.ToLower(entry.ModelName), searchLower) {
			results = append(results, entry)
		}
		return nil
	})
	require.NoError(t, err, "Should perform search")
	return results
}

func createRealisticDatabaseEntry() models.DatabaseEntry {
	now := time.Now()
	return models.DatabaseEntry{
		ModelID:   12345,
		ModelName: "Test Realistic Model v2",
		ModelType: "Checkpoint",
		Version: models.ModelVersion{
			ID:                   67890,
			ModelId:              12345,
			Name:                 "v2.0",
			PublishedAt:          now.Add(-24 * time.Hour).Format(time.RFC3339),
			UpdatedAt:            now.Format(time.RFC3339),
			Description:          "A comprehensive test model for integration testing",
			TrainedWords:         []string{"test", "realistic", "integration"},
			BaseModel:            "SDXL 1.0",
			EarlyAccessTimeFrame: 0,
			Stats: models.Stats{
				DownloadCount: 2500,
				FavoriteCount: 450,
				CommentCount:  120,
				RatingCount:   85,
				Rating:        4.2,
			},
			Files: []models.File{
				{
					ID:          111333,
					Name:        "test-realistic-model-v2.safetensors",
					SizeKB:      6700000, // ~6.7GB
					Type:        "Model",
					Primary:     true,
					DownloadUrl: "https://civitai.com/api/download/models/67890",
					Hashes: models.Hashes{
						SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
						BLAKE3: "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262",
						CRC32:  "D202EF8D",
						AutoV2: "AUTOV2TEST",
					},
					Metadata: models.Metadata{
						Fp:     "fp16",
						Size:   "full",
						Format: "SafeTensor",
					},
					PickleScanResult:  "Success",
					PickleScanMessage: "No pickle imports detected",
					VirusScanResult:   "Success",
					ScannedAt:         now.Add(-12 * time.Hour).Format(time.RFC3339),
				},
				{
					ID:          111334,
					Name:        "test-realistic-model-v2.yaml",
					SizeKB:      3,
					Type:        "Config",
					Primary:     false,
					DownloadUrl: "https://civitai.com/api/download/models/67890?type=Config",
				},
			},
			Images: []models.ModelImage{
				{
					ID:        444555,
					URL:       "https://image.civitai.com/test/example1.jpeg",
					Hash:      "blurhash_test_1",
					Width:     1024,
					Height:    1024,
					Nsfw:      false,
					NsfwLevel: "None",
					CreatedAt: now.Add(-6 * time.Hour).Format(time.RFC3339),
					Stats: models.ImageStats{
						CryCount:     1,
						LaughCount:   8,
						LikeCount:    234,
						HeartCount:   89,
						CommentCount: 15,
					},
					Username: "testuser",
				},
			},
		},
		File: models.File{
			ID:          111333,
			Name:        "test-realistic-model-v2.safetensors",
			SizeKB:      6700000,
			Type:        "Model",
			Primary:     true,
			DownloadUrl: "https://civitai.com/api/download/models/67890",
			Hashes: models.Hashes{
				SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				BLAKE3: "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262",
				CRC32:  "D202EF8D",
				AutoV2: "AUTOV2TEST",
			},
		},
		Creator: models.Creator{
			Username: "integrationtester",
			Image:    "https://image.civitai.com/avatar/test.jpeg",
		},
		Timestamp:    now.Unix(),
		Filename:     "67890_test-realistic-model-v2.safetensors",
		Folder:       "Checkpoint/SDXL 1.0/12345-test-realistic-model-v2",
		Status:       models.StatusPending,
		ErrorDetails: "",
	}
}
