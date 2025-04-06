package fileutil

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"codezilla/pkg/logger"
	"codezilla/pkg/style"
)

// Controller manages file utilities for the application
type Controller struct {
	Indexer     *Indexer
	Watcher     *FileWatcher
	Config      ControllerConfig
	initialized bool
}

// ControllerConfig holds configuration for the controller
type ControllerConfig struct {
	IndexConfig      IndexConfig
	WatcherConfig    FileWatcherConfig
	AutoStartIndex   bool
	AutoStartWatcher bool
}

// DefaultControllerConfig returns default configuration
func DefaultControllerConfig() ControllerConfig {
	// Default directories to index (current directory)
	currentDir, err := os.Getwd()
	if err != nil {
		currentDir = "."
	}

	return ControllerConfig{
		IndexConfig: IndexConfig{
			Directories:     []string{currentDir},
			Recursive:       true,
			IgnorePatterns:  []string{".git", "node_modules", "*.log"},
			KeywordsToIndex: []string{"func", "type", "struct", "interface", "const", "var"},
			MaxFileSize:     5 * 1024 * 1024, // 5MB
			ParallelJobs:    4,
			UpdateInterval:  5 * time.Minute,
		},
		WatcherConfig: FileWatcherConfig{
			Directories:     []string{currentDir},
			Recursive:       true,
			IgnorePatterns:  []string{".git", "node_modules", "*.log"},
			PollingInterval: 5 * time.Second,
			UsePolling:      true,
		},
		AutoStartIndex:   true,
		AutoStartWatcher: true,
	}
}

// NewController creates a new file utility controller
func NewController(config ControllerConfig) *Controller {
	logger.Info("Creating file utility controller")

	// Create indexer
	indexer := NewIndexer(config.IndexConfig)

	// Create watcher
	watcher := NewFileWatcher(config.WatcherConfig, indexer)

	return &Controller{
		Indexer:     indexer,
		Watcher:     watcher,
		Config:      config,
		initialized: false,
	}
}

// Initialize initializes the controller
func (c *Controller) Initialize() error {
	if c.initialized {
		return nil
	}

	logger.Info("Initializing file utility controller")

	// Auto-start indexing if configured
	if c.Config.AutoStartIndex {
		err := c.Indexer.StartIndexing()
		if err != nil {
			return fmt.Errorf("failed to start indexing: %w", err)
		}

		logger.Info("Auto-starting index updates")
		c.Indexer.StartAutoUpdate()
	}

	// Auto-start watcher if configured
	if c.Config.AutoStartWatcher {
		err := c.Watcher.StartWatching()
		if err != nil {
			return fmt.Errorf("failed to start watcher: %w", err)
		}

		// Handle watcher events
		go c.handleWatcherEvents()
	}

	c.initialized = true
	return nil
}

// handleWatcherEvents processes events from the file watcher
func (c *Controller) handleWatcherEvents() {
	logger.Info("Starting file watcher event handler")

	for {
		select {
		case event, ok := <-c.Watcher.Events:
			if !ok {
				// Channel closed
				logger.Info("File watcher event channel closed")
				return
			}

			logger.Debug("File event",
				"type", event.Type.String(),
				"path", event.Path)

			// Process event
			c.Watcher.HandleFileEvent(event)

		case err, ok := <-c.Watcher.Errors:
			if !ok {
				// Channel closed
				logger.Info("File watcher error channel closed")
				return
			}

			logger.Error("File watcher error", "error", err)
		}
	}
}

// SearchFiles searches for files matching a pattern
func (c *Controller) SearchFiles(pattern string, isRegex bool, caseSensitive bool) ([]SearchResult, error) {
	logger.Info("Searching files",
		"pattern", pattern,
		"regex", isRegex,
		"case_sensitive", caseSensitive)

	// Build search options
	options := SearchOptions{
		Pattern:       pattern,
		IsRegex:       isRegex,
		CaseSensitive: caseSensitive,
		Directories:   c.Config.IndexConfig.Directories,
		ContextLines:  2,
		UseIndex:      true,
		Indexer:       c.Indexer,
	}

	// Perform search
	return Search(options)
}

// PrintSearchResults prints search results
func (c *Controller) PrintSearchResults(results []SearchResult) {
	if len(results) == 0 {
		fmt.Printf("%sNo matching files found%s\n", style.Red, style.Reset)
		return
	}

	fmt.Printf("%sFound %d matches:%s\n", style.Bold, len(results), style.Reset)

	// Group by file
	fileGroups := make(map[string][]SearchResult)
	for _, result := range results {
		fileGroups[result.FilePath] = append(fileGroups[result.FilePath], result)
	}

	// Print results by file
	for filePath, matches := range fileGroups {
		fmt.Printf("\n%s%s%s (%d matches):\n", style.Bold, filePath, style.Reset, len(matches))

		for _, match := range matches {
			// Print line number and content
			fmt.Printf("%sLine %d:%s %s\n", style.Green, match.Line, style.Reset, match.Content)

			// Print context lines
			if len(match.Context) > 0 {
				fmt.Printf("%sContext:%s\n", style.Blue, style.Reset)
				for i, context := range match.Context {
					// Determine line number for context line
					contextLine := match.Line
					if i < len(match.Context)/2 {
						contextLine = match.Line - (len(match.Context)/2 - i)
					} else {
						contextLine = match.Line + (i - len(match.Context)/2 + 1)
					}
					fmt.Printf("  %d: %s\n", contextLine, context)
				}
			}

			fmt.Println()
		}
	}
}

// GetFileStats returns statistics about indexed files
func (c *Controller) GetFileStats() map[string]interface{} {
	return c.Indexer.GetIndexStats()
}

// PrintFileStats prints statistics about indexed files
func (c *Controller) PrintFileStats() {
	stats := c.GetFileStats()

	fmt.Printf("%sFile Index Statistics:%s\n", style.Bold, style.Reset)
	fmt.Printf("Total files: %d\n", stats["totalFiles"])
	fmt.Printf("Total lines: %d\n", stats["totalLines"])
	fmt.Printf("Total size: %s\n", formatSize(stats["totalSize"].(int64)))
	fmt.Printf("Directories: %d\n", stats["directoriesCount"])
	fmt.Printf("Extensions: %d\n", stats["extensionsCount"])
	fmt.Printf("Keywords indexed: %d\n", stats["keywordsIndexed"])

	// Print indexing status
	if stats["isIndexing"].(bool) {
		fmt.Printf("%sIndexing is in progress%s\n", style.Green, style.Reset)
	} else {
		fmt.Printf("%sIndexing is complete%s\n", style.Green, style.Reset)
	}

	// Print extension stats
	if extCounts, ok := stats["extensionCounts"].(map[string]int); ok && len(extCounts) > 0 {
		fmt.Printf("\n%sFiles by extension:%s\n", style.Bold, style.Reset)
		for ext, count := range extCounts {
			fmt.Printf("  %s: %d\n", ext, count)
		}
	}
}

// formatSize formats a size in bytes to a human-readable format
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// FindFilesByExtension returns files with a given extension
func (c *Controller) FindFilesByExtension(extension string) []string {
	// Ensure extension starts with a dot
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}

	return c.Indexer.Index.SearchByExtension(extension)
}

// PrintFilesByExtension prints files with a given extension
func (c *Controller) PrintFilesByExtension(extension string) {
	files := c.FindFilesByExtension(extension)

	if len(files) == 0 {
		fmt.Printf("%sNo files found with extension '%s'%s\n", style.Red, extension, style.Reset)
		return
	}

	fmt.Printf("%sFound %d files with extension '%s':%s\n", style.Bold, len(files), extension, style.Reset)

	for i, file := range files {
		fmt.Printf("[%d] %s\n", i+1, file)
	}
}

// FindFilesByKeyword returns files containing a given keyword
func (c *Controller) FindFilesByKeyword(keyword string) []string {
	return c.Indexer.Index.SearchByKeyword(keyword)
}

// PrintFilesByKeyword prints files containing a given keyword
func (c *Controller) PrintFilesByKeyword(keyword string) {
	files := c.FindFilesByKeyword(keyword)

	if len(files) == 0 {
		fmt.Printf("%sNo files found containing '%s'%s\n", style.Red, keyword, style.Reset)
		return
	}

	fmt.Printf("%sFound %d files containing '%s':%s\n", style.Bold, len(files), keyword, style.Reset)

	for i, file := range files {
		fmt.Printf("[%d] %s\n", i+1, file)
	}
}

// GetFileContent returns the content of a file
func (c *Controller) GetFileContent(filePath string) ([]string, error) {
	// Try to get from index first
	fileInfo := c.Indexer.SearchByPath(filePath)
	if fileInfo != nil {
		return fileInfo.ContentLines, nil
	}

	// If not in index, read file directly
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

// Stop stops the controller
func (c *Controller) Stop() {
	logger.Info("Stopping file utility controller")

	// Stop watcher
	if c.Watcher != nil && c.Watcher.IsWatching() {
		c.Watcher.StopWatching()
	}

	// Stop indexer auto-update
	if c.Indexer != nil {
		c.Indexer.StopAutoUpdate()
	}
}
