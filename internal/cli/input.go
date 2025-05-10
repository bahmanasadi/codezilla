package cli

import (
	"io"
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
func NewReadlineInput(prompt string) (*ReadlineInput, error) {
	// Configure readline with appropriate settings
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt,
		HistoryFile:     "", // We'll manage history in memory
		HistoryLimit:    100,
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
			r.instance.SaveHistory(line)
		}
	}

	return line, nil
}

// Close closes the readline instance
func (r *ReadlineInput) Close() error {
	return r.instance.Close()
}
