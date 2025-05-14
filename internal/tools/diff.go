package tools

import (
	"fmt"
	"strings"

	"codezilla/pkg/style"
)

// GenerateDiff creates a colored diff output between the two content strings
func GenerateDiff(contentA, contentB string, contextLines int) string {
	// Split content into lines
	linesA := strings.Split(contentA, "\n")
	linesB := strings.Split(contentB, "\n")

	// Prepare result
	var result strings.Builder

	// Print minimal line showing what the colors mean
	result.WriteString(style.ColorBold(style.ColorCodeRed, "- Red: ") + "Removed content  ")
	result.WriteString(style.ColorBold(style.ColorCodeGreen, "+ Green: ") + "Added content\n")
	result.WriteString("\n")

	// Find common prefix and suffix
	i := 0
	for i < len(linesA) && i < len(linesB) && linesA[i] == linesB[i] {
		i++
	}
	prefixLen := i

	j := 0
	for j < len(linesA)-prefixLen && j < len(linesB)-prefixLen &&
		linesA[len(linesA)-1-j] == linesB[len(linesB)-1-j] {
		j++
	}
	suffixLen := j

	// Calculate the end of common prefix and start of common suffix
	prefixEnd := prefixLen
	suffixStart := len(linesA) - suffixLen

	// If there are no differences, say so
	if prefixLen == len(linesA) && prefixLen == len(linesB) {
		result.WriteString(style.ColorBold(style.ColorCodeWhite, "\nNo differences found.\n"))
		return result.String()
	}

	// Helper function to get slice with context
	getContextRange := func(start, end, max int) (int, int) {
		contextStart := start - contextLines
		if contextStart < 0 {
			contextStart = 0
		}

		contextEnd := end + contextLines
		if contextEnd > max {
			contextEnd = max
		}

		return contextStart, contextEnd
	}

	// Adjust the range to include context lines
	displayStartA, displayEndA := getContextRange(prefixEnd, suffixStart, len(linesA))
	displayStartB, displayEndB := getContextRange(prefixEnd, len(linesB)-suffixLen, len(linesB))

	// Add line header showing line numbers
	lineHeader := fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		displayStartA+1, displayEndA-displayStartA,
		displayStartB+1, displayEndB-displayStartB)
	result.WriteString(style.ColorBold(style.ColorCodeCyan, lineHeader+"\n"))

	// Display separator line
	result.WriteString(style.ColorCodeWhite + "─────────────────────────────────────────────────────────\n")

	// Helper to format line numbers
	formatLineNum := func(num int, width int) string {
		return fmt.Sprintf("%"+fmt.Sprintf("%d", width)+"d", num)
	}

	// Determine line number width
	lineNumWidth := len(fmt.Sprintf("%d", max(displayEndA, displayEndB)))

	// Display prefix context (unchanged lines)
	for i := displayStartA; i < prefixEnd; i++ {
		lineNumA := formatLineNum(i+1, lineNumWidth)
		result.WriteString(fmt.Sprintf(" %s │ %s\n",
			style.ColorCodeWhite+lineNumA,
			linesA[i]))
	}

	// Find and display changes
	var diffA, diffB []string
	if suffixStart > prefixEnd {
		diffA = linesA[prefixEnd:suffixStart]
	}
	if len(linesB)-suffixLen > prefixEnd {
		diffB = linesB[prefixEnd : len(linesB)-suffixLen]
	}

	// Display removed lines with background color
	for i, line := range diffA {
		lineNumA := formatLineNum(prefixEnd+i+1, lineNumWidth)
		lineDisplay := fmt.Sprintf("-%s │ %s", lineNumA, line)
		result.WriteString(style.ColorBgRed(lineDisplay) + "\n")
	}

	// Display added lines with background color
	for i, line := range diffB {
		lineNumB := formatLineNum(prefixEnd+i+1, lineNumWidth)
		lineDisplay := fmt.Sprintf("+%s │ %s", lineNumB, line)
		result.WriteString(style.ColorBgGreen(lineDisplay) + "\n")
	}

	// Display separator line before suffix context
	if len(diffA) > 0 || len(diffB) > 0 {
		result.WriteString(style.ColorCodeWhite + "─────────────────────────────────────────────────────────\n")
	}

	// Display suffix context (unchanged lines)
	for i := suffixStart; i < displayEndA; i++ {
		if i < len(linesA) {
			lineNumA := formatLineNum(i+1, lineNumWidth)
			result.WriteString(fmt.Sprintf(" %s │ %s\n",
				style.ColorCodeWhite+lineNumA,
				linesA[i]))
		}
	}

	// Add footer
	result.WriteString(style.ColorCodeWhite + "─────────────────────────────────────────────────────────\n")

	return result.String()
}

// Helper function to find max of two ints
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
