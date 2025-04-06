package tags

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTagLine parses a single line from a ctags file
// Format: <tag_name><TAB><file_path><TAB><ex_cmd>;"<TAB><extension_fields>
func ParseTagLine(line string) (*TagEntry, error) {
	// Skip comment lines
	if strings.HasPrefix(line, "!") {
		return nil, fmt.Errorf("comment line")
	}

	parts := strings.Split(line, "\t")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid tag format: too few fields")
	}

	// Basic tag info
	name := parts[0]
	filePath := parts[1]

	// Parse the ex command which contains line number or pattern
	exCmd := parts[2]
	lineNo := 0

	// In ex command format, we can have /^pattern$/;"
	// or just line number like 123;"
	if strings.HasPrefix(exCmd, "/^") && strings.Contains(exCmd, "$/;\"") {
		// Pattern-based location, we can't determine line directly
	} else {
		// Try to get line number from ex command
		numStr := strings.TrimSuffix(exCmd, ";\"")
		if num, err := strconv.Atoi(numStr); err == nil {
			lineNo = num
		}
	}

	// Create tag entry with basic info
	tag := &TagEntry{
		Name:     name,
		FilePath: filePath,
		LineNo:   lineNo,
		Fields:   make(map[string]string),
	}

	// Parse extension fields
	for i := 3; i < len(parts); i++ {
		// Extension fields are in format key:value
		if strings.Contains(parts[i], ":") {
			kv := strings.SplitN(parts[i], ":", 2)
			if len(kv) == 2 {
				if kv[0] == "kind" {
					tag.Kind = kv[1]
				}
				tag.Fields[kv[0]] = kv[1]
			}
		}
	}

	return tag, nil
}
