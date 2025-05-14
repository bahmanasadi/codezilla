package style

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

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

	// Background colors
	ColorCodeBgRed    = "\033[41m"
	ColorCodeBgGreen  = "\033[42m"
	ColorCodeBgYellow = "\033[43m"
	ColorCodeBgBlue   = "\033[44m"
	ColorCodeBgPurple = "\033[45m"
	ColorCodeBgCyan   = "\033[46m"
	ColorCodeBgWhite  = "\033[47m"
)

var (
	// UseColors determines if colors should be used in output
	UseColors = true
)

func init() {
	// Check if colors should be disabled
	if os.Getenv("NO_COLOR") != "" || os.Getenv("CODEZILLA_NO_COLOR") != "" {
		UseColors = false
		return
	}

	// Try to detect if we're in GoLand's emulated terminal
	if v, ok := os.LookupEnv("GOLAND_TERMINAL_EMULATION"); ok && strings.ToLower(v) == "disabled" {
		UseColors = false
		return
	}

	// Check TERM environment variable
	term := os.Getenv("TERM")
	if term == "dumb" {
		UseColors = false
		return
	}

	// We'd normally check if output is to a terminal with term.IsTerminal,
	// but we'll use other methods to be compatible with GoLand

	// Check if FORCE_COLOR is set, which explicitly enables colors
	if os.Getenv("FORCE_COLOR") == "" {
		// If not explicitly forced, we can try other heuristics

		// Simple heuristic using file descriptor properties (non-TTY detection)
		fileInfo, err := os.Stdout.Stat()
		if err == nil && (fileInfo.Mode()&os.ModeCharDevice) == 0 {
			// This is likely a pipe or redirection, not a terminal
			UseColors = false
		}
	}

	// Check color capability from env
	if colorEnv := os.Getenv("COLORTERM"); colorEnv == "" &&
		os.Getenv("CLICOLOR_FORCE") == "" && os.Getenv("FORCE_COLOR") == "" {
		// Try to detect terminal color capability
		if _, err := strconv.Atoi(os.Getenv("TERM_COLORS")); err != nil {
			// If there's no indication of color support, be conservative
			// This mainly helps with GoLand's emulated terminal
			if !strings.Contains(term, "color") && !strings.Contains(term, "xterm") {
				UseColors = false
			}
		}
	}
}

// EnableColors forces colors to be enabled
func EnableColors() {
	UseColors = true
}

// DisableColors forces colors to be disabled
func DisableColors() {
	UseColors = false
}

// Color adds color to a string
func Color(color string, text string) string {
	if !UseColors {
		return text
	}
	return color + text + ColorCodeReset
}

// ColorBold adds color and bold to a string
func ColorBold(color string, text string) string {
	if !UseColors {
		return text
	}
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

// Background color shortcuts
func ColorBgRed(text string) string {
	if !UseColors {
		return text
	}
	return ColorCodeBgRed + text + ColorCodeReset
}

func ColorBgGreen(text string) string {
	if !UseColors {
		return text
	}
	return ColorCodeBgGreen + text + ColorCodeReset
}

func ColorRedBg(text string) string {
	if !UseColors {
		return text
	}
	return ColorCodeRed + ColorCodeBgWhite + text + ColorCodeReset
}

func ColorGreenBg(text string) string {
	if !UseColors {
		return text
	}
	return ColorCodeGreen + ColorCodeBgWhite + text + ColorCodeReset
}

// Colorize formats a string with embedded color codes
// Example: Colorize("This is {red}red{reset} and this is {green}green{reset}")
func Colorize(format string, args ...interface{}) string {
	if !UseColors {
		// Strip color markers if colors are disabled
		replacements := map[string]string{
			"{reset}":  "",
			"{bold}":   "",
			"{red}":    "",
			"{green}":  "",
			"{yellow}": "",
			"{blue}":   "",
			"{purple}": "",
			"{cyan}":   "",
			"{white}":  "",
		}

		result := format
		for marker, replacement := range replacements {
			result = replaceAllLiteral(result, marker, replacement)
		}

		// Apply format arguments if provided
		if len(args) > 0 {
			result = fmt.Sprintf(result, args...)
		}

		return result
	}

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
