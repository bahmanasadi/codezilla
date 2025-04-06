package tools

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadFile reads a file and returns its content
// If contextLines is specified, it includes that many lines of context around each match
// If query is provided, it only returns the lines containing the query with context
func ReadFile(path string, query string, contextLines int) (string, error) {
	// Check if file exists
	_, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("file not found: %v", err)
	}

	// Open file
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// If no query is provided, read the entire file
	if query == "" {
		return readEntireFile(file)
	}

	// Otherwise, search for the query with context
	return searchFileWithContext(file, query, contextLines)
}

// readEntireFile reads and returns the entire file content
func readEntireFile(file *os.File) (string, error) {
	var sb strings.Builder
	scanner := bufio.NewScanner(file)

	// Add the file path as a header
	absPath, _ := filepath.Abs(file.Name())
	sb.WriteString(fmt.Sprintf("File: %s\n\n", absPath))

	lineNum := 1
	for scanner.Scan() {
		sb.WriteString(fmt.Sprintf("%d: %s\n", lineNum, scanner.Text()))
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}

	return sb.String(), nil
}

// searchFileWithContext searches for query in file and returns matching lines with context
func searchFileWithContext(file *os.File, query string, contextLines int) (string, error) {
	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}

	// If contextLines is not provided, use a default
	if contextLines <= 0 {
		contextLines = 2
	}

	var sb strings.Builder
	// Add the file path as a header
	absPath, _ := filepath.Abs(file.Name())
	sb.WriteString(fmt.Sprintf("File: %s (showing lines containing '%s' with %d lines of context)\n\n",
		absPath, query, contextLines))

	// Find matches
	found := false
	for i, line := range lines {
		if strings.Contains(line, query) {
			found = true

			// Determine context range
			startLine := i - contextLines
			if startLine < 0 {
				startLine = 0
			}
			endLine := i + contextLines
			if endLine >= len(lines) {
				endLine = len(lines) - 1
			}

			// Add section header
			sb.WriteString(fmt.Sprintf("--- Match at line %d ---\n", i+1))

			// Add context lines
			for j := startLine; j <= endLine; j++ {
				prefix := "  "
				if j == i {
					prefix = "> " // Highlight the matching line
				}
				sb.WriteString(fmt.Sprintf("%s%d: %s\n", prefix, j+1, lines[j]))
			}
			sb.WriteString("\n")
		}
	}

	if !found {
		sb.WriteString(fmt.Sprintf("No matches found for '%s'\n", query))
	}

	return sb.String(), nil
}
