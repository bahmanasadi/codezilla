package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codezilla/internal/cli"
)

var (
	// Command line flags
	configFile string
	logFile    string
	logLevel   string
	logSilent  bool
	modelName  string
	ollamaURL  string
	noColor    bool
	forceColor bool
)

func init() {
	// Define command line flags
	flag.StringVar(&configFile, "config", filepath.Join(getConfigDir(), "config.json"), "Path to config file")
	flag.StringVar(&logFile, "log", "", "Path to log file (overrides config)")
	flag.StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error (overrides config)")
	flag.BoolVar(&logSilent, "log-silent", false, "Disable console logging (overrides config)")
	flag.StringVar(&modelName, "model", "", "Model name to use (overrides config)")
	flag.StringVar(&ollamaURL, "ollama-url", "", "Ollama API URL (overrides config)")
	flag.BoolVar(&noColor, "no-color", false, "Disable colorized output (useful for GoLand's emulated terminal)")
	flag.BoolVar(&forceColor, "force-color", false, "Force colorized output even in non-terminal environments")
}

func main() {
	// Parse command line flags
	flag.Parse()

	// Load configuration
	config, err := cli.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Override config with command line flags if provided
	if logFile != "" {
		config.LogFile = logFile
	}
	if logLevel != "" {
		config.LogLevel = logLevel
	}
	// If log file is provided, we want to set silent mode by default
	// to ensure logs go only to the file
	if logFile != "" {
		config.LogSilent = true
	}

	// But if logSilent flag was explicitly set, honor that value
	if logSilent {
		config.LogSilent = true
	}
	if modelName != "" {
		config.DefaultModel = modelName
	}
	if ollamaURL != "" {
		config.OllamaURL = ollamaURL
	}

	// Handle color-related flags
	if noColor {
		// Set environment variable for the style package
		os.Setenv("CODEZILLA_NO_COLOR", "true")
	}
	if forceColor {
		os.Setenv("FORCE_COLOR", "true")
	}

	// Create and run the application
	app, err := cli.NewApp(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating application: %v\n", err)
		os.Exit(1)
	}

	// Create a context that will be canceled on interrupt
	ctx := context.Background()

	// Run the application
	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}
}

// getConfigDir returns the directory for configuration files
func getConfigDir() string {
	// Get user config directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		// Fall back to current directory
		return "./config"
	}

	// Use application-specific subdirectory
	return filepath.Join(configDir, "codezilla")
}
