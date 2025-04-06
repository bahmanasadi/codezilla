package fsutil

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"codezilla/pkg/logger"
)

// SearchOptions represents options for file searching
type SearchOptions struct {
	Pattern       string   // Search pattern
	IsRegex       bool     // Whether pattern is a regex
	CaseSensitive bool     // Whether search is case sensitive
	Directories   []string // Directories to search in
	FilePatterns  []string // File patterns to include
	MaxResults    int      // Maximum number of results
	MaxDepth      int      // Maximum directory depth
	ContextLines  int      // Number of context lines to include
	UseIndex      bool     // Whether to use the file index
	Indexer       *Indexer // File indexer to use
}

// SearchResult represents a search result
type SearchResult struct {
	FilePath   string   // Path to the file
	Line       int      // Line number
	Content    string   // Line content
	Context    []string // Context lines
	MatchStart int      // Start position of match in line
	MatchEnd   int      // End position of match in line
}

// Search searches for a pattern in files
func Search(options SearchOptions) ([]SearchResult, error) {
	logger.Info("Searching for files",
		"pattern", options.Pattern,
		"regex", options.IsRegex,
		"case_sensitive", options.CaseSensitive)

	var results []SearchResult
	var resultsLock sync.Mutex
	var wg sync.WaitGroup
	var jobChan = make(chan string)
	var errorsChan = make(chan error)
	var done = make(chan struct{})

	// Prepare regex if needed
	var re *regexp.Regexp
	var err error
	if options.IsRegex {
		if options.CaseSensitive {
			re, err = regexp.Compile(options.Pattern)
		} else {
			re, err = regexp.Compile("(?i)" + options.Pattern)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %w", err)
		}
	}

	// Set defaults
	if options.MaxResults <= 0 {
		options.MaxResults = 1000
	}
	if options.MaxDepth <= 0 {
		options.MaxDepth = 10
	}
	if options.ContextLines < 0 {
		options.ContextLines = 0
	}

	// If using index and indexer is provided
	if options.UseIndex && options.Indexer != nil {
		// Wait for indexing to complete
		options.Indexer.WaitForIndexing()

		// Search the index
		contentResults := options.Indexer.Index.SearchContent(options.Pattern, options.CaseSensitive)
		for _, result := range contentResults {
			filePath := result["file"].(string)
			matches := result["matches"].([]int)

			for _, lineNum := range matches {
				// Check if max results reached
				resultsLock.Lock()
				if len(results) >= options.MaxResults {
					resultsLock.Unlock()
					break
				}
				resultsLock.Unlock()

				// Get file info
				fileInfo := options.Indexer.SearchByPath(filePath)
				if fileInfo == nil {
					continue
				}

				// Get line content
				lineContent := fileInfo.ContentLines[lineNum]

				// Get context lines
				var context []string
				contextStart := lineNum - options.ContextLines
				if contextStart < 0 {
					contextStart = 0
				}
				contextEnd := lineNum + options.ContextLines
				if contextEnd >= len(fileInfo.ContentLines) {
					contextEnd = len(fileInfo.ContentLines) - 1
				}
				for i := contextStart; i <= contextEnd; i++ {
					if i != lineNum {
						context = append(context, fileInfo.ContentLines[i])
					}
				}

				// Find match positions
				matchStart := -1
				matchEnd := -1
				if options.IsRegex {
					indices := re.FindStringIndex(lineContent)
					if indices != nil {
						matchStart = indices[0]
						matchEnd = indices[1]
					}
				} else {
					var haystack, needle string
					if options.CaseSensitive {
						haystack = lineContent
						needle = options.Pattern
					} else {
						haystack = strings.ToLower(lineContent)
						needle = strings.ToLower(options.Pattern)
					}
					matchStart = strings.Index(haystack, needle)
					if matchStart >= 0 {
						matchEnd = matchStart + len(needle)
					}
				}

				// Add result
				resultsLock.Lock()
				results = append(results, SearchResult{
					FilePath:   filePath,
					Line:       lineNum + 1, // 1-based line number
					Content:    lineContent,
					Context:    context,
					MatchStart: matchStart,
					MatchEnd:   matchEnd,
				})
				resultsLock.Unlock()
			}

			// Check if max results reached
			resultsLock.Lock()
			if len(results) >= options.MaxResults {
				resultsLock.Unlock()
				break
			}
			resultsLock.Unlock()
		}

		return results, nil
	}

	// Start worker goroutines
	workerCount := 8 // Number of worker goroutines
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range jobChan {
				searchInFile(filePath, options, re, &results, &resultsLock)

				// Check if max results reached
				resultsLock.Lock()
				maxReached := len(results) >= options.MaxResults
				resultsLock.Unlock()

				if maxReached {
					return
				}
			}
		}()
	}

	// Start error collector
	var searchErrors []error
	var errorsLock sync.Mutex
	go func() {
		for err := range errorsChan {
			errorsLock.Lock()
			searchErrors = append(searchErrors, err)
			errorsLock.Unlock()
		}
		close(done)
	}()

	// Collect files to search
	go func() {
		defer close(jobChan)
		defer close(errorsChan)

		// For each directory
		for _, dir := range options.Directories {
			// Walk directory
			walkFn := func(path string, info os.FileInfo, err error) error {
				if err != nil {
					errorsChan <- err
					return nil
				}

				// Skip directories
				if info.IsDir() {
					// Check max depth
					relPath, err := filepath.Rel(dir, path)
					if err != nil {
						errorsChan <- err
						return nil
					}
					depth := len(strings.Split(relPath, string(os.PathSeparator)))
					if depth > options.MaxDepth && path != dir {
						return filepath.SkipDir
					}
					return nil
				}

				// Check file patterns
				if len(options.FilePatterns) > 0 {
					matched := false
					for _, pattern := range options.FilePatterns {
						match, err := filepath.Match(pattern, filepath.Base(path))
						if err != nil {
							errorsChan <- err
							return nil
						}
						if match {
							matched = true
							break
						}
					}
					if !matched {
						return nil
					}
				}

				// Send file to workers
				jobChan <- path

				// Check if max results reached
				resultsLock.Lock()
				maxReached := len(results) >= options.MaxResults
				resultsLock.Unlock()

				if maxReached {
					return filepath.SkipAll
				}

				return nil
			}

			err := filepath.Walk(dir, walkFn)
			if err != nil {
				errorsChan <- err
			}
		}
	}()

	// Wait for workers to finish
	wg.Wait()
	<-done

	// Check for errors
	errorsLock.Lock()
	if len(searchErrors) > 0 {
		err = searchErrors[0] // Return the first error
	}
	errorsLock.Unlock()

	return results, err
}

// searchInFile searches for a pattern in a file
func searchInFile(filePath string, options SearchOptions, re *regexp.Regexp, results *[]SearchResult, resultsLock *sync.Mutex) {
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		logger.Error("Error opening file", "path", filePath, "error", err)
		return
	}
	defer file.Close()

	// Read file
	scanner := bufio.NewScanner(file)

	// Read all lines
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Error reading file", "path", filePath, "error", err)
		return
	}

	// For each line
	for i, line := range lines {
		// Check if this line matches the pattern
		var matches bool
		var matchStart, matchEnd int

		if options.IsRegex {
			indices := re.FindStringIndex(line)
			if indices != nil {
				matches = true
				matchStart = indices[0]
				matchEnd = indices[1]
			}
		} else {
			var lineToCheck string
			searchPattern := options.Pattern

			if !options.CaseSensitive {
				lineToCheck = strings.ToLower(line)
				searchPattern = strings.ToLower(searchPattern)
			} else {
				lineToCheck = line
			}

			matchStart = strings.Index(lineToCheck, searchPattern)
			if matchStart >= 0 {
				matches = true
				matchEnd = matchStart + len(searchPattern)
			}
		}

		if matches {
			// Get context lines
			var context []string
			contextStart := i - options.ContextLines
			if contextStart < 0 {
				contextStart = 0
			}
			contextEnd := i + options.ContextLines
			if contextEnd >= len(lines) {
				contextEnd = len(lines) - 1
			}
			for j := contextStart; j <= contextEnd; j++ {
				if j != i {
					context = append(context, lines[j])
				}
			}

			// Add result
			resultsLock.Lock()
			*results = append(*results, SearchResult{
				FilePath:   filePath,
				Line:       i + 1, // 1-based line number
				Content:    line,
				Context:    context,
				MatchStart: matchStart,
				MatchEnd:   matchEnd,
			})

			// Check if max results reached
			maxReached := len(*results) >= options.MaxResults
			resultsLock.Unlock()

			if maxReached {
				return
			}
		}
	}
}
