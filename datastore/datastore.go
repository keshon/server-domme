package datastore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Config holds configuration options for the DataStore
type Config struct {
	FilePath         string
	AutoSaveInterval time.Duration
	MaxMemorySize    int64 // Maximum memory usage in bytes (0 = unlimited)
	BackupCount      int   // Number of backup files to keep
	Logger           *log.Logger
}

// DefaultConfig returns a default configuration
func DefaultConfig(filePath string) *Config {
	return &Config{
		FilePath:         filePath,
		AutoSaveInterval: 10 * time.Second,
		MaxMemorySize:    100 * 1024 * 1024, // 100MB
		BackupCount:      3,
		Logger:           log.New(os.Stderr, "[datastore] ", log.LstdFlags),
	}
}

type DataStore struct {
	data         map[string]any     // in-memory data storage
	file         string             // file path for persistent storage
	mu           sync.RWMutex       // mutex for thread-safe access
	ctx          context.Context    // context for cancellation
	cancel       context.CancelFunc // cancel function
	wg           sync.WaitGroup     // wait group for graceful shutdown
	config       *Config            // configuration
	memorySize   int64              // approximate memory usage
	lastChecksum string             // checksum of last saved data
	closed       bool               // flag to indicate if store is closed
	closeMu      sync.RWMutex       // mutex for close flag
}

// New creates a new DataStore with default configuration
func New(filePath string) (*DataStore, error) {
	return NewWithConfig(DefaultConfig(filePath))
}

// NewWithConfig creates a new DataStore with custom configuration
func NewWithConfig(config *Config) (*DataStore, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if config.FilePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	// Ensure directory exists
	dir := filepath.Dir(config.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	store := &DataStore{
		data:   make(map[string]any),
		file:   config.FilePath,
		ctx:    ctx,
		cancel: cancel,
		config: config,
	}

	// Initialize empty file if it doesn't exist
	if _, err := os.Stat(config.FilePath); os.IsNotExist(err) {
		if err := store.writeFileAtomic([]byte("{}")); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create empty JSON file: %v", err)
		}
	} else if err == nil {
		if err := store.loadFromFile(); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to load data from file: %v", err)
		}
	} else {
		cancel()
		return nil, fmt.Errorf("failed to check file existence: %v", err)
	}

	// Start background routines
	store.wg.Add(2)
	go store.autoSave()
	go store.handleShutdown()

	return store, nil
}

// Add stores a key-value pair
func (ds *DataStore) Add(key string, value any) {
	ds.closeMu.RLock()
	if ds.closed {
		ds.closeMu.RUnlock()
		return
	}
	ds.closeMu.RUnlock()

	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Estimate memory impact
	oldSize := ds.estimateSize(ds.data[key])
	newSize := ds.estimateSize(value)

	// Check memory limit
	if ds.config.MaxMemorySize > 0 {
		newMemorySize := ds.memorySize - oldSize + newSize
		if newMemorySize > ds.config.MaxMemorySize {
			ds.config.Logger.Printf("Memory limit would be exceeded, operation rejected")
			return
		}
		ds.memorySize = newMemorySize
	}

	ds.data[key] = value
}

// Get retrieves a value by key
func (ds *DataStore) Get(key string) (any, bool) {
	ds.closeMu.RLock()
	if ds.closed {
		ds.closeMu.RUnlock()
		return nil, false
	}
	ds.closeMu.RUnlock()

	ds.mu.RLock()
	defer ds.mu.RUnlock()
	value, exists := ds.data[key]
	return value, exists
}

// Delete removes a key-value pair
func (ds *DataStore) Delete(key string) {
	ds.closeMu.RLock()
	if ds.closed {
		ds.closeMu.RUnlock()
		return
	}
	ds.closeMu.RUnlock()

	ds.mu.Lock()
	defer ds.mu.Unlock()

	if value, exists := ds.data[key]; exists {
		ds.memorySize -= ds.estimateSize(value)
		delete(ds.data, key)
	}
}

// SaveToFile forces an immediate save to disk
func (ds *DataStore) SaveToFile() error {
	ds.closeMu.RLock()
	if ds.closed {
		ds.closeMu.RUnlock()
		return fmt.Errorf("datastore is closed")
	}
	ds.closeMu.RUnlock()

	return ds.saveToFile()
}

// Close gracefully shuts down the DataStore
func (ds *DataStore) Close() error {
	ds.closeMu.Lock()
	if ds.closed {
		ds.closeMu.Unlock()
		return nil
	}
	ds.closed = true
	ds.closeMu.Unlock()

	// Cancel context to stop background routines
	ds.cancel()

	// Wait for all goroutines to finish
	ds.wg.Wait()

	// Final save
	return ds.saveToFile()
}

// saveToFile saves data to disk with atomic write and integrity checking
func (ds *DataStore) saveToFile() error {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	// Marshal data
	data, err := json.MarshalIndent(ds.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}

	// Calculate checksum
	checksum := ds.calculateChecksum(data)

	// Skip save if data hasn't changed
	if checksum == ds.lastChecksum {
		return nil
	}

	// Create backup if file exists
	if ds.config.BackupCount > 0 {
		if err := ds.createBackup(); err != nil {
			ds.config.Logger.Printf("Failed to create backup: %v", err)
		}
	}

	// Atomic write
	if err := ds.writeFileAtomic(data); err != nil {
		return err
	}

	// Verify the write
	if err := ds.verifyFile(data); err != nil {
		return fmt.Errorf("file verification failed: %v", err)
	}

	ds.lastChecksum = checksum
	return nil
}

// loadFromFile loads data from disk with validation
func (ds *DataStore) loadFromFile() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	data, err := os.ReadFile(ds.file)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	var temp map[string]any
	if err := json.Unmarshal(data, &temp); err != nil {
		return fmt.Errorf("invalid JSON format: %v", err)
	}

	ds.data = temp
	ds.memorySize = ds.calculateMemoryUsage()
	ds.lastChecksum = ds.calculateChecksum(data)

	return nil
}

// writeFileAtomic performs atomic file write using temporary file and rename
func (ds *DataStore) writeFileAtomic(data []byte) error {
	tmpFile := ds.file + ".tmp"

	// Write to temporary file
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write to temp file: %v", err)
	}

	// Sync to ensure data is written to disk
	file, err := os.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to open temp file for sync: %v", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("failed to sync temp file: %v", err)
	}
	file.Close()

	// Atomic rename
	if err := os.Rename(tmpFile, ds.file); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temp file: %v", err)
	}

	return nil
}

// verifyFile verifies that the written file matches expected data
func (ds *DataStore) verifyFile(expectedData []byte) error {
	actualData, err := os.ReadFile(ds.file)
	if err != nil {
		return fmt.Errorf("failed to read file for verification: %v", err)
	}

	if ds.calculateChecksum(actualData) != ds.calculateChecksum(expectedData) {
		return fmt.Errorf("file checksum mismatch")
	}

	return nil
}

// createBackup creates a timestamped backup of the current file
func (ds *DataStore) createBackup() error {
	if _, err := os.Stat(ds.file); os.IsNotExist(err) {
		return nil // No file to backup
	}

	timestamp := time.Now().Format("20060102_150405")
	backupFile := fmt.Sprintf("%s.backup.%s", ds.file, timestamp)

	// Copy current file to backup
	src, err := os.Open(ds.file)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(backupFile)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	// Clean up old backups
	ds.cleanupOldBackups()

	return nil
}

// cleanupOldBackups removes old backup files beyond the configured limit
func (ds *DataStore) cleanupOldBackups() {
	pattern := ds.file + ".backup.*"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	if len(matches) <= ds.config.BackupCount {
		return
	}

	// Sort by modification time and remove oldest
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var files []fileInfo
	for _, match := range matches {
		if info, err := os.Stat(match); err == nil {
			files = append(files, fileInfo{match, info.ModTime()})
		}
	}

	// Sort by modification time (oldest first)
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[i].modTime.After(files[j].modTime) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	// Remove excess files
	toRemove := len(files) - ds.config.BackupCount
	for i := 0; i < toRemove; i++ {
		os.Remove(files[i].path)
	}
}

// autoSave runs the periodic save routine
func (ds *DataStore) autoSave() {
	defer ds.wg.Done()

	ticker := time.NewTicker(ds.config.AutoSaveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ds.ctx.Done():
			return
		case <-ticker.C:
			if err := ds.saveToFile(); err != nil {
				ds.config.Logger.Printf("Auto-save error: %v", err)
			}
		}
	}
}

// handleShutdown handles graceful shutdown on system signals
func (ds *DataStore) handleShutdown() {
	defer ds.wg.Done()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case <-ds.ctx.Done():
		return
	case <-c:
		ds.config.Logger.Println("Received shutdown signal, closing gracefully...")
		ds.Close()
	}
}

// calculateChecksum computes SHA-256 checksum of data
func (ds *DataStore) calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// estimateSize estimates the memory size of a value
func (ds *DataStore) estimateSize(value any) int64 {
	if value == nil {
		return 0
	}

	// This is a rough estimation - for production, consider using
	// a more sophisticated memory measurement library
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return int64(len(data))
}

// calculateMemoryUsage calculates total memory usage
func (ds *DataStore) calculateMemoryUsage() int64 {
	var total int64
	for _, value := range ds.data {
		total += ds.estimateSize(value)
	}
	return total
}

// Stats returns statistics about the DataStore
func (ds *DataStore) Stats() map[string]any {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	return map[string]any{
		"keys":        len(ds.data),
		"memory_size": ds.memorySize,
		"file_path":   ds.file,
		"last_save":   ds.lastChecksum != "",
	}
}
