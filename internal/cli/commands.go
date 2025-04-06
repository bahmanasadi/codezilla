package cli

import (
	"fmt"
	"strings"

	"codezilla/pkg/logger"
	"codezilla/pkg/style"
	"codezilla/pkg/util"
)

// LoadTagsFile loads a ctags file
func (cli *TagsCLI) LoadTagsFile(path string) error {
	// Handle relative paths
	resolvedPath, err := util.ResolvePath(path)
	if err != nil {
		logger.Error("Failed to resolve path", "path", path, "error", err)
		return err
	}

	logger.Info("Loading tags file", "path", resolvedPath)
	
	// Load tags file
	fmt.Printf("Loading tags from %s...\n", resolvedPath)
	err = cli.tagIndex.LoadTags(resolvedPath)
	if err != nil {
		logger.Error("Failed to load tags file", "path", resolvedPath, "error", err)
		return err
	}

	// Collect and log stats
	tagCount := cli.tagIndex.CountTags()
	fileCount := cli.tagIndex.CountFiles()
	kindCount := cli.tagIndex.CountKinds()

	logger.Info("Successfully loaded tags", 
		"tag_count", tagCount, 
		"file_count", fileCount, 
		"kind_count", kindCount)
	
	// Print summary
	fmt.Printf("%sSuccessfully loaded tags%s\n", style.Green, style.Reset)
	fmt.Printf("Total unique tag names: %d\n", tagCount)
	fmt.Printf("Total files indexed: %d\n", fileCount)
	fmt.Printf("Total kinds of tags: %d\n", kindCount)
	return nil
}

// PrintAvailableKinds prints all available tag kinds
func (cli *TagsCLI) PrintAvailableKinds() {
	logger.Debug("Retrieving available kinds")
	
	kinds := cli.tagIndex.GetAvailableKinds()
	if len(kinds) == 0 {
		logger.Info("No kinds available")
		fmt.Println("No tag kinds available. Load a tags file first.")
		return
	}

	logger.Info("Found kinds", "count", len(kinds))
	fmt.Printf("%sAvailable kinds:%s\n", style.Bold, style.Reset)
	for _, kind := range kinds {
		fmt.Printf("- %s\n", kind)
	}
	fmt.Println()
}

// FileSearchCommand searches for files containing a pattern
func (cli *TagsCLI) FileSearchCommand(pattern string, isRegex bool, caseSensitive bool) error {
	// Use the file controller to search for files
	results, err := cli.fileCtrl.SearchFiles(pattern, isRegex, caseSensitive)
	if err != nil {
		return err
	}
	
	// Print results
	cli.fileCtrl.PrintSearchResults(results)
	return nil
}

// FileStatsCommand prints file statistics
func (cli *TagsCLI) FileStatsCommand() error {
	cli.fileCtrl.PrintFileStats()
	return nil
}

// FileExtensionCommand prints files with a specific extension
func (cli *TagsCLI) FileExtensionCommand(extension string) error {
	cli.fileCtrl.PrintFilesByExtension(extension)
	return nil
}

// FileKeywordCommand prints files containing a specific keyword
func (cli *TagsCLI) FileKeywordCommand(keyword string) error {
	cli.fileCtrl.PrintFilesByKeyword(keyword)
	return nil
}

// RunTagsCommand runs a specific tag-related command
func (cli *TagsCLI) RunTagsCommand(command string) error {
	logger.Debug("Processing command", "command", command)

	// Tags commands
	if strings.HasPrefix(command, "tag:") {
		tagName := util.TrimPrefix(command, "tag:")
		logger.Info("Running tag command", "tag", tagName)
		cli.PrintTagInfo(tagName)
		return nil
	}

	if strings.HasPrefix(command, "file:") {
		filePath := util.TrimPrefix(command, "file:")
		logger.Info("Running file command", "file", filePath)
		cli.PrintTagsByFile(filePath)
		return nil
	}

	if strings.HasPrefix(command, "kind:") {
		kind := util.TrimPrefix(command, "kind:")
		logger.Info("Running kind command", "kind", kind)
		cli.PrintTagsByKind(kind)
		return nil
	}

	if command == "kinds" {
		logger.Info("Running kinds command")
		cli.PrintAvailableKinds()
		return nil
	}

	// File utility commands
	if strings.HasPrefix(command, "search:") {
		pattern := util.TrimPrefix(command, "search:")
		logger.Info("Running file search command", "pattern", pattern)
		return cli.FileSearchCommand(pattern, false, false)
	}

	if strings.HasPrefix(command, "regex:") {
		pattern := util.TrimPrefix(command, "regex:")
		logger.Info("Running regex search command", "pattern", pattern)
		return cli.FileSearchCommand(pattern, true, false)
	}

	if strings.HasPrefix(command, "ext:") {
		ext := util.TrimPrefix(command, "ext:")
		logger.Info("Running extension search command", "ext", ext)
		return cli.FileExtensionCommand(ext)
	}

	if strings.HasPrefix(command, "keyword:") {
		keyword := util.TrimPrefix(command, "keyword:")
		logger.Info("Running keyword search command", "keyword", keyword)
		return cli.FileKeywordCommand(keyword)
	}

	if command == "stats" {
		logger.Info("Running file stats command")
		return cli.FileStatsCommand()
	}

	// If it's not a command, try to load it as a tags file
	logger.Info("Treating command as tags file path", "path", command)
	return cli.LoadTagsFile(command)
}

// PrintHelp prints available commands
func (cli *TagsCLI) PrintHelp() {
	logger.Debug("Displaying help")
	
	fmt.Println("\nCtags Commands:")
	fmt.Println("  <tags_file>   - Load a ctags file")
	fmt.Println("  tag:<n>       - Show info for a specific tag")
	fmt.Println("  file:<path>   - Show all tags in a file")
	fmt.Println("  kind:<kind>   - Show all tags of a specific kind")
	fmt.Println("  kinds         - List all available kinds")
	
	fmt.Println("\nFile Utility Commands:")
	fmt.Println("  search:<text> - Search for files containing text")
	fmt.Println("  regex:<expr>  - Search for files using regex")
	fmt.Println("  ext:<ext>     - Find files with a specific extension")
	fmt.Println("  keyword:<key> - Find files containing a keyword")
	fmt.Println("  stats         - Show file indexing statistics")
	fmt.Println()
}
