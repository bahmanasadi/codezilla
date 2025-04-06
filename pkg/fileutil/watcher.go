package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codezilla/pkg/logger"
)

// FileEvent represents a file system event
type FileEvent struct {
	Type     FileEventType
	Path     string
	Info     os.FileInfo
	OldPath  string    // For rename events
	Time     time.Time // When the event occurred
	Metadata map[string]interface{}
}

// FileEventType represents the type of file system event
type FileEventType int

const (
	// EventCreate is triggered when a file is created
	EventCreate FileEventType = iota
	// EventModify is triggered when a file is modified
	EventModify
	// EventDelete is triggered when a file is deleted
	EventDelete
	// EventRename is triggered when a file is renamed
	EventRename
)

// String returns a string representation of the event type
func (t FileEventType) String() string {
	switch t {
	case EventCreate:
		return "CREATE"
	case EventModify:
		return "MODIFY"
	case EventDelete:
		return "DELETE"
	case EventRename:
		return "RENAME"
	default:
		return "UNKNOWN"
	}
}

// FileWatcherConfig holds configuration for file watching
type FileWatcherConfig struct {
	Directories     []string      // Directories to watch
	Recursive       bool          // Whether to watch subdirectories
	IgnorePatterns  []string      // Patterns to ignore
	PollingInterval time.Duration // Interval for polling (used as fallback)
	UsePolling      bool          // Force polling even if native is available
}

// FileWatcher watches file system events
type FileWatcher struct {
	Config        FileWatcherConfig    // Watcher configuration
	Events        chan FileEvent       // Channel to receive events
	Errors        chan error           // Channel to receive errors
	indexer       *Indexer             // File indexer
	stopChan      chan struct{}        // Channel to stop watching
	watchingMutex sync.Mutex           // Mutex for watching operations
	isWatching    bool                 // Whether watching is in progress
	fileStates    map[string]fileState // Map of file states
}

// fileState holds information about a file's state
type fileState struct {
	Path    string
	ModTime time.Time
	Size    int64
	Exists  bool
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(config FileWatcherConfig, indexer *Indexer) *FileWatcher {
	return &FileWatcher{
		Config:     config,
		Events:     make(chan FileEvent),
		Errors:     make(chan error),
		indexer:    indexer,
		stopChan:   make(chan struct{}),
		isWatching: false,
		fileStates: make(map[string]fileState),
	}
}

// StartWatching starts watching for file system events
func (watcher *FileWatcher) StartWatching() error {
	watcher.watchingMutex.Lock()
	defer watcher.watchingMutex.Unlock()

	if watcher.isWatching {
		return fmt.Errorf("already watching")
	}

	logger.Info("Starting file watcher",
		"directories", watcher.Config.Directories,
		"recursive", watcher.Config.Recursive)

	watcher.isWatching = true

	// Start watching in a separate goroutine
	go func() {
		defer func() {
			watcher.watchingMutex.Lock()
			watcher.isWatching = false
			close(watcher.Events)
			close(watcher.Errors)
			watcher.watchingMutex.Unlock()

			logger.Info("File watcher stopped")
		}()

		// Initialize file states
		for _, dir := range watcher.Config.Directories {
			err := watcher.initializeFileStates(dir)
			if err != nil {
				watcher.Errors <- fmt.Errorf("failed to initialize file states: %w", err)
			}
		}

		// Start polling if native watching is not available or forced
		if watcher.Config.UsePolling {
			watcher.startPolling()
		} else {
			// Native watching falls back to polling if not available
			watcher.startPolling()
		}
	}()

	return nil
}

// initializeFileStates initializes the file states for a directory
func (watcher *FileWatcher) initializeFileStates(dirPath string) error {
	// Walk the directory
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories unless recursive is enabled
		if info.IsDir() && path != dirPath {
			if !watcher.Config.Recursive {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file should be ignored
		for _, pattern := range watcher.Config.IgnorePatterns {
			matched, err := filepath.Match(pattern, filepath.Base(path))
			if err != nil {
				return err
			}
			if matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Add file state
		if !info.IsDir() {
			watcher.fileStates[path] = fileState{
				Path:    path,
				ModTime: info.ModTime(),
				Size:    info.Size(),
				Exists:  true,
			}
		}

		return nil
	}

	return filepath.Walk(dirPath, walkFn)
}

// startPolling starts polling for file system changes
func (watcher *FileWatcher) startPolling() {
	ticker := time.NewTicker(watcher.Config.PollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			watcher.pollChanges()
		case <-watcher.stopChan:
			return
		}
	}
}

// pollChanges polls for file system changes
func (watcher *FileWatcher) pollChanges() {
	// Copy current file states
	currentStates := make(map[string]fileState)
	for path, state := range watcher.fileStates {
		currentStates[path] = state
	}

	// For each watched directory
	for _, dir := range watcher.Config.Directories {
		// Walk the directory
		walkFn := func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip directories unless recursive is enabled
			if info.IsDir() && path != dir {
				if !watcher.Config.Recursive {
					return filepath.SkipDir
				}
				return nil
			}

			// Check if file should be ignored
			for _, pattern := range watcher.Config.IgnorePatterns {
				matched, err := filepath.Match(pattern, filepath.Base(path))
				if err != nil {
					return err
				}
				if matched {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}

			// Check if file is new or modified
			if !info.IsDir() {
				oldState, exists := currentStates[path]
				if !exists {
					// New file
					watcher.Events <- FileEvent{
						Type: EventCreate,
						Path: path,
						Info: info,
						Time: time.Now(),
					}
					// Update file state
					watcher.fileStates[path] = fileState{
						Path:    path,
						ModTime: info.ModTime(),
						Size:    info.Size(),
						Exists:  true,
					}
				} else {
					// Check if file is modified
					if oldState.ModTime != info.ModTime() || oldState.Size != info.Size() {
						watcher.Events <- FileEvent{
							Type: EventModify,
							Path: path,
							Info: info,
							Time: time.Now(),
						}
						// Update file state
						watcher.fileStates[path] = fileState{
							Path:    path,
							ModTime: info.ModTime(),
							Size:    info.Size(),
							Exists:  true,
						}
					}
				}

				// Remove from current states to track deletions
				delete(currentStates, path)
			}

			return nil
		}

		err := filepath.Walk(dir, walkFn)
		if err != nil {
			watcher.Errors <- err
		}
	}

	// Process deleted files
	for path, _ := range currentStates {
		// Check if file has directory in watched directories
		inWatchedDir := false
		for _, dir := range watcher.Config.Directories {
			rel, err := filepath.Rel(dir, path)
			if err == nil && !filepath.IsAbs(rel) {
				inWatchedDir = true
				break
			}
		}

		if !inWatchedDir {
			// Skip files not in watched directories
			continue
		}

		// File is deleted
		watcher.Events <- FileEvent{
			Type: EventDelete,
			Path: path,
			Time: time.Now(),
		}
		delete(watcher.fileStates, path)
	}
}

// StopWatching stops watching for file system events
func (watcher *FileWatcher) StopWatching() {
	close(watcher.stopChan)
}

// IsWatching returns whether watching is in progress
func (watcher *FileWatcher) IsWatching() bool {
	watcher.watchingMutex.Lock()
	defer watcher.watchingMutex.Unlock()
	return watcher.isWatching
}

// HandleFileEvent processes a file event
func (watcher *FileWatcher) HandleFileEvent(event FileEvent) {
	logger.Debug("Handling file event",
		"type", event.Type.String(),
		"path", event.Path)

	// Update file indexer
	switch event.Type {
	case EventCreate, EventModify:
		if watcher.indexer != nil {
			err := watcher.indexer.RefreshFile(event.Path)
			if err != nil {
				logger.Error("Error refreshing file", "path", event.Path, "error", err)
			}
		}
	case EventDelete:
		if watcher.indexer != nil {
			watcher.indexer.Index.RemoveFile(event.Path)
		}
	case EventRename:
		if watcher.indexer != nil {
			// Remove old file
			watcher.indexer.Index.RemoveFile(event.OldPath)
			// Add new file
			err := watcher.indexer.RefreshFile(event.Path)
			if err != nil {
				logger.Error("Error refreshing renamed file", "path", event.Path, "error", err)
			}
		}
	}
}
