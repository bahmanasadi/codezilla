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
	"time"

	"golang.org/x/term"
)

// ANSI escape sequences for handling terminal input
const (
	escapeChar     = '\x1b'
	upArrow        = "[A"
	downArrow      = "[B"
	rightArrow     = "[C"
	leftArrow      = "[D"
	ctrlLeftArrow  = "1;5D"     // CTRL + Left Arrow
	ctrlRightArrow = "1;5C"     // CTRL + Right Arrow
	clearLine      = "\r\033[K" // Carriage return + clear line
	moveUp         = "\033[1A"  // Move cursor up one line
	clearDown      = "\033[J"   // Clear from cursor to end of screen
	backspace      = '\x7f'     // DEL character
	ctrlC          = '\x03'
	ctrlW          = '\x17' // Delete previous word
	ctrlU          = '\x15' // Clear entire line
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
	width        int         // Terminal width
}

// NewReadlineInput creates a new input reader with history support
// This is a replacement for the github.com/chzyer/readline based implementation
func NewReadlineInput(prompt string, historyFile string) (*SimpleInput, error) {
	fd := int(os.Stdin.Fd())

	// Get terminal width
	width, _, err := term.GetSize(fd)
	if err != nil {
		width = 80 // Default width if we can't get terminal size
	}

	input := &SimpleInput{
		prompt:       prompt,
		reader:       bufio.NewReader(os.Stdin),
		historyFile:  historyFile,
		history:      make([]string, 0, 100),
		historyIndex: -1,
		fd:           fd,
		width:        width,
	}

	// Check if stdin is a terminal
	if term.IsTerminal(input.fd) {
		// It's a terminal, we can enable raw mode for arrow key input
		input.rawMode = true

		// Start a goroutine to monitor terminal size changes
		go input.watchTerminalSize()
	}

	// Load history from file if it exists
	if historyFile != "" {
		input.loadHistory()
	}

	return input, nil
}

// watchTerminalSize monitors for terminal size changes
func (s *SimpleInput) watchTerminalSize() {
	// Check terminal size periodically
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		if width, _, err := term.GetSize(s.fd); err == nil {
			if width != s.width {
				s.mu.Lock()
				s.width = width
				s.mu.Unlock()
			}
		}
	}
}

// calculateLinePosition calculates the number of lines and cursor position
func (s *SimpleInput) calculateLinePosition(text string, cursorPos int) (int, int) {
	promptLen := len(s.prompt)
	totalLen := promptLen + len(text)
	promptLines := promptLen / s.width
	if promptLen%s.width > 0 {
		promptLines++
	}

	totalLines := totalLen / s.width
	if totalLen%s.width > 0 {
		totalLines++
	}

	cursorAbsolutePos := promptLen + cursorPos
	cursorLine := cursorAbsolutePos / s.width

	return totalLines, cursorLine
}

// clearAndRedraw clears multiline input and redraws it
func (s *SimpleInput) clearAndRedraw(text string, cursorPos int) {
	totalLines, cursorLine := s.calculateLinePosition(text, cursorPos)

	// Move cursor to the start of the input
	if cursorLine > 0 {
		fmt.Printf("\033[%dA", cursorLine)
	}
	fmt.Print("\r")

	// Clear all lines
	fmt.Print(clearLine)
	if totalLines > 1 {
		fmt.Print(clearDown)
	}

	// Redraw prompt and text
	fmt.Print(s.prompt)
	fmt.Print(text)

	// Position cursor
	if cursorPos < len(text) {
		endPos := len(s.prompt) + len(text)
		endLine := endPos / s.width
		endCol := endPos % s.width

		targetPos := len(s.prompt) + cursorPos
		targetLine := targetPos / s.width
		targetCol := targetPos % s.width

		// Move lines if needed
		if endLine > targetLine {
			fmt.Printf("\033[%dA", endLine-targetLine)
		}

		// Move columns
		if endCol > targetCol {
			fmt.Printf("\033[%dD", endCol-targetCol)
		}
	}
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

				// Use the new clearing mechanism for multiline support
				s.clearAndRedraw(buf.String(), currentPos)
			}

		case r == ctrlC:
			// Ctrl+C pressed, terminate
			fmt.Print("^C\r\n")
			return "", io.EOF

		case r == ctrlW:
			// Ctrl+W pressed, delete previous word
			if currentPos > 0 {
				line := buf.String()

				// Find the start of the previous word
				wordStart := currentPos

				// Skip any trailing spaces before cursor
				for wordStart > 0 && wordStart <= len(line) && (wordStart == len(line) || line[wordStart-1] == ' ') {
					wordStart--
				}

				// Find the beginning of the word
				for wordStart > 0 && line[wordStart-1] != ' ' {
					wordStart--
				}

				// Delete from word start to current position
				if wordStart < currentPos {
					newLine := line[:wordStart] + line[currentPos:]
					buf.Reset()
					buf.WriteString(newLine)
					currentPos = wordStart

					// Use the new clearing mechanism for multiline support
					s.clearAndRedraw(buf.String(), currentPos)
				}
			}

		case r == ctrlU:
			// Ctrl+U pressed, clear entire line
			buf.Reset()
			currentPos = 0

			// Use the new clearing mechanism for multiline support
			s.clearAndRedraw(buf.String(), currentPos)

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

			// Check for special escape sequences that have more characters
			if direction == '1' {
				// Could be a ctrl+arrow key
				separator, _, err := s.reader.ReadRune()
				if err != nil || separator != ';' {
					continue
				}

				ctrlKey, _, err := s.reader.ReadRune()
				if err != nil || ctrlKey != '5' {
					continue
				}

				arrowKey, _, err := s.reader.ReadRune()
				if err != nil {
					continue
				}

				sequence := fmt.Sprintf("%c;%c%c", direction, ctrlKey, arrowKey)

				switch sequence {
				case ctrlLeftArrow:
					// Ctrl+Left arrow - move cursor to beginning of previous word
					if currentPos > 0 {
						line := buf.String()

						// Start from current position and move left
						newPos := currentPos

						// Skip any spaces before the current position
						for newPos > 0 && newPos-1 < len(line) && line[newPos-1] == ' ' {
							newPos--
						}

						// Skip until we find a space or beginning of line
						for newPos > 0 && line[newPos-1] != ' ' {
							newPos--
						}

						if newPos != currentPos {
							currentPos = newPos
							s.clearAndRedraw(line, currentPos)
						}
					}
				case ctrlRightArrow:
					// Ctrl+Right arrow - move cursor to beginning of next word
					line := buf.String()
					if currentPos < len(line) {
						// Start from current position and move right
						newPos := currentPos

						// Skip until we find a space or end of line
						for newPos < len(line) && line[newPos] != ' ' {
							newPos++
						}

						// Skip any spaces
						for newPos < len(line) && line[newPos] == ' ' {
							newPos++
						}

						if newPos != currentPos {
							currentPos = newPos
							s.clearAndRedraw(line, currentPos)
						}
					}
				}
			} else {
				// Handle regular arrow keys for navigation and history
				switch string([]rune{next, direction}) {
				case leftArrow:
					// Left arrow - move cursor one character to the left
					if currentPos > 0 {
						currentPos--
						s.clearAndRedraw(buf.String(), currentPos)
					}

				case rightArrow:
					// Right arrow - move cursor one character to the right
					if currentPos < buf.Len() {
						currentPos++
						s.clearAndRedraw(buf.String(), currentPos)
					}

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

						// Use the new clearing mechanism for multiline support
						s.clearAndRedraw(historyItem, currentPos)
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

						// Use the new clearing mechanism for multiline support
						s.clearAndRedraw(historyItem, currentPos)
					} else if s.historyIndex == historyLen-1 {
						// At the end of history, show empty line
						s.historyIndex = historyLen
						buf.Reset()
						currentPos = 0

						// Use the new clearing mechanism for multiline support
						s.clearAndRedraw(buf.String(), currentPos)
					}
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

				// Use the new clearing mechanism for multiline support
				s.clearAndRedraw(buf.String(), currentPos)
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

	go s.saveHistory() // Save asynchronously
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
