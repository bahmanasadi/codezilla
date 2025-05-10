package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// FileWriteTool allows writing content to a file
type FileWriteTool struct{}

// NewFileWriteTool creates a new file write tool
func NewFileWriteTool() *FileWriteTool {
	return &FileWriteTool{}
}

// Name returns the tool name
func (t *FileWriteTool) Name() string {
	return "fileWrite"
}

// Description returns the tool description
func (t *FileWriteTool) Description() string {
	return "Writes content to a file on the local filesystem"
}

// ParameterSchema returns the JSON schema for this tool's parameters
func (t *FileWriteTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"file_path": {
				Type:        "string",
				Description: "The path to the file to write",
			},
			"content": {
				Type:        "string",
				Description: "The content to write to the file",
			},
			"append": {
				Type:        "boolean",
				Description: "Whether to append to the file instead of overwriting it",
				Default:     false,
			},
		},
		Required: []string{"file_path", "content"},
	}
}

// Execute writes the content to the specified file
func (t *FileWriteTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Validate parameters
	if err := ValidateToolParams(t, params); err != nil {
		return nil, err
	}

	// Extract parameters
	filePath, ok := params["file_path"].(string)
	if !ok {
		return nil, &ErrInvalidToolParams{
			ToolName: t.Name(),
			Message:  "file_path must be a string",
		}
	}

	content, ok := params["content"].(string)
	if !ok {
		return nil, &ErrInvalidToolParams{
			ToolName: t.Name(),
			Message:  "content must be a string",
		}
	}

	// Default append to false
	append := false
	if appendParam, ok := params["append"].(bool); ok {
		append = appendParam
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

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, &ErrToolExecution{
			ToolName: t.Name(),
			Message:  fmt.Sprintf("failed to create directory: %s", dir),
			Err:      err,
		}
	}

	// Determine flags based on append mode
	flag := os.O_WRONLY | os.O_CREATE
	if append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	// Open file
	file, err := os.OpenFile(filePath, flag, 0644)
	if err != nil {
		return nil, &ErrToolExecution{
			ToolName: t.Name(),
			Message:  fmt.Sprintf("failed to open file for writing: %s", filePath),
			Err:      err,
		}
	}
	defer file.Close()

	// Write content
	_, err = file.WriteString(content)
	if err != nil {
		return nil, &ErrToolExecution{
			ToolName: t.Name(),
			Message:  fmt.Sprintf("failed to write to file: %s", filePath),
			Err:      err,
		}
	}

	return map[string]interface{}{
		"success":   true,
		"file_path": filePath,
		"bytes":     len(content),
		"appended":  append,
	}, nil
}
