package tools

import (
	"regexp"
	"strings"
)

// CodeBlock represents a code block found in markdown
type CodeBlock struct {
	Language string `json:"language"`
	Content  string `json:"content"`
	LineNum  int    `json:"line_number"`
	IsShell  bool   `json:"is_shell"`
}

// MarkdownAnalysis represents the analysis of a markdown file
type MarkdownAnalysis struct {
	HasCodeBlocks    bool        `json:"has_code_blocks"`
	HasShellCommands bool        `json:"has_shell_commands"`
	CodeBlocks       []CodeBlock `json:"code_blocks"`
	ShellCommands    []string    `json:"shell_commands"`
	TotalLines       int         `json:"total_lines"`
}

// ShellLanguages defines languages that typically contain shell commands
var ShellLanguages = map[string]bool{
	"bash":       true,
	"sh":         true,
	"shell":      true,
	"zsh":        true,
	"fish":       true,
	"powershell": true,
	"ps1":        true,
	"cmd":        true,
	"bat":        true,
	"console":    true,
	"terminal":   true,
}

// AnalyzeMarkdown analyzes markdown content for code blocks and shell commands
func AnalyzeMarkdown(content string) *MarkdownAnalysis {
	analysis := &MarkdownAnalysis{
		CodeBlocks:    make([]CodeBlock, 0),
		ShellCommands: make([]string, 0),
	}

	lines := strings.Split(content, "\n")
	analysis.TotalLines = len(lines)

	// Regular expressions for different code block formats
	fencedCodePattern := regexp.MustCompile("^```([a-zA-Z0-9_+-]*)(.*)$")
	indentedCodePattern := regexp.MustCompile("^    (.+)$")

	var currentBlock *CodeBlock
	var inCodeBlock bool

	for i, line := range lines {
		lineNum := i + 1

		// Check for fenced code blocks (```language)
		if matches := fencedCodePattern.FindStringSubmatch(line); matches != nil {
			if !inCodeBlock {
				// Starting a new code block
				language := strings.ToLower(strings.TrimSpace(matches[1]))
				currentBlock = &CodeBlock{
					Language: language,
					Content:  "",
					LineNum:  lineNum,
					IsShell:  isShellLanguage(language),
				}
				inCodeBlock = true
			} else {
				// Ending current code block
				if currentBlock != nil {
					currentBlock.Content = strings.TrimSpace(currentBlock.Content)
					if currentBlock.Content != "" {
						analysis.CodeBlocks = append(analysis.CodeBlocks, *currentBlock)
						analysis.HasCodeBlocks = true

						if currentBlock.IsShell {
							analysis.HasShellCommands = true
							analysis.ShellCommands = append(analysis.ShellCommands,
								extractShellCommands(currentBlock.Content)...)
						}
					}
				}
				inCodeBlock = false
				currentBlock = nil
			}
		} else if inCodeBlock && currentBlock != nil {
			// Add line to current code block
			if currentBlock.Content != "" {
				currentBlock.Content += "\n"
			}
			currentBlock.Content += line
		} else if !inCodeBlock {
			// Check for indented code blocks (4 spaces)
			if matches := indentedCodePattern.FindStringSubmatch(line); matches != nil {
				// This is an indented code block line
				codeContent := matches[1]

				// Check if it looks like shell commands
				if looksLikeShellCommand(codeContent) {
					block := CodeBlock{
						Language: "shell",
						Content:  codeContent,
						LineNum:  lineNum,
						IsShell:  true,
					}
					analysis.CodeBlocks = append(analysis.CodeBlocks, block)
					analysis.HasCodeBlocks = true
					analysis.HasShellCommands = true
					analysis.ShellCommands = append(analysis.ShellCommands, codeContent)
				}
			}
		}
	}

	// Handle case where file ends while in a code block
	if inCodeBlock && currentBlock != nil {
		currentBlock.Content = strings.TrimSpace(currentBlock.Content)
		if currentBlock.Content != "" {
			analysis.CodeBlocks = append(analysis.CodeBlocks, *currentBlock)
			analysis.HasCodeBlocks = true

			if currentBlock.IsShell {
				analysis.HasShellCommands = true
				analysis.ShellCommands = append(analysis.ShellCommands,
					extractShellCommands(currentBlock.Content)...)
			}
		}
	}

	return analysis
}

// isShellLanguage checks if a language identifier represents a shell language
func isShellLanguage(language string) bool {
	if language == "" {
		return false
	}
	return ShellLanguages[strings.ToLower(language)]
}

// looksLikeShellCommand uses heuristics to detect if a line looks like a shell command
func looksLikeShellCommand(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}

	// Common shell command patterns
	shellPatterns := []string{
		"^[$#] ",    // Prompt indicators
		"^sudo ",    // sudo commands
		"^cd ",      // cd commands
		"^ls ",      // ls commands
		"^mkdir ",   // mkdir commands
		"^rm ",      // rm commands
		"^cp ",      // cp commands
		"^mv ",      // mv commands
		"^chmod ",   // chmod commands
		"^chown ",   // chown commands
		"^grep ",    // grep commands
		"^find ",    // find commands
		"^curl ",    // curl commands
		"^wget ",    // wget commands
		"^git ",     // git commands
		"^npm ",     // npm commands
		"^yarn ",    // yarn commands
		"^docker ",  // docker commands
		"^kubectl ", // kubectl commands
		"^make ",    // make commands
		"^go ",      // go commands
		"^python ",  // python commands
		"^pip ",     // pip commands
		"^apt ",     // apt commands
		"^yum ",     // yum commands
		"^brew ",    // brew commands
	}

	for _, pattern := range shellPatterns {
		matched, _ := regexp.MatchString(pattern, line)
		if matched {
			return true
		}
	}

	// Check for pipe operators and redirections
	if strings.Contains(line, " | ") || strings.Contains(line, " > ") ||
		strings.Contains(line, " >> ") || strings.Contains(line, " < ") {
		return true
	}

	// Check for environment variable assignments
	envVarPattern := regexp.MustCompile(`^[A-Z_][A-Z0-9_]*=`)
	return envVarPattern.MatchString(line)
}

// extractShellCommands extracts individual shell commands from a code block
func extractShellCommands(content string) []string {
	var commands []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		// Remove prompt indicators
		line = regexp.MustCompile(`^[$#] `).ReplaceAllString(line, "")

		if line != "" {
			commands = append(commands, line)
		}
	}

	return commands
}

// GetMarkdownMetadata returns enhanced metadata for markdown files
func GetMarkdownMetadata(content string, filePath string) map[string]interface{} {
	analysis := AnalyzeMarkdown(content)

	metadata := map[string]interface{}{
		"file_type":           "markdown",
		"has_code_blocks":     analysis.HasCodeBlocks,
		"has_shell_commands":  analysis.HasShellCommands,
		"total_lines":         analysis.TotalLines,
		"code_block_count":    len(analysis.CodeBlocks),
		"shell_command_count": len(analysis.ShellCommands),
	}

	if analysis.HasCodeBlocks {
		// Add language summary
		languageCounts := make(map[string]int)
		for _, block := range analysis.CodeBlocks {
			if block.Language != "" {
				languageCounts[block.Language]++
			} else {
				languageCounts["unknown"]++
			}
		}
		metadata["languages"] = languageCounts
	}

	if analysis.HasShellCommands {
		metadata["shell_commands"] = analysis.ShellCommands

		// Categorize commands
		commandTypes := categorizeShellCommands(analysis.ShellCommands)
		if len(commandTypes) > 0 {
			metadata["command_types"] = commandTypes
		}
	}

	return metadata
}

// categorizeShellCommands categorizes shell commands by type
func categorizeShellCommands(commands []string) map[string][]string {
	categories := map[string][]string{
		"file_operations":  make([]string, 0),
		"package_managers": make([]string, 0),
		"version_control":  make([]string, 0),
		"network":          make([]string, 0),
		"build_tools":      make([]string, 0),
		"containers":       make([]string, 0),
		"system":           make([]string, 0),
		"other":            make([]string, 0),
	}

	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}

		// Get the first word (command name)
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			continue
		}

		command := parts[0]

		switch command {
		case "ls", "cd", "mkdir", "rm", "cp", "mv", "chmod", "chown", "find", "grep", "cat", "tail", "head":
			categories["file_operations"] = append(categories["file_operations"], cmd)
		case "npm", "yarn", "pip", "apt", "yum", "brew", "composer":
			categories["package_managers"] = append(categories["package_managers"], cmd)
		case "git", "svn", "hg":
			categories["version_control"] = append(categories["version_control"], cmd)
		case "curl", "wget", "ssh", "scp", "rsync":
			categories["network"] = append(categories["network"], cmd)
		case "make", "cmake", "go", "cargo", "mvn", "gradle", "ant":
			categories["build_tools"] = append(categories["build_tools"], cmd)
		case "docker", "podman", "kubectl", "helm":
			categories["containers"] = append(categories["containers"], cmd)
		case "ps", "top", "kill", "systemctl", "service", "crontab", "sudo":
			categories["system"] = append(categories["system"], cmd)
		default:
			categories["other"] = append(categories["other"], cmd)
		}
	}

	// Remove empty categories
	for category, commands := range categories {
		if len(commands) == 0 {
			delete(categories, category)
		}
	}

	return categories
}
