package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"codezilla/internal/cli"
	"codezilla/internal/ui"
	"codezilla/pkg/logger"

	"github.com/charmbracelet/bubbletea"
)

var (
	// Command line flags
	logFile     string
	logLevel    string
	logJSON     bool
	logSilent   bool
	showVersion bool
	versionInfo = "0.1.0" // This should be set during build
)

func init() {
	// Define command line flags
	flag.StringVar(&logFile, "log", "", "Path to log file (default: logs to stderr)")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.BoolVar(&logJSON, "log-json", false, "Use JSON format for logging")
	flag.BoolVar(&logSilent, "log-silent", false, "Suppress logging to stderr when a log file is specified")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	
	// Parse flags but don't handle commands yet
	flag.Parse()
	
	// Setup logging based on flags
	setupLogging()
}

func setupLogging() {
	// Determine log level
	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// If log file is specified, ensure directory exists
	if logFile != "" {
		// Get absolute path if not provided
		if !filepath.IsAbs(logFile) {
			dir, err := os.Getwd()
			if err == nil {
				logFile = filepath.Join(dir, logFile)
			}
		}
	}

	// Setup logger
	var err error
	if logFile != "" && logSilent {
		// Use file-only logging if both file path and silent flag are specified
		err = logger.SetupFile(logFile, level, logJSON)
	} else {
		// Use regular setup with the silent flag
		cfg := logger.Config{
			Level:      level,
			FilePath:   logFile,
			JSONFormat: logJSON,
			Silent:     logSilent,
		}
		err = logger.Setup(cfg)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up logger: %v\n", err)
		os.Exit(1)
	}

	// Log initial message (this will be suppressed in stderr if silent mode)
	logger.Info("Codezilla starting", "version", versionInfo)
}

func main() {
	// Ensure we clean up logger on exit
	defer logger.Close()

	// Handle version flag first
	if showVersion {
		fmt.Printf("Codezilla version %s\n", versionInfo)
		return
	}

	// Get remaining arguments (excluding flags)
	args := flag.Args()

	// If command-line arguments are provided, run in CLI mode
	if len(args) > 0 {
		runCLI(args)
		return
	}

	// Otherwise, run in TUI mode
	runTUI()
}

func runCLI(args []string) {
	// Create CLI instance
	tagsCLI := cli.New()

	// Process the command
	command := args[0]
	logger.Debug("Running CLI command", "command", command)
	
	if err := tagsCLI.RunTagsCommand(command); err != nil {
		logger.Error("Command failed", "error", err)
		fmt.Printf("Error: %v\n", err)
		tagsCLI.PrintHelp()
		os.Exit(1)
	}
}

func runTUI() {
	logger.Info("Starting TUI mode")
	
	// Print a welcome message
	fmt.Println("Welcome to Codezilla File Utilities!")
	fmt.Println("Indexing files in the current directory...")
	fmt.Println("Use 'help' command to see available options.")
	fmt.Println("Press Ctrl+Enter to process commands.")
	
	// Create a new program with alt screen mode and mouse support
	p := tea.NewProgram(
		ui.NewModel(),
		// Uncomment these for alt screen mode and mouse support
		// tea.WithAltScreen(),
		// tea.WithMouseCellMotion(),
	)
	
	if _, err := p.Run(); err != nil {
		logger.Error("TUI failed", "error", err)
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
