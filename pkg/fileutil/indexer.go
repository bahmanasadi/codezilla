package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codezilla/pkg/logger"
)

// IndexConfig holds configuration for file indexing
type IndexConfig struct {
	Directories     []string   // Directories to index
	Recursive       bool       // Whether to index subdirectories
	IgnorePatterns  []string   // Patterns to ignore
	KeywordsToIndex []string   // Keywords to index
	MaxFileSize     int64      // Maximum file size to index (bytes)
	ExcludeExts     []string   // Extensions to exclude
	IncludeExts     []string   // Extensions to include (empty = all)
	ParallelJobs    int        // Number of parallel indexing jobs
	UpdateInterval  time.Duration // Interval for auto-updating the index
}

// Indexer manages the file indexing process
type Indexer struct {
	Config        IndexConfig // Indexing configuration
	Index         *FileIndex  // The file index
	stopChan      chan struct{} // Channel to stop background indexing
	indexingMutex sync.Mutex // Mutex for indexing operations
	isIndexing    bool       // Whether indexing is in progress
}

// NewIndexer creates a new file indexer
func NewIndexer(config IndexConfig) *Indexer {
	// Set default values if not specified
	if config.ParallelJobs <= 0 {
		config.ParallelJobs = 4 // Default to 4 parallel jobs
	}

	if config.MaxFileSize <= 0 {
		config.MaxFileSize = 10 * 1024 * 1024 // Default to 10MB
	}

	return &Indexer{
		Config:    config,
		Index:     NewFileIndex(),
		stopChan:  make(chan struct{}),
		isIndexing: false,
	}
}

// StartIndexing starts the indexing process
func (indexer *Indexer) StartIndexing() error {
	indexer.indexingMutex.Lock()
	defer indexer.indexingMutex.Unlock()

	if indexer.isIndexing {
		return fmt.Errorf("indexing is already in progress")
	}

	logger.Info("Starting file indexing", 
		"directories", indexer.Config.Directories,
		"recursive", indexer.Config.Recursive)

	indexer.isIndexing = true

	// Start indexing in a separate goroutine
	go func() {
		defer func() {
			indexer.indexingMutex.Lock()
			indexer.isIndexing = false
			indexer.indexingMutex.Unlock()
			
			logger.Info("Indexing completed")
		}()

		// Index each directory
		for _, dir := range indexer.Config.Directories {
			err := indexer.indexDirectory(dir)
			if err != nil {
				logger.Error("Error indexing directory", "dir", dir, "error", err)
			}
		}

		// Index keywords if specified
		if len(indexer.Config.KeywordsToIndex) > 0 {
			indexer.Index.IndexKeywords(indexer.Config.KeywordsToIndex)
		}
	}()

	return nil
}

// indexDirectory indexes a directory
func (indexer *Indexer) indexDirectory(dirPath string) error {
	// Check if directory exists
	info, err := os.Stat(dirPath)
	if err != nil {
		return fmt.Errorf("failed to access directory %s: %w", dirPath, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dirPath)
	}

	// Index the directory
	return indexer.Index.IndexDirectory(dirPath, indexer.Config.Recursive, indexer.Config.IgnorePatterns)
}

// StartAutoUpdate starts auto-updating the index at regular intervals
func (indexer *Indexer) StartAutoUpdate() {
	if indexer.Config.UpdateInterval <= 0 {
		return // Auto-update disabled
	}

	go func() {
		ticker := time.NewTicker(indexer.Config.UpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Check if indexing is in progress
				indexer.indexingMutex.Lock()
				isIndexing := indexer.isIndexing
				indexer.indexingMutex.Unlock()

				if !isIndexing {
					logger.Info("Auto-updating index")
					err := indexer.StartIndexing()
					if err != nil {
						logger.Error("Error auto-updating index", "error", err)
					}
				}

			case <-indexer.stopChan:
				return
			}
		}
	}()
}

// StopAutoUpdate stops auto-updating the index
func (indexer *Indexer) StopAutoUpdate() {
	close(indexer.stopChan)
}

// IsIndexingInProgress returns whether indexing is in progress
func (indexer *Indexer) IsIndexingInProgress() bool {
	indexer.indexingMutex.Lock()
	defer indexer.indexingMutex.Unlock()
	return indexer.isIndexing
}

// GetIndexStats returns statistics about the index
func (indexer *Indexer) GetIndexStats() map[string]interface{} {
	stats := indexer.Index.GetStats()
	
	// Add additional indexer stats
	stats["isIndexing"] = indexer.IsIndexingInProgress()
	stats["directoriesConfigured"] = len(indexer.Config.Directories)
	stats["keywordsConfigured"] = len(indexer.Config.KeywordsToIndex)
	
	return stats
}

// RefreshFile updates a single file in the index
func (indexer *Indexer) RefreshFile(filePath string) error {
	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil {
		// If file doesn't exist, remove it from the index
		if os.IsNotExist(err) {
			indexer.Index.RemoveFile(filePath)
			return nil
		}
		return err
	}

	// Skip directories
	if info.IsDir() {
		return nil
	}

	// Check if file should be excluded by pattern
	for _, pattern := range indexer.Config.IgnorePatterns {
		matched, err := filepath.Match(pattern, filepath.Base(filePath))
		if err != nil {
			return err
		}
		if matched {
			return nil
		}
	}

	// Check if file should be excluded by extension
	if len(indexer.Config.ExcludeExts) > 0 {
		ext := filepath.Ext(filePath)
		for _, excludeExt := range indexer.Config.ExcludeExts {
			if ext == excludeExt {
				return nil
			}
		}
	}

	// Check if file should be included by extension
	if len(indexer.Config.IncludeExts) > 0 {
		ext := filepath.Ext(filePath)
		included := false
		for _, includeExt := range indexer.Config.IncludeExts {
			if ext == includeExt {
				included = true
				break
			}
		}
		if !included {
			return nil
		}
	}

	// Check if file is too large
	if info.Size() > indexer.Config.MaxFileSize {
		return nil
	}

	// Add or update file in the index
	return indexer.Index.AddFile(filePath)
}

// SearchByPath returns file info for a given path
func (indexer *Indexer) SearchByPath(path string) *FileInfo {
	// Convert to absolute path if necessary
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}

	// Check if file is in index
	fileInfo, ok := indexer.Index.Files[path]
	if !ok {
		// If not in index, try to add it
		err := indexer.RefreshFile(path)
		if err != nil {
			return nil
		}
		
		// Check again
		fileInfo, ok = indexer.Index.Files[path]
		if !ok {
			return nil
		}
	}

	return fileInfo
}

// WaitForIndexing waits for indexing to complete
func (indexer *Indexer) WaitForIndexing() {
	for {
		if !indexer.IsIndexingInProgress() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}