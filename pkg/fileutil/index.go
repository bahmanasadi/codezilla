package fileutil

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codezilla/pkg/logger"
)

// FileInfo represents information about a file in the index
type FileInfo struct {
	Path         string            // Full path to the file
	Size         int64             // Size of file in bytes
	ModTime      time.Time         // Last modification time
	ContentLines []string          // Content of the file, line by line
	ContentStats map[string]int    // Stats about the content (e.g., word count)
	Metadata     map[string]string // Additional metadata
}

// FileIndex represents an index of files and their content
type FileIndex struct {
	Files       map[string]*FileInfo   // Map of file paths to file info
	Extensions  map[string][]string    // Map of file extensions to file paths
	Directories map[string][]string    // Map of directories to file paths
	Keywords    map[string][]string    // Map of keywords to file paths
	mu          sync.RWMutex           // Mutex for concurrent access
}

// NewFileIndex creates a new file index
func NewFileIndex() *FileIndex {
	return &FileIndex{
		Files:       make(map[string]*FileInfo),
		Extensions:  make(map[string][]string),
		Directories: make(map[string][]string),
		Keywords:    make(map[string][]string),
	}
}

// IndexDirectory adds all files in a directory (and optionally subdirectories) to the index
func (fi *FileIndex) IndexDirectory(dirPath string, recursive bool, ignorePatterns []string) error {
	logger.Info("Indexing directory", "dir", dirPath, "recursive", recursive)

	// Create a matcher function for ignore patterns
	shouldIgnore := func(path string) bool {
		for _, pattern := range ignorePatterns {
			matched, err := filepath.Match(pattern, filepath.Base(path))
			if err != nil {
				logger.Error("Invalid ignore pattern", "pattern", pattern, "error", err)
				continue
			}
			if matched {
				return true
			}
		}
		return false
	}

	// Walk the directory
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error("Error accessing path", "path", path, "error", err)
			return nil // Continue walking despite errors
		}

		// Check if we should ignore this path
		if shouldIgnore(path) {
			logger.Debug("Ignoring path", "path", path)
			if info.IsDir() && path != dirPath {
				return filepath.SkipDir // Skip this directory
			}
			return nil
		}

		// Skip directories unless it's the root directory
		if info.IsDir() {
			// If not recursive, skip subdirectories
			if !recursive && path != dirPath {
				return filepath.SkipDir
			}
			return nil
		}

		// Add the file to the index
		if err := fi.AddFile(path); err != nil {
			logger.Error("Error adding file to index", "path", path, "error", err)
		}

		return nil
	}

	return filepath.Walk(dirPath, walkFn)
}

// AddFile adds a single file to the index
func (fi *FileIndex) AddFile(filePath string) error {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	logger.Debug("Adding file to index", "path", filePath)

	// Get file information
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Skip directories
	if info.IsDir() {
		return nil
	}

	// Create a new FileInfo
	fileInfo := &FileInfo{
		Path:         filePath,
		Size:         info.Size(),
		ModTime:      info.ModTime(),
		ContentLines: []string{},
		ContentStats: make(map[string]int),
		Metadata:     make(map[string]string),
	}

	// Read file content
	if err := fi.readFileContent(fileInfo); err != nil {
		return fmt.Errorf("failed to read file content: %w", err)
	}

	// Calculate content statistics
	fi.calculateContentStats(fileInfo)

	// Add file to the main index
	fi.Files[filePath] = fileInfo

	// Add file to extension index
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != "" {
		fi.Extensions[ext] = append(fi.Extensions[ext], filePath)
	}

	// Add file to directory index
	dir := filepath.Dir(filePath)
	fi.Directories[dir] = append(fi.Directories[dir], filePath)

	logger.Debug("File added to index", "path", filePath, "size", info.Size(), "lines", len(fileInfo.ContentLines))
	return nil
}

// readFileContent reads the content of a file and stores it in the FileInfo
func (fi *FileIndex) readFileContent(fileInfo *FileInfo) error {
	file, err := os.Open(fileInfo.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fileInfo.ContentLines = append(fileInfo.ContentLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

// calculateContentStats calculates statistics about the file content
func (fi *FileIndex) calculateContentStats(fileInfo *FileInfo) {
	// Count total lines
	fileInfo.ContentStats["lines"] = len(fileInfo.ContentLines)

	// Count non-empty lines
	nonEmptyLines := 0
	for _, line := range fileInfo.ContentLines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}
	fileInfo.ContentStats["nonEmptyLines"] = nonEmptyLines

	// Count words
	wordCount := 0
	for _, line := range fileInfo.ContentLines {
		words := strings.Fields(line)
		wordCount += len(words)
	}
	fileInfo.ContentStats["words"] = wordCount
}

// IndexKeywords adds keywords found in files to the keyword index
func (fi *FileIndex) IndexKeywords(keywords []string) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	logger.Info("Indexing keywords", "keywords", keywords)

	// Reset keyword index
	fi.Keywords = make(map[string][]string)

	// For each file
	for path, fileInfo := range fi.Files {
		// For each keyword
		for _, keyword := range keywords {
			keywordFound := false

			// Check content lines
			for _, line := range fileInfo.ContentLines {
				if strings.Contains(line, keyword) {
					keywordFound = true
					break
				}
			}

			if keywordFound {
				fi.Keywords[keyword] = append(fi.Keywords[keyword], path)
			}
		}
	}
}

// SearchByKeyword returns files containing a given keyword
func (fi *FileIndex) SearchByKeyword(keyword string) []string {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	logger.Info("Searching by keyword", "keyword", keyword)

	// Check if keyword is indexed
	if files, ok := fi.Keywords[keyword]; ok {
		return files
	}

	// If not indexed, search all files
	var results []string
	for path, fileInfo := range fi.Files {
		for _, line := range fileInfo.ContentLines {
			if strings.Contains(line, keyword) {
				results = append(results, path)
				break
			}
		}
	}

	return results
}

// SearchByExtension returns files with a given extension
func (fi *FileIndex) SearchByExtension(extension string) []string {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	logger.Info("Searching by extension", "extension", extension)

	// Ensure extension starts with a dot
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}

	extension = strings.ToLower(extension)
	return fi.Extensions[extension]
}

// SearchByDirectory returns files in a given directory
func (fi *FileIndex) SearchByDirectory(directory string) []string {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	logger.Info("Searching by directory", "directory", directory)

	// Normalize the directory path
	directory, _ = filepath.Abs(directory)
	return fi.Directories[directory]
}

// GetLineContexts returns lines containing a keyword with context
func (fi *FileIndex) GetLineContexts(filePath, keyword string, contextLines int) []map[string]interface{} {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	logger.Info("Getting line contexts", "file", filePath, "keyword", keyword, "context", contextLines)

	fileInfo, ok := fi.Files[filePath]
	if !ok {
		return nil
	}

	var results []map[string]interface{}

	// Find keyword occurrences
	for i, line := range fileInfo.ContentLines {
		if strings.Contains(line, keyword) {
			// Prepare context
			startLine := i - contextLines
			if startLine < 0 {
				startLine = 0
			}

			endLine := i + contextLines
			if endLine >= len(fileInfo.ContentLines) {
				endLine = len(fileInfo.ContentLines) - 1
			}

			// Create context
			context := make([]string, endLine-startLine+1)
			for j := startLine; j <= endLine; j++ {
				context[j-startLine] = fileInfo.ContentLines[j]
			}

			// Add result
			results = append(results, map[string]interface{}{
				"line":    i + 1, // 1-based line number
				"content": line,
				"context": context,
				"start":   startLine + 1, // 1-based line number
				"end":     endLine + 1,   // 1-based line number
			})
		}
	}

	return results
}

// SearchContent performs a full-text search on file content
func (fi *FileIndex) SearchContent(query string, caseSensitive bool) []map[string]interface{} {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	logger.Info("Searching content", "query", query, "case_sensitive", caseSensitive)

	var results []map[string]interface{}

	// If not case sensitive, convert query to lowercase
	if !caseSensitive {
		query = strings.ToLower(query)
	}

	// For each file
	for path, fileInfo := range fi.Files {
		matches := []int{} // Line numbers

		// For each line
		for i, line := range fileInfo.ContentLines {
			// Check if line contains query
			var lineToCheck string
			if caseSensitive {
				lineToCheck = line
			} else {
				lineToCheck = strings.ToLower(line)
			}

			if strings.Contains(lineToCheck, query) {
				matches = append(matches, i)
			}
		}

		// If matches found, add to results
		if len(matches) > 0 {
			results = append(results, map[string]interface{}{
				"file":    path,
				"matches": matches,
				"count":   len(matches),
			})
		}
	}

	return results
}

// GetStats returns statistics about the file index
func (fi *FileIndex) GetStats() map[string]interface{} {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	// Count files by extension
	extensionCounts := make(map[string]int)
	for ext, files := range fi.Extensions {
		extensionCounts[ext] = len(files)
	}

	// Count files by directory
	directoryCounts := make(map[string]int)
	for dir, files := range fi.Directories {
		directoryCounts[dir] = len(files)
	}

	// Calculate total sizes
	var totalSize int64
	var totalLines int
	for _, fileInfo := range fi.Files {
		totalSize += fileInfo.Size
		totalLines += fileInfo.ContentStats["lines"]
	}

	// Return stats
	return map[string]interface{}{
		"totalFiles":       len(fi.Files),
		"extensionCounts":  extensionCounts,
		"directoryCounts":  directoryCounts,
		"totalSize":        totalSize,
		"totalLines":       totalLines,
		"keywordsIndexed":  len(fi.Keywords),
		"extensionsCount":  len(fi.Extensions),
		"directoriesCount": len(fi.Directories),
	}
}

// RemoveFile removes a file from the index
func (fi *FileIndex) RemoveFile(filePath string) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	logger.Debug("Removing file from index", "path", filePath)

	// Get file info
	fileInfo, ok := fi.Files[filePath]
	if !ok {
		return
	}

	// Remove from main index
	delete(fi.Files, filePath)

	// Remove from extension index
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != "" {
		files := fi.Extensions[ext]
		for i, path := range files {
			if path == filePath {
				fi.Extensions[ext] = append(files[:i], files[i+1:]...)
				break
			}
		}
	}

	// Remove from directory index
	dir := filepath.Dir(filePath)
	files := fi.Directories[dir]
	for i, path := range files {
		if path == filePath {
			fi.Directories[dir] = append(files[:i], files[i+1:]...)
			break
		}
	}

	// Remove from keyword index
	for keyword, files := range fi.Keywords {
		for i, path := range files {
			if path == filePath {
				fi.Keywords[keyword] = append(files[:i], files[i+1:]...)
				break
			}
		}
	}

	logger.Debug("File removed from index", "path", filePath, "size", fileInfo.Size)
}

// FilterFiles returns files matching a set of criteria
func (fi *FileIndex) FilterFiles(criteria map[string]interface{}) []string {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	logger.Info("Filtering files", "criteria", criteria)

	// Start with all files
	paths := make([]string, 0, len(fi.Files))
	for path := range fi.Files {
		paths = append(paths, path)
	}

	// Apply extension filter
	if ext, ok := criteria["extension"].(string); ok {
		// Ensure extension starts with a dot
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		ext = strings.ToLower(ext)

		filtered := make([]string, 0)
		for _, path := range paths {
			if strings.ToLower(filepath.Ext(path)) == ext {
				filtered = append(filtered, path)
			}
		}
		paths = filtered
	}

	// Apply directory filter
	if dir, ok := criteria["directory"].(string); ok {
		dir, _ = filepath.Abs(dir)

		filtered := make([]string, 0)
		for _, path := range paths {
			if filepath.Dir(path) == dir {
				filtered = append(filtered, path)
			}
		}
		paths = filtered
	}

	// Apply size filter
	if minSize, ok := criteria["minSize"].(int64); ok {
		filtered := make([]string, 0)
		for _, path := range paths {
			if fi.Files[path].Size >= minSize {
				filtered = append(filtered, path)
			}
		}
		paths = filtered
	}

	if maxSize, ok := criteria["maxSize"].(int64); ok {
		filtered := make([]string, 0)
		for _, path := range paths {
			if fi.Files[path].Size <= maxSize {
				filtered = append(filtered, path)
			}
		}
		paths = filtered
	}

	// Apply modification time filter
	if minTime, ok := criteria["minTime"].(time.Time); ok {
		filtered := make([]string, 0)
		for _, path := range paths {
			if fi.Files[path].ModTime.After(minTime) {
				filtered = append(filtered, path)
			}
		}
		paths = filtered
	}

	if maxTime, ok := criteria["maxTime"].(time.Time); ok {
		filtered := make([]string, 0)
		for _, path := range paths {
			if fi.Files[path].ModTime.Before(maxTime) {
				filtered = append(filtered, path)
			}
		}
		paths = filtered
	}

	// Apply keyword filter
	if keyword, ok := criteria["keyword"].(string); ok {
		filtered := make([]string, 0)
		for _, path := range paths {
			for _, line := range fi.Files[path].ContentLines {
				if strings.Contains(line, keyword) {
					filtered = append(filtered, path)
					break
				}
			}
		}
		paths = filtered
	}

	return paths
}