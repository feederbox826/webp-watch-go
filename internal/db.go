package internal

import (
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"sync"
)

type fileRecord struct {
	Mtime int64
	Size  int64
}

type DB struct {
	mu     sync.Mutex
	cache  map[string]fileRecord
	dbFile string
}

func NewDB(dbFile string) (*DB, error) {
	db := &DB{
		cache:  make(map[string]fileRecord),
		dbFile: dbFile,
	}

	// Load existing cache from disk if it exists
	if err := db.load(); err != nil {
		log.Printf("Warning: failed to load cache from %s: %v", dbFile, err)
		// Continue with empty cache if load fails
	}

	return db, nil
}

func (d *DB) load() error {
	if d.dbFile == "" {
		return nil
	}

	// Try to open with read lock (non-blocking)
	file, err := os.Open(d.dbFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's okay
		}
		return err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&d.cache); err != nil {
		return fmt.Errorf("failed to decode cache: %w", err)
	}

	log.Printf("Loaded %d entries from cache", len(d.cache))
	return nil
}

func (d *DB) save() error {
	if d.dbFile == "" {
		return nil
	}

	// Merge with on-disk cache to avoid losing updates from other processes
	// This is a best-effort merge - if the file is being written by another
	// process, we might still have conflicts, but atomic writes prevent corruption
	d.mu.Lock()
	localCache := make(map[string]fileRecord)
	for k, v := range d.cache {
		localCache[k] = v
	}
	d.mu.Unlock()

	// Try to load current on-disk cache and merge
	if file, err := os.Open(d.dbFile); err == nil {
		var diskCache map[string]fileRecord
		decoder := gob.NewDecoder(file)
		if err := decoder.Decode(&diskCache); err == nil {
			// Merge: local cache takes precedence (newer updates)
			for k, v := range diskCache {
				if _, exists := localCache[k]; !exists {
					localCache[k] = v
				}
			}
		}
		file.Close()
	}

	// Use atomic write: write to temp file, then rename
	// Rename is atomic on most filesystems and prevents corruption
	tempFile := d.dbFile + ".tmp"

	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp cache file: %w", err)
	}

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(localCache); err != nil {
		file.Close()
		os.Remove(tempFile) // Clean up on error
		return fmt.Errorf("failed to encode cache: %w", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(tempFile)
		return fmt.Errorf("failed to sync cache file: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to close temp cache file: %w", err)
	}

	// Atomic rename - this is the critical operation
	// On most filesystems, rename is atomic, preventing corruption
	if err := os.Rename(tempFile, d.dbFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp cache file: %w", err)
	}

	// Update our in-memory cache with merged version
	d.mu.Lock()
	d.cache = localCache
	d.mu.Unlock()

	log.Printf("Saved %d entries to cache", len(localCache))
	return nil
}

func (d *DB) NeedsProcessing(filePath string, info os.FileInfo) (bool, error) {

	fileMtime := info.ModTime().Unix()
	fileSize := info.Size()

	d.mu.Lock()
	record, exists := d.cache[filePath]
	d.mu.Unlock()

	if !exists { // not in cache
		return true, nil
	}
	// mtime and size match
	if record.Mtime >= fileMtime && record.Size == fileSize {
		return false, nil
	}
	return true, nil
}

// UpdateStatus updates the cache with the current status of a source file
func (d *DB) UpdateStatus(filePath string, info os.FileInfo) error {
	d.mu.Lock()
	d.cache[filePath] = fileRecord{
		Mtime: info.ModTime().Unix(),
		Size:  info.Size(),
	}
	d.mu.Unlock()
	return nil
}

func (d *DB) Close() error {
	// Don't lock here - save() handles its own locking
	// Locking here would cause a deadlock since save() also locks
	return d.save()
}
