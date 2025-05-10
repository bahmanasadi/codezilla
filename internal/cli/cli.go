package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"codezilla/internal/agent"
	"codezilla/internal/tools"
	"codezilla/llm/ollama"
	"codezilla/pkg/logger"
	"codezilla/pkg/style"
)

// Config contains configuration for the CLI application
type Config struct {
	OllamaURL    string  `json:"ollama_url"`
	DefaultModel string  `json:"default_model"`
	SystemPrompt string  `json:"system_prompt"`
	LogFile      string  `json:"log_file"`
	LogLevel     string  `json:"log_level"`
	LogSilent    bool    `json:"log_silent"`
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"max_tokens"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		OllamaURL:    "http://localhost:11434/api",
		DefaultModel: "qwen2.5-coder:3b",
		SystemPrompt: defaultSystemPrompt,
		LogFile:      "logs/codezilla.log",
		LogLevel:     "info",
		LogSilent:    false,
		Temperature:  0.7,
		MaxTokens:    4000,
	}
}

// LoadConfig loads configuration from a file
func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	config := DefaultConfig()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return config, nil
}

// SaveConfig saves the configuration to a file
func SaveConfig(config *Config, path string) error {
	// Ensure directory exists
	dir := strings.TrimSuffix(path, "/"+strings.SplitN(path, "/", -1)[len(strings.SplitN(path, "/", -1))-1])
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	return nil
}

// App represents the CLI application
type App struct {
	Agent        agent.Agent
	ToolRegistry tools.ToolRegistry
	LLMClient    ollama.Client
	Logger       *logger.Logger
	Config       *Config
	Reader       *bufio.Reader
	Writer       io.Writer
}

// NewApp creates a new CLI application with the given configuration
func NewApp(config *Config) (*App, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Set up logger
	loggerConfig := logger.Config{
		LogFile:  config.LogFile,
		LogLevel: config.LogLevel,
		Silent:   config.LogSilent,
	}

	log, err := logger.New(loggerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Create Ollama client
	ollamaClient := ollama.NewClient(ollama.WithBaseURL(config.OllamaURL))

	// Create tool registry
	toolRegistry := tools.NewToolRegistry()

	// Register default tools
	registerDefaultTools(toolRegistry)

	// Create agent
	agentConfig := &agent.Config{
		Model:        config.DefaultModel,
		MaxTokens:    config.MaxTokens,
		Temperature:  config.Temperature,
		SystemPrompt: config.SystemPrompt,
		OllamaURL:    config.OllamaURL,
		ToolRegistry: toolRegistry,
		Logger:       log,
	}

	agent := agent.NewAgent(agentConfig)

	return &App{
		Agent:        agent,
		ToolRegistry: toolRegistry,
		LLMClient:    ollamaClient,
		Logger:       log,
		Config:       config,
		Reader:       bufio.NewReader(os.Stdin),
		Writer:       os.Stdout,
	}, nil
}

// Run starts the CLI application
func (a *App) Run(ctx context.Context) error {
	// Set up signal handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalCh
		a.Logger.Info("Received interrupt signal, shutting down...")
		cancel()
	}()

	// Print welcome message
	a.printWelcome()

	// Main loop
	for {
		select {
		case <-ctx.Done():
			a.printGoodbye()
			return nil
		default:
			// Prompt for input
			fmt.Fprint(a.Writer, style.ColorBold(style.ColorCodeBlue, "\nuser> "))

			// Read input line
			input, err := a.Reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					a.printGoodbye()
					return nil
				}
				return fmt.Errorf("failed to read input: %w", err)
			}

			// Trim whitespace
			input = strings.TrimSpace(input)

			// Check for exit command
			if input == "exit" || input == "quit" {
				a.printGoodbye()
				return nil
			}

			// Check for empty input
			if input == "" {
				continue
			}

			// Process the input
			if err := a.ProcessInput(ctx, input); err != nil {
				fmt.Fprintf(a.Writer, style.ColorRed("Error: %v\n"), err)
			}
		}
	}
}

// ProcessInput processes user input
func (a *App) ProcessInput(ctx context.Context, input string) error {
	// Check for special commands
	if strings.HasPrefix(input, "/") {
		return a.HandleCommand(ctx, input)
	}

	// Process as a message to the agent
	fmt.Fprintf(a.Writer, style.ColorBold(style.ColorCodeGreen, "assistant> "))

	// Log query information
	fmt.Fprintf(os.Stderr, "\n==== USER QUERY ====\n")
	fmt.Fprintf(os.Stderr, "Query: %s\n", input)
	fmt.Fprintf(os.Stderr, "Model: %s\n", a.Config.DefaultModel)
	fmt.Fprintf(os.Stderr, "Ollama URL: %s\n", a.Config.OllamaURL)
	fmt.Fprintf(os.Stderr, "Start time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "====================\n\n")

	a.Logger.Info("Received user query",
		"query", input,
		"model", a.Config.DefaultModel)

	startTime := time.Now()
	response, err := a.Agent.ProcessMessage(ctx, input)
	duration := time.Since(startTime)

	if err != nil {
		// Print error to both stderr and return formatted error
		fmt.Fprintf(os.Stderr, "\n==== ERROR PROCESSING QUERY ====\n")
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		fmt.Fprintf(os.Stderr, "Duration: %s\n", duration.String())
		fmt.Fprintf(os.Stderr, "==============================\n\n")

		a.Logger.Error("Failed to process message", "error", err, "duration", duration.String())
		return fmt.Errorf("failed to process message: %w", err)
	}

	// Log completion information
	fmt.Fprintf(os.Stderr, "\n==== PROCESSING COMPLETE ====\n")
	fmt.Fprintf(os.Stderr, "Total processing time: %s\n", duration.String())
	fmt.Fprintf(os.Stderr, "Response length: %d characters\n", len(response))
	fmt.Fprintf(os.Stderr, "Finish time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "===========================\n\n")

	// Print the response
	fmt.Fprintln(a.Writer, response)

	// Log the completion
	a.Logger.Info("Message processed successfully",
		"duration", duration.String(),
		"responseLength", len(response))

	return nil
}

// HandleCommand handles special commands
func (a *App) HandleCommand(ctx context.Context, cmd string) error {
	// Split command and arguments
	parts := strings.SplitN(cmd, " ", 2)
	command := parts[0]

	var args string
	if len(parts) > 1 {
		args = parts[1]
	}

	switch command {
	case "/help":
		a.printHelp()
	case "/models":
		return a.listModels(ctx)
	case "/model":
		return a.setModel(ctx, args)
	case "/tools":
		a.listTools()
	case "/clear":
		a.clearScreen()
	case "/reset":
		a.resetContext()
		fmt.Fprintln(a.Writer, style.ColorGreen("Conversation context has been reset. System messages are preserved."))
	case "/version":
		a.printVersion()
	case "/config":
		if args == "" {
			a.printConfig()
		} else {
			return a.handleConfigCommand(args)
		}
	default:
		return fmt.Errorf("unknown command: %s", command)
	}

	return nil
}

// Helper methods for commands

// listModels lists available models from Ollama
func (a *App) listModels(ctx context.Context) error {
	// Get available models
	resp, err := a.LLMClient.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	// Print models
	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeWhite, "Available models:"))
	for _, model := range resp.Models {
		current := ""
		if model.Name == a.Config.DefaultModel {
			current = style.ColorGreen(" (current)")
		}
		fmt.Fprintf(a.Writer, "- %s%s\n", model.Name, current)
	}

	return nil
}

// setModel changes the active model
func (a *App) setModel(ctx context.Context, modelName string) error {
	if modelName == "" {
		return fmt.Errorf("model name required")
	}

	// Check if model exists
	resp, err := a.LLMClient.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	modelExists := false
	for _, model := range resp.Models {
		if model.Name == modelName {
			modelExists = true
			break
		}
	}

	if !modelExists {
		return fmt.Errorf("model not found: %s", modelName)
	}

	// Update the config
	a.Config.DefaultModel = modelName
	fmt.Fprintf(a.Writer, "Model changed to: %s\n", style.ColorGreen(modelName))

	return nil
}

// listTools lists available tools
func (a *App) listTools() {
	tools := a.ToolRegistry.ListTools()

	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeWhite, "Available tools:"))
	for _, tool := range tools {
		fmt.Fprintf(a.Writer, "- %s: %s\n", style.ColorBold(style.ColorCodeCyan, tool.Name()), tool.Description())
	}
}

// clearScreen clears the terminal screen
func (a *App) clearScreen() {
	fmt.Fprint(a.Writer, "\033[H\033[2J")
}

// resetContext resets the agent's conversation context
func (a *App) resetContext() {
	a.Agent.ClearContext()
}

// printConfig prints the current configuration
func (a *App) printConfig() {
	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeWhite, "Current configuration:"))
	fmt.Fprintf(a.Writer, "Model: %s\n", a.Config.DefaultModel)
	fmt.Fprintf(a.Writer, "Ollama URL: %s\n", a.Config.OllamaURL)
	fmt.Fprintf(a.Writer, "Temperature: %.2f\n", a.Config.Temperature)
	fmt.Fprintf(a.Writer, "Max tokens: %d\n", a.Config.MaxTokens)
	fmt.Fprintf(a.Writer, "Log file: %s\n", a.Config.LogFile)
	fmt.Fprintf(a.Writer, "Log level: %s\n", a.Config.LogLevel)
	fmt.Fprintf(a.Writer, "Log silent: %t\n", a.Config.LogSilent)
}

// handleConfigCommand handles configuration changes
func (a *App) handleConfigCommand(args string) error {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid config command format. Use /config key value")
	}

	key := parts[0]
	value := parts[1]

	switch key {
	case "temperature":
		temp, err := parseFloat(value)
		if err != nil {
			return fmt.Errorf("invalid temperature: %w", err)
		}
		if temp < 0 || temp > 2 {
			return fmt.Errorf("temperature must be between 0 and 2")
		}
		a.Config.Temperature = temp
		fmt.Fprintf(a.Writer, "Temperature set to: %.2f\n", temp)

	case "max_tokens":
		tokens, err := parseInt(value)
		if err != nil {
			return fmt.Errorf("invalid max_tokens: %w", err)
		}
		if tokens < 100 || tokens > 10000 {
			return fmt.Errorf("max_tokens must be between 100 and 10000")
		}
		a.Config.MaxTokens = tokens
		fmt.Fprintf(a.Writer, "Max tokens set to: %d\n", tokens)

	case "log_level":
		if value != "debug" && value != "info" && value != "warn" && value != "error" {
			return fmt.Errorf("log_level must be one of: debug, info, warn, error")
		}
		a.Config.LogLevel = value
		fmt.Fprintf(a.Writer, "Log level set to: %s\n", value)

	case "log_silent":
		silent, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid log_silent value: %w", err)
		}
		a.Config.LogSilent = silent
		fmt.Fprintf(a.Writer, "Log silent set to: %t\n", silent)

	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	return nil
}

// printVersion prints the current version
func (a *App) printVersion() {
	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeWhite, "Codezilla v0.1.0"))
	fmt.Fprintln(a.Writer, "A CLI agent powered by Ollama")
}

// printHelp prints the help message
func (a *App) printHelp() {
	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeWhite, "Codezilla - CLI Agent"))
	fmt.Fprintln(a.Writer, "")
	fmt.Fprintln(a.Writer, "Commands:")
	fmt.Fprintln(a.Writer, "  /help     - Show this help message")
	fmt.Fprintln(a.Writer, "  /models   - List available models")
	fmt.Fprintln(a.Writer, "  /model    - Set the active model")
	fmt.Fprintln(a.Writer, "  /tools    - List available tools")
	fmt.Fprintln(a.Writer, "  /clear    - Clear the screen")
	fmt.Fprintln(a.Writer, "  /reset    - Reset conversation context (preserves system messages)")
	fmt.Fprintln(a.Writer, "  /version  - Show version information")
	fmt.Fprintln(a.Writer, "  /config   - Show or modify configuration")
	fmt.Fprintln(a.Writer, "  exit      - Exit the application")
}

// printWelcome prints the welcome message
func (a *App) printWelcome() {
	width, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if width == 0 {
		width = 80
	}

	title := "Codezilla"
	padding := (width - len(title)) / 2

	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeBlue, strings.Repeat("=", width)))
	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeBlue,
		strings.Repeat(" ", padding)+title+strings.Repeat(" ", padding)))
	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeBlue, strings.Repeat("=", width)))
	fmt.Fprintln(a.Writer, "A CLI agent powered by Ollama")
	fmt.Fprintln(a.Writer, "Type '/help' to see available commands")
	fmt.Fprintln(a.Writer, "Current model: "+style.ColorGreen(a.Config.DefaultModel))
}

// printGoodbye prints the goodbye message
func (a *App) printGoodbye() {
	fmt.Fprintln(a.Writer, "\nThank you for using Codezilla! Goodbye.")
}

// registerDefaultTools registers the default tools
func registerDefaultTools(registry tools.ToolRegistry) {
	// Register file tools
	registry.RegisterTool(tools.NewFileReadTool())
	registry.RegisterTool(tools.NewFileWriteTool())

	// Register shell execution tool
	registry.RegisterTool(tools.NewExecuteTool(30 * time.Second))
}

// Helper functions for parsing values
func parseFloat(s string) (float64, error) {
	var result float64
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return 0, err
	}
	return result, nil
}

func parseInt(s string) (int, error) {
	var result int
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return 0, err
	}
	return result, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "yes", "1", "y":
		return true, nil
	case "false", "no", "0", "n":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", s)
	}
}

// Default system prompt for the agent
const defaultSystemPrompt = `You are Codezilla, a helpful AI assistant with access to local system tools.

You have the following tools available:
{{tools}}

GUIDELINES:
1. When using tools, you MUST format your response using <tool> tags exactly as shown below:
   <tool>
   {
     "name": "execute",
     "params": {
       "command": "ls -la"
     }
   }
   </tool>

2. Think through problems step by step
3. For complex tasks, break them down into smaller steps
4. Be concise in your responses - code-focused, not chatty
5. When uncertain, use tools to gather information
6. Prioritize accuracy over speed
7. When executing commands, be careful and prefer safer options
8. Never make up information - use tools to verify facts

EXAMPLES:
- To execute a shell command:
  <tool>
  {
    "name": "execute",
    "params": {
      "command": "ls -la"
    }
  }
  </tool>

- To read a file:
  <tool>
  {
    "name": "fileRead",
    "params": {
      "file_path": "/path/to/file.txt"
    }
  }
  </tool>

Remember that you're running on a local machine with access to the filesystem and shell commands. Be responsible with these capabilities.`
