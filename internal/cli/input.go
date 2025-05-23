package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ANSI escape sequences
const (
	escapeChar  = '\x1b'
	backspace   = '\x7f'
	ctrlA       = '\x01'
	ctrlB       = '\x02'
	ctrlC       = '\x03'
	ctrlD       = '\x04'
	ctrlE       = '\x05'
	ctrlF       = '\x06'
	ctrlK       = '\x0b'
	ctrlL       = '\x0c'
	ctrlU       = '\x15'
	ctrlW       = '\x17'
	tab         = '\x09'
	newline     = '\n'
	carriageRet = '\r'
)

// InputReader provides an interface for reading user input
type InputReader interface {
	ReadLine() (string, error)
	Close() error
}

// SimpleInput implements InputReader with bash-like functionality
type SimpleInput struct {
	prompt       string
	reader       *bufio.Reader
	historyFile  string
	history      []string
	historyIndex int
	mu           sync.Mutex
	rawMode      bool
	fd           int
	oldState     *term.State
	width        int
	height       int
}

// NewReadlineInput creates a new input reader with history support
func NewReadlineInput(prompt string, historyFile string) (*SimpleInput, error) {
	fd := int(os.Stdin.Fd())

	// Get terminal dimensions
	width, height, err := term.GetSize(fd)
	if err != nil {
		width = 80  // Default width
		height = 24 // Default height
	}

	input := &SimpleInput{
		prompt:       prompt,
		reader:       bufio.NewReader(os.Stdin),
		historyFile:  historyFile,
		history:      make([]string, 0, 100),
		historyIndex: -1,
		fd:           fd,
		width:        width,
		height:       height,
	}

	// Check if stdin is a terminal
	if term.IsTerminal(input.fd) {
		input.rawMode = true
		go input.watchTerminalSize()
	}

	// Load history from file
	if historyFile != "" {
		input.loadHistory()
	}

	return input, nil
}

// watchTerminalSize monitors for terminal size changes
func (s *SimpleInput) watchTerminalSize() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		if width, height, err := term.GetSize(s.fd); err == nil {
			s.mu.Lock()
			s.width = width
			s.height = height
			s.mu.Unlock()
		}
	}
}

// State for tracking previous display
var (
	lastLineCount int = 0
)

// refresh redraws the current line
func (s *SimpleInput) refresh(buffer []rune, pos int) {
	// Get current terminal width
	s.mu.Lock()
	width := s.width
	s.mu.Unlock()

	// Calculate total content length
	promptRunes := []rune(s.prompt)
	promptLen := len(promptRunes)
	display := append(promptRunes, buffer...)

	// Clear previous display
	if lastLineCount > 0 {
		// Move to start of input
		for i := 1; i < lastLineCount; i++ {
			fmt.Print("\033[A") // Move up
		}
		fmt.Print("\r")

		// Clear all lines
		for i := 0; i < lastLineCount; i++ {
			fmt.Print("\033[K") // Clear line
			if i < lastLineCount-1 {
				fmt.Print("\n")
			}
		}

		// Move back to start
		for i := 1; i < lastLineCount; i++ {
			fmt.Print("\033[A")
		}
		fmt.Print("\r")
	} else {
		// First display - just move to start
		fmt.Print("\r")
		fmt.Print("\033[K")
	}

	// Calculate new line count
	totalLen := len(display)
	newLineCount := (totalLen + width - 1) / width
	if newLineCount == 0 {
		newLineCount = 1
	}

	// Print content
	for i := 0; i < totalLen; i++ {
		fmt.Print(string(display[i]))
	}

	// Calculate cursor position
	absolutePos := promptLen + pos
	cursorLine := absolutePos / width
	cursorCol := absolutePos % width

	// Current position is at end
	currentLine := (totalLen - 1) / width

	// Move cursor to correct position
	if currentLine > cursorLine {
		fmt.Printf("\033[%dA", currentLine-cursorLine)
	}

	fmt.Print("\r")
	if cursorCol > 0 {
		fmt.Printf("\033[%dC", cursorCol)
	}

	// Remember state for next refresh
	lastLineCount = newLineCount
}

// ReadLine reads a line of input from the terminal with bash-like behavior
func (s *SimpleInput) ReadLine() (string, error) {
	if !s.rawMode {
		return s.readLineSimple()
	}

	// Enter raw mode
	oldState, err := term.MakeRaw(s.fd)
	if err != nil {
		return s.readLineSimple()
	}
	s.oldState = oldState
	defer func() {
		if err := term.Restore(s.fd, oldState); err != nil {
			// Log error but don't return it as we're in a deferred function
			fmt.Fprintf(os.Stderr, "Failed to restore terminal: %v\n", err)
		}
	}()

	// Reset display state
	lastLineCount = 0

	// Initialize
	var buffer []rune
	pos := 0

	// History state
	s.mu.Lock()
	s.historyIndex = len(s.history)
	historySize := len(s.history)
	s.mu.Unlock()

	savedLine := ""
	inHistory := false

	// Initial display
	s.refresh(buffer, pos)

	// Main input loop
	for {
		// Read a single rune
		r, _, err := s.reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				fmt.Print("\r\n")
				return "", io.EOF
			}
			return "", err
		}

		switch r {
		case carriageRet, newline:
			// Enter pressed
			fmt.Print("\r\n")
			line := string(buffer)
			if line != "" {
				s.addToHistory(line)
			}
			return line, nil

		case ctrlC:
			// Ctrl-C
			fmt.Print("^C\r\n")
			return "", io.EOF

		case ctrlD:
			// Ctrl-D - EOF if buffer empty, delete char otherwise
			if len(buffer) == 0 {
				fmt.Print("\r\n")
				return "", io.EOF
			}
			if pos < len(buffer) {
				buffer = append(buffer[:pos], buffer[pos+1:]...)
				s.refresh(buffer, pos)
			}

		case ctrlA:
			// Move to beginning of line
			pos = 0
			s.refresh(buffer, pos)

		case ctrlE:
			// Move to end of line
			pos = len(buffer)
			s.refresh(buffer, pos)

		case ctrlB:
			// Move backward one character (same as left arrow)
			if pos > 0 {
				pos--
				s.refresh(buffer, pos)
			}

		case ctrlF:
			// Move forward one character (same as right arrow)
			if pos < len(buffer) {
				pos++
				s.refresh(buffer, pos)
			}

		case ctrlK:
			// Kill to end of line
			if pos < len(buffer) {
				buffer = buffer[:pos]
				s.refresh(buffer, pos)
			}

		case ctrlU:
			// Kill to beginning of line
			if pos > 0 {
				buffer = append(buffer[:0], buffer[pos:]...)
				pos = 0
				s.refresh(buffer, pos)
			}

		case ctrlW:
			// Delete word backward
			if pos > 0 {
				// Find start of word
				start := pos
				// Skip trailing spaces
				for start > 0 && buffer[start-1] == ' ' {
					start--
				}
				// Skip word chars
				for start > 0 && buffer[start-1] != ' ' {
					start--
				}
				// Delete from start to pos
				buffer = append(buffer[:start], buffer[pos:]...)
				pos = start
				s.refresh(buffer, pos)
			}

		case ctrlL:
			// Clear screen
			fmt.Print("\033[2J\033[H")
			s.refresh(buffer, pos)

		case backspace, 0x08:
			// Backspace
			if pos > 0 {
				buffer = append(buffer[:pos-1], buffer[pos:]...)
				pos--
				s.refresh(buffer, pos)
			}

		case tab:
			// Tab - insert tab character
			buffer = append(buffer[:pos], append([]rune{'\t'}, buffer[pos:]...)...)
			pos++
			s.refresh(buffer, pos)

		case escapeChar:
			// Handle escape sequences
			seq := s.readEscapeSequence()
			s.handleEscapeSequence(seq, &buffer, &pos, &savedLine, &inHistory, historySize)

		default:
			// Regular character
			if r >= 32 && r < 0x7f {
				// Insert character at position
				buffer = append(buffer[:pos], append([]rune{r}, buffer[pos:]...)...)
				pos++
				s.refresh(buffer, pos)
			}
		}
	}
}

// readEscapeSequence reads an ANSI escape sequence
func (s *SimpleInput) readEscapeSequence() string {
	var seq []rune

	// Read [
	r, _, err := s.reader.ReadRune()
	if err != nil || r != '[' {
		return ""
	}

	// Read until we get a letter
	for {
		r, _, err := s.reader.ReadRune()
		if err != nil {
			return ""
		}
		seq = append(seq, r)
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '~' {
			break
		}
	}

	return string(seq)
}

// handleEscapeSequence handles ANSI escape sequences
func (s *SimpleInput) handleEscapeSequence(seq string, buffer *[]rune, pos *int, savedLine *string, inHistory *bool, historySize int) {
	switch seq {
	case "D": // Left arrow
		if *pos > 0 {
			(*pos)--
			s.refresh(*buffer, *pos)
		}

	case "C": // Right arrow
		if *pos < len(*buffer) {
			(*pos)++
			s.refresh(*buffer, *pos)
		}

	case "A": // Up arrow - previous history
		if historySize > 0 && s.historyIndex > 0 {
			// Save current line if first time
			if !*inHistory {
				*savedLine = string(*buffer)
				*inHistory = true
			}

			s.historyIndex--
			s.mu.Lock()
			*buffer = []rune(s.history[s.historyIndex])
			s.mu.Unlock()
			*pos = len(*buffer)
			s.refresh(*buffer, *pos)
		}

	case "B": // Down arrow - next history
		if *inHistory && s.historyIndex < historySize {
			s.historyIndex++

			if s.historyIndex == historySize {
				// Back to saved line
				*buffer = []rune(*savedLine)
				*inHistory = false
			} else {
				s.mu.Lock()
				*buffer = []rune(s.history[s.historyIndex])
				s.mu.Unlock()
			}
			*pos = len(*buffer)
			s.refresh(*buffer, *pos)
		}

	case "H", "1~": // Home
		*pos = 0
		s.refresh(*buffer, *pos)

	case "F", "4~": // End
		*pos = len(*buffer)
		s.refresh(*buffer, *pos)

	case "3~": // Delete
		if *pos < len(*buffer) {
			*buffer = append((*buffer)[:*pos], (*buffer)[*pos+1:]...)
			s.refresh(*buffer, *pos)
		}

	case "1;5C": // Ctrl+Right - word forward
		if *pos < len(*buffer) {
			// Skip current word
			for *pos < len(*buffer) && (*buffer)[*pos] != ' ' {
				(*pos)++
			}
			// Skip spaces
			for *pos < len(*buffer) && (*buffer)[*pos] == ' ' {
				(*pos)++
			}
			s.refresh(*buffer, *pos)
		}

	case "1;5D": // Ctrl+Left - word backward
		if *pos > 0 {
			// Skip spaces
			for *pos > 0 && (*buffer)[*pos-1] == ' ' {
				(*pos)--
			}
			// Skip word
			for *pos > 0 && (*buffer)[*pos-1] != ' ' {
				(*pos)--
			}
			s.refresh(*buffer, *pos)
		}
	}
}

// readLineSimple provides a fallback for non-terminal input
func (s *SimpleInput) readLineSimple() (string, error) {
	fmt.Print(s.prompt)
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line != "" {
		s.addToHistory(line)
	}
	return line, nil
}

// Close cleans up resources
func (s *SimpleInput) Close() error {
	if s.rawMode && s.oldState != nil {
		if err := term.Restore(s.fd, s.oldState); err != nil {
			return fmt.Errorf("failed to restore terminal: %w", err)
		}
	}
	if s.historyFile != "" {
		if err := s.saveHistory(); err != nil {
			return fmt.Errorf("failed to save history: %w", err)
		}
	}
	return nil
}

// History management functions

func (s *SimpleInput) loadHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.historyFile)
	if err != nil {
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

func (s *SimpleInput) saveHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.historyFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(s.historyFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// Keep last 500 entries
	start := 0
	if len(s.history) > 500 {
		start = len(s.history) - 500
	}

	for i := start; i < len(s.history); i++ {
		fmt.Fprintln(f, s.history[i])
	}

	return nil
}

func (s *SimpleInput) addToHistory(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Skip duplicates
	if len(s.history) > 0 && s.history[len(s.history)-1] == line {
		return
	}

	s.history = append(s.history, line)
	s.historyIndex = len(s.history)

	go func() {
		if err := s.saveHistory(); err != nil {
			// Log error but don't block
			fmt.Fprintf(os.Stderr, "Failed to save history: %v\n", err)
		}
	}()
}

// GetDefaultHistoryFilePath returns the default path for the command history file
func GetDefaultHistoryFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".config", "codezilla")
	return filepath.Join(configDir, "history"), nil
}
