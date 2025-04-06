package util

import "strings"

// TrimPrefix trims a prefix from a string and returns the trimmed string
func TrimPrefix(s, prefix string) string {
	return strings.TrimSpace(strings.TrimPrefix(s, prefix))
}

// SplitKV splits a string by a separator and returns the key and value
// If no separator is found, returns the string as key and empty string as value
func SplitKV(s, sep string) (string, string) {
	if !strings.Contains(s, sep) {
		return s, ""
	}

	parts := strings.SplitN(s, sep, 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	return s, ""
}
