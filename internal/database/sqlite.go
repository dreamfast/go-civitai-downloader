package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"go-civitai-download/internal/models"

	log "github.com/sirupsen/logrus"
	_ "github.com/mattn/go-sqlite3"
)

// ErrNotFound is returned when a key is not found in the database.
var ErrNotFound = errors.New("key not found")

// DB wraps the SQLite database instance and provides helper methods.
type DB struct {
	db        *sql.DB
	sync.RWMutex
	closeOnce sync.Once
	closed    bool
	closeErr  error
}

// Open initializes and returns a DB instance.
func Open(path string) (*DB, error) {
	// Ensure the directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create database directory %s: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database at %s: %w", path, err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database at %s: %w", path, err)
	}

	dbWrapper := &DB{db: db}
	
	// Initialize schema
	if err := dbWrapper.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	log.Infof("SQLite database opened successfully at %s", path)
	return dbWrapper, nil
}

// initSchema creates the database schema if it doesn't exist
func (d *DB) initSchema() error {
	schema := `
	-- Main models table
	CREATE TABLE IF NOT EXISTS models (
		version_id INTEGER PRIMARY KEY,
		model_id INTEGER NOT NULL,
		model_name TEXT NOT NULL,
		model_type TEXT NOT NULL,
		version_name TEXT NOT NULL,
		version_published_at TEXT,
		version_updated_at TEXT,
		version_description TEXT,
		trained_words TEXT, -- JSON array
		base_model TEXT,
		early_access_timeframe INTEGER,
		creator_username TEXT,
		creator_image TEXT,
		filename TEXT NOT NULL,
		folder TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('Pending', 'Downloaded', 'Error')),
		error_details TEXT,
		timestamp INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Files table (normalized from File struct)
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY,
		version_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		size_kb REAL,
		type TEXT,
		metadata_fp TEXT,
		metadata_size TEXT,
		metadata_format TEXT,
		pickle_scan_result TEXT,
		pickle_scan_message TEXT,
		virus_scan_result TEXT,
		scanned_at TEXT,
		download_url TEXT,
		is_primary BOOLEAN,
		hash_autov2 TEXT,
		hash_sha256 TEXT,
		hash_crc32 TEXT,
		hash_blake3 TEXT,
		FOREIGN KEY (version_id) REFERENCES models(version_id) ON DELETE CASCADE
	);

	-- Model stats
	CREATE TABLE IF NOT EXISTS model_stats (
		version_id INTEGER PRIMARY KEY,
		download_count INTEGER,
		favorite_count INTEGER,
		comment_count INTEGER,
		rating_count INTEGER,
		rating REAL,
		FOREIGN KEY (version_id) REFERENCES models(version_id) ON DELETE CASCADE
	);

	-- Images associated with versions
	CREATE TABLE IF NOT EXISTS model_images (
		id INTEGER PRIMARY KEY,
		version_id INTEGER NOT NULL,
		url TEXT,
		hash TEXT,
		width INTEGER,
		height INTEGER,
		nsfw BOOLEAN,
		nsfw_level TEXT,
		created_at TEXT,
		post_id INTEGER,
		stats_cry_count INTEGER,
		stats_laugh_count INTEGER,
		stats_like_count INTEGER,
		stats_heart_count INTEGER,
		stats_comment_count INTEGER,
		username TEXT,
		FOREIGN KEY (version_id) REFERENCES models(version_id) ON DELETE CASCADE
	);

	-- Pagination state (replaces current_page_ keys)
	CREATE TABLE IF NOT EXISTS pagination_state (
		query_hash TEXT PRIMARY KEY,
		current_page INTEGER NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Indexes for performance
	CREATE INDEX IF NOT EXISTS idx_models_model_id ON models(model_id);
	CREATE INDEX IF NOT EXISTS idx_models_status ON models(status);
	CREATE INDEX IF NOT EXISTS idx_models_model_name ON models(model_name);
	CREATE INDEX IF NOT EXISTS idx_models_creator ON models(creator_username);
	CREATE INDEX IF NOT EXISTS idx_files_version_id ON files(version_id);
	CREATE INDEX IF NOT EXISTS idx_files_primary ON files(is_primary);

	-- Triggers to update updated_at timestamp
	CREATE TRIGGER IF NOT EXISTS update_models_timestamp 
		AFTER UPDATE ON models
		BEGIN
			UPDATE models SET updated_at = CURRENT_TIMESTAMP WHERE version_id = NEW.version_id;
		END;

	CREATE TRIGGER IF NOT EXISTS update_pagination_timestamp 
		AFTER UPDATE ON pagination_state
		BEGIN
			UPDATE pagination_state SET updated_at = CURRENT_TIMESTAMP WHERE query_hash = NEW.query_hash;
		END;
	`

	_, err := d.db.Exec(schema)
	return err
}

// Lock acquires a write lock.
func (d *DB) Lock() {
	d.RWMutex.Lock()
}

// Unlock releases a write lock.
func (d *DB) Unlock() {
	d.RWMutex.Unlock()
}

// RLock acquires a read lock.
func (d *DB) RLock() {
	d.RWMutex.RLock()
}

// RUnlock releases a read lock.
func (d *DB) RUnlock() {
	d.RWMutex.RUnlock()
}

// Close safely closes the database connection.
func (d *DB) Close() error {
	d.closeOnce.Do(func() {
		log.Info("Closing database...")
		d.Lock()
		defer d.Unlock()

		d.closeErr = d.db.Close()
		d.closed = true

		if d.closeErr != nil {
			log.Errorf("Error during database close operation: %v", d.closeErr)
		} else {
			log.Info("Database closed successfully.")
		}
	})

	return d.closeErr
}

// Has checks if a key exists in the database.
func (d *DB) Has(key []byte) bool {
	keyStr := string(key)
	
	if strings.HasPrefix(keyStr, "v_") {
		versionIDStr := strings.TrimPrefix(keyStr, "v_")
		versionID, err := strconv.Atoi(versionIDStr)
		if err != nil {
			return false
		}
		
		d.RLock()
		defer d.RUnlock()
		
		var exists bool
		err = d.db.QueryRow("SELECT EXISTS(SELECT 1 FROM models WHERE version_id = ?)", versionID).Scan(&exists)
		return err == nil && exists
	} else if strings.HasPrefix(keyStr, "current_page_") {
		queryHash := strings.TrimPrefix(keyStr, "current_page_")
		
		d.RLock()
		defer d.RUnlock()
		
		var exists bool
		err := d.db.QueryRow("SELECT EXISTS(SELECT 1 FROM pagination_state WHERE query_hash = ?)", queryHash).Scan(&exists)
		return err == nil && exists
	}
	
	return false
}

// Get retrieves the value associated with a key.
func (d *DB) Get(key []byte) ([]byte, error) {
	keyStr := string(key)
	
	if strings.HasPrefix(keyStr, "v_") {
		return d.getDatabaseEntry(keyStr)
	} else if strings.HasPrefix(keyStr, "current_page_") {
		return d.getPageState(keyStr)
	}
	
	return nil, ErrNotFound
}

// getDatabaseEntry retrieves a DatabaseEntry by version ID key
func (d *DB) getDatabaseEntry(key string) ([]byte, error) {
	versionIDStr := strings.TrimPrefix(key, "v_")
	versionID, err := strconv.Atoi(versionIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid version ID in key %s: %w", key, err)
	}

	d.RLock()
	defer d.RUnlock()

	// Get main model data
	var entry models.DatabaseEntry
	var trainedWordsJSON sql.NullString
	
	err = d.db.QueryRow(`
		SELECT 
			m.version_id, m.model_id, m.model_name, m.model_type, m.version_name,
			m.version_published_at, m.version_updated_at, m.version_description,
			m.trained_words, m.base_model, m.early_access_timeframe,
			m.creator_username, m.creator_image, m.filename, m.folder,
			m.status, m.error_details, m.timestamp,
			ms.download_count, ms.favorite_count, ms.comment_count, ms.rating_count, ms.rating
		FROM models m
		LEFT JOIN model_stats ms ON m.version_id = ms.version_id
		WHERE m.version_id = ?
	`, versionID).Scan(
		&entry.Version.ID, &entry.ModelID, &entry.ModelName, &entry.ModelType, &entry.Version.Name,
		&entry.Version.PublishedAt, &entry.Version.UpdatedAt, &entry.Version.Description,
		&trainedWordsJSON, &entry.Version.BaseModel, &entry.Version.EarlyAccessTimeFrame,
		&entry.Creator.Username, &entry.Creator.Image, &entry.Filename, &entry.Folder,
		&entry.Status, &entry.ErrorDetails, &entry.Timestamp,
		&entry.Version.Stats.DownloadCount, &entry.Version.Stats.FavoriteCount,
		&entry.Version.Stats.CommentCount, &entry.Version.Stats.RatingCount, &entry.Version.Stats.Rating,
	)
	
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("error querying model data for key %s: %w", key, err)
	}

	// Parse trained words JSON
	if trainedWordsJSON.Valid {
		json.Unmarshal([]byte(trainedWordsJSON.String), &entry.Version.TrainedWords)
	}

	// Get file data
	rows, err := d.db.Query(`
		SELECT 
			id, name, size_kb, type, metadata_fp, metadata_size, metadata_format,
			pickle_scan_result, pickle_scan_message, virus_scan_result, scanned_at,
			download_url, is_primary, hash_autov2, hash_sha256, hash_crc32, hash_blake3
		FROM files WHERE version_id = ?
	`, versionID)
	
	if err != nil {
		return nil, fmt.Errorf("error querying files for key %s: %w", key, err)
	}
	defer rows.Close()

	for rows.Next() {
		var file models.File
		var metadataFp, metadataSize, metadataFormat sql.NullString
		
		err := rows.Scan(
			&file.ID, &file.Name, &file.SizeKB, &file.Type,
			&metadataFp, &metadataSize, &metadataFormat,
			&file.PickleScanResult, &file.PickleScanMessage, &file.VirusScanResult, &file.ScannedAt,
			&file.DownloadUrl, &file.Primary,
			&file.Hashes.AutoV2, &file.Hashes.SHA256, &file.Hashes.CRC32, &file.Hashes.BLAKE3,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning file row for key %s: %w", key, err)
		}

		// Set metadata
		if metadataFp.Valid {
			file.Metadata.Fp = metadataFp.String
		}
		if metadataSize.Valid {
			file.Metadata.Size = metadataSize.String
		}
		if metadataFormat.Valid {
			file.Metadata.Format = metadataFormat.String
		}

		entry.Version.Files = append(entry.Version.Files, file)
		
		// Set the primary file as entry.File for compatibility
		if file.Primary {
			entry.File = file
		}
	}

	// Get images
	imageRows, err := d.db.Query(`
		SELECT 
			id, url, hash, width, height, nsfw, nsfw_level, created_at, post_id,
			stats_cry_count, stats_laugh_count, stats_like_count, stats_heart_count,
			stats_comment_count, username
		FROM model_images WHERE version_id = ?
	`, versionID)
	
	if err != nil {
		return nil, fmt.Errorf("error querying images for key %s: %w", key, err)
	}
	defer imageRows.Close()

	for imageRows.Next() {
		var img models.ModelImage
		var postID sql.NullInt64
		
		err := imageRows.Scan(
			&img.ID, &img.URL, &img.Hash, &img.Width, &img.Height, &img.Nsfw, &img.NsfwLevel, &img.CreatedAt, &postID,
			&img.Stats.CryCount, &img.Stats.LaughCount, &img.Stats.LikeCount, &img.Stats.HeartCount,
			&img.Stats.CommentCount, &img.Username,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning image row for key %s: %w", key, err)
		}
		
		if postID.Valid {
			postIDInt := int(postID.Int64)
			img.PostID = &postIDInt
		}

		entry.Version.Images = append(entry.Version.Images, img)
	}

	// Marshal to JSON for compatibility with existing code
	return json.Marshal(entry)
}

// getPageState retrieves pagination state
func (d *DB) getPageState(key string) ([]byte, error) {
	queryHash := strings.TrimPrefix(key, "current_page_")
	
	d.RLock()
	defer d.RUnlock()
	
	var currentPage int
	err := d.db.QueryRow("SELECT current_page FROM pagination_state WHERE query_hash = ?", queryHash).Scan(&currentPage)
	
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("error querying page state for key %s: %w", key, err)
	}
	
	return []byte(strconv.Itoa(currentPage)), nil
}

// Put stores a key-value pair in the database.
func (d *DB) Put(key []byte, value []byte) error {
	keyStr := string(key)
	
	if strings.HasPrefix(keyStr, "v_") {
		return d.putDatabaseEntry(keyStr, value)
	} else if strings.HasPrefix(keyStr, "current_page_") {
		return d.putPageState(keyStr, value)
	}
	
	return fmt.Errorf("unsupported key format: %s", keyStr)
}

// putDatabaseEntry stores a DatabaseEntry
func (d *DB) putDatabaseEntry(key string, value []byte) error {
	var entry models.DatabaseEntry
	if err := json.Unmarshal(value, &entry); err != nil {
		return fmt.Errorf("error unmarshaling database entry for key %s: %w", key, err)
	}

	d.Lock()
	defer d.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("error starting transaction for key %s: %w", key, err)
	}
	defer tx.Rollback()

	// Marshal trained words to JSON
	trainedWordsJSON, _ := json.Marshal(entry.Version.TrainedWords)

	// Insert/update main model entry
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO models (
			version_id, model_id, model_name, model_type, version_name,
			version_published_at, version_updated_at, version_description,
			trained_words, base_model, early_access_timeframe,
			creator_username, creator_image, filename, folder,
			status, error_details, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.Version.ID, entry.ModelID, entry.ModelName, entry.ModelType, entry.Version.Name,
		entry.Version.PublishedAt, entry.Version.UpdatedAt, entry.Version.Description,
		string(trainedWordsJSON), entry.Version.BaseModel, entry.Version.EarlyAccessTimeFrame,
		entry.Creator.Username, entry.Creator.Image, entry.Filename, entry.Folder,
		entry.Status, entry.ErrorDetails, entry.Timestamp)
	
	if err != nil {
		return fmt.Errorf("error inserting model for key %s: %w", key, err)
	}

	// Insert/update stats
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO model_stats (
			version_id, download_count, favorite_count, comment_count, rating_count, rating
		) VALUES (?, ?, ?, ?, ?, ?)
	`, entry.Version.ID, entry.Version.Stats.DownloadCount, entry.Version.Stats.FavoriteCount,
		entry.Version.Stats.CommentCount, entry.Version.Stats.RatingCount, entry.Version.Stats.Rating)
	
	if err != nil {
		return fmt.Errorf("error inserting stats for key %s: %w", key, err)
	}

	// Delete existing files and images to replace them
	_, err = tx.Exec("DELETE FROM files WHERE version_id = ?", entry.Version.ID)
	if err != nil {
		return fmt.Errorf("error deleting existing files for key %s: %w", key, err)
	}
	
	_, err = tx.Exec("DELETE FROM model_images WHERE version_id = ?", entry.Version.ID)
	if err != nil {
		return fmt.Errorf("error deleting existing images for key %s: %w", key, err)
	}

	// Insert files
	for _, file := range entry.Version.Files {
		_, err = tx.Exec(`
			INSERT INTO files (
				id, version_id, name, size_kb, type, metadata_fp, metadata_size, metadata_format,
				pickle_scan_result, pickle_scan_message, virus_scan_result, scanned_at,
				download_url, is_primary, hash_autov2, hash_sha256, hash_crc32, hash_blake3
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, file.ID, entry.Version.ID, file.Name, file.SizeKB, file.Type,
			file.Metadata.Fp, file.Metadata.Size, file.Metadata.Format,
			file.PickleScanResult, file.PickleScanMessage, file.VirusScanResult, file.ScannedAt,
			file.DownloadUrl, file.Primary,
			file.Hashes.AutoV2, file.Hashes.SHA256, file.Hashes.CRC32, file.Hashes.BLAKE3)
		
		if err != nil {
			return fmt.Errorf("error inserting file %d for key %s: %w", file.ID, key, err)
		}
	}

	// Insert images
	for _, img := range entry.Version.Images {
		var postID interface{}
		if img.PostID != nil {
			postID = *img.PostID
		}
		
		_, err = tx.Exec(`
			INSERT INTO model_images (
				id, version_id, url, hash, width, height, nsfw, nsfw_level, created_at, post_id,
				stats_cry_count, stats_laugh_count, stats_like_count, stats_heart_count,
				stats_comment_count, username
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, img.ID, entry.Version.ID, img.URL, img.Hash, img.Width, img.Height, img.Nsfw, img.NsfwLevel,
			img.CreatedAt, postID, img.Stats.CryCount, img.Stats.LaughCount, img.Stats.LikeCount,
			img.Stats.HeartCount, img.Stats.CommentCount, img.Username)
		
		if err != nil {
			return fmt.Errorf("error inserting image %d for key %s: %w", img.ID, key, err)
		}
	}

	return tx.Commit()
}

// putPageState stores pagination state
func (d *DB) putPageState(key string, value []byte) error {
	queryHash := strings.TrimPrefix(key, "current_page_")
	currentPage, err := strconv.Atoi(string(value))
	if err != nil {
		return fmt.Errorf("invalid page number in value for key %s: %w", key, err)
	}

	d.Lock()
	defer d.Unlock()

	_, err = d.db.Exec(`
		INSERT OR REPLACE INTO pagination_state (query_hash, current_page)
		VALUES (?, ?)
	`, queryHash, currentPage)
	
	if err != nil {
		return fmt.Errorf("error storing page state for key %s: %w", key, err)
	}
	
	return nil
}

// Delete removes a key from the database.
func (d *DB) Delete(key []byte) error {
	keyStr := string(key)
	
	d.Lock()
	defer d.Unlock()

	if strings.HasPrefix(keyStr, "v_") {
		versionIDStr := strings.TrimPrefix(keyStr, "v_")
		versionID, err := strconv.Atoi(versionIDStr)
		if err != nil {
			return fmt.Errorf("invalid version ID in key %s: %w", keyStr, err)
		}
		
		result, err := d.db.Exec("DELETE FROM models WHERE version_id = ?", versionID)
		if err != nil {
			return fmt.Errorf("error deleting key %s: %w", keyStr, err)
		}
		
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return ErrNotFound
		}
		
	} else if strings.HasPrefix(keyStr, "current_page_") {
		queryHash := strings.TrimPrefix(keyStr, "current_page_")
		
		result, err := d.db.Exec("DELETE FROM pagination_state WHERE query_hash = ?", queryHash)
		if err != nil {
			return fmt.Errorf("error deleting key %s: %w", keyStr, err)
		}
		
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return ErrNotFound
		}
	} else {
		return fmt.Errorf("unsupported key format: %s", keyStr)
	}
	
	return nil
}

// Fold iterates over all key-value pairs and calls the provided function.
func (d *DB) Fold(fn func(key []byte, value []byte) error) error {
	d.RLock()
	defer d.RUnlock()

	// Iterate over model entries
	rows, err := d.db.Query("SELECT version_id FROM models ORDER BY version_id")
	if err != nil {
		return fmt.Errorf("error querying models for fold: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var versionID int
		if err := rows.Scan(&versionID); err != nil {
			log.WithError(err).Warn("Fold: Error scanning version ID")
			continue
		}

		key := []byte(fmt.Sprintf("v_%d", versionID))
		
		// Get the full entry data
		value, err := d.getDatabaseEntry(string(key))
		if err != nil {
			log.WithError(err).Warnf("Fold: Error getting value for key %s", string(key))
			continue
		}

		if err := fn(key, value); err != nil {
			return err
		}
	}

	// Iterate over pagination state
	pageRows, err := d.db.Query("SELECT query_hash, current_page FROM pagination_state")
	if err != nil {
		return fmt.Errorf("error querying pagination state for fold: %w", err)
	}
	defer pageRows.Close()

	for pageRows.Next() {
		var queryHash string
		var currentPage int
		if err := pageRows.Scan(&queryHash, &currentPage); err != nil {
			log.WithError(err).Warn("Fold: Error scanning pagination state")
			continue
		}

		key := []byte(fmt.Sprintf("current_page_%s", queryHash))
		value := []byte(strconv.Itoa(currentPage))

		if err := fn(key, value); err != nil {
			return err
		}
	}

	return nil
}

// Keys returns a channel of all keys in the database.
func (d *DB) Keys() <-chan []byte {
	keysChan := make(chan []byte)

	go func() {
		defer close(keysChan)
		
		d.RLock()
		defer d.RUnlock()

		// Get model keys
		rows, err := d.db.Query("SELECT version_id FROM models ORDER BY version_id")
		if err != nil {
			log.WithError(err).Error("Keys: Error querying models")
			return
		}
		defer rows.Close()

		for rows.Next() {
			var versionID int
			if err := rows.Scan(&versionID); err != nil {
				log.WithError(err).Warn("Keys: Error scanning version ID")
				continue
			}
			keysChan <- []byte(fmt.Sprintf("v_%d", versionID))
		}

		// Get pagination keys
		pageRows, err := d.db.Query("SELECT query_hash FROM pagination_state")
		if err != nil {
			log.WithError(err).Error("Keys: Error querying pagination state")
			return
		}
		defer pageRows.Close()

		for pageRows.Next() {
			var queryHash string
			if err := pageRows.Scan(&queryHash); err != nil {
				log.WithError(err).Warn("Keys: Error scanning query hash")
				continue
			}
			keysChan <- []byte(fmt.Sprintf("current_page_%s", queryHash))
		}
	}()

	return keysChan
}

// GetPageState retrieves the saved page number for a given query hash.
func (d *DB) GetPageState(queryHash string) (int, error) {
	d.RLock()
	defer d.RUnlock()
	
	var currentPage int
	err := d.db.QueryRow("SELECT current_page FROM pagination_state WHERE query_hash = ?", queryHash).Scan(&currentPage)
	
	if err == sql.ErrNoRows {
		return 1, nil // Default to page 1 if not found
	} else if err != nil {
		return 0, fmt.Errorf("error reading page state for %s: %w", queryHash, err)
	}
	
	log.WithField("queryHash", queryHash).Debugf("Retrieved page state: %d", currentPage)
	return currentPage, nil
}

// SetPageState saves the next page number for a given query hash.
func (d *DB) SetPageState(queryHash string, nextPage int) error {
	d.Lock()
	defer d.Unlock()
	
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO pagination_state (query_hash, current_page)
		VALUES (?, ?)
	`, queryHash, nextPage)
	
	if err != nil {
		return fmt.Errorf("error setting page state for %s: %w", queryHash, err)
	}
	
	log.WithField("queryHash", queryHash).Debugf("Set page state to: %d", nextPage)
	return nil
}

// DeletePageState removes the saved page number for a given query hash.
func (d *DB) DeletePageState(queryHash string) error {
	d.Lock()
	defer d.Unlock()
	
	_, err := d.db.Exec("DELETE FROM pagination_state WHERE query_hash = ?", queryHash)
	if err != nil {
		return fmt.Errorf("error deleting page state for %s: %w", queryHash, err)
	}
	
	log.WithField("queryHash", queryHash).Info("Deleted page state")
	return nil
}