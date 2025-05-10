package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// FileReadTool allows reading file contents
type FileReadTool struct{}

// NewFileReadTool creates a new file read tool
func NewFileReadTool() *FileReadTool {
	return &FileReadTool{}
}

// Name returns the tool name
func (t *FileReadTool) Name() string {
	return "fileRead"
}

// Description returns the tool description
func (t *FileReadTool) Description() string {
	return "Reads a file from the local filesystem and returns its contents as a string"
}

// ParameterSchema returns the JSON schema for this tool's parameters
func (t *FileReadTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"file_path": {
				Type:        "string",
				Description: "The path to the file to read",
			},
		},
		Required: []string{"file_path"},
	}
}

// Execute reads the file and returns its contents
func (t *FileReadTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Validate parameters
	if err := ValidateToolParams(t, params); err != nil {
		return nil, err
	}

	// Extract file path
	filePath, ok := params["file_path"].(string)
	if !ok {
		return nil, &ErrInvalidToolParams{
			ToolName: t.Name(),
			Message:  "file_path must be a string",
		}
	}

	// Expand ~ to home directory
	if len(filePath) > 0 && filePath[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, &ErrToolExecution{
				ToolName: t.Name(),
				Message:  "failed to expand home directory",
				Err:      err,
			}
		}
		filePath = filepath.Join(homeDir, filePath[1:])
	}

	// Make sure the file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, &ErrToolExecution{
			ToolName: t.Name(),
			Message:  fmt.Sprintf("failed to access file: %s", filePath),
			Err:      err,
		}
	}

	// Make sure it's a regular file (not a directory)
	if fileInfo.IsDir() {
		return nil, &ErrToolExecution{
			ToolName: t.Name(),
			Message:  fmt.Sprintf("path is a directory, not a file: %s", filePath),
		}
	}

	// Check file size to prevent loading very large files
	if fileInfo.Size() > 10*1024*1024 { // 10MB limit
		return nil, &ErrToolExecution{
			ToolName: t.Name(),
			Message:  fmt.Sprintf("file too large (>10MB): %s", filePath),
		}
	}

	// Read file contents
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, &ErrToolExecution{
			ToolName: t.Name(),
			Message:  fmt.Sprintf("failed to read file: %s", filePath),
			Err:      err,
		}
	}

	return string(content), nil
}
