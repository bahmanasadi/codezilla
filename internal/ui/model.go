package ui

import (
	"fmt"
	"strings"
	"time"

	"codezilla/pkg/fileutil"
	"codezilla/pkg/logger"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	minTextareaHeight       = 1                      // Minimum height it will shrink to
	headerHeight            = 1                      // For the "Growing input..." header
	footerHeight            = 1                      // For status/help text or just spacing
	textareaVerticalPadding = 2                      // Combined top/bottom padding + border for the container
	textareaHorizPadding    = 2                      // Combined left/right padding + border for the container
	doubleEscTimeout        = 500 * time.Millisecond // Timeout for double-escape detection
)

// Helper function using lipgloss to calculate lines needed for text within a given width
func calculateLinesNeeded(text string, maxWidth int) int {
	if maxWidth <= 0 {
		return 1 // Avoid division by zero or invalid width
	}
	// Use a plain style, only defining width for calculation purposes
	style := lipgloss.NewStyle().Width(maxWidth)
	renderedText := style.Render(text)
	lines := strings.Split(renderedText, "\n")

	// If the text is empty, lipgloss renders "", split makes [""] (len 1)
	// If the text is not empty but fits on one line, lipgloss renders it, split makes [text] (len 1)
	// If the text wraps, lipgloss adds \n, split makes [line1, line2, ...]
	lineCount := len(lines)
	if text == "" { // Treat empty input as 1 line high for the component view
		return 1
	}
	return lineCount
}

// tickMsg is a message sent after the timer expires
type tickMsg struct{}

// EscPressed is a custom command to set a timer for double-escape detection
func EscPressed() tea.Cmd {
	return tea.Tick(doubleEscTimeout, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// Model represents the UI model
type Model struct {
	textarea        textarea.Model
	terminalWidth   int
	terminalHeight  int
	helpText        string
	lastEscTime     time.Time
	waitingForEsc   bool
	doubleEscStatus string
	fileController  *fileutil.Controller
	searchResults   []fileutil.SearchResult
	showingResults  bool
}

// NewModel creates a new UI model
func NewModel() Model {
	logger.Debug("Initializing UI model")

	ta := textarea.New()
	ta.Placeholder = "Type commands or search queries ..."
	ta.Focus()
	// We'll set width and height dynamically in Update/calculateLayout
	ta.ShowLineNumbers = false // Keep line numbers if you like them

	// Initial dimensions (will be updated immediately on WindowSizeMsg)
	initialWidth := 80
	initialHeight := minTextareaHeight

	ta.SetWidth(initialWidth - textareaHorizPadding) // Account for container padding/border
	ta.SetHeight(initialHeight)

	// Create file controller
	fileController := fileutil.NewController(fileutil.DefaultControllerConfig())
	
	// Initialize file controller (this will start indexing and watching files)
	err := fileController.Initialize()
	if err != nil {
		logger.Error("Failed to initialize file controller", "error", err)
	}

	model := Model{
		textarea:        ta,
		terminalWidth:   initialWidth,
		terminalHeight:  24, // Default terminal height assumption
		helpText:        "Ctrl+C or double-Escape to quit, Ctrl+Enter to process query",
		waitingForEsc:   false,
		doubleEscStatus: "",
		fileController:  fileController,
		searchResults:   []fileutil.SearchResult{},
		showingResults:  false,
	}

	logger.Debug("UI model initialized",
		"width", model.terminalWidth,
		"height", model.terminalHeight)

	return model
}

func (m Model) Init() tea.Cmd {
	logger.Debug("UI model init")
	return textarea.Blink
}

func (m Model) calculateLayout() {
	// Calculate the width for the textarea
	// We need to subtract the horizontal padding to account for borders and padding
	availableWidth := m.terminalWidth - textareaHorizPadding
	if availableWidth < 1 {
		availableWidth = 1
	}

	// Log width calculations for debugging
	logger.Debug("Width calculation",
		"terminalWidth", m.terminalWidth,
		"padding", textareaHorizPadding,
		"availableWidth", availableWidth)

	// Set the textarea width to fill the available space
	m.textarea.SetWidth(availableWidth)

	content := m.textarea.Value()
	neededLines := calculateLinesNeeded(content, m.textarea.Width()) // Use textarea's internal width

	desiredHeight := neededLines
	if desiredHeight < minTextareaHeight {
		desiredHeight = minTextareaHeight
	}

	maxPossibleHeight := m.terminalHeight - headerHeight - footerHeight - textareaVerticalPadding
	if maxPossibleHeight < minTextareaHeight {
		maxPossibleHeight = minTextareaHeight + 1 // Ensure it's at least the minimum
	}

	finalHeight := desiredHeight
	//if finalHeight > maxPossibleHeight {
	//	finalHeight = maxPossibleHeight
	//}

	previousHeight := m.textarea.Height()
	m.textarea.SetHeight(finalHeight)
	m.textarea.SetValue(m.textarea.Value())

	// Log layout calculations if height changed
	if previousHeight != finalHeight {
		logger.Debug("Layout recalculated",
			"terminalSize", fmt.Sprintf("%dx%d", m.terminalWidth, m.terminalHeight),
			"contentLength", len(content),
			"neededLines", neededLines,
			"previousHeight", previousHeight,
			"newHeight", finalHeight)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var taCmd tea.Cmd

	switch msg := msg.(type) {
	case tickMsg:
		// Timer expired, reset the double-escape detection state
		if m.waitingForEsc {
			m.waitingForEsc = false
			m.doubleEscStatus = ""
			logger.Debug("Double-escape timeout expired")
		}
		return m, nil

	case tea.WindowSizeMsg:
		oldWidth, oldHeight := m.terminalWidth, m.terminalHeight
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height

		logger.Debug("Window size changed",
			"from", fmt.Sprintf("%dx%d", oldWidth, oldHeight),
			"to", fmt.Sprintf("%dx%d", msg.Width, msg.Height))

		// Recalculate layout completely on resize
		m.calculateLayout()
		// Ensure viewport within textarea is adjusted if necessary
		// This helps if the resize made content scroll out of view.
		m.textarea.SetHeight(m.textarea.Height()) // Re-setting seems to trigger internal adjustments
		m.textarea.SetWidth(m.terminalWidth - textareaHorizPadding)

	case tea.KeyMsg:
		keyStr := msg.String()
		logger.Debug("Key pressed", "key", keyStr, "type", msg.Type, "ctrl", msg.Alt)

		// Handle Enter key with Ctrl modifier
		if msg.Type == tea.KeyEnter && msg.Alt == true {
			logger.Info("Ctrl+Enter pressed", "content_length", len(m.textarea.Value()))

			// Get the current content
			content := m.textarea.Value()

			// Process the query or command
			m.processQuery(content)

			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			logger.Info("User quit application with Ctrl+C")
			fmt.Println("Final content:\n", m.textarea.Value())
			return m, tea.Quit

		case tea.KeyEsc:
			// Handle double-escape detection
			now := time.Now()

			if m.waitingForEsc {
				// This is the second escape - quit
				logger.Info("User quit application with double-Escape",
					"time_between", now.Sub(m.lastEscTime).String())
				fmt.Println("Final content:\n", m.textarea.Value())
				return m, tea.Quit
			} else {
				// This is the first escape - start timer
				m.waitingForEsc = true
				m.lastEscTime = now
				m.doubleEscStatus = "Press Esc again to quit"
				logger.Debug("First Escape detected, waiting for second")

				// Return a command to reset the state after the timeout
				return m, EscPressed()
			}

		case tea.KeyEnter:
			// Regular Enter key (not with Ctrl already handled above)
			logger.Debug("Regular Enter pressed", "content_length", len(m.textarea.Value()))

		case tea.KeyRunes:
			if len(msg.Runes) > 1 {
				logger.Info("Paste detected", "characters", len(msg.Runes))
				m.textarea.SetValue(m.textarea.Value() + "Pasted value: " + string(msg.Runes))
				return m, nil
			}
		}

		// If any other key is pressed while waiting for double-escape, cancel the wait
		if m.waitingForEsc && msg.Type != tea.KeyEsc {
			m.waitingForEsc = false
			m.doubleEscStatus = ""
			logger.Debug("Double-escape cancelled by other key press", "key", keyStr)
		}
	}

	// Handle textarea updates (typing, backspace, paste, etc.)
	m.textarea, taCmd = m.textarea.Update(msg)
	cmds = append(cmds, taCmd)

	m.calculateLayout()

	return m, tea.Batch(cmds...)
}

// processQuery processes a query or command
func (m *Model) processQuery(query string) {
	logger.Info("Processing query", "query", query)
	
	// Trim whitespace
	query = strings.TrimSpace(query)
	
	// Skip empty queries
	if query == "" {
		return
	}
	
	// Handle commands
	if strings.HasPrefix(query, "search:") {
		pattern := strings.TrimPrefix(query, "search:")
		pattern = strings.TrimSpace(pattern)
		m.searchFiles(pattern, false, false)
		return
	}
	
	if strings.HasPrefix(query, "regex:") {
		pattern := strings.TrimPrefix(query, "regex:")
		pattern = strings.TrimSpace(pattern)
		m.searchFiles(pattern, true, false)
		return
	}
	
	if strings.HasPrefix(query, "ext:") {
		ext := strings.TrimPrefix(query, "ext:")
		ext = strings.TrimSpace(ext)
		m.findByExtension(ext)
		return
	}
	
	if strings.HasPrefix(query, "keyword:") {
		keyword := strings.TrimPrefix(query, "keyword:")
		keyword = strings.TrimSpace(keyword)
		m.findByKeyword(keyword)
		return
	}
	
	if query == "stats" {
		m.showStats()
		return
	}
	
	if query == "help" {
		m.showHelp()
		return
	}
	
	// Default to search
	m.searchFiles(query, false, false)
}

// searchFiles searches for files containing a pattern
func (m *Model) searchFiles(pattern string, isRegex bool, caseSensitive bool) {
	// Search files
	results, err := m.fileController.SearchFiles(pattern, isRegex, caseSensitive)
	if err != nil {
		m.textarea.SetValue(fmt.Sprintf("Error searching files: %v", err))
		return
	}
	
	// Save results
	m.searchResults = results
	m.showingResults = true
	
	// Format results
	var output strings.Builder
	
	if len(results) == 0 {
		output.WriteString("No matching files found\n")
	} else {
		output.WriteString(fmt.Sprintf("Found %d matches:\n\n", len(results)))
		
		// Group by file
		fileGroups := make(map[string][]fileutil.SearchResult)
		for _, result := range results {
			fileGroups[result.FilePath] = append(fileGroups[result.FilePath], result)
		}
		
		// Format results by file
		for filePath, matches := range fileGroups {
			output.WriteString(fmt.Sprintf("%s (%d matches):\n", filePath, len(matches)))
			
			for _, match := range matches {
				// Add line number and content
				output.WriteString(fmt.Sprintf("  Line %d: %s\n", match.Line, match.Content))
			}
			
			output.WriteString("\n")
		}
	}
	
	// Update textarea with results
	m.textarea.SetValue(output.String())
}

// findByExtension finds files with a specific extension
func (m *Model) findByExtension(extension string) {
	// Find files by extension
	files := m.fileController.FindFilesByExtension(extension)
	
	// Format results
	var output strings.Builder
	
	if len(files) == 0 {
		output.WriteString(fmt.Sprintf("No files found with extension '%s'\n", extension))
	} else {
		output.WriteString(fmt.Sprintf("Found %d files with extension '%s':\n\n", len(files), extension))
		
		for i, file := range files {
			output.WriteString(fmt.Sprintf("[%d] %s\n", i+1, file))
		}
	}
	
	// Update textarea with results
	m.textarea.SetValue(output.String())
}

// findByKeyword finds files containing a specific keyword
func (m *Model) findByKeyword(keyword string) {
	// Find files by keyword
	files := m.fileController.FindFilesByKeyword(keyword)
	
	// Format results
	var output strings.Builder
	
	if len(files) == 0 {
		output.WriteString(fmt.Sprintf("No files found containing '%s'\n", keyword))
	} else {
		output.WriteString(fmt.Sprintf("Found %d files containing '%s':\n\n", len(files), keyword))
		
		for i, file := range files {
			output.WriteString(fmt.Sprintf("[%d] %s\n", i+1, file))
		}
	}
	
	// Update textarea with results
	m.textarea.SetValue(output.String())
}

// showStats shows file index statistics
func (m *Model) showStats() {
	// Get file stats
	stats := m.fileController.GetFileStats()
	
	// Format results
	var output strings.Builder
	
	output.WriteString("File Index Statistics:\n\n")
	output.WriteString(fmt.Sprintf("Total files: %d\n", stats["totalFiles"]))
	output.WriteString(fmt.Sprintf("Total lines: %d\n", stats["totalLines"]))
	
	// Format size
	size := stats["totalSize"].(int64)
	output.WriteString(fmt.Sprintf("Total size: %s\n", formatSize(size)))
	
	output.WriteString(fmt.Sprintf("Directories: %d\n", stats["directoriesCount"]))
	output.WriteString(fmt.Sprintf("Extensions: %d\n", stats["extensionsCount"]))
	output.WriteString(fmt.Sprintf("Keywords indexed: %d\n", stats["keywordsIndexed"]))
	
	// Print indexing status
	if stats["isIndexing"].(bool) {
		output.WriteString("\nIndexing is in progress\n")
	} else {
		output.WriteString("\nIndexing is complete\n")
	}
	
	// Print extension stats
	if extCounts, ok := stats["extensionCounts"].(map[string]int); ok && len(extCounts) > 0 {
		output.WriteString("\nFiles by extension:\n")
		for ext, count := range extCounts {
			output.WriteString(fmt.Sprintf("  %s: %d\n", ext, count))
		}
	}
	
	// Update textarea with results
	m.textarea.SetValue(output.String())
}

// showHelp shows help information
func (m *Model) showHelp() {
	// Format help message
	var output strings.Builder
	
	output.WriteString("File Utility Commands:\n\n")
	output.WriteString("search:<text> - Search for files containing text\n")
	output.WriteString("regex:<expr>  - Search for files using regex\n")
	output.WriteString("ext:<ext>     - Find files with a specific extension\n")
	output.WriteString("keyword:<key> - Find files containing a keyword\n")
	output.WriteString("stats         - Show file indexing statistics\n")
	output.WriteString("help          - Show this help message\n\n")
	output.WriteString("Press Ctrl+Enter to execute commands\n")
	output.WriteString("Press Ctrl+C or double-Escape to quit\n")
	
	// Update textarea with help
	m.textarea.SetValue(output.String())
}

// formatSize formats a size in bytes to a human-readable format
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// View renders the UI
func (m Model) View() string {
	// Style for the container around the textarea
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")). // Brighter blue color for border
		Padding(0, 0). // Textarea component itself might have padding, container has 0
		// Width is handled by the terminalWidth ensuring it fills horizontally
		// Height is determined by the textarea's content via calculateLayout
		Width(m.terminalWidth - textareaHorizPadding). // Match textarea width + padding
		Height(m.textarea.Height() + textareaVerticalPadding - 3) // Match textarea height + padding/border
	// The -3 accounts for the border and padding

	// Render the textarea *inside* the styled container
	textareaView := containerStyle.Render(m.textarea.View())
	m.textarea.Focus()

	// Simple header and footer
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1).
		Render("File Utilities:")

	// Display the double-escape status if active, otherwise show the regular help text
	footerText := m.helpText
	if m.waitingForEsc {
		// Using a brighter, more vibrant color (bright magenta/pink) with bold for emphasis
		footerText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("213")). // Brighter pink color
			Bold(true).
			Render(m.doubleEscStatus)
	}

	footer := lipgloss.NewStyle().MarginTop(1).Render(footerText)

	// Combine parts - Use JoinVertical for potentially better alignment control
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		textareaView,
		footer,
	)
}