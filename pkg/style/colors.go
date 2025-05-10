package style

import "fmt"

// ANSI color codes
const (
	ColorCodeReset  = "\033[0m"
	ColorCodeBold   = "\033[1m"
	ColorCodeRed    = "\033[31m"
	ColorCodeGreen  = "\033[32m"
	ColorCodeYellow = "\033[33m"
	ColorCodeBlue   = "\033[34m"
	ColorCodePurple = "\033[35m"
	ColorCodeCyan   = "\033[36m"
	ColorCodeWhite  = "\033[37m"
)

// Color adds color to a string
func Color(color string, text string) string {
	return color + text + ColorCodeReset
}

// ColorBold adds color and bold to a string
func ColorBold(color string, text string) string {
	return color + ColorCodeBold + text + ColorCodeReset
}

// Color function shortcuts
func ColorRed(text string) string {
	return Color(ColorCodeRed, text)
}

func ColorGreen(text string) string {
	return Color(ColorCodeGreen, text)
}

func ColorYellow(text string) string {
	return Color(ColorCodeYellow, text)
}

func ColorBlue(text string) string {
	return Color(ColorCodeBlue, text)
}

func ColorPurple(text string) string {
	return Color(ColorCodePurple, text)
}

func ColorCyan(text string) string {
	return Color(ColorCodeCyan, text)
}

func ColorWhite(text string) string {
	return Color(ColorCodeWhite, text)
}

// Colorize formats a string with embedded color codes
// Example: Colorize("This is {red}red{reset} and this is {green}green{reset}")
func Colorize(format string, args ...interface{}) string {
	// Replace color codes
	replacements := map[string]string{
		"{reset}":  ColorCodeReset,
		"{bold}":   ColorCodeBold,
		"{red}":    ColorCodeRed,
		"{green}":  ColorCodeGreen,
		"{yellow}": ColorCodeYellow,
		"{blue}":   ColorCodeBlue,
		"{purple}": ColorCodePurple,
		"{cyan}":   ColorCodeCyan,
		"{white}":  ColorCodeWhite,
	}

	result := format
	for marker, code := range replacements {
		result = replaceAllLiteral(result, marker, code)
	}

	// Apply format arguments if provided
	if len(args) > 0 {
		result = fmt.Sprintf(result, args...)
	}

	return result
}

// Helper function to replace all instances of a string literal (not regex)
func replaceAllLiteral(s, old, new string) string {
	// If old is empty, return the original string
	if old == "" {
		return s
	}

	// Create a new string builder
	var result string
	pos := 0

	// Find and replace all occurrences
	for {
		i := indexOf(s[pos:], old)
		if i == -1 {
			result += s[pos:]
			break
		}
		result += s[pos : pos+i]
		result += new
		pos += i + len(old)
	}

	return result
}

// Helper function to find index of substring
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
