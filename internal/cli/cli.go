package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
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
	OllamaURL           string                   `json:"ollama_url"`
	DefaultModel        string                   `json:"default_model"`
	ModelProfiles       map[string]*ModelProfile `json:"model_profiles"` // Profiles for different models
	SystemPrompt        string                   `json:"system_prompt"`
	LogFile             string                   `json:"log_file"`
	LogLevel            string                   `json:"log_level"`
	LogSilent           bool                     `json:"log_silent"`
	Temperature         float64                  `json:"temperature"`
	MaxTokens           int                      `json:"max_tokens"`
	ToolPermissions     map[string]string        `json:"tool_permissions"` // Maps tool name to permission level: "always_ask", "ask_once", "never_ask"
	DangerousToolsWarn  bool                     `json:"dangerous_tools_warn"`
	AlwaysAskPermission bool                     `json:"always_ask_permission"` // If true, always prompt for permission regardless of saved settings
	DisableColors       bool                     `json:"disable_colors"`        // If true, disable ANSI color output
}

// ModelProfile contains model-specific configuration
type ModelProfile struct {
	Model        string  `json:"model"`
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"max_tokens"`
	SystemPrompt string  `json:"system_prompt"`
	Alias        string  `json:"alias"` // Short name for quick switching
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		OllamaURL:           "http://localhost:11434/api",
		DefaultModel:        "qwen2.5-coder:3b",
		ModelProfiles:       getDefaultModelProfiles(),
		SystemPrompt:        defaultSystemPrompt,
		LogFile:             "logs/codezilla.log",
		LogLevel:            "info",
		LogSilent:           false,
		Temperature:         0.7,
		MaxTokens:           4000,
		ToolPermissions:     make(map[string]string),
		DangerousToolsWarn:  true,
		AlwaysAskPermission: false,
		DisableColors:       false,
	}
}

// getDefaultModelProfiles returns a set of default model profiles/
func getDefaultModelProfiles() map[string]*ModelProfile {
	return map[string]*ModelProfile{
		"coder": {
			Model:        "qwen2.5-coder:32b",
			Temperature:  0.3,
			MaxTokens:    4000,
			SystemPrompt: defaultSystemPrompt,
			Alias:        "code",
		},
		"general": {
			Model:        "qwen3:14b",
			Temperature:  0.7,
			MaxTokens:    4000,
			SystemPrompt: "You are a helpful AI assistant.",
			Alias:        "chat",
		},
		"creative": {
			Model:        "qwen3:14b",
			Temperature:  0.9,
			MaxTokens:    4000,
			SystemPrompt: "You are a creative and imaginative AI assistant.",
			Alias:        "write",
		},
		"analyze": {
			Model:        "qwen3:14b",
			Temperature:  0.1,
			MaxTokens:    4000,
			SystemPrompt: "You are a precise and analytical AI assistant focused on accuracy.",
			Alias:        "think",
		},
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
	Reader       InputReader // Using our InputReader interface for readline support
	Writer       io.Writer
}

// NewApp creates a new CLI application with the given configuration
func NewApp(config *Config) (*App, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Set up color settings
	if config.DisableColors {
		style.DisableColors()
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

	// Initialize permissions from config
	if config.ToolPermissions == nil {
		config.ToolPermissions = make(map[string]string)
	}

	// Create permission manager with interactive CLI callback
	permissionManager := tools.NewPermissionManager(func(ctx context.Context, request tools.PermissionRequest) (tools.PermissionResponse, error) {
		toolName := request.ToolContext.ToolName

		// If AlwaysAskPermission is not set, check saved permissions
		if !config.AlwaysAskPermission {
			// Check if we already have a persistent permission setting for this tool
			if permLevel, exists := config.ToolPermissions[toolName]; exists {
				if permLevel == "never_ask" {
					// Auto-approve without prompting
					return tools.PermissionResponse{Granted: true, RememberMe: true}, nil
				} else if permLevel == "always_deny" {
					// Auto-deny without prompting
					return tools.PermissionResponse{Granted: false, RememberMe: true}, nil
				}
			}
		}

		// Create an interactive CLI permission request
		response, err := cliPermissionCallback(os.Stdin, os.Stdout, request)

		// If the response should be remembered and was successful, update the config
		if err == nil && response.RememberMe {
			var permLevel string
			if response.Granted {
				permLevel = "never_ask"
			} else {
				permLevel = "always_deny"
			}

			// Update configuration
			config.ToolPermissions[toolName] = permLevel

			// Save the updated config
			_ = SaveConfig(config, "config.json") // Ignore error for now
		}

		return response, err
	})

	// Create agent
	agentConfig := &agent.Config{
		Model:         config.DefaultModel,
		MaxTokens:     config.MaxTokens,
		Temperature:   config.Temperature,
		SystemPrompt:  config.SystemPrompt,
		OllamaURL:     config.OllamaURL,
		ToolRegistry:  toolRegistry,
		Logger:        log,
		PermissionMgr: permissionManager,
	}

	agent := agent.NewAgent(agentConfig)

	// Set up readline with history support
	historyPath, err := GetDefaultHistoryFilePath()
	if err != nil {
		log.Warn("Could not get history file path", "error", err)
		historyPath = "" // Fallback to in-memory history only
	}

	reader, err := NewReadlineInput(style.ColorBold(style.ColorCodeBlue, "user> "), historyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create input reader: %w", err)
	}

	return &App{
		Agent:        agent,
		ToolRegistry: toolRegistry,
		LLMClient:    ollamaClient,
		Logger:       log,
		Config:       config,
		Reader:       reader,
		Writer:       os.Stdout,
	}, nil
}

// Run starts the CLI application
func (a *App) Run(ctx context.Context) error {
	// Set up signal handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Make sure to close readline when we're done
	if closer, ok := a.Reader.(io.Closer); ok {
		defer closer.Close()
	}

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
			// Prompt is handled by readline

			// Read input line
			input, err := a.Reader.ReadLine()
			if err != nil {
				if err == io.EOF {
					a.printGoodbye()
					return nil
				}
				return fmt.Errorf("failed to read input: %w", err)
			}

			// No need to trim - already handled by ReadLine

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

	// Check for inline model switching syntax @modelname or @profile
	if strings.HasPrefix(input, "@") {
		parts := strings.SplitN(input, " ", 2)
		if len(parts) > 0 {
			modelSpec := strings.TrimPrefix(parts[0], "@")

			// Look for profile first
			if profile, ok := a.Config.ModelProfiles[modelSpec]; ok {
				// Switch to profile
				a.switchToProfile(profile)
				if len(parts) > 1 {
					input = parts[1] // Continue with the rest of the input
				} else {
					fmt.Fprintf(a.Writer, "Switched to profile: %s (model: %s)\n", modelSpec, profile.Model)
					return nil
				}
			} else {
				// Try as direct model name
				if err := a.setModel(ctx, modelSpec); err == nil {
					if len(parts) > 1 {
						input = parts[1] // Continue with the rest of the input
					} else {
						return nil
					}
				} else {
					// Neither profile nor model found
					return fmt.Errorf("unknown model or profile: %s", modelSpec)
				}
			}
		}
	}

	// Process as a message to the agent
	fmt.Fprint(a.Writer, style.ColorBold(style.ColorCodeGreen, "assistant> "))

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
		if args == "" {
			fmt.Fprintf(a.Writer, "Current model: %s\n", style.ColorGreen(a.Config.DefaultModel))
			fmt.Fprintln(a.Writer, "To change model, use: /model <model-name>")
			fmt.Fprintln(a.Writer, "\nQuick model switching:")
			fmt.Fprintln(a.Writer, "  @<profile>         - Switch to a profile (e.g., @coder)")
			fmt.Fprintln(a.Writer, "  @<model> <query>   - Use a specific model inline")
			return nil
		}
		return a.setModel(ctx, args)
	case "/profiles":
		return a.handleProfilesCommand(args)
	case "/tools":
		a.listTools()
	case "/clear":
		a.clearScreen()
	case "/reset":
		a.resetContext()
		fmt.Fprintln(a.Writer, style.ColorGreen("Conversation context has been reset. System messages are preserved."))
	case "/version":
		a.printVersion()
	case "/permissions":
		return a.handlePermissionsCommand(args)
	case "/color":
		args = strings.TrimSpace(args)
		if args == "" {
			// Show current status
			if style.UseColors {
				fmt.Fprintln(a.Writer, "Color output is currently: ON")
			} else {
				fmt.Fprintln(a.Writer, "Color output is currently: OFF")
			}
			fmt.Fprintln(a.Writer, "Usage: /color [on|off]")
			return nil
		}

		switch strings.ToLower(args) {
		case "on", "enable", "true", "yes", "1":
			style.EnableColors()
			a.Config.DisableColors = false
			fmt.Fprintln(a.Writer, "Color output has been enabled")
		case "off", "disable", "false", "no", "0":
			style.DisableColors()
			a.Config.DisableColors = true
			fmt.Fprintln(a.Writer, "Color output has been disabled")
		default:
			return fmt.Errorf("invalid argument: %s. Use 'on' or 'off'", args)
		}

		// Save configuration
		if err := SaveConfig(a.Config, "config.json"); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		return nil
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

// switchToProfile switches to a model profile
func (a *App) switchToProfile(profile *ModelProfile) {
	// Update config from profile
	a.Config.DefaultModel = profile.Model
	a.Config.Temperature = profile.Temperature
	a.Config.MaxTokens = profile.MaxTokens

	// Update the agent's model and configuration
	a.Agent.SetModel(profile.Model)
	a.Agent.SetTemperature(profile.Temperature)
	a.Agent.SetMaxTokens(profile.MaxTokens)

	// Re-initialize agent with new configuration if system prompt changed
	if profile.SystemPrompt != "" && profile.SystemPrompt != a.Config.SystemPrompt {
		a.Config.SystemPrompt = profile.SystemPrompt
		// Clear context and add new system prompt
		a.Agent.ClearContext()
		a.Agent.AddSystemMessage(profile.SystemPrompt)
	}

	// Save the updated configuration
	_ = SaveConfig(a.Config, "config.json")
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

	// Update the agent's model
	a.Agent.SetModel(modelName)

	fmt.Fprintf(a.Writer, "Model changed to: %s\n", style.ColorGreen(modelName))

	// Save the updated configuration
	if err := SaveConfig(a.Config, "config.json"); err != nil {
		a.Logger.Warn("Failed to save config after model change", "error", err)
		// Don't return error as the model change itself succeeded
	}

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

	colorStatus := "enabled"
	if !style.UseColors {
		colorStatus = "disabled"
	}
	fmt.Fprintf(a.Writer, "Color output: %s\n", colorStatus)

	// Check if we're using a profile
	if a.Config.ModelProfiles != nil {
		for name, profile := range a.Config.ModelProfiles {
			if profile.Model == a.Config.DefaultModel &&
				profile.Temperature == a.Config.Temperature &&
				profile.MaxTokens == a.Config.MaxTokens {
				fmt.Fprintf(a.Writer, "\nActive profile: %s\n", style.ColorGreen(name))
				break
			}
		}
	}
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

	case "color", "colors":
		enabled, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid color value: %w", err)
		}

		a.Config.DisableColors = !enabled
		if enabled {
			style.EnableColors()
			fmt.Fprintf(a.Writer, "Color output enabled\n")
		} else {
			style.DisableColors()
			fmt.Fprintf(a.Writer, "Color output disabled\n")
		}

		// Save configuration
		if err := SaveConfig(a.Config, "config.json"); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	return nil
}

// handleProfilesCommand handles profile management commands
func (a *App) handleProfilesCommand(args string) error {
	parts := strings.Fields(args)

	if len(parts) == 0 {
		// List all profiles
		a.listProfiles()
		return nil
	}

	switch parts[0] {
	case "add":
		if len(parts) < 2 {
			return fmt.Errorf("usage: /profiles add <name>")
		}
		return a.addProfile(parts[1])
	case "remove":
		if len(parts) < 2 {
			return fmt.Errorf("usage: /profiles remove <name>")
		}
		return a.removeProfile(parts[1])
	case "edit":
		if len(parts) < 2 {
			return fmt.Errorf("usage: /profiles edit <name>")
		}
		return a.editProfile(parts[1])
	default:
		return fmt.Errorf("unknown profiles command: %s", parts[0])
	}
}

// listProfiles lists all available model profiles
func (a *App) listProfiles() {
	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeWhite, "Model Profiles:"))

	if a.Config.ModelProfiles == nil || len(a.Config.ModelProfiles) == 0 {
		fmt.Fprintln(a.Writer, "No profiles configured")
		return
	}

	// Sort profiles by name for consistent display
	var names []string
	for name := range a.Config.ModelProfiles {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		profile := a.Config.ModelProfiles[name]
		current := ""
		if profile.Model == a.Config.DefaultModel {
			current = style.ColorGreen(" (current)")
		}

		fmt.Fprintf(a.Writer, "\n%s%s:\n", style.ColorBold(style.ColorCodeCyan, name), current)
		fmt.Fprintf(a.Writer, "  Model: %s\n", profile.Model)
		fmt.Fprintf(a.Writer, "  Alias: %s\n", profile.Alias)
		fmt.Fprintf(a.Writer, "  Temperature: %.2f\n", profile.Temperature)
		fmt.Fprintf(a.Writer, "  Max Tokens: %d\n", profile.MaxTokens)
		if profile.SystemPrompt != "" {
			prompt := profile.SystemPrompt
			if len(prompt) > 50 {
				prompt = prompt[:47] + "..."
			}
			fmt.Fprintf(a.Writer, "  System Prompt: %s\n", prompt)
		}
	}
}

// addProfile adds a new model profile
func (a *App) addProfile(name string) error {
	if a.Config.ModelProfiles == nil {
		a.Config.ModelProfiles = make(map[string]*ModelProfile)
	}

	if _, exists := a.Config.ModelProfiles[name]; exists {
		return fmt.Errorf("profile '%s' already exists", name)
	}

	// Create a new profile with defaults
	profile := &ModelProfile{
		Model:        a.Config.DefaultModel,
		Temperature:  a.Config.Temperature,
		MaxTokens:    a.Config.MaxTokens,
		SystemPrompt: a.Config.SystemPrompt,
		Alias:        name,
	}

	a.Config.ModelProfiles[name] = profile

	// Save configuration
	if err := SaveConfig(a.Config, "config.json"); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(a.Writer, "Profile '%s' created with current settings\n", name)
	fmt.Fprintln(a.Writer, "Use '/profiles edit "+name+"' to customize")
	return nil
}

// removeProfile removes a model profile
func (a *App) removeProfile(name string) error {
	if a.Config.ModelProfiles == nil {
		return fmt.Errorf("no profiles configured")
	}

	if _, exists := a.Config.ModelProfiles[name]; !exists {
		return fmt.Errorf("profile '%s' not found", name)
	}

	delete(a.Config.ModelProfiles, name)

	// Save configuration
	if err := SaveConfig(a.Config, "config.json"); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(a.Writer, "Profile '%s' removed\n", name)
	return nil
}

// editProfile edits a model profile (placeholder for now)
func (a *App) editProfile(name string) error {
	if a.Config.ModelProfiles == nil {
		return fmt.Errorf("no profiles configured")
	}

	profile, exists := a.Config.ModelProfiles[name]
	if !exists {
		return fmt.Errorf("profile '%s' not found", name)
	}

	fmt.Fprintf(a.Writer, "Editing profile '%s':\n", name)
	fmt.Fprintf(a.Writer, "Current settings:\n")
	fmt.Fprintf(a.Writer, "  Model: %s\n", profile.Model)
	fmt.Fprintf(a.Writer, "  Temperature: %.2f\n", profile.Temperature)
	fmt.Fprintf(a.Writer, "  Max Tokens: %d\n", profile.MaxTokens)
	fmt.Fprintln(a.Writer, "\nTo edit, manually update config.json for now")
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
	fmt.Fprintln(a.Writer, "  /help        - Show this help message")
	fmt.Fprintln(a.Writer, "  /models      - List available models")
	fmt.Fprintln(a.Writer, "  /model       - Show current model or change to a new model")
	fmt.Fprintln(a.Writer, "  /profiles    - Manage model profiles (list, add, remove, edit)")
	fmt.Fprintln(a.Writer, "  /tools       - List available tools")
	fmt.Fprintln(a.Writer, "  /permissions - Manage tool permission settings")
	fmt.Fprintln(a.Writer, "  /color       - Toggle color output (on/off)")
	fmt.Fprintln(a.Writer, "  /clear       - Clear the screen")
	fmt.Fprintln(a.Writer, "  /reset       - Reset conversation context (preserves system messages)")
	fmt.Fprintln(a.Writer, "  /version     - Show version information")
	fmt.Fprintln(a.Writer, "  /config      - Show or modify configuration")
	fmt.Fprintln(a.Writer, "  exit         - Exit the application")
	fmt.Fprintln(a.Writer, "")
	fmt.Fprintln(a.Writer, "Quick Model Switching:")
	fmt.Fprintln(a.Writer, "  @<profile>         - Switch to a model profile (e.g., @coder, @general)")
	fmt.Fprintln(a.Writer, "  @<model> <query>   - Use a specific model for one query")
	fmt.Fprintln(a.Writer, "")
	fmt.Fprintln(a.Writer, "Examples:")
	fmt.Fprintln(a.Writer, "  @coder How do I implement a binary search?")
	fmt.Fprintln(a.Writer, "  @creative Write a poem about programming")
	fmt.Fprintln(a.Writer, "  @analyze Explain the time complexity of this algorithm")
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
