package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/term"
)

// ANSI escape sequences for handling terminal input
const (
	escapeChar = '\x1b'
	upArrow    = "[A"
	downArrow  = "[B"
	clearLine  = "\r\033[K" // Carriage return + clear line
	backspace  = '\x7f'     // DEL character
	ctrlC      = '\x03'
)

// InputReader provides an interface for reading user input
type InputReader interface {
	ReadLine() (string, error)
	Close() error
}

// SimpleInput implements InputReader with basic functionality
type SimpleInput struct {
	prompt       string
	reader       *bufio.Reader
	historyFile  string
	history      []string
	historyIndex int
	mu           sync.Mutex  // For concurrent history access
	rawMode      bool        // Whether terminal is in raw mode
	fd           int         // File descriptor for terminal
	oldState     *term.State // Original terminal state
}

// NewReadlineInput creates a new input reader with history support
// This is a replacement for the github.com/chzyer/readline based implementation
func NewReadlineInput(prompt string, historyFile string) (*SimpleInput, error) {
	input := &SimpleInput{
		prompt:       prompt,
		reader:       bufio.NewReader(os.Stdin),
		historyFile:  historyFile,
		history:      make([]string, 0, 100),
		historyIndex: -1,
		fd:           int(os.Stdin.Fd()),
	}

	// Check if stdin is a terminal
	if term.IsTerminal(input.fd) {
		// It's a terminal, we can enable raw mode for arrow key input
		input.rawMode = true
	}

	// Load history from file if it exists
	if historyFile != "" {
		input.loadHistory()
	}

	return input, nil
}

// ReadLine reads a line of input from the terminal with arrow key support for history
func (s *SimpleInput) ReadLine() (string, error) {
	if !s.rawMode {
		// Fall back to simple line reading if not a terminal
		return s.readLineSimple()
	}

	// Enter raw mode
	oldState, err := term.MakeRaw(s.fd)
	if err != nil {
		// If raw mode fails, fall back to simple line reading
		return s.readLineSimple()
	}
	s.oldState = oldState
	defer term.Restore(s.fd, oldState)

	// Print prompt
	fmt.Print(s.prompt)

	var buf bytes.Buffer
	currentPos := 0   // Current cursor position
	currentLine := "" // Current input line

	// Start history at the end
	s.mu.Lock()
	s.historyIndex = len(s.history)
	s.mu.Unlock()

	for {
		r, size, err := s.reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				return "", io.EOF
			}
			return "", err
		}

		// Process input
		switch {
		case r == '\r' || r == '\n':
			// Enter pressed, return the line
			fmt.Print("\r\n") // Move to next line

			// Get the final line
			line := buf.String()

			// Add to history if non-empty and not duplicate
			if line != "" {
				s.addToHistory(line)
			}

			return line, nil

		case r == backspace:
			// Backspace pressed, remove last character if possible
			if currentPos > 0 {
				currentPos--
				line := buf.String()
				newLine := line[:currentPos] + line[currentPos+1:]
				buf.Reset()
				buf.WriteString(newLine)

				// Redraw the line
				fmt.Print(clearLine)
				fmt.Print(s.prompt)
				fmt.Print(buf.String())

				// Move cursor to correct position
				if currentPos < buf.Len() {
					// Move cursor back from end of line to current position
					fmt.Printf("\033[%dD", buf.Len()-currentPos)
				}
			}

		case r == ctrlC:
			// Ctrl+C pressed, terminate
			fmt.Print("^C\r\n")
			return "", io.EOF

		case r == escapeChar:
			// Could be an arrow key
			next, _, err := s.reader.ReadRune()
			if err != nil || next != '[' {
				continue
			}

			direction, _, err := s.reader.ReadRune()
			if err != nil {
				continue
			}

			s.mu.Lock()
			historyLen := len(s.history)
			s.mu.Unlock()

			// Handle arrow keys for history
			switch string([]rune{next, direction}) {
			case upArrow:
				// Up arrow - show previous history item
				if historyLen > 0 && s.historyIndex > 0 {
					s.historyIndex--

					// Get history item
					s.mu.Lock()
					historyItem := s.history[s.historyIndex]
					s.mu.Unlock()

					// Replace current input with history item
					buf.Reset()
					buf.WriteString(historyItem)
					currentPos = buf.Len()

					// Redraw the line
					fmt.Print(clearLine)
					fmt.Print(s.prompt)
					fmt.Print(historyItem)
				}

			case downArrow:
				// Down arrow - show next history item
				if historyLen > 0 && s.historyIndex < historyLen-1 {
					s.historyIndex++

					// Get history item
					s.mu.Lock()
					historyItem := s.history[s.historyIndex]
					s.mu.Unlock()

					// Replace current input with history item
					buf.Reset()
					buf.WriteString(historyItem)
					currentPos = buf.Len()

					// Redraw the line
					fmt.Print(clearLine)
					fmt.Print(s.prompt)
					fmt.Print(historyItem)
				} else if s.historyIndex == historyLen-1 {
					// At the end of history, show empty line
					s.historyIndex = historyLen
					buf.Reset()
					currentPos = 0

					// Redraw the line
					fmt.Print(clearLine)
					fmt.Print(s.prompt)
				}
			}

		default:
			// Regular character input
			if size > 0 {
				// Insert at current position
				currentLine = buf.String()
				newLine := currentLine[:currentPos] + string(r) + currentLine[currentPos:]
				buf.Reset()
				buf.WriteString(newLine)
				currentPos++

				// Redraw the line
				fmt.Print(clearLine)
				fmt.Print(s.prompt)
				fmt.Print(buf.String())

				// Move cursor to correct position
				if currentPos < buf.Len() {
					// Move cursor back from end of line to current position
					fmt.Printf("\033[%dD", buf.Len()-currentPos)
				}
			}
		}
	}
}

// readLineSimple provides a fallback method for reading input without raw mode
func (s *SimpleInput) readLineSimple() (string, error) {
	// Print the prompt
	fmt.Print(s.prompt)

	// Read the line
	line, err := s.reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return "", io.EOF
		}
		return "", err
	}

	// Trim leading/trailing whitespace including the newline
	line = strings.TrimSpace(line)

	// Add to history if non-empty and not a duplicate of the last entry
	if line != "" {
		s.addToHistory(line)
	}

	return line, nil
}

// Close cleans up resources
func (s *SimpleInput) Close() error {
	// Restore terminal state if needed
	if s.rawMode && s.oldState != nil {
		term.Restore(s.fd, s.oldState)
	}

	// Save history
	if s.historyFile != "" {
		s.saveHistory()
	}
	return nil
}

// loadHistory loads command history from the history file
func (s *SimpleInput) loadHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.historyFile == "" {
		return
	}

	f, err := os.Open(s.historyFile)
	if err != nil {
		// File might not exist yet, which is fine
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			s.history = append(s.history, line)
		}
	}
}

// saveHistory saves command history to the history file
func (s *SimpleInput) saveHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.historyFile == "" {
		return nil
	}

	// Ensure the directory exists
	dir := filepath.Dir(s.historyFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(s.historyFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// Only save up to the last 500 commands
	startIdx := 0
	if len(s.history) > 500 {
		startIdx = len(s.history) - 500
	}

	for i := startIdx; i < len(s.history); i++ {
		if _, err := fmt.Fprintln(f, s.history[i]); err != nil {
			return err
		}
	}

	return nil
}

// addToHistory adds a command to the history, avoiding duplicates
func (s *SimpleInput) addToHistory(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Avoid consecutive duplicates
	if len(s.history) > 0 && s.history[len(s.history)-1] == line {
		return
	}

	s.history = append(s.history, line)
	s.historyIndex = len(s.history)

	// If we have a history file, periodically save history
	// Only save every 10 commands to avoid excessive writes
	if s.historyFile != "" && len(s.history)%10 == 0 {
		go s.saveHistory() // Save asynchronously
	}
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
