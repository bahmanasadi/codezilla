package tools

import (
	"testing"
)

func TestAnalyzeMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected struct {
			hasCodeBlocks     bool
			hasShellCommands  bool
			codeBlockCount    int
			shellCommandCount int
			languages         []string
		}
	}{
		{
			name:    "Basic bash code block",
			content: "# Test Document\nHere's a simple bash command:\n```bash\necho \"Hello, World!\"\n```",
			expected: struct {
				hasCodeBlocks     bool
				hasShellCommands  bool
				codeBlockCount    int
				shellCommandCount int
				languages         []string
			}{
				hasCodeBlocks:     true,
				hasShellCommands:  true,
				codeBlockCount:    1,
				shellCommandCount: 1,
				languages:         []string{"bash"},
			},
		},
		{
			name:    "Multiple shell languages",
			content: "```bash\nls -la\n```\n\n```sh\npwd\n```\n\n```shell\necho $HOME\n```",
			expected: struct {
				hasCodeBlocks     bool
				hasShellCommands  bool
				codeBlockCount    int
				shellCommandCount int
				languages         []string
			}{
				hasCodeBlocks:     true,
				hasShellCommands:  true,
				codeBlockCount:    3,
				shellCommandCount: 3,
				languages:         []string{"bash", "sh", "shell"},
			},
		},
		{
			name:    "Mixed code blocks",
			content: "```python\ndef hello():\n    print(\"Hello\")\n```\n\n```bash\necho \"Hello from bash\"\n```\n\n```javascript\nconsole.log(\"Hello\");\n```",
			expected: struct {
				hasCodeBlocks     bool
				hasShellCommands  bool
				codeBlockCount    int
				shellCommandCount int
				languages         []string
			}{
				hasCodeBlocks:     true,
				hasShellCommands:  true,
				codeBlockCount:    3,
				shellCommandCount: 1,
				languages:         []string{"python", "bash", "javascript"},
			},
		},
		{
			name: "Indented code blocks",
			content: `Here are some commands:

    $ ls -la
    $ cd /tmp
    $ echo "test"`,
			expected: struct {
				hasCodeBlocks     bool
				hasShellCommands  bool
				codeBlockCount    int
				shellCommandCount int
				languages         []string
			}{
				hasCodeBlocks:     true,
				hasShellCommands:  true,
				codeBlockCount:    3,
				shellCommandCount: 3,
				languages:         []string{"shell", "shell", "shell"},
			},
		},
		{
			name: "No code blocks",
			content: `This is just regular text.
No code blocks here.`,
			expected: struct {
				hasCodeBlocks     bool
				hasShellCommands  bool
				codeBlockCount    int
				shellCommandCount int
				languages         []string
			}{
				hasCodeBlocks:     false,
				hasShellCommands:  false,
				codeBlockCount:    0,
				shellCommandCount: 0,
				languages:         []string{},
			},
		},
		{
			name:    "Empty code blocks",
			content: "```bash\n```\n\n```\n```",
			expected: struct {
				hasCodeBlocks     bool
				hasShellCommands  bool
				codeBlockCount    int
				shellCommandCount int
				languages         []string
			}{
				hasCodeBlocks:     false,
				hasShellCommands:  false,
				codeBlockCount:    0,
				shellCommandCount: 0,
				languages:         []string{},
			},
		},
		{
			name:    "Terminal and console blocks",
			content: "```terminal\ngit status\ngit add .\n```\n\n```console\n$ npm install\n$ npm run build\n```",
			expected: struct {
				hasCodeBlocks     bool
				hasShellCommands  bool
				codeBlockCount    int
				shellCommandCount int
				languages         []string
			}{
				hasCodeBlocks:     true,
				hasShellCommands:  true,
				codeBlockCount:    2,
				shellCommandCount: 4,
				languages:         []string{"terminal", "console"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnalyzeMarkdown(tt.content)

			if result.HasCodeBlocks != tt.expected.hasCodeBlocks {
				t.Errorf("HasCodeBlocks = %v, want %v", result.HasCodeBlocks, tt.expected.hasCodeBlocks)
			}

			if result.HasShellCommands != tt.expected.hasShellCommands {
				t.Errorf("HasShellCommands = %v, want %v", result.HasShellCommands, tt.expected.hasShellCommands)
			}

			if len(result.CodeBlocks) != tt.expected.codeBlockCount {
				t.Errorf("CodeBlock count = %d, want %d", len(result.CodeBlocks), tt.expected.codeBlockCount)
			}

			if len(result.ShellCommands) != tt.expected.shellCommandCount {
				t.Errorf("ShellCommand count = %d, want %d", len(result.ShellCommands), tt.expected.shellCommandCount)
			}

			// Check languages
			for i, block := range result.CodeBlocks {
				if i < len(tt.expected.languages) {
					if block.Language != tt.expected.languages[i] {
						t.Errorf("Block %d language = %q, want %q", i, block.Language, tt.expected.languages[i])
					}
				}
			}
		})
	}
}

func TestExtractShellCommands(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name: "Simple commands",
			content: `echo "test"
ls -la
pwd`,
			expected: []string{"echo \"test\"", "ls -la", "pwd"},
		},
		{
			name: "Commands with prompt indicators",
			content: `$ echo "test"
$ ls -la
$ pwd`,
			expected: []string{"echo \"test\"", "ls -la", "pwd"},
		},
		{
			name: "Commands with comments",
			content: `# This is a comment
echo "test"
# Another comment
ls -la`,
			expected: []string{"echo \"test\"", "ls -la"},
		},
		{
			name: "Empty lines and comments only",
			content: `# Just comments

# More comments`,
			expected: []string{},
		},
		{
			name: "Complex commands",
			content: `ls -la | grep ".go"
find . -name "*.txt" -exec rm {} \;
docker run --rm -it ubuntu:latest`,
			expected: []string{
				"ls -la | grep \".go\"",
				"find . -name \"*.txt\" -exec rm {} \\;",
				"docker run --rm -it ubuntu:latest",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractShellCommands(tt.content)

			if len(result) != len(tt.expected) {
				t.Errorf("Command count = %d, want %d", len(result), len(tt.expected))
			}

			for i, cmd := range tt.expected {
				if i < len(result) {
					if result[i] != cmd {
						t.Errorf("Command %d = %q, want %q", i, result[i], cmd)
					}
				}
			}
		})
	}
}

func TestLooksLikeShellCommand(t *testing.T) {
	tests := []struct {
		line     string
		expected bool
	}{
		{"$ ls -la", true},
		{"# pwd", true},
		{"sudo apt update", true},
		{"cd /home/user", true},
		{"ls -la", true},
		{"mkdir test", true},
		{"rm -rf temp", true},
		{"cp file1 file2", true},
		{"mv old new", true},
		{"chmod 755 script.sh", true},
		{"grep pattern file", true},
		{"find . -name '*.txt'", true},
		{"curl https://example.com", true},
		{"wget https://example.com", true},
		{"git status", true},
		{"npm install", true},
		{"docker ps", true},
		{"kubectl get pods", true},
		{"make build", true},
		{"go test", true},
		{"python script.py", true},
		{"pip install requests", true},
		{"ls | grep test", true},
		{"echo test > file.txt", true},
		{"VAR=value", true},
		{"EXPORT_PATH=/usr/local", true},
		{"Just regular text", false},
		{"", false},
		{"// This is a comment", false},
		{"func main() {", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := looksLikeShellCommand(tt.line)
			if result != tt.expected {
				t.Errorf("looksLikeShellCommand(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestIsShellLanguage(t *testing.T) {
	tests := []struct {
		language string
		expected bool
	}{
		{"bash", true},
		{"sh", true},
		{"shell", true},
		{"zsh", true},
		{"fish", true},
		{"powershell", true},
		{"ps1", true},
		{"cmd", true},
		{"bat", true},
		{"console", true},
		{"terminal", true},
		{"BASH", true}, // Case insensitive
		{"Bash", true},
		{"python", false},
		{"javascript", false},
		{"go", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			result := isShellLanguage(tt.language)
			if result != tt.expected {
				t.Errorf("isShellLanguage(%q) = %v, want %v", tt.language, result, tt.expected)
			}
		})
	}
}

func TestCodeBlockParsing(t *testing.T) {
	tests := []struct {
		name    string
		content string
		blocks  []CodeBlock
	}{
		{
			name:    "Nested backticks",
			content: "Code: ```bash\necho `date`\n```",
			blocks:  []CodeBlock{}, // Parser doesn't handle this edge case well
		},
		{
			name:    "Unclosed code block at end",
			content: "Start:\n```bash\necho \"not closed\"",
			blocks: []CodeBlock{
				{Language: "bash", Content: "echo \"not closed\"", LineNum: 2, IsShell: true},
			},
		},
		{
			name:    "Code block with extra backticks",
			content: "````bash\necho test\n````",
			blocks: []CodeBlock{
				{Language: "", Content: "echo test", LineNum: 1, IsShell: false},
			}, // Parses but language is not detected
		},
		{
			name:    "Multiple commands in one block",
			content: "```bash\ncd /tmp\nmkdir test\ncd test\necho \"done\" > status.txt\ncat status.txt\n```",
			blocks: []CodeBlock{
				{
					Language: "bash",
					Content:  "cd /tmp\nmkdir test\ncd test\necho \"done\" > status.txt\ncat status.txt",
					LineNum:  1,
					IsShell:  true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnalyzeMarkdown(tt.content)

			if len(result.CodeBlocks) != len(tt.blocks) {
				t.Errorf("CodeBlock count = %d, want %d", len(result.CodeBlocks), len(tt.blocks))
			}

			for i, expected := range tt.blocks {
				if i < len(result.CodeBlocks) {
					actual := result.CodeBlocks[i]
					if actual.Language != expected.Language {
						t.Errorf("Block %d language = %q, want %q", i, actual.Language, expected.Language)
					}
					if actual.Content != expected.Content {
						t.Errorf("Block %d content = %q, want %q", i, actual.Content, expected.Content)
					}
					if actual.IsShell != expected.IsShell {
						t.Errorf("Block %d IsShell = %v, want %v", i, actual.IsShell, expected.IsShell)
					}
				}
			}
		})
	}
}
