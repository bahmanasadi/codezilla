package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration
type Config struct {
	// Model configuration
	DefaultModel string  `json:"default_model"`
	OllamaURL    string  `json:"ollama_url"`
	Temperature  float32 `json:"temperature"`
	MaxTokens    int     `json:"max_tokens"`
	SystemPrompt string  `json:"system_prompt"`

	// Authentication
	OllamaAPIKey   string            `json:"ollama_api_key,omitempty"`
	OllamaAuthType string            `json:"ollama_auth_type,omitempty"` // "bearer", "basic", or "custom"
	OllamaUsername string            `json:"ollama_username,omitempty"`
	OllamaPassword string            `json:"ollama_password,omitempty"`
	OllamaHeaders  map[string]string `json:"ollama_headers,omitempty"`

	// Log configuration
	LogFile   string `json:"log_file"`
	LogLevel  string `json:"log_level"`
	LogSilent bool   `json:"log_silent"`

	// Context management
	RetainContext   bool   `json:"retain_context"`
	MaxContextChars int    `json:"max_context_chars"`
	HistoryFile     string `json:"history_file"`

	// Permission settings
	DangerousToolsWarn  bool              `json:"dangerous_tools_warn"`
	AlwaysAskPermission bool              `json:"always_ask_permission"`
	ToolPermissions     map[string]string `json:"tool_permissions"`

	// UI settings
	ForceColor bool `json:"force_color"`
	NoColor    bool `json:"no_color"`

	// Working directory
	WorkingDirectory string `json:"working_directory"`

	// Analyzer settings
	AnalyzerSettings AnalyzerSettings `json:"analyzer_settings"`
}

// AnalyzerSettings contains configuration for the file analyzer
type AnalyzerSettings struct {
	UseLLM             bool    `json:"use_llm"`             // Use LLM for file analysis
	Concurrency        int     `json:"concurrency"`         // Number of files to analyze concurrently
	RelevanceThreshold float64 `json:"relevance_threshold"` // Minimum relevance score
	AnalysisTimeout    int     `json:"analysis_timeout"`    // Timeout per file in seconds
	MaxFileSize        int64   `json:"max_file_size"`       // Maximum file size to analyze
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	systemPrompt := fmt.Sprintf(`You are Codezilla, a helpful AI assistant powered by Ollama. You have access to various tools that allow you to interact with the local system, read and write files, execute commands, and more.

Current working directory: %s

When the user refers to "the project", "this project", "search", or uses relative paths, assume they mean the current working directory and its contents. Always strive to be helpful, accurate, and safe in your responses.`, cwd)

	return &Config{
		DefaultModel:        "qwen3:14b",
		OllamaURL:           "http://localhost:11434/api",
		Temperature:         0.7,
		MaxTokens:           1024 * 32,
		SystemPrompt:        systemPrompt,
		LogFile:             filepath.Join("logs", "codezilla.log"),
		LogLevel:            "info",
		LogSilent:           false,
		RetainContext:       true,
		MaxContextChars:     50000,
		HistoryFile:         filepath.Join(getConfigDir(), "history"),
		DangerousToolsWarn:  true,
		AlwaysAskPermission: false,
		ToolPermissions: map[string]string{
			"fileRead":            "never_ask",
			"fileReadBatch":       "never_ask",
			"listFiles":           "never_ask",
			"projectScan":         "never_ask",
			"projectScanAnalyzer": "never_ask",
			"fileWrite":           "always_ask",
			"execute":             "always_ask",
		},
		ForceColor:       false,
		NoColor:          false,
		WorkingDirectory: cwd,
		AnalyzerSettings: AnalyzerSettings{
			UseLLM:             true,
			Concurrency:        5,
			RelevanceThreshold: 0.3,
			AnalysisTimeout:    30,
			MaxFileSize:        1024 * 1024, // 1MB
		},
	}
}

// LoadConfig loads configuration from a file
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()

	// If path doesn't exist, return default config
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return config, nil
	}

	// Read the config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Ensure tool permissions map is initialized
	if config.ToolPermissions == nil {
		config.ToolPermissions = make(map[string]string)
	}

	// Always use current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	config.WorkingDirectory = cwd

	// Update system prompt with current working directory
	config.SystemPrompt = fmt.Sprintf(`You are Codezilla, a helpful AI assistant powered by Ollama. You have access to various tools that allow you to interact with the local system, read and write files, execute commands, and more.

Current working directory: %s

When the user refers to "the project", "this project", "search", or uses relative paths, assume they mean the current working directory and its contents. Always strive to be helpful, accurate, and safe in your responses.`, cwd)

	// Check environment variables for authentication (these override config file)
	if apiKey := os.Getenv("OLLAMA_API_KEY"); apiKey != "" {
		config.OllamaAPIKey = apiKey
		if config.OllamaAuthType == "" {
			config.OllamaAuthType = "bearer"
		}
	}
	if username := os.Getenv("OLLAMA_USERNAME"); username != "" {
		config.OllamaUsername = username
		config.OllamaAuthType = "basic"
	}
	if password := os.Getenv("OLLAMA_PASSWORD"); password != "" {
		config.OllamaPassword = password
	}
	if baseURL := os.Getenv("OLLAMA_BASE_URL"); baseURL != "" {
		config.OllamaURL = baseURL
	}

	return config, nil
}

// SaveConfig saves configuration to a file
func SaveConfig(config *Config, path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file with secure permissions
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
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
