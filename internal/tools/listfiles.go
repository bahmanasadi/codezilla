package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ListFilesTool recursively finds files in a directory, with optional pattern matching
type ListFilesTool struct{}

// NewListFilesTool creates a new ListFilesTool
func NewListFilesTool() *ListFilesTool {
	return &ListFilesTool{}
}

// Name returns the tool name
func (t *ListFilesTool) Name() string {
	return "listFiles"
}

// Description returns the tool description
func (t *ListFilesTool) Description() string {
	return "Recursively lists all files in a directory with optional pattern matching"
}

// ParameterSchema returns the JSON schema for this tool's parameters
func (t *ListFilesTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"dir": {
				Type:        "string",
				Description: "Directory path to search (defaults to current directory if empty)",
			},
			"pattern": {
				Type:        "string",
				Description: "Optional glob pattern to filter files (e.g., '*.go', '*.txt')",
			},
			"maxDepth": {
				Type:        "integer",
				Description: "Maximum recursion depth (0 for unlimited)",
				Default:     0,
			},
			"includeHidden": {
				Type:        "boolean",
				Description: "Whether to include hidden files and directories",
				Default:     false,
			},
			"readContents": {
				Type:        "boolean",
				Description: "Whether to also read file contents (use projectScan for better performance with many files)",
				Default:     false,
			},
			"maxFileSize": {
				Type:        "integer",
				Description: "Maximum file size to read when readContents is true (default: 1MB)",
				Default:     1024 * 1024,
			},
		},
		Required: []string{},
	}
}

// Execute recursively lists files in the specified directory
func (t *ListFilesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get directory path
	dir, _ := params["dir"].(string)
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Validate and clean the path
	dir, err := ValidateAndCleanPath(dir)
	if err != nil {
		return nil, &ErrToolExecution{
			ToolName: t.Name(),
			Message:  "invalid directory path",
			Err:      err,
		}
	}

	// Make sure the directory exists
	fileInfo, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("directory not found or accessible: %w", err)
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dir)
	}

	// Get optional parameters
	pattern, _ := params["pattern"].(string)
	includeHidden := false
	if val, ok := params["includeHidden"].(bool); ok {
		includeHidden = val
	}

	readContents := false
	if val, ok := params["readContents"].(bool); ok {
		readContents = val
	}

	maxDepth := 0
	if val, ok := params["maxDepth"].(float64); ok {
		maxDepth = int(val)
	} else if val, ok := params["maxDepth"].(int); ok {
		maxDepth = val
	}

	maxFileSize := int64(1024 * 1024) // 1MB default
	if val, ok := params["maxFileSize"].(float64); ok {
		maxFileSize = int64(val)
	} else if val, ok := params["maxFileSize"].(int); ok {
		maxFileSize = int64(val)
	}

	// Find files recursively
	files, err := findFiles(dir, pattern, maxDepth, includeHidden)
	if err != nil {
		return nil, fmt.Errorf("error listing files: %w", err)
	}

	// If reading contents is requested, enhance the result
	if readContents {
		return t.createEnhancedResult(ctx, dir, files, maxFileSize)
	}

	// Return the simple result
	result := map[string]interface{}{
		"directory": dir,
		"files":     files,
		"count":     len(files),
	}

	return result, nil
}

// findFiles recursively finds files in a directory with pattern matching
func findFiles(root, pattern string, maxDepth int, includeHidden bool) ([]string, error) {
	var files []string

	// Walk the directory tree
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		// Check for context cancellation or errors
		if err != nil {
			return err
		}

		// Check if we should include hidden files/directories
		if !includeHidden {
			// Skip hidden files and directories (those starting with a dot)
			name := filepath.Base(path)
			if name != "." && strings.HasPrefix(name, ".") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Skip directories and add only files
		if !info.IsDir() {
			// Check max depth
			if maxDepth > 0 {
				// Calculate depth relative to root
				relPath, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}

				// Count directory separators to determine depth
				depth := strings.Count(relPath, string(os.PathSeparator)) + 1
				if depth > maxDepth {
					return nil
				}
			}

			// If pattern is provided, match it
			if pattern != "" {
				matched, err := filepath.Match(pattern, filepath.Base(path))
				if err != nil {
					return err
				}
				if !matched {
					return nil
				}
			}

			// Add file to the result list
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// EnhancedFileResult represents a file with optional content
type EnhancedFileResult struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
	WasRead bool   `json:"was_read"`
}

// createEnhancedResult creates a result that includes file contents
func (t *ListFilesTool) createEnhancedResult(ctx context.Context, dir string, filePaths []string, maxFileSize int64) (interface{}, error) {
	enhancedFiles := make([]EnhancedFileResult, 0, len(filePaths))
	var totalSize int64
	readCount := 0
	skipCount := 0

	for _, filePath := range filePaths {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		fileResult := EnhancedFileResult{
			Path: filePath,
		}

		// Get file info
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			fileResult.Error = fmt.Sprintf("Failed to stat file: %v", err)
			enhancedFiles = append(enhancedFiles, fileResult)
			skipCount++
			continue
		}

		fileResult.Size = fileInfo.Size()
		totalSize += fileInfo.Size()

		// Check if file is too large
		if fileInfo.Size() > maxFileSize {
			fileResult.Error = fmt.Sprintf("File too large (%d bytes > %d limit)", fileInfo.Size(), maxFileSize)
			enhancedFiles = append(enhancedFiles, fileResult)
			skipCount++
			continue
		}

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			fileResult.Error = fmt.Sprintf("Failed to read file: %v", err)
			skipCount++
		} else {
			fileResult.Content = string(content)
			fileResult.WasRead = true
			readCount++
		}

		enhancedFiles = append(enhancedFiles, fileResult)
	}

	// Return enhanced result
	result := map[string]interface{}{
		"directory":     dir,
		"files":         enhancedFiles,
		"total_files":   len(filePaths),
		"files_read":    readCount,
		"files_skipped": skipCount,
		"total_size":    totalSize,
	}

	return result, nil
}
