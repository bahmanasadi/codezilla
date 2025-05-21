package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileReadBatchTool allows reading multiple files in a single operation
type FileReadBatchTool struct{}

// NewFileReadBatchTool creates a new batch file read tool
func NewFileReadBatchTool() *FileReadBatchTool {
	return &FileReadBatchTool{}
}

// Name returns the tool name
func (t *FileReadBatchTool) Name() string {
	return "fileReadBatch"
}

// Description returns the tool description
func (t *FileReadBatchTool) Description() string {
	return "Reads multiple files from the local filesystem at once and returns their contents as a structured result. More efficient than reading files individually."
}

// ParameterSchema returns the JSON schema for this tool's parameters
func (t *FileReadBatchTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"file_paths": {
				Type: "array",
				Items: &JSONSchema{
					Type: "string",
				},
				Description: "Array of file paths to read",
			},
			"max_file_size": {
				Type:        "integer",
				Description: "Maximum size per file in bytes (default: 10MB)",
				Default:     10 * 1024 * 1024,
			},
			"continue_on_error": {
				Type:        "boolean",
				Description: "Whether to continue reading other files if one fails (default: true)",
				Default:     true,
			},
			"include_metadata": {
				Type:        "boolean",
				Description: "Whether to include file metadata (size, modified time) in results",
				Default:     false,
			},
		},
		Required: []string{"file_paths"},
	}
}

// FileReadResult represents the result of reading a single file
type FileReadResult struct {
	Path     string                 `json:"path"`
	Content  string                 `json:"content,omitempty"`
	Size     int64                  `json:"size,omitempty"`
	Modified string                 `json:"modified,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Success  bool                   `json:"success"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// BatchReadResult represents the overall result of the batch operation
type BatchReadResult struct {
	Files        []FileReadResult `json:"files"`
	TotalFiles   int              `json:"total_files"`
	SuccessCount int              `json:"success_count"`
	ErrorCount   int              `json:"error_count"`
	TotalSize    int64            `json:"total_size"`
}

// Execute reads multiple files and returns their contents
func (t *FileReadBatchTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Validate parameters
	if err := ValidateToolParams(t, params); err != nil {
		return nil, err
	}

	// Extract file paths
	filePathsParam, ok := params["file_paths"]
	if !ok {
		return nil, &ErrInvalidToolParams{
			ToolName: t.Name(),
			Message:  "file_paths parameter is required",
		}
	}

	// Convert to string slice
	var filePaths []string
	switch v := filePathsParam.(type) {
	case []interface{}:
		filePaths = make([]string, len(v))
		for i, path := range v {
			if str, ok := path.(string); ok {
				filePaths[i] = str
			} else {
				return nil, &ErrInvalidToolParams{
					ToolName: t.Name(),
					Message:  fmt.Sprintf("file path at index %d is not a string", i),
				}
			}
		}
	case []string:
		filePaths = v
	default:
		return nil, &ErrInvalidToolParams{
			ToolName: t.Name(),
			Message:  "file_paths must be an array of strings",
		}
	}

	if len(filePaths) == 0 {
		return nil, &ErrInvalidToolParams{
			ToolName: t.Name(),
			Message:  "at least one file path must be provided",
		}
	}

	// Get optional parameters
	maxFileSize := int64(10 * 1024 * 1024) // 10MB default
	if val, ok := params["max_file_size"]; ok {
		if size, ok := val.(float64); ok {
			maxFileSize = int64(size)
		} else if size, ok := val.(int); ok {
			maxFileSize = int64(size)
		}
	}

	continueOnError := true
	if val, ok := params["continue_on_error"].(bool); ok {
		continueOnError = val
	}

	includeMetadata := false
	if val, ok := params["include_metadata"].(bool); ok {
		includeMetadata = val
	}

	// Process files
	result := &BatchReadResult{
		Files:      make([]FileReadResult, 0, len(filePaths)),
		TotalFiles: len(filePaths),
	}

	for _, filePath := range filePaths {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		fileResult := t.readSingleFile(filePath, maxFileSize, includeMetadata)
		result.Files = append(result.Files, fileResult)

		if fileResult.Success {
			result.SuccessCount++
			result.TotalSize += fileResult.Size
		} else {
			result.ErrorCount++
			if !continueOnError {
				break
			}
		}
	}

	return result, nil
}

// readSingleFile reads a single file and returns its result
func (t *FileReadBatchTool) readSingleFile(filePath string, maxFileSize int64, includeMetadata bool) FileReadResult {
	result := FileReadResult{
		Path: filePath,
	}

	// Expand ~ to home directory
	expandedPath := filePath
	if len(filePath) > 0 && filePath[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			result.Error = fmt.Sprintf("failed to expand home directory: %v", err)
			return result
		}
		expandedPath = filepath.Join(homeDir, filePath[1:])
	}

	// Get file info
	fileInfo, err := os.Stat(expandedPath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to access file: %v", err)
		return result
	}

	// Check if it's a regular file
	if fileInfo.IsDir() {
		result.Error = "path is a directory, not a file"
		return result
	}

	// Include metadata if requested
	if includeMetadata {
		result.Size = fileInfo.Size()
		result.Modified = fileInfo.ModTime().Format("2006-01-02 15:04:05")
	} else {
		result.Size = fileInfo.Size()
	}

	// Check file size
	if fileInfo.Size() > maxFileSize {
		result.Error = fmt.Sprintf("file too large (%d bytes > %d bytes limit)", fileInfo.Size(), maxFileSize)
		return result
	}

	// Read file content
	content, err := os.ReadFile(expandedPath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to read file: %v", err)
		return result
	}

	result.Content = string(content)
	result.Success = true

	// Analyze content for special file types
	ext := strings.ToLower(filepath.Ext(expandedPath))
	result.Metadata = t.analyzeContent(string(content), ext, expandedPath)

	return result
}

// analyzeContent analyzes file content based on file type and extension
func (t *FileReadBatchTool) analyzeContent(content, extension, filePath string) map[string]interface{} {
	switch strings.ToLower(extension) {
	case ".md", ".markdown":
		return GetMarkdownMetadata(content, filePath)
	case ".readme", ".txt":
		// Check if text files contain markdown-like content
		if strings.Contains(content, "```") || strings.Contains(content, "    ") {
			return GetMarkdownMetadata(content, filePath)
		}
	}

	// For other file types, return basic metadata
	lines := strings.Split(content, "\n")
	metadata := map[string]interface{}{
		"line_count": len(lines),
		"char_count": len(content),
	}

	// Check if any file contains shell-like commands
	if t.containsShellPatterns(content) {
		metadata["contains_shell_patterns"] = true
	}

	return metadata
}

// containsShellPatterns checks if content contains shell command patterns
func (t *FileReadBatchTool) containsShellPatterns(content string) bool {
	shellIndicators := []string{
		"#!/bin/bash", "#!/bin/sh", "#!/usr/bin/env bash",
		"$ ", "# ", "sudo ", "docker ", "git ", "npm ", "yarn ",
	}

	contentLower := strings.ToLower(content)
	for _, indicator := range shellIndicators {
		if strings.Contains(contentLower, strings.ToLower(indicator)) {
			return true
		}
	}

	return false
}
