package tools

import (
	"fmt"
	"time"
)

// ProgressReporter interface for reporting analysis progress
type ProgressReporter interface {
	// OnFileStart is called when starting to read a file
	OnFileStart(filePath string, fileIndex, totalFiles int)

	// OnFileRead is called after successfully reading a file
	OnFileRead(filePath string, sizeBytes int)

	// OnAnalysisStart is called when starting to analyze a file
	OnAnalysisStart(filePath string)

	// OnAnalysisComplete is called when analysis is complete
	OnAnalysisComplete(filePath string, analysis FileAnalysis, duration time.Duration)

	// OnFileSkipped is called when a file is skipped
	OnFileSkipped(filePath string, reason string)

	// OnError is called when an error occurs
	OnError(filePath string, err error)
}

// TerminalProgressReporter implements progress reporting to terminal
type TerminalProgressReporter struct {
	// Function to print to terminal
	print func(format string, args ...interface{})
}

// NewTerminalProgressReporter creates a new terminal progress reporter
func NewTerminalProgressReporter(printFunc func(format string, args ...interface{})) *TerminalProgressReporter {
	if printFunc == nil {
		printFunc = func(format string, args ...interface{}) {
			fmt.Printf(format, args...)
		}
	}
	return &TerminalProgressReporter{
		print: printFunc,
	}
}

// OnFileStart reports when starting to read a file
func (r *TerminalProgressReporter) OnFileStart(filePath string, fileIndex, totalFiles int) {
	r.print("\n📄 [%d/%d] Reading: %s\n", fileIndex, totalFiles, filePath)
}

// OnFileRead reports after successfully reading a file
func (r *TerminalProgressReporter) OnFileRead(filePath string, sizeBytes int) {
	r.print("   ✓ Read %d bytes\n", sizeBytes)
}

// OnAnalysisStart reports when starting to analyze a file
func (r *TerminalProgressReporter) OnAnalysisStart(filePath string) {
	r.print("   🔍 Analyzing...\n")
}

// OnAnalysisComplete reports when analysis is complete
func (r *TerminalProgressReporter) OnAnalysisComplete(filePath string, analysis FileAnalysis, duration time.Duration) {
	r.print("   ✅ Analysis complete (%.2fs)\n", duration.Seconds())
	r.print("      Relevance: %.2f\n", analysis.Relevance)

	if analysis.Summary != "" {
		r.print("      Summary: %s\n", analysis.Summary)
	}

	if len(analysis.KeyFindings) > 0 {
		r.print("      Key findings:\n")
		for _, finding := range analysis.KeyFindings {
			r.print("        • %s\n", finding)
		}
	}

	if len(analysis.CodeIssues) > 0 {
		r.print("      Issues found:\n")
		for _, issue := range analysis.CodeIssues {
			icon := "⚠️"
			if issue.Severity == "critical" {
				icon = "🚨"
			} else if issue.Severity == "high" {
				icon = "❗"
			}
			r.print("        %s %s (%s): %s\n", icon, issue.Type, issue.Severity, issue.Description)
		}
	}
}

// OnFileSkipped reports when a file is skipped
func (r *TerminalProgressReporter) OnFileSkipped(filePath string, reason string) {
	r.print("\n⏭️  Skipping: %s\n", filePath)
	r.print("   Reason: %s\n", reason)
}

// OnError reports when an error occurs
func (r *TerminalProgressReporter) OnError(filePath string, err error) {
	r.print("\n❌ Error with: %s\n", filePath)
	r.print("   Error: %v\n", err)
}

// NullProgressReporter is a no-op progress reporter
type NullProgressReporter struct{}

// OnFileStart does nothing
func (n *NullProgressReporter) OnFileStart(filePath string, fileIndex, totalFiles int) {}

// OnFileRead does nothing
func (n *NullProgressReporter) OnFileRead(filePath string, sizeBytes int) {}

// OnAnalysisStart does nothing
func (n *NullProgressReporter) OnAnalysisStart(filePath string) {}

// OnAnalysisComplete does nothing
func (n *NullProgressReporter) OnAnalysisComplete(filePath string, analysis FileAnalysis, duration time.Duration) {
}

// OnFileSkipped does nothing
func (n *NullProgressReporter) OnFileSkipped(filePath string, reason string) {}

// OnError does nothing
func (n *NullProgressReporter) OnError(filePath string, err error) {}
