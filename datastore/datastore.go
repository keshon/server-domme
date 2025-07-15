package datastore

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type DataStore struct {
	data   map[string]any // in-memory data storage
	file   string         // file path for persistent storage
	mu     sync.RWMutex   // mutex for thread-safe access
	ticker *time.Ticker   // ticker for periodic saving
	done   chan struct{}  // shutdown signal channel, buffered
	wg     sync.WaitGroup // wait group for goroutines
	once   sync.Once      // ensure done channel closes only once
}

func New(filePath string) (*DataStore, error) {
	store := &DataStore{
		data:   make(map[string]interface{}),
		file:   filePath,
		ticker: time.NewTicker(5 * time.Minute), // safer save interval
		done:   make(chan struct{}, 1),          // buffered to avoid blocking
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := os.WriteFile(filePath, []byte("{}"), 0644); err != nil {
			return nil, fmt.Errorf("failed to create empty JSON file: %v", err)
		}
	} else if err == nil {
		if err := store.loadFromFile(); err != nil {
			return nil, fmt.Errorf("failed to load data from file: %v", err)
		}
	} else {
		return nil, fmt.Errorf("failed to check file existence: %v", err)
	}

	store.wg.Add(1)
	go store.autoSave()

	store.handleShutdown()

	return store, nil
}

func (ds *DataStore) Add(key string, value interface{}) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.data[key] = value
}

func (ds *DataStore) Get(key string) (interface{}, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	value, exists := ds.data[key]
	return value, exists
}

func (ds *DataStore) GetAll() map[string]any {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data
}

func (ds *DataStore) Delete(key string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	delete(ds.data, key)
}

func (ds *DataStore) Keys() []string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	keys := make([]string, 0, len(ds.data))
	for key := range ds.data {
		keys = append(keys, key)
	}
	return keys
}

func (ds *DataStore) saveToFile() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	data, err := json.MarshalIndent(ds.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}

	// Write atomically to avoid corruption
	tmpFile := ds.file + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %v", err)
	}
	if err := os.Rename(tmpFile, ds.file); err != nil {
		return fmt.Errorf("failed to rename temp file: %v", err)
	}

	return nil
}

func (ds *DataStore) loadFromFile() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	data, err := os.ReadFile(ds.file)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	if err := json.Unmarshal(data, &ds.data); err != nil {
		return fmt.Errorf("failed to unmarshal data: %v", err)
	}
	return nil
}

func (ds *DataStore) autoSave() {
	defer ds.wg.Done()

	for {
		select {
		case <-ds.done:
			return
		case <-ds.ticker.C:
			if err := ds.saveToFile(); err != nil {
				fmt.Println("Auto-save error:", err)
			}
		}
	}
}

func (ds *DataStore) handleShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("Shutdown signal received, saving data...")
		ds.ticker.Stop()

		ds.closeDone()

		// Wait for autosave goroutine to finish
		ds.wg.Wait()

		if err := ds.saveToFile(); err != nil {
			fmt.Println("Error saving data on shutdown:", err)
		}

		os.Exit(0)
	}()
}

// closeDone safely closes the done channel once
func (ds *DataStore) closeDone() {
	ds.once.Do(func() {
		close(ds.done)
	})
}

func (ds *DataStore) Close() error {
	ds.ticker.Stop()
	ds.closeDone()
	ds.wg.Wait()
	return ds.saveToFile()
}
