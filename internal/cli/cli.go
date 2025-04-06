package cli

import (
	"fmt"

	"codezilla/internal/tags"
	"codezilla/pkg/fsutil"
	"codezilla/pkg/logger"
	"codezilla/pkg/style"
)

// TagsCLI provides a command-line interface for working with ctags and files
type TagsCLI struct {
	tagIndex *tags.TagIndex
	fileCtrl *fsutil.Controller
}

// New creates a new tags CLI
func New() *TagsCLI {
	logger.Debug("Creating new TagsCLI instance")

	// Create file controller with default config
	fileCtrl := fsutil.NewController(fsutil.DefaultControllerConfig())

	// Initialize the controller
	err := fileCtrl.Initialize()
	if err != nil {
		logger.Error("Failed to initialize file controller", "error", err)
	}

	return &TagsCLI{
		tagIndex: tags.NewTagIndex(),
		fileCtrl: fileCtrl,
	}
}

// PrintTagInfo prints detailed info about a tag
func (cli *TagsCLI) PrintTagInfo(tagName string) {
	logger.Debug("Retrieving tag info", "tag", tagName)

	tags := cli.tagIndex.GetTagsByName(tagName)
	if len(tags) == 0 {
		logger.Info("Tag not found", "tag", tagName)
		fmt.Printf("%sTag '%s' not found%s\n", style.Red, tagName, style.Reset)
		return
	}

	logger.Info("Found tag entries", "tag", tagName, "count", len(tags))
	fmt.Printf("%sFound %d entries for tag '%s':%s\n", style.Bold, len(tags), tagName, style.Reset)

	for i, tag := range tags {
		fmt.Printf("%s[%d] %s%s\n", style.Bold, i+1, tag.Name, style.Reset)
		fmt.Printf("  File: %s\n", tag.FilePath)
		if tag.LineNo > 0 {
			fmt.Printf("  Line: %d\n", tag.LineNo)
		}
		if tag.Kind != "" {
			fmt.Printf("  Kind: %s\n", tag.Kind)
		}

		// Print additional fields
		if len(tag.Fields) > 0 {
			fmt.Println("  Fields:")
			for k, v := range tag.Fields {
				if k != "kind" { // already printed above
					fmt.Printf("    %s: %s\n", k, v)
				}
			}
		}
		fmt.Println()
	}
}

// PrintTagsByFile prints all tags defined in a file
func (cli *TagsCLI) PrintTagsByFile(filePath string) {
	logger.Debug("Retrieving tags for file", "file", filePath)

	tags := cli.tagIndex.GetTagsByFile(filePath)
	if len(tags) == 0 {
		logger.Info("No tags found for file", "file", filePath)
		fmt.Printf("%sNo tags found for file '%s'%s\n", style.Red, filePath, style.Reset)
		return
	}

	logger.Info("Found tags in file", "file", filePath, "count", len(tags))
	fmt.Printf("%sFound %d tags in file '%s':%s\n", style.Bold, len(tags), filePath, style.Reset)

	for i, tag := range tags {
		fmt.Printf("%s[%d] %s%s", style.Bold, i+1, tag.Name, style.Reset)
		if tag.Kind != "" {
			fmt.Printf(" (%s)", tag.Kind)
		}
		if tag.LineNo > 0 {
			fmt.Printf(" at line %d", tag.LineNo)
		}
		fmt.Println()
	}
	fmt.Println()
}

// PrintTagsByKind prints all tags of a specific kind
func (cli *TagsCLI) PrintTagsByKind(kind string) {
	logger.Debug("Retrieving tags by kind", "kind", kind)

	tags := cli.tagIndex.GetTagsByKind(kind)
	if len(tags) == 0 {
		logger.Info("No tags found of kind", "kind", kind)
		fmt.Printf("%sNo tags found of kind '%s'%s\n", style.Red, kind, style.Reset)
		return
	}

	logger.Info("Found tags of kind", "kind", kind, "count", len(tags))
	fmt.Printf("%sFound %d tags of kind '%s':%s\n", style.Bold, len(tags), kind, style.Reset)

	for i, tag := range tags {
		fmt.Printf("%s[%d] %s%s", style.Bold, i+1, tag.Name, style.Reset)
		fmt.Printf(" in %s", tag.FilePath)
		if tag.LineNo > 0 {
			fmt.Printf(" at line %d", tag.LineNo)
		}
		fmt.Println()
	}
	fmt.Println()
}
