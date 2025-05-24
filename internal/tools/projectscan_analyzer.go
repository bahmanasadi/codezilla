package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ProjectScanAnalyzerTool scans and analyzes files individually with fresh context
type ProjectScanAnalyzerTool struct {
	analyzer         FileAnalyzer
	progressReporter ProgressReporter
}

// FileAnalyzer interface for analyzing individual files
type FileAnalyzer interface {
	AnalyzeFile(ctx context.Context, filePath string, content string, userQuery string) (FileAnalysis, error)
}

// FileAnalysis represents the analysis result for a single file
type FileAnalysis struct {
	FilePath     string                 `json:"file_path"`
	Summary      string                 `json:"summary"`
	Relevance    float64                `json:"relevance"` // 0-1 score
	KeyFindings  []string               `json:"key_findings,omitempty"`
	CodeIssues   []CodeIssue            `json:"code_issues,omitempty"`
	Suggestions  []string               `json:"suggestions,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	AnalysisTime time.Duration          `json:"analysis_time_ms"`
}

// CodeIssue represents a potential issue found in the code
type CodeIssue struct {
	Type        string `json:"type"`     // bug, security, performance, style
	Severity    string `json:"severity"` // critical, high, medium, low
	Line        int    `json:"line,omitempty"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// NewProjectScanAnalyzerTool creates a new project scan analyzer tool
func NewProjectScanAnalyzerTool(analyzer FileAnalyzer) *ProjectScanAnalyzerTool {
	if analyzer == nil {
		analyzer = NewDefaultFileAnalyzer()
	}
	return &ProjectScanAnalyzerTool{
		analyzer:         analyzer,
		progressReporter: &NullProgressReporter{},
	}
}

// SetProgressReporter sets the progress reporter for the tool
func (t *ProjectScanAnalyzerTool) SetProgressReporter(reporter ProgressReporter) {
	if reporter != nil {
		t.progressReporter = reporter
	}
}

// Name returns the tool name
func (t *ProjectScanAnalyzerTool) Name() string {
	return "projectScanAnalyzer"
}

// Description returns the tool description
func (t *ProjectScanAnalyzerTool) Description() string {
	return "Scans a project directory and analyzes each file individually with fresh context using the user's query. Perfect for code review, security analysis, or finding specific patterns across a codebase."
}

// ParameterSchema returns the JSON schema for this tool's parameters
func (t *ProjectScanAnalyzerTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"dir": {
				Type:        "string",
				Description: "Directory path to scan (defaults to current directory if empty)",
			},
			"userQuery": {
				Type:        "string",
				Description: "The user's query or analysis criteria to apply to each file",
			},
			"pattern": {
				Type:        "string",
				Description: "Optional glob pattern to include only specific files (e.g., '*.go', '*.{js,ts}')",
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
			"maxFileSize": {
				Type:        "integer",
				Description: "Maximum size per file to analyze in bytes (default: 1MB)",
				Default:     1024 * 1024,
			},
			"excludePatterns": {
				Type: "array",
				Items: &JSONSchema{
					Type: "string",
				},
				Description: "Additional glob patterns to exclude beyond defaults",
			},
			"concurrency": {
				Type:        "integer",
				Description: "Number of files to analyze concurrently (default: 5)",
				Default:     5,
			},
			"relevanceThreshold": {
				Type:        "number",
				Description: "Minimum relevance score (0-1) to include in results (default: 0.3)",
				Default:     0.3,
			},
			"analysisTimeout": {
				Type:        "integer",
				Description: "Timeout per file analysis in seconds (default: 30)",
				Default:     30,
			},
			"showProgress": {
				Type:        "boolean",
				Description: "Show progress for each file being analyzed (default: true)",
				Default:     true,
			},
		},
		Required: []string{"userQuery"},
	}
}

// ProjectScanAnalysisResult represents the complete analysis result
type ProjectScanAnalysisResult struct {
	Directory         string                 `json:"directory"`
	UserQuery         string                 `json:"user_query"`
	AnalyzedFiles     []FileAnalysis         `json:"analyzed_files"`
	SkippedFiles      []SkippedFile          `json:"skipped_files,omitempty"`
	Summary           ProjectAnalysisSummary `json:"summary"`
	TotalFiles        int                    `json:"total_files"`
	FilesAnalyzed     int                    `json:"files_analyzed"`
	FilesSkipped      int                    `json:"files_skipped"`
	TotalAnalysisTime time.Duration          `json:"total_analysis_time_ms"`
	Errors            []string               `json:"errors,omitempty"`
}

// SkippedFile represents a file that was skipped during analysis
type SkippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ProjectAnalysisSummary provides high-level analysis insights
type ProjectAnalysisSummary struct {
	TopRelevantFiles   []string            `json:"top_relevant_files"`
	CommonIssues       map[string]int      `json:"common_issues"`
	FilesByRelevance   map[string][]string `json:"files_by_relevance"` // high, medium, low
	OverallFindings    []string            `json:"overall_findings"`
	RecommendedActions []string            `json:"recommended_actions"`
}

// Execute performs the file-by-file analysis
func (t *ProjectScanAnalyzerTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	startTime := time.Now()

	// Get parameters
	dir, _ := params["dir"].(string)
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	userQuery, ok := params["userQuery"].(string)
	if !ok || userQuery == "" {
		return nil, fmt.Errorf("userQuery is required")
	}

	// Check if we should enable progress reporting
	enableProgress := getBoolParam(params, "showProgress", true)
	if enableProgress {
		// Create a terminal progress reporter that prints to stderr
		progressReporter := NewTerminalProgressReporter(func(format string, args ...interface{}) {
			fmt.Fprintf(os.Stderr, format, args...)
		})
		t.progressReporter = progressReporter
	} else {
		t.progressReporter = &NullProgressReporter{}
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

	// Get other parameters
	pattern, _ := params["pattern"].(string)
	includeHidden := getBoolParam(params, "includeHidden", false)
	maxDepth := getIntParam(params, "maxDepth", 0)
	maxFileSize := getInt64Param(params, "maxFileSize", 1024*1024)
	concurrency := getIntParam(params, "concurrency", 5)
	relevanceThreshold := getFloatParam(params, "relevanceThreshold", 0.3)
	analysisTimeout := getIntParam(params, "analysisTimeout", 30)

	// Get exclusion patterns
	excludePatterns := getDefaultExclusions()
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
	result := &ProjectScanAnalysisResult{
		Directory:     dir,
		UserQuery:     userQuery,
		AnalyzedFiles: make([]FileAnalysis, 0),
		SkippedFiles:  make([]SkippedFile, 0),
		Summary: ProjectAnalysisSummary{
			CommonIssues:     make(map[string]int),
			FilesByRelevance: make(map[string][]string),
		},
	}

	// Collect files to analyze
	files, err := t.collectFiles(ctx, dir, pattern, maxDepth, includeHidden, maxFileSize, excludePatterns)
	if err != nil {
		return nil, fmt.Errorf("error collecting files: %w", err)
	}

	result.TotalFiles = len(files)

	// Analyze files concurrently
	err = t.analyzeFiles(ctx, files, userQuery, concurrency, relevanceThreshold,
		time.Duration(analysisTimeout)*time.Second, result)

	if err != nil && err != context.Canceled {
		result.Errors = append(result.Errors, fmt.Sprintf("Analysis error: %v", err))
	}

	// Generate summary
	t.generateSummary(result)

	result.TotalAnalysisTime = time.Since(startTime)

	return result, nil
}

// collectFiles gathers all files to be analyzed
func (t *ProjectScanAnalyzerTool) collectFiles(ctx context.Context, rootDir, pattern string,
	maxDepth int, includeHidden bool, maxFileSize int64, excludePatterns []string) ([]string, error) {

	var files []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return nil // Skip files with errors
		}

		// Skip directories
		if info.IsDir() {
			// Check if should skip this directory
			if !includeHidden && strings.HasPrefix(filepath.Base(path), ".") && path != rootDir {
				return filepath.SkipDir
			}

			// Check exclusion patterns
			for _, excl := range excludePatterns {
				if matched, _ := filepath.Match(excl, filepath.Base(path)); matched {
					return filepath.SkipDir
				}
			}

			return nil
		}

		// Check file criteria
		if !t.shouldAnalyzeFile(path, info, rootDir, pattern, maxDepth, includeHidden,
			maxFileSize, excludePatterns) {
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
}

// shouldAnalyzeFile checks if a file should be analyzed
func (t *ProjectScanAnalyzerTool) shouldAnalyzeFile(path string, info os.FileInfo, rootDir, pattern string,
	maxDepth int, includeHidden bool, maxFileSize int64, excludePatterns []string) bool {

	// Check hidden files
	if !includeHidden && strings.HasPrefix(filepath.Base(path), ".") {
		return false
	}

	// Check size
	if info.Size() > maxFileSize {
		return false
	}

	// Check pattern
	if pattern != "" {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); !matched {
			return false
		}
	}

	// Check depth
	if maxDepth > 0 {
		relPath, _ := filepath.Rel(rootDir, path)
		depth := strings.Count(relPath, string(os.PathSeparator))
		if depth > maxDepth {
			return false
		}
	}

	// Check exclusions
	baseName := filepath.Base(path)
	relPath, _ := filepath.Rel(rootDir, path)

	for _, excl := range excludePatterns {
		if matched, _ := filepath.Match(excl, baseName); matched {
			return false
		}
		if matched, _ := filepath.Match(excl, relPath); matched {
			return false
		}
	}

	return true
}

// analyzeFiles performs concurrent analysis of files
func (t *ProjectScanAnalyzerTool) analyzeFiles(ctx context.Context, files []string, userQuery string,
	concurrency int, relevanceThreshold float64, timeout time.Duration, result *ProjectScanAnalysisResult) error {

	// Create semaphore for concurrency control
	sem := make(chan struct{}, concurrency)

	// Results channel
	type fileResult struct {
		filePath  string
		fileIndex int
		analysis  FileAnalysis
		skipped   *SkippedFile
		err       error
	}

	results := make(chan fileResult, len(files))

	// WaitGroup for goroutines
	var wg sync.WaitGroup

	// Process files
	for idx, filePath := range files {
		wg.Add(1)
		go func(path string, index int) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- fileResult{filePath: path, fileIndex: index, err: ctx.Err()}
				return
			}

			// Report file start
			t.progressReporter.OnFileStart(path, index+1, len(files))

			// Create timeout context for this file
			fileCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Read file content
			content, err := os.ReadFile(path)
			if err != nil {
				t.progressReporter.OnError(path, err)
				results <- fileResult{
					filePath:  path,
					fileIndex: index,
					skipped: &SkippedFile{
						Path:   path,
						Reason: fmt.Sprintf("Read error: %v", err),
					},
				}
				return
			}

			// Report file read
			t.progressReporter.OnFileRead(path, len(content))

			// Report analysis start
			t.progressReporter.OnAnalysisStart(path)

			// Analyze file
			startTime := time.Now()
			analysis, err := t.analyzer.AnalyzeFile(fileCtx, path, string(content), userQuery)
			analysis.AnalysisTime = time.Since(startTime)

			if err != nil {
				t.progressReporter.OnError(path, err)
				results <- fileResult{
					filePath:  path,
					fileIndex: index,
					skipped: &SkippedFile{
						Path:   path,
						Reason: fmt.Sprintf("Analysis error: %v", err),
					},
				}
				return
			}

			// Check relevance threshold
			if analysis.Relevance < relevanceThreshold {
				reason := fmt.Sprintf("Below relevance threshold (%.2f < %.2f)",
					analysis.Relevance, relevanceThreshold)
				t.progressReporter.OnFileSkipped(path, reason)
				results <- fileResult{
					filePath:  path,
					fileIndex: index,
					skipped: &SkippedFile{
						Path:   path,
						Reason: reason,
					},
				}
				return
			}

			// Report analysis complete
			t.progressReporter.OnAnalysisComplete(path, analysis, analysis.AnalysisTime)

			results <- fileResult{
				filePath:  path,
				fileIndex: index,
				analysis:  analysis,
			}
		}(filePath, idx)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for res := range results {
		if res.err != nil {
			return res.err
		}

		if res.skipped != nil {
			result.SkippedFiles = append(result.SkippedFiles, *res.skipped)
			result.FilesSkipped++
		} else {
			result.AnalyzedFiles = append(result.AnalyzedFiles, res.analysis)
			result.FilesAnalyzed++

			// Update common issues
			for _, issue := range res.analysis.CodeIssues {
				result.Summary.CommonIssues[issue.Type]++
			}
		}
	}

	return nil
}

// generateSummary creates the analysis summary
func (t *ProjectScanAnalyzerTool) generateSummary(result *ProjectScanAnalysisResult) {
	// Sort files by relevance
	type relevantFile struct {
		path      string
		relevance float64
	}

	var sortedFiles []relevantFile
	for _, analysis := range result.AnalyzedFiles {
		sortedFiles = append(sortedFiles, relevantFile{
			path:      analysis.FilePath,
			relevance: analysis.Relevance,
		})
	}

	// Simple bubble sort for top files
	for i := 0; i < len(sortedFiles); i++ {
		for j := i + 1; j < len(sortedFiles); j++ {
			if sortedFiles[j].relevance > sortedFiles[i].relevance {
				sortedFiles[i], sortedFiles[j] = sortedFiles[j], sortedFiles[i]
			}
		}
	}

	// Get top 10 relevant files
	for i := 0; i < len(sortedFiles) && i < 10; i++ {
		result.Summary.TopRelevantFiles = append(result.Summary.TopRelevantFiles,
			sortedFiles[i].path)
	}

	// Categorize files by relevance
	for _, analysis := range result.AnalyzedFiles {
		category := "low"
		if analysis.Relevance >= 0.7 {
			category = "high"
		} else if analysis.Relevance >= 0.5 {
			category = "medium"
		}

		result.Summary.FilesByRelevance[category] = append(
			result.Summary.FilesByRelevance[category], analysis.FilePath)
	}

	// Generate overall findings
	if result.FilesAnalyzed > 0 {
		result.Summary.OverallFindings = append(result.Summary.OverallFindings,
			fmt.Sprintf("Analyzed %d files out of %d total files",
				result.FilesAnalyzed, result.TotalFiles))

		if len(result.Summary.CommonIssues) > 0 {
			for issueType, count := range result.Summary.CommonIssues {
				result.Summary.OverallFindings = append(result.Summary.OverallFindings,
					fmt.Sprintf("Found %d %s issues", count, issueType))
			}
		}

		if len(result.Summary.TopRelevantFiles) > 0 {
			result.Summary.OverallFindings = append(result.Summary.OverallFindings,
				fmt.Sprintf("Identified %d highly relevant files",
					len(result.Summary.TopRelevantFiles)))
		}
	}
}

// getDefaultExclusions returns default exclusion patterns
func getDefaultExclusions() []string {
	return []string{
		".git", ".svn", ".hg", ".bzr",
		"node_modules", "vendor", ".vendor",
		"build", "dist", "target", ".build",
		"*.log", "*.tmp", "*.temp",
		".DS_Store", "Thumbs.db",
		"*.pyc", "*.pyo", "__pycache__",
		"*.class", "*.jar",
		"*.o", "*.so", "*.dll", "*.exe",
	}
}

// getFloatParam extracts float parameter with default
func getFloatParam(params map[string]interface{}, key string, defaultVal float64) float64 {
	if val, ok := params[key].(float64); ok {
		return val
	}
	return defaultVal
}

// DefaultFileAnalyzer provides basic file analysis
type DefaultFileAnalyzer struct{}

// NewDefaultFileAnalyzer creates a default analyzer
func NewDefaultFileAnalyzer() *DefaultFileAnalyzer {
	return &DefaultFileAnalyzer{}
}

// AnalyzeFile performs basic analysis based on content patterns
func (a *DefaultFileAnalyzer) AnalyzeFile(ctx context.Context, filePath string, content string, userQuery string) (FileAnalysis, error) {
	analysis := FileAnalysis{
		FilePath:    filePath,
		KeyFindings: make([]string, 0),
		CodeIssues:  make([]CodeIssue, 0),
		Suggestions: make([]string, 0),
		Metadata:    make(map[string]interface{}),
	}

	// Basic relevance scoring based on query keywords
	queryLower := strings.ToLower(userQuery)
	contentLower := strings.ToLower(content)

	// Count keyword matches
	keywords := strings.Fields(queryLower)
	matchCount := 0
	for _, keyword := range keywords {
		if strings.Contains(contentLower, keyword) {
			matchCount++
		}
	}

	// Calculate relevance
	if len(keywords) > 0 {
		analysis.Relevance = float64(matchCount) / float64(len(keywords))
	}

	// Basic analysis based on file extension
	ext := strings.ToLower(filepath.Ext(filePath))

	// Check for common code issues
	if isCodeFile(ext) {
		// Check for TODOs and FIXMEs
		todoCount := strings.Count(contentLower, "todo")
		fixmeCount := strings.Count(contentLower, "fixme")

		if todoCount > 0 {
			analysis.KeyFindings = append(analysis.KeyFindings,
				fmt.Sprintf("Found %d TODO comments", todoCount))
		}

		if fixmeCount > 0 {
			analysis.KeyFindings = append(analysis.KeyFindings,
				fmt.Sprintf("Found %d FIXME comments", fixmeCount))
		}

		// Basic security checks
		if strings.Contains(contentLower, "password") && strings.Contains(contentLower, "=") {
			analysis.CodeIssues = append(analysis.CodeIssues, CodeIssue{
				Type:        "security",
				Severity:    "high",
				Description: "Potential hardcoded password detected",
				Suggestion:  "Use environment variables or secure configuration",
			})
		}
	}

	// Generate summary
	lines := strings.Split(content, "\n")
	analysis.Summary = fmt.Sprintf("File with %d lines, relevance: %.2f",
		len(lines), analysis.Relevance)

	// Add metadata
	analysis.Metadata["line_count"] = len(lines)
	analysis.Metadata["size_bytes"] = len(content)
	analysis.Metadata["extension"] = ext

	return analysis, nil
}

// isCodeFile checks if the file extension indicates a code file
func isCodeFile(ext string) bool {
	codeExtensions := []string{
		".go", ".js", ".ts", ".py", ".java", ".c", ".cpp", ".cs",
		".rb", ".php", ".swift", ".kt", ".rs", ".sh", ".yaml", ".yml",
	}

	for _, codeExt := range codeExtensions {
		if ext == codeExt {
			return true
		}
	}

	return false
}
