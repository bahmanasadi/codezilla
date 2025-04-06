package tools

import (
	"fmt"
	"time"
)

// GetDateTime returns the current date and time in the specified format
// If format is empty, it uses the standard format
func GetDateTime(format ...string) string {
	now := time.Now()

	if len(format) == 0 || format[0] == "" || format[0] == "standard" {
		return now.Format("Monday, Jan 2, 2006 - 15:04:05 MST")
	}

	switch format[0] {
	case "iso":
		return now.Format(time.RFC3339)
	case "unix":
		return fmt.Sprintf("%d", now.Unix())
	default:
		return now.Format("Monday, Jan 2, 2006 - 15:04:05 MST")
	}
}
