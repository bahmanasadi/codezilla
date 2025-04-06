package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/term"

	"codezilla/pkg/fsutil"
	"codezilla/pkg/logger"
	"codezilla/pkg/style"
)

// runPlayground starts the terminal file exploration playground
func main() {
	// Initialize logger
	err := logger.Setup(logger.Config{
		Level:    slog.LevelDebug,
		FilePath: "./logs/playground.log",
		Silent:   false,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\r\n", err)
	}

	// Initialize file controller with default config
	controller := fsutil.NewController(fsutil.DefaultControllerConfig())
	if err := controller.Initialize(); err != nil {
		logger.Error("Failed to initialize file controller", "error", err)
		fmt.Printf("%sError: Failed to initialize file controller: %v%s\r\n", style.Red, err, style.Reset)
		os.Exit(1)
	}

	// Set terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		logger.Error("Failed to set terminal to raw mode", "error", err)
		panic(err)
	}
	defer func() {
		if err := term.Restore(int(os.Stdin.Fd()), oldState); err != nil {
			logger.Error("Failed to restore terminal state", "error", err)
		}
	}()

	// Handle Ctrl+C gracefully
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		if err := term.Restore(int(os.Stdin.Fd()), oldState); err != nil {
			logger.Error("Failed to restore terminal state on signal", "error", err)
		}
		controller.Stop()
		fmt.Println("\r\nStopping playground...")
		os.Exit(0)
	}()

	// Input state
	input := ""
	commandHistory := []string{}
	currentHistoryIdx := -1
	buf := make([]byte, 1)

	// Print help
	printHelp()
	printPrompt()

	// Wait for indexing to complete
	time.Sleep(500 * time.Millisecond) // Give indexer a moment to start
	for {
		stats := controller.GetFileStats()
		if !stats["isIndexing"].(bool) {
			break
		}
		fmt.Print("\r\033[KIndexing files, please wait...")
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Print("\r\033[K") // Clear the line
	printPrompt()         // Re-print prompt

	var n int
	for {
		if n, err = os.Stdin.Read(buf); err != nil {
			logger.Error("Failed to read from stdin", "error", err)
			break
		}
		if n == 0 {
			continue
		}

		b := buf[0]

		switch b {
		case 3: // Ctrl+C
			fmt.Printf("\r\nExiting...")
			controller.Stop()
			return

		case 13: // Enter
			// Clear line and process command
			fmt.Print("\033[2K") // Clear the line
			fmt.Print("\r")      // Move cursor to start of line

			// Process command
			if input != "" {
				// Add to history
				commandHistory = append(commandHistory, input)
				currentHistoryIdx = len(commandHistory)

				// Process the command
				processCommand(input, controller)

				// Reset input for next command
				input = ""
			}

			printPrompt()

		case 9: // Tab - auto-complete
			// Simple file path completion
			if strings.Contains(input, " ") {
				parts := strings.Split(input, " ")
				lastPart := parts[len(parts)-1]

				if strings.Contains(lastPart, "/") || strings.Contains(lastPart, ".") {
					// This looks like a path, try to complete it
					matches, _ := filepath.Glob(lastPart + "*")
					if len(matches) == 1 {
						// Replace the last part with the match
						parts[len(parts)-1] = matches[0]
						newInput := strings.Join(parts, " ")

						// Clear current line
						for range input {
							fmt.Print("\b \b")
						}

						// Print the new input
						input = newInput
						fmt.Print(input)
					} else if len(matches) > 1 {
						// Show possible completions
						fmt.Print("\r\n")
						for _, match := range matches {
							fmt.Println(match)
						}
						fmt.Print("\r")
						printPrompt()
						fmt.Print(input)
					}
				}
			}

		case 127: // Backspace
			if len(input) > 0 {
				input = input[:len(input)-1]
				fmt.Print("\b \b")
			}

		case 27: // Escape sequence
			// Read the next two bytes
			escSeq := make([]byte, 2)
			var escN int
			if escN, err = os.Stdin.Read(escSeq); err != nil || escN != 2 {
				logger.Error("Failed to read escape sequence", "error", err, "bytes_read", escN)
				continue
			}

			if escSeq[0] == 91 { // '['
				switch escSeq[1] {
				case 65: // Up arrow
					if len(commandHistory) > 0 {
						if currentHistoryIdx > 0 {
							currentHistoryIdx--
						}

						// Clear current line
						fmt.Print("\r\033[K")
						printPrompt()

						// Print the history item
						input = commandHistory[currentHistoryIdx]
						fmt.Print(input)
					}

				case 66: // Down arrow
					if len(commandHistory) > 0 {
						if currentHistoryIdx < len(commandHistory)-1 {
							currentHistoryIdx++

							// Clear current line
							fmt.Print("\r\033[K")
							printPrompt()

							// Print the history item
							input = commandHistory[currentHistoryIdx]
							fmt.Print(input)
						} else if currentHistoryIdx == len(commandHistory)-1 {
							// At the end of history, clear the input
							currentHistoryIdx = len(commandHistory)

							// Clear current line
							fmt.Print("\r\033[K")
							printPrompt()

							input = ""
						}
					}

				case 67: // Right arrow
					// Not implemented

				case 68: // Left arrow
					// Not implemented
				}
			}

		default:
			// Add to input if printable character
			if b >= 32 && b <= 126 {
				input += string(b)
				fmt.Print(string(b))
			}
		}
	}
}

func printPrompt() {
	fmt.Printf("%s> %s", style.Green, style.Reset)
}

func printHelp() {
	fmt.Printf("\n%sCodezilla Playground%s\r\n", style.Bold, style.Reset)
	fmt.Printf("Available commands:\r\n")
	fmt.Printf("  %sstats%s - Show file statistics\r\n", style.Yellow, style.Reset)
	fmt.Printf("  %sls [ext]%s - List files (optionally filtered by extension)\r\n", style.Yellow, style.Reset)
	fmt.Printf("  %sfind <keyword>%s - Find files containing a keyword\r\n", style.Yellow, style.Reset)
	fmt.Printf("  %scat <file>%s - Show file contents\r\n", style.Yellow, style.Reset)
	fmt.Printf("  %sdiff <file1> <file2>%s - Show differences between two files\r\n", style.Yellow, style.Reset)
	fmt.Printf("  %shelp%s - Show this help\r\n", style.Yellow, style.Reset)
	fmt.Printf("  %squit%s - Exit the playground\r\n", style.Yellow, style.Reset)
	fmt.Printf("\r\n")
}

// lineInfo represents a line with its diff type
type lineInfo struct {
	content  string
	diffType diffmatchpatch.Operation
	lineNum  int
}

// printDiff displays a diff with limited context around changes
func printDiff(diffs []diffmatchpatch.Diff) {
	// Process diffs into lines with type information
	var allLines []lineInfo
	lineNum := 1

	// Extract lines from diffs
	for _, diff := range diffs {
		text := diff.Text
		lines := strings.Split(text, "\n")

		for i, line := range lines {
			// Skip empty last line from split
			if i == len(lines)-1 && line == "" {
				continue
			}

			allLines = append(allLines, lineInfo{
				content:  line,
				diffType: diff.Type,
				lineNum:  lineNum,
			})
			lineNum++
		}
	}

	// Print diffs with context
	const contextLines = 3 // Number of context lines before/after changes

	// First pass: find changed lines and their context
	var relevantLines = make(map[int]bool)
	for i, line := range allLines {
		if line.diffType != diffmatchpatch.DiffEqual {
			// Mark the changed line
			relevantLines[i] = true

			// Mark context lines before
			for j := i - contextLines; j < i; j++ {
				if j >= 0 {
					relevantLines[j] = true
				}
			}

			// Mark context lines after
			for j := i + 1; j <= i+contextLines && j < len(allLines); j++ {
				relevantLines[j] = true
			}
		}
	}

	// Second pass: print relevant lines with dividers between blocks
	var lastRelevant = -1
	for i, line := range allLines {
		if !relevantLines[i] {
			// If we just finished a block, print separator
			if lastRelevant == i-1 && lastRelevant >= 0 {
				skipped := 0
				for j := i; j < len(allLines); j++ {
					if relevantLines[j] {
						skipped = j - i
						break
					}
				}
				if skipped > 0 {
					fmt.Printf("%s... %d lines skipped ...%s\r\n", style.Blue, skipped, style.Reset)
				}
			}
			continue
		}

		// Print separator if needed
		if lastRelevant >= 0 && i-lastRelevant > 1 {
			fmt.Printf("%s... %d lines skipped ...%s\r\n", style.Blue, i-lastRelevant-1, style.Reset)
		}

		// Print the line
		switch line.diffType {
		case diffmatchpatch.DiffDelete:
			// Red for deletions
			fmt.Printf("%s%3d |%s %s%s%s\r\n",
				style.Red, line.lineNum, style.Reset,
				style.Red, line.content, style.Reset)
		case diffmatchpatch.DiffInsert:
			// Green for insertions
			fmt.Printf("%s%3d |%s %s%s%s\r\n",
				style.Green, line.lineNum, style.Reset,
				style.Green, line.content, style.Reset)
		case diffmatchpatch.DiffEqual:
			// Normal for unchanged lines
			fmt.Printf("%s%3d |%s %s\r\n",
				style.Blue, line.lineNum, style.Reset, line.content)
		}

		lastRelevant = i
	}
}

func processCommand(cmd string, controller *fsutil.Controller) {
	// Split command and arguments
	parts := strings.SplitN(cmd, " ", 2)
	command := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	fmt.Printf("\r\n") // Add newline after command

	switch command {
	case "stats":
		controller.PrintFileStats()

	case "ls":
		// Filter by extension if provided
		if args != "" {
			controller.PrintFilesByExtension(args)
		} else {
			// List all files
			stats := controller.GetFileStats()
			totalFiles := stats["totalFiles"].(int)
			fmt.Printf("%sTotal files indexed: %d%s\r\n", style.Bold, totalFiles, style.Reset)
			fmt.Println("Use 'ls <extension>' to filter by extension")

			// Show extension counts
			if extCounts, ok := stats["extensionCounts"].(map[string]int); ok && len(extCounts) > 0 {
				fmt.Printf("\r\n%sFiles by extension:%s\r\n", style.Bold, style.Reset)
				for ext, count := range extCounts {
					fmt.Printf("  %s: %d\r\n", ext, count)
				}
			}
		}

	case "find":
		if args == "" {
			fmt.Printf("%sError: Missing keyword argument%s\r\n", style.Red, style.Reset)
			fmt.Println("Usage: find <keyword>")
			break
		}
		controller.PrintFilesByKeyword(args)

	case "cat":
		if args == "" {
			fmt.Printf("%sError: Missing file path argument%s\r\n", style.Red, style.Reset)
			fmt.Println("Usage: cat <file_path>")
			break
		}

		// Get file content
		content, err := controller.GetFileContent(args)
		if err != nil {
			fmt.Printf("%sError: Failed to read file: %v%s\r\n", style.Red, err, style.Reset)
			break
		}

		// Print with line numbers
		fmt.Printf("%sFile: %s%s\r\n", style.Bold, args, style.Reset)
		fmt.Printf("%s%s lines%s\r\n", style.Blue, strconv.Itoa(len(content)), style.Reset)
		fmt.Printf("%s\r\n", strings.Repeat("-", 50))

		for i, line := range content {
			fmt.Printf("%s%3d |%s %s\r\n", style.Blue, i+1, style.Reset, line)
		}

	case "diff":
		// Split args into file1 and file2
		argParts := strings.SplitN(args, " ", 2)
		if len(argParts) < 2 {
			fmt.Printf("%sError: Missing file arguments%s\r\n", style.Red, style.Reset)
			fmt.Println("Usage: diff <file1> <file2>")
			break
		}

		file1 := argParts[0]
		file2 := argParts[1]

		// Get content of both files
		content1, err := controller.GetFileContent(file1)
		if err != nil {
			fmt.Printf("%sError: Failed to read file1: %v%s\r\n", style.Red, err, style.Reset)
			break
		}

		content2, err := controller.GetFileContent(file2)
		if err != nil {
			fmt.Printf("%sError: Failed to read file2: %v%s\r\n", style.Red, err, style.Reset)
			break
		}

		// Convert to single strings
		text1 := strings.Join(content1, "\n")
		text2 := strings.Join(content2, "\n")

		// Create diff
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(text1, text2, true)

		// Print diff header
		fmt.Printf("%sDiff: %s ↔ %s%s\r\n", style.Bold, file1, file2, style.Reset)
		fmt.Printf("%s\r\n", strings.Repeat("-", 50))

		// Print pretty diff
		printDiff(diffs)

	case "help":
		printHelp()

	case "quit", "exit":
		fmt.Printf("Exiting...\r\n")
		controller.Stop()
		os.Exit(0)

	default:
		fmt.Printf("%sUnknown command: %s%s\r\n", style.Red, command, style.Reset)
		fmt.Print("Type 'help' for a list of commands\r\n")
	}

	fmt.Print("\r\n") // Add newline after command output
}
