package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectScanTool combines file listing and reading in one efficient operation
type ProjectScanTool struct{}

// NewProjectScanTool creates a new project scan tool
func NewProjectScanTool() *ProjectScanTool {
	return &ProjectScanTool{}
}

// Name returns the tool name
func (t *ProjectScanTool) Name() string {
	return "projectScan"
}

// Description returns the tool description
func (t *ProjectScanTool) Description() string {
	return "Efficiently scans a project directory, listing all files and optionally reading their contents in a single operation. Scans everything by default - use pattern/excludePatterns to filter specific file types. Perfect for understanding project structure and codebase analysis."
}

// ParameterSchema returns the JSON schema for this tool's parameters
func (t *ProjectScanTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"dir": {
				Type:        "string",
				Description: "Directory path to scan (defaults to current directory if empty)",
			},
			"pattern": {
				Type:        "string",
				Description: "Optional glob pattern to include only specific files (e.g., '*.go', '*.{js,ts}'). Leave empty to scan all files.",
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
				Description: "Whether to read file contents (if false, only lists files)",
				Default:     true,
			},
			"maxFileSize": {
				Type:        "integer",
				Description: "Maximum size per file to read in bytes (default: 1MB for batch reading)",
				Default:     1024 * 1024,
			},
			"maxTotalSize": {
				Type:        "integer",
				Description: "Maximum total size of all files to read (default: 50MB)",
				Default:     50 * 1024 * 1024,
			},
			"excludePatterns": {
				Type: "array",
				Items: &JSONSchema{
					Type: "string",
				},
				Description: "Additional glob patterns to exclude beyond defaults. Defaults exclude: .git, node_modules, build dirs, temp files, etc.",
			},
			"includeMetadata": {
				Type:        "boolean",
				Description: "Whether to include file metadata (size, modified time)",
				Default:     true,
			},
		},
		Required: []string{},
	}
}

// ProjectScanResult represents the result of scanning a project
type ProjectScanResult struct {
	Directory    string              `json:"directory"`
	Files        []ProjectFileResult `json:"files"`
	TotalFiles   int                 `json:"total_files"`
	FilesRead    int                 `json:"files_read"`
	FilesSkipped int                 `json:"files_skipped"`
	TotalSize    int64               `json:"total_size"`
	ReadSize     int64               `json:"read_size"`
	Errors       []string            `json:"errors,omitempty"`
	Summary      ProjectSummary      `json:"summary"`
}

// ProjectFileResult represents information about a single file
type ProjectFileResult struct {
	Path         string                 `json:"path"`
	RelativePath string                 `json:"relative_path"`
	Content      string                 `json:"content,omitempty"`
	Size         int64                  `json:"size"`
	Modified     string                 `json:"modified,omitempty"`
	Extension    string                 `json:"extension"`
	WasRead      bool                   `json:"was_read"`
	SkipReason   string                 `json:"skip_reason,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ProjectSummary provides a high-level overview
type ProjectSummary struct {
	FilesByExtension   map[string]int `json:"files_by_extension"`
	LargestFiles       []string       `json:"largest_files"`
	RecentFiles        []string       `json:"recent_files"`
	MarkdownWithShell  []string       `json:"markdown_with_shell,omitempty"`
	ShellCommandCount  int            `json:"shell_command_count,omitempty"`
	ExecutableMarkdown int            `json:"executable_markdown_count,omitempty"`
}

// Execute scans the project directory
func (t *ProjectScanTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get directory path
	dir, _ := params["dir"].(string)
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Validate directory
	fileInfo, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("directory not found or accessible: %w", err)
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dir)
	}

	// Get parameters
	pattern, _ := params["pattern"].(string)
	includeHidden := getBoolParam(params, "includeHidden", false)
	readContents := getBoolParam(params, "readContents", true)
	includeMetadata := getBoolParam(params, "includeMetadata", true)

	maxDepth := getIntParam(params, "maxDepth", 0)
	maxFileSize := getInt64Param(params, "maxFileSize", 1024*1024)
	maxTotalSize := getInt64Param(params, "maxTotalSize", 50*1024*1024)

	// Get exclusion patterns with sensible defaults
	defaultExclusions := []string{
		".git", ".svn", ".hg", ".bzr", // Version control
		"node_modules", "vendor", ".vendor", // Dependencies
		"build", "dist", "target", ".build", // Build outputs
		"*.log", "*.tmp", "*.temp", // Temp files
		".DS_Store", "Thumbs.db", // OS files
		"*.pyc", "*.pyo", "__pycache__", // Python bytecode
		"*.class", "*.jar", // Java bytecode
		"*.o", "*.so", "*.dll", "*.exe", // Compiled binaries
	}

	excludePatterns := make([]string, len(defaultExclusions))
	copy(excludePatterns, defaultExclusions)

	// Add user-specified exclusions
	if val, ok := params["excludePatterns"]; ok {
		if patterns, ok := val.([]interface{}); ok {
			for _, p := range patterns {
				if str, ok := p.(string); ok {
					excludePatterns = append(excludePatterns, str)
				}
			}
		}
	}

	// Initialize result
	result := &ProjectScanResult{
		Directory: dir,
		Files:     make([]ProjectFileResult, 0),
		Summary: ProjectSummary{
			FilesByExtension:  make(map[string]int),
			LargestFiles:      make([]string, 0),
			RecentFiles:       make([]string, 0),
			MarkdownWithShell: make([]string, 0),
		},
	}

	// Scan directory
	err = t.scanDirectory(ctx, dir, dir, pattern, maxDepth, includeHidden, readContents,
		maxFileSize, maxTotalSize, excludePatterns, includeMetadata, result)

	if err != nil {
		return nil, fmt.Errorf("error scanning directory: %w", err)
	}

	// Finalize summary
	t.finalizeSummary(result)

	return result, nil
}

// scanDirectory recursively scans the directory
func (t *ProjectScanTool) scanDirectory(ctx context.Context, currentDir, rootDir, pattern string,
	maxDepth int, includeHidden, readContents bool, maxFileSize, maxTotalSize int64,
	excludePatterns []string, includeMetadata bool, result *ProjectScanResult) error {

	return filepath.Walk(currentDir, func(path string, info os.FileInfo, err error) error {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Error accessing %s: %v", path, err))
			return nil // Continue with other files
		}

		// Skip hidden files/directories if not requested
		if !includeHidden {
			name := filepath.Base(path)
			if name != "." && strings.HasPrefix(name, ".") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Check exclusion patterns
		baseName := filepath.Base(path)
		relPath, _ := filepath.Rel(rootDir, path)

		for _, pattern := range excludePatterns {
			// Check if pattern matches the base name or relative path
			matchedBase, _ := filepath.Match(pattern, baseName)
			matchedRel, _ := filepath.Match(pattern, relPath)

			// Also check if it's a directory name pattern (e.g., "node_modules")
			matchedDir := strings.Contains(relPath, pattern)

			if matchedBase || matchedRel || (info.IsDir() && matchedDir) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Skip directories in file results
		if info.IsDir() {
			return nil
		}

		// Check depth limit
		if maxDepth > 0 {
			relPath, err := filepath.Rel(rootDir, path)
			if err != nil {
				return err
			}
			depth := strings.Count(relPath, string(os.PathSeparator)) + 1
			if depth > maxDepth {
				return nil
			}
		}

		// Check pattern match
		if pattern != "" {
			matched, err := filepath.Match(pattern, filepath.Base(path))
			if err != nil {
				return err
			}
			if !matched {
				return nil
			}
		}

		// Process file
		relPath, _ = filepath.Rel(rootDir, path)
		ext := strings.ToLower(filepath.Ext(path))

		fileResult := ProjectFileResult{
			Path:         path,
			RelativePath: relPath,
			Size:         info.Size(),
			Extension:    ext,
		}

		// Include metadata if requested
		if includeMetadata {
			fileResult.Modified = info.ModTime().Format("2006-01-02 15:04:05")
		}

		// Update summary
		result.TotalFiles++
		result.TotalSize += info.Size()
		result.Summary.FilesByExtension[ext]++

		// Read content if requested and within limits
		if readContents {
			shouldRead, skipReason := t.shouldReadFile(info.Size(), maxFileSize, result.ReadSize, maxTotalSize)
			if shouldRead {
				content, err := os.ReadFile(path)
				if err != nil {
					fileResult.SkipReason = fmt.Sprintf("Read error: %v", err)
					result.FilesSkipped++
				} else {
					fileResult.Content = string(content)
					fileResult.WasRead = true
					result.FilesRead++
					result.ReadSize += info.Size()

					// Analyze content for special file types
					fileResult.Metadata = t.analyzeFileContent(string(content), ext, relPath)

					// Track markdown files with shell commands
					if strings.ToLower(ext) == ".md" || strings.ToLower(ext) == ".markdown" {
						if metadata := fileResult.Metadata; metadata != nil {
							if hasShell, ok := metadata["has_shell_commands"].(bool); ok && hasShell {
								result.Summary.MarkdownWithShell = append(result.Summary.MarkdownWithShell, relPath)
								result.Summary.ExecutableMarkdown++
								if shellCount, ok := metadata["shell_command_count"].(int); ok {
									result.Summary.ShellCommandCount += shellCount
								}
							}
						}
					}
				}
			} else {
				fileResult.SkipReason = skipReason
				result.FilesSkipped++
			}
		}

		result.Files = append(result.Files, fileResult)
		return nil
	})
}

// shouldReadFile determines if a file should be read based on size limits
func (t *ProjectScanTool) shouldReadFile(fileSize, maxFileSize, currentReadSize, maxTotalSize int64) (bool, string) {
	if fileSize > maxFileSize {
		return false, fmt.Sprintf("File too large (%d bytes > %d limit)", fileSize, maxFileSize)
	}
	if currentReadSize+fileSize > maxTotalSize {
		return false, fmt.Sprintf("Would exceed total read limit (%d + %d > %d)", currentReadSize, fileSize, maxTotalSize)
	}
	return true, ""
}

// finalizeSummary completes the summary with top files
func (t *ProjectScanTool) finalizeSummary(result *ProjectScanResult) {
	// Find largest files (top 5)
	type fileSize struct {
		path string
		size int64
	}

	var files []fileSize
	for _, f := range result.Files {
		files = append(files, fileSize{f.RelativePath, f.Size})
	}

	// Sort by size (simple bubble sort for small lists)
	for i := 0; i < len(files) && i < 5; i++ {
		maxIdx := i
		for j := i + 1; j < len(files); j++ {
			if files[j].size > files[maxIdx].size {
				maxIdx = j
			}
		}
		if maxIdx != i {
			files[i], files[maxIdx] = files[maxIdx], files[i]
		}
		result.Summary.LargestFiles = append(result.Summary.LargestFiles,
			fmt.Sprintf("%s (%d bytes)", files[i].path, files[i].size))
	}
}

// Helper functions for parameter extraction
func getBoolParam(params map[string]interface{}, key string, defaultVal bool) bool {
	if val, ok := params[key].(bool); ok {
		return val
	}
	return defaultVal
}

func getIntParam(params map[string]interface{}, key string, defaultVal int) int {
	if val, ok := params[key].(float64); ok {
		return int(val)
	}
	if val, ok := params[key].(int); ok {
		return val
	}
	return defaultVal
}

func getInt64Param(params map[string]interface{}, key string, defaultVal int64) int64 {
	if val, ok := params[key].(float64); ok {
		return int64(val)
	}
	if val, ok := params[key].(int); ok {
		return int64(val)
	}
	return defaultVal
}

// analyzeFileContent analyzes file content based on file type and extension
func (t *ProjectScanTool) analyzeFileContent(content, extension, filePath string) map[string]interface{} {
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

	// Check if any file contains shell-like commands in comments or strings
	if t.containsShellPatterns(content) {
		metadata["contains_shell_patterns"] = true
	}

	return metadata
}

// containsShellPatterns checks if content contains shell command patterns
func (t *ProjectScanTool) containsShellPatterns(content string) bool {
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
