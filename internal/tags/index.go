package tags

import (
	"bufio"
	"fmt"
	"os"
)

// TagIndex represents a collection of tag entries
type TagIndex struct {
	Tags      map[string][]*TagEntry // Tags mapped by name (can have multiple entries with same name)
	FileIndex map[string][]*TagEntry // Tags mapped by file path
	KindIndex map[string][]*TagEntry // Tags mapped by kind
}

// NewTagIndex creates a new tag index
func NewTagIndex() *TagIndex {
	return &TagIndex{
		Tags:      make(map[string][]*TagEntry),
		FileIndex: make(map[string][]*TagEntry),
		KindIndex: make(map[string][]*TagEntry),
	}
}

// LoadTags loads tags from a ctags file
func (ti *TagIndex) LoadTags(tagFilePath string) error {
	file, err := os.Open(tagFilePath)
	if err != nil {
		return fmt.Errorf("failed to open tags file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Parse the tag line
		tag, err := ParseTagLine(line)
		if err != nil {
			fmt.Printf("Warning: skipping invalid tag at line %d: %v\n", lineNum, err)
			continue
		}

		// Add to indices
		ti.Tags[tag.Name] = append(ti.Tags[tag.Name], tag)
		ti.FileIndex[tag.FilePath] = append(ti.FileIndex[tag.FilePath], tag)
		ti.KindIndex[tag.Kind] = append(ti.KindIndex[tag.Kind], tag)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading tags file: %v", err)
	}

	return nil
}

// GetTagsByName returns all tags matching a given name
func (ti *TagIndex) GetTagsByName(tagName string) []*TagEntry {
	return ti.Tags[tagName]
}

// GetTagsByFile returns all tags defined in a file
func (ti *TagIndex) GetTagsByFile(filePath string) []*TagEntry {
	return ti.FileIndex[filePath]
}

// GetTagsByKind returns all tags of a specific kind
func (ti *TagIndex) GetTagsByKind(kind string) []*TagEntry {
	return ti.KindIndex[kind]
}

// GetAvailableKinds returns all kinds of tags in the index
func (ti *TagIndex) GetAvailableKinds() []string {
	kinds := make([]string, 0, len(ti.KindIndex))
	for kind := range ti.KindIndex {
		kinds = append(kinds, kind)
	}
	return kinds
}

// CountTags returns the total number of unique tag names
func (ti *TagIndex) CountTags() int {
	return len(ti.Tags)
}

// CountFiles returns the total number of files
func (ti *TagIndex) CountFiles() int {
	return len(ti.FileIndex)
}

// CountKinds returns the total number of kinds
func (ti *TagIndex) CountKinds() int {
	return len(ti.KindIndex)
}
