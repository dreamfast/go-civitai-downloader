package database

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-civitai-download/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLiteIntegrationBasicOperations tests core database operations
func TestSQLiteIntegrationBasicOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_integration.db")

	// Open database
	db, err := Open(dbPath)
	require.NoError(t, err, "Failed to open database")
	defer db.Close()

	// Create a realistic test entry based on Civitai API structure
	testEntry := createTestDatabaseEntry()

	// Test Put operation
	t.Run("Put Operation", func(t *testing.T) {
		key := fmt.Sprintf("v_%d", testEntry.Version.ID)
		data, err := json.Marshal(testEntry)
		require.NoError(t, err, "Failed to marshal test entry")

		err = db.Put([]byte(key), data)
		assert.NoError(t, err, "Put operation should succeed")
	})

	// Test Has operation
	t.Run("Has Operation", func(t *testing.T) {
		key := fmt.Sprintf("v_%d", testEntry.Version.ID)
		exists := db.Has([]byte(key))
		assert.True(t, exists, "Key should exist after Put")

		nonExistentKey := "v_999999999"
		exists = db.Has([]byte(nonExistentKey))
		assert.False(t, exists, "Non-existent key should not exist")
	})

	// Test Get operation
	t.Run("Get Operation", func(t *testing.T) {
		key := fmt.Sprintf("v_%d", testEntry.Version.ID)
		retrieved, err := db.Get([]byte(key))
		require.NoError(t, err, "Get operation should succeed")

		var retrievedEntry models.DatabaseEntry
		err = json.Unmarshal(retrieved, &retrievedEntry)
		require.NoError(t, err, "Should unmarshal retrieved entry")

		// Verify critical fields
		assert.Equal(t, testEntry.ModelID, retrievedEntry.ModelID, "Model ID should match")
		assert.Equal(t, testEntry.ModelName, retrievedEntry.ModelName, "Model name should match")
		assert.Equal(t, testEntry.Version.ID, retrievedEntry.Version.ID, "Version ID should match")
		assert.Equal(t, testEntry.Status, retrievedEntry.Status, "Status should match")
		assert.Equal(t, len(testEntry.Version.Files), len(retrievedEntry.Version.Files), "Files count should match")
		assert.Equal(t, len(testEntry.Version.Images), len(retrievedEntry.Version.Images), "Images count should match")
	})

	// Test pagination state operations
	t.Run("Pagination State", func(t *testing.T) {
		queryHash := "test_query_hash"
		expectedPage := 5

		// Set page state
		err := db.SetPageState(queryHash, expectedPage)
		assert.NoError(t, err, "SetPageState should succeed")

		// Get page state
		actualPage, err := db.GetPageState(queryHash)
		require.NoError(t, err, "GetPageState should succeed")
		assert.Equal(t, expectedPage, actualPage, "Retrieved page should match set page")

		// Test default value for non-existent query
		nonExistentPage, err := db.GetPageState("non_existent_query")
		require.NoError(t, err, "GetPageState should succeed for non-existent query")
		assert.Equal(t, 1, nonExistentPage, "Default page should be 1")

		// Delete page state
		err = db.DeletePageState(queryHash)
		assert.NoError(t, err, "DeletePageState should succeed")

		// Verify deletion
		deletedPage, err := db.GetPageState(queryHash)
		require.NoError(t, err, "GetPageState should succeed after deletion")
		assert.Equal(t, 1, deletedPage, "Page should return to default after deletion")
	})

	// Test Fold operation
	t.Run("Fold Operation", func(t *testing.T) {
		foundEntries := make(map[string]bool)
		err := db.Fold(func(key []byte, value []byte) error {
			keyStr := string(key)
			foundEntries[keyStr] = true

			if strings.HasPrefix(keyStr, "v_") {
				// Verify we can unmarshal the entry
				var entry models.DatabaseEntry
				err := json.Unmarshal(value, &entry)
				assert.NoError(t, err, "Should be able to unmarshal folded entry")
			}

			return nil
		})

		require.NoError(t, err, "Fold operation should succeed")
		assert.True(t, len(foundEntries) > 0, "Should find at least one entry")

		expectedKey := fmt.Sprintf("v_%d", testEntry.Version.ID)
		assert.True(t, foundEntries[expectedKey], "Should find our test entry during fold")
	})

	// Test Delete operation
	t.Run("Delete Operation", func(t *testing.T) {
		key := fmt.Sprintf("v_%d", testEntry.Version.ID)

		// Verify exists before delete
		exists := db.Has([]byte(key))
		assert.True(t, exists, "Key should exist before delete")

		// Delete
		err := db.Delete([]byte(key))
		assert.NoError(t, err, "Delete operation should succeed")

		// Verify doesn't exist after delete
		exists = db.Has([]byte(key))
		assert.False(t, exists, "Key should not exist after delete")

		// Test delete non-existent key
		err = db.Delete([]byte("v_999999999"))
		assert.Equal(t, ErrNotFound, err, "Deleting non-existent key should return ErrNotFound")
	})
}

// TestSQLiteDataIntegrity tests database constraints and data integrity
func TestSQLiteDataIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_integrity.db")

	db, err := Open(dbPath)
	require.NoError(t, err, "Failed to open database")
	defer db.Close()

	t.Run("Status Constraint", func(t *testing.T) {
		entry := createTestDatabaseEntry()
		entry.Status = "InvalidStatus" // This should violate the CHECK constraint

		key := fmt.Sprintf("v_%d", entry.Version.ID)
		data, err := json.Marshal(entry)
		require.NoError(t, err)

		// This should fail due to constraint violation
		err = db.Put([]byte(key), data)
		assert.Error(t, err, "Should reject invalid status")
		assert.Contains(t, err.Error(), "constraint", "Error should mention constraint violation")
	})

	t.Run("Foreign Key Constraints", func(t *testing.T) {
		// Insert a valid entry first
		validEntry := createTestDatabaseEntry()
		key := fmt.Sprintf("v_%d", validEntry.Version.ID)
		data, err := json.Marshal(validEntry)
		require.NoError(t, err)

		err = db.Put([]byte(key), data)
		require.NoError(t, err, "Should insert valid entry")

		// Now delete it and verify cascading deletes work
		err = db.Delete([]byte(key))
		assert.NoError(t, err, "Should delete entry")

		// Verify data was cascaded properly by checking the database directly
		var fileCount, imageCount, statsCount int
		db.db.QueryRow("SELECT COUNT(*) FROM files WHERE version_id = ?", validEntry.Version.ID).Scan(&fileCount)
		db.db.QueryRow("SELECT COUNT(*) FROM model_images WHERE version_id = ?", validEntry.Version.ID).Scan(&imageCount)
		db.db.QueryRow("SELECT COUNT(*) FROM model_stats WHERE version_id = ?", validEntry.Version.ID).Scan(&statsCount)

		assert.Equal(t, 0, fileCount, "Files should be cascaded on delete")
		assert.Equal(t, 0, imageCount, "Images should be cascaded on delete")
		assert.Equal(t, 0, statsCount, "Stats should be cascaded on delete")
	})
}

// TestSQLitePerformance tests basic performance characteristics
func TestSQLitePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_performance.db")

	db, err := Open(dbPath)
	require.NoError(t, err, "Failed to open database")
	defer db.Close()

	// Test batch insertions
	t.Run("Batch Insertions", func(t *testing.T) {
		numEntries := 100
		start := time.Now()

		for i := 0; i < numEntries; i++ {
			entry := createTestDatabaseEntry()
			entry.Version.ID = 100000 + i // Unique IDs
			entry.ModelID = 50000 + i
			entry.ModelName = fmt.Sprintf("Test Model %d", i)

			// Make file IDs unique too
			for j := range entry.Version.Files {
				entry.Version.Files[j].ID = 200000 + i*10 + j
			}
			// Update primary file ID
			entry.File.ID = 200000 + i*10

			// Make image IDs unique too
			for j := range entry.Version.Images {
				entry.Version.Images[j].ID = 300000 + i*10 + j
			}

			key := fmt.Sprintf("v_%d", entry.Version.ID)
			data, err := json.Marshal(entry)
			require.NoError(t, err)

			err = db.Put([]byte(key), data)
			assert.NoError(t, err, "Batch insert should succeed")
		}

		duration := time.Since(start)
		t.Logf("Inserted %d entries in %v (%.2f entries/sec)", numEntries, duration, float64(numEntries)/duration.Seconds())

		// Basic performance check - should be able to insert at least 10 entries per second
		assert.Less(t, duration, time.Duration(numEntries)*100*time.Millisecond, "Batch insertions should be reasonably fast")
	})

	t.Run("Search Performance", func(t *testing.T) {
		start := time.Now()

		count := 0
		err := db.Fold(func(key []byte, value []byte) error {
			if strings.HasPrefix(string(key), "v_") {
				count++
			}
			return nil
		})

		duration := time.Since(start)
		require.NoError(t, err, "Fold should succeed")

		t.Logf("Scanned %d entries in %v", count, duration)
		assert.Greater(t, count, 0, "Should find some entries")
	})
}

// createTestDatabaseEntry creates a realistic test entry
func createTestDatabaseEntry() models.DatabaseEntry {
	return models.DatabaseEntry{
		ModelID:   123456,
		ModelName: "Test Realistic Model",
		ModelType: "Checkpoint",
		Version: models.ModelVersion{
			ID:                   789012,
			ModelId:              123456,
			Name:                 "v1.0",
			PublishedAt:          "2024-01-01T12:00:00Z",
			UpdatedAt:            "2024-01-02T12:00:00Z",
			Description:          "A test model for integration testing",
			TrainedWords:         []string{"test", "realistic", "model"},
			BaseModel:            "SD 1.5",
			EarlyAccessTimeFrame: 0,
			Stats: models.Stats{
				DownloadCount: 1500,
				FavoriteCount: 250,
				CommentCount:  75,
				RatingCount:   50,
				Rating:        4.3,
			},
			Files: []models.File{
				{
					ID:          111222,
					Name:        "test-model-v1.safetensors",
					SizeKB:      4096000, // 4GB
					Type:        "Model",
					Primary:     true,
					DownloadUrl: "https://civitai.com/api/download/models/789012",
					Hashes: models.Hashes{
						SHA256: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
						BLAKE3: "123456abcdef789012345678901234567890abcdef123456789012345678901234",
						CRC32:  "ABC12345",
						AutoV2: "AUTOV2HASH",
					},
					Metadata: models.Metadata{
						Fp:     "fp16",
						Size:   "full",
						Format: "SafeTensor",
					},
					PickleScanResult:  "Success",
					PickleScanMessage: "No pickle imports found",
					VirusScanResult:   "Success",
					ScannedAt:         "2024-01-01T10:00:00Z",
				},
				{
					ID:          111223,
					Name:        "test-model-v1.yaml",
					SizeKB:      2,
					Type:        "Config",
					Primary:     false,
					DownloadUrl: "https://civitai.com/api/download/models/789012?type=Config",
					Hashes: models.Hashes{
						SHA256: "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
					},
				},
			},
			Images: []models.ModelImage{
				{
					ID:        333444,
					URL:       "https://image.civitai.com/xG1nkqKTMzGDvpLrqFT7WA/abc123.jpeg",
					Hash:      "blurhash123",
					Width:     512,
					Height:    768,
					Nsfw:      false,
					NsfwLevel: "None",
					CreatedAt: "2024-01-01T11:00:00Z",
					Stats: models.ImageStats{
						CryCount:     2,
						LaughCount:   5,
						LikeCount:    125,
						HeartCount:   45,
						CommentCount: 8,
					},
					Username: "testuser",
				},
				{
					ID:        333445,
					URL:       "https://image.civitai.com/xG1nkqKTMzGDvpLrqFT7WA/def456.jpeg",
					Hash:      "blurhash456",
					Width:     768,
					Height:    512,
					Nsfw:      false,
					NsfwLevel: "None",
					CreatedAt: "2024-01-01T11:30:00Z",
					Stats: models.ImageStats{
						LikeCount:    89,
						HeartCount:   23,
						CommentCount: 3,
					},
					Username: "testuser",
				},
			},
		},
		File: models.File{
			ID:          111222,
			Name:        "test-model-v1.safetensors",
			SizeKB:      4096000,
			Type:        "Model",
			Primary:     true,
			DownloadUrl: "https://civitai.com/api/download/models/789012",
			Hashes: models.Hashes{
				SHA256: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				BLAKE3: "123456abcdef789012345678901234567890abcdef123456789012345678901234",
				CRC32:  "ABC12345",
				AutoV2: "AUTOV2HASH",
			},
			Metadata: models.Metadata{
				Fp:     "fp16",
				Size:   "full",
				Format: "SafeTensor",
			},
		},
		Creator: models.Creator{
			Username: "testuser",
			Image:    "https://image.civitai.com/avatar/abc123.jpeg",
		},
		Timestamp:    time.Now().Unix(),
		Filename:     "789012_test-model-v1.safetensors",
		Folder:       "Checkpoint/SD 1.5/123456-test-realistic-model",
		Status:       models.StatusDownloaded,
		ErrorDetails: "",
	}
}
