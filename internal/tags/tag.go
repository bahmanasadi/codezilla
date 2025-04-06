package tags

// TagEntry represents a single ctags entry
type TagEntry struct {
	Name     string            // Tag name
	FilePath string            // Path to the file
	LineNo   int               // Line number in the file
	Kind     string            // Kind of tag (function, class, etc.)
	Fields   map[string]string // Additional fields
}
