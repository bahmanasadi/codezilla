package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codezilla/pkg/logger"
)

// LLMFileAnalyzer uses an LLM to analyze files based on user queries
type LLMFileAnalyzer struct {
	llmClient LLMClient
	logger    *logger.Logger
}

// NewLLMFileAnalyzer creates a new LLM-based file analyzer
func NewLLMFileAnalyzer(llmClient LLMClient, logger *logger.Logger) *LLMFileAnalyzer {
	return &LLMFileAnalyzer{
		llmClient: llmClient,
		logger:    logger,
	}
}

// AnalyzeFile uses the LLM to analyze a file based on the user query
func (a *LLMFileAnalyzer) AnalyzeFile(ctx context.Context, filePath string, content string, userQuery string) (FileAnalysis, error) {
	analysis := FileAnalysis{
		FilePath:    filePath,
		KeyFindings: make([]string, 0),
		CodeIssues:  make([]CodeIssue, 0),
		Suggestions: make([]string, 0),
		Metadata:    make(map[string]interface{}),
	}

	// Prepare the analysis prompt
	prompt := a.buildAnalysisPrompt(filePath, content, userQuery)

	// Create messages for LLM
	messages := []LLMMessage{
		{
			Role:    "system",
			Content: getAnalysisSystemPrompt(),
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// Get LLM response
	startTime := time.Now()
	response, err := a.llmClient.GenerateResponse(ctx, messages)
	if err != nil {
		a.logger.Error("LLM analysis failed", "file", filePath, "error", err)
		return analysis, fmt.Errorf("LLM analysis failed: %w", err)
	}
	analysis.AnalysisTime = time.Since(startTime)

	// Parse the LLM response
	err = a.parseAnalysisResponse(response, &analysis)
	if err != nil {
		a.logger.Warn("Failed to parse LLM response", "file", filePath, "error", err)
		// Fall back to basic analysis
		return a.fallbackAnalysis(filePath, content, userQuery)
	}

	// Add file metadata
	lines := strings.Split(content, "\n")
	analysis.Metadata["line_count"] = len(lines)
	analysis.Metadata["size_bytes"] = len(content)
	analysis.Metadata["extension"] = filepath.Ext(filePath)

	return analysis, nil
}

// buildAnalysisPrompt creates the prompt for file analysis
func (a *LLMFileAnalyzer) buildAnalysisPrompt(filePath string, content string, userQuery string) string {
	// Truncate content if too long
	maxContentLength := 10000
	truncated := false
	if len(content) > maxContentLength {
		content = content[:maxContentLength]
		truncated = true
	}

	prompt := fmt.Sprintf(`Analyze the following file based on the user's query.

User Query: %s

File Path: %s
File Content:
%s

%s

Please provide your analysis in the following XML format:

<analysis>
  <relevance>0.0-1.0</relevance>
  <summary>Brief summary of the file's relevance to the query</summary>
  <findings>
    <finding>Key finding 1</finding>
    <finding>Key finding 2</finding>
  </findings>
  <issues>
    <issue>
      <type>bug|security|performance|style</type>
      <severity>critical|high|medium|low</severity>
      <line>line number (optional)</line>
      <description>Issue description</description>
      <suggestion>How to fix it</suggestion>
    </issue>
  </issues>
  <suggestions>
    <suggestion>Improvement suggestion 1</suggestion>
    <suggestion>Improvement suggestion 2</suggestion>
  </suggestions>
</analysis>`,
		userQuery,
		filePath,
		content,
		func() string {
			if truncated {
				return "(Note: File content was truncated for analysis)"
			}
			return ""
		}(),
	)

	return prompt
}

// getAnalysisSystemPrompt returns the system prompt for file analysis
func getAnalysisSystemPrompt() string {
	return `You are a code analysis assistant. Your task is to analyze source code files based on user queries and provide structured feedback.

Focus on:
1. How relevant the file is to the user's query (0.0 = not relevant, 1.0 = highly relevant)
2. Key findings that relate to the user's query
3. Any code issues you identify (bugs, security vulnerabilities, performance issues, style problems)
4. Constructive suggestions for improvement

Be concise but thorough. Provide actionable insights.`
}

// parseAnalysisResponse parses the XML response from the LLM
func (a *LLMFileAnalyzer) parseAnalysisResponse(response string, analysis *FileAnalysis) error {
	// Simple XML parsing (could be replaced with proper XML parsing)

	// Extract relevance
	if relevance := extractXMLValue(response, "relevance"); relevance != "" {
		var score float64
		fmt.Sscanf(relevance, "%f", &score)
		analysis.Relevance = score
	}

	// Extract summary
	if summary := extractXMLValue(response, "summary"); summary != "" {
		analysis.Summary = summary
	}

	// Extract findings
	findings := extractXMLValues(response, "finding")
	analysis.KeyFindings = findings

	// Extract issues
	issuesXML := extractXMLSection(response, "issues")
	if issuesXML != "" {
		issues := a.parseIssues(issuesXML)
		analysis.CodeIssues = issues
	}

	// Extract suggestions
	suggestions := extractXMLValues(response, "suggestion")
	analysis.Suggestions = suggestions

	return nil
}

// parseIssues parses issue elements from XML
func (a *LLMFileAnalyzer) parseIssues(issuesXML string) []CodeIssue {
	var issues []CodeIssue

	// Find all issue sections
	issueStart := 0
	for {
		start := strings.Index(issuesXML[issueStart:], "<issue>")
		if start == -1 {
			break
		}
		start += issueStart

		end := strings.Index(issuesXML[start:], "</issue>")
		if end == -1 {
			break
		}
		end += start

		issueXML := issuesXML[start:end]

		issue := CodeIssue{
			Type:        extractXMLValue(issueXML, "type"),
			Severity:    extractXMLValue(issueXML, "severity"),
			Description: extractXMLValue(issueXML, "description"),
			Suggestion:  extractXMLValue(issueXML, "suggestion"),
		}

		if lineStr := extractXMLValue(issueXML, "line"); lineStr != "" {
			var line int
			fmt.Sscanf(lineStr, "%d", &line)
			issue.Line = line
		}

		if issue.Type != "" && issue.Description != "" {
			issues = append(issues, issue)
		}

		issueStart = end
	}

	return issues
}

// fallbackAnalysis provides basic analysis when LLM fails
func (a *LLMFileAnalyzer) fallbackAnalysis(filePath string, content string, userQuery string) (FileAnalysis, error) {
	// Use the default analyzer as fallback
	defaultAnalyzer := NewDefaultFileAnalyzer()
	return defaultAnalyzer.AnalyzeFile(context.Background(), filePath, content, userQuery)
}

// extractXMLValue extracts a single value from XML tags
func extractXMLValue(xml, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"

	start := strings.Index(xml, startTag)
	if start == -1 {
		return ""
	}
	start += len(startTag)

	end := strings.Index(xml[start:], endTag)
	if end == -1 {
		return ""
	}

	return strings.TrimSpace(xml[start : start+end])
}

// extractXMLValues extracts multiple values with the same tag
func extractXMLValues(xml, tag string) []string {
	var values []string

	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"

	searchStart := 0
	for {
		start := strings.Index(xml[searchStart:], startTag)
		if start == -1 {
			break
		}
		start += searchStart + len(startTag)

		end := strings.Index(xml[start:], endTag)
		if end == -1 {
			break
		}

		value := strings.TrimSpace(xml[start : start+end])
		if value != "" {
			values = append(values, value)
		}

		searchStart = start + end
	}

	return values
}

// extractXMLSection extracts an entire XML section
func extractXMLSection(xml, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"

	start := strings.Index(xml, startTag)
	if start == -1 {
		return ""
	}

	end := strings.Index(xml[start:], endTag)
	if end == -1 {
		return ""
	}
	end += start + len(endTag)

	return xml[start:end]
}
