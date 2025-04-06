package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Config holds the logger configuration
type Config struct {
	Level      slog.Level
	FilePath   string
	JSONFormat bool
	Silent     bool // If true, suppresses logs to stderr when no file is specified
}

var (
	defaultConfig = Config{
		Level:      slog.LevelInfo,
		FilePath:   "",
		JSONFormat: false,
		Silent:     false,
	}

	logFile *os.File
	logger  *slog.Logger
	mu      sync.Mutex
)

// nullWriter is an io.Writer that discards all writes
type nullWriter struct{}

func (nw nullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil // pretend we wrote everything successfully
}

// Setup initializes the logging system with the provided configuration
func Setup(cfg Config) error {
	mu.Lock()
	defer mu.Unlock()

	// Close any existing log file
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			return fmt.Errorf("failed to close previous log file: %w", err)
		}
		logFile = nil
	}

	var writer io.Writer

	// If FilePath is specified, use file for logging
	if cfg.FilePath != "" {
		// Ensure directory exists
		dir := filepath.Dir(cfg.FilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}

		// Open log file
		file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}

		writer = file
		logFile = file
	} else if cfg.Silent {
		// Use null writer if silent mode is requested
		writer = nullWriter{}
	} else {
		// Default to stderr only if no file is specified and not in silent mode
		writer = os.Stderr
	}

	// Create handler based on format preference
	var handler slog.Handler
	if cfg.JSONFormat {
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
			Level: cfg.Level,
		})
	} else {
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{
			Level: cfg.Level,
		})
	}

	// Create the logger
	logger = slog.New(handler)
	slog.SetDefault(logger)

	// Only log initialization if we're not using a null writer
	if _, isNull := writer.(nullWriter); !isNull {
		logger.Info("Logger initialized",
			"level", cfg.Level.String(),
			"file", cfg.FilePath,
			"format", map[bool]string{true: "JSON", false: "Text"}[cfg.JSONFormat],
		)
	}

	return nil
}

// SetupDefault initializes the logging system with default configuration
func SetupDefault() {
	if err := Setup(defaultConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup default logger: %v\n", err)
	}
}

// SetupFile configures logging to write only to the specified file
func SetupFile(filePath string, level slog.Level, jsonFormat bool) error {
	return Setup(Config{
		Level:      level,
		FilePath:   filePath,
		JSONFormat: jsonFormat,
		Silent:     true, // Suppress stderr output
	})
}

// Debug logs a debug message
func Debug(msg string, args ...any) {
	ensureLoggerInitialized()
	logger.Debug(msg, args...)
}

// Info logs an info message
func Info(msg string, args ...any) {
	ensureLoggerInitialized()
	logger.Info(msg, args...)
}

// Warn logs a warning message
func Warn(msg string, args ...any) {
	ensureLoggerInitialized()
	logger.Warn(msg, args...)
}

// Error logs an error message
func Error(msg string, args ...any) {
	ensureLoggerInitialized()
	logger.Error(msg, args...)
}

// LogWithContext logs a message with context
func LogWithContext(ctx context.Context, level slog.Level, msg string, args ...any) {
	ensureLoggerInitialized()
	logger.Log(ctx, level, msg, args...)
}

// GetLogger returns the current logger instance
func GetLogger() *slog.Logger {
	ensureLoggerInitialized()
	return logger
}

// Close closes the logger resources, particularly the log file if one is open
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		return logFile.Close()
	}
	return nil
}

// ChangeLogFile changes the log file while preserving the current logger's settings
func ChangeLogFile(filePath string, silent bool) error {
	mu.Lock()
	defer mu.Unlock()

	// If no logger exists yet, just set up with defaults
	if logger == nil {
		cfg := defaultConfig
		cfg.FilePath = filePath
		cfg.Silent = silent
		return Setup(cfg)
	}

	// Get the current handler to determine format and level
	var isJSON bool
	var level slog.Level

	oldHandler := logger.Handler()
	if jsonHandler, ok := oldHandler.(*slog.JSONHandler); ok {
		isJSON = true
		level = slog.LevelDebug
		jsonHandler.Enabled(context.Background(), level)
	} else if textHandler, ok := oldHandler.(*slog.TextHandler); ok {
		isJSON = false
		level = slog.LevelDebug
		textHandler.Enabled(context.Background(), level)
	} else {
		// Default values if we can't determine
		isJSON = false
		level = slog.LevelInfo
	}

	// Close any existing log file
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			return fmt.Errorf("failed to close previous log file: %w", err)
		}
		logFile = nil
	}

	// Determine writer based on file path and silent flag
	var writer io.Writer

	if filePath != "" {
		// Ensure directory exists
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}

		// Open log file
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}

		writer = file
		logFile = file
	} else if silent {
		writer = nullWriter{}
	} else {
		writer = os.Stderr
	}

	// Create handler with same format as before
	var handler slog.Handler
	if isJSON {
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{
			Level: level,
		})
	}

	// Create and set the new logger
	logger = slog.New(handler)
	slog.SetDefault(logger)

	// Only log the change if we're not using a null writer
	if _, isNull := writer.(nullWriter); !isNull {
		logger.Info("Log file changed", "file", filePath)
	}

	return nil
}

// ensureLoggerInitialized makes sure we have a logger, creating a default one if needed
func ensureLoggerInitialized() {
	mu.Lock()
	defer mu.Unlock()

	if logger == nil {
		// Default to stderr with text format
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
		slog.SetDefault(logger)
	}
}
