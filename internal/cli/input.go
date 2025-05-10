package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
)

// InputReader provides an interface for reading user input
type InputReader interface {
	ReadLine() (string, error)
	Close() error
}

// ReadlineInput implements InputReader using the chzyer/readline library
type ReadlineInput struct {
	instance *readline.Instance
	history  []string
}

// NewReadlineInput creates a new ReadlineInput
func NewReadlineInput(prompt string, historyFile string) (*ReadlineInput, error) {
	// Configure readline with appropriate settings
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt,
		HistoryFile:     historyFile, // Use a history file for persistent history
		HistoryLimit:    500,         // Allow more history entries
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		FuncGetWidth:    func() int { return 0 },
	})

	if err != nil {
		return nil, err
	}

	return &ReadlineInput{
		instance: rl,
		history:  make([]string, 0),
	}, nil
}

// ReadLine reads a line of input
func (r *ReadlineInput) ReadLine() (string, error) {
	line, err := r.instance.Readline()
	if err != nil {
		// Convert common errors to standard ones
		if err == readline.ErrInterrupt {
			return "", io.EOF
		}
		return "", err
	}

	// Trim whitespace
	line = strings.TrimSpace(line)

	// Add to history if non-empty and not a repeat of last command
	if line != "" {
		// Don't add duplicate consecutive commands
		if len(r.history) == 0 || r.history[len(r.history)-1] != line {
			r.history = append(r.history, line)

			// Save to history
			err := r.instance.SaveHistory(line)
			if err != nil {
				return "", err
			}
		}
	}

	return line, nil
}

// Close closes the readline instance
func (r *ReadlineInput) Close() error {
	return r.instance.Close()
}

// GetDefaultHistoryFilePath returns the default path for the command history file
func GetDefaultHistoryFilePath() (string, error) {
	// First, determine the user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Create the config directory if it doesn't exist
	configDir := filepath.Join(homeDir, ".config", "codezilla")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}

	// Return the history file path
	return filepath.Join(configDir, "history"), nil
}
