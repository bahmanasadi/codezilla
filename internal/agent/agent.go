package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"codezilla/internal/tools"
	"codezilla/llm/ollama"
	"codezilla/pkg/logger"
)

var (
	ErrLLMResponseFormat   = errors.New("invalid LLM response format")
	ErrToolExecutionFailed = errors.New("tool execution failed")
	ErrToolNotFound        = errors.New("tool not found")
)

// Agent interface defines the core functionality of an agent
type Agent interface {
	// ProcessMessage processes a user message and returns the agent's response
	ProcessMessage(ctx context.Context, message string) (string, error)

	// ExecuteTool executes a tool with the given parameters
	ExecuteTool(ctx context.Context, toolName string, params map[string]interface{}) (interface{}, error)

	// AddSystemMessage adds a system message to the context
	AddSystemMessage(message string)

	// AddUserMessage adds a user message to the context
	AddUserMessage(message string)

	// AddAssistantMessage adds an assistant message to the context
	AddAssistantMessage(message string)

	// ClearContext clears all non-system messages from the conversation context
	ClearContext()

	// SetModel changes the active model used by the agent
	SetModel(model string)

	// SetTemperature changes the temperature setting
	SetTemperature(temperature float64)

	// SetMaxTokens changes the max tokens setting
	SetMaxTokens(maxTokens int)
}

// Config contains configuration for the agent
type Config struct {
	Model          string
	MaxTokens      int
	Temperature    float64
	SystemPrompt   string
	OllamaURL      string
	ToolRegistry   tools.ToolRegistry
	PromptTemplate *PromptTemplate
	Logger         *logger.Logger
	PermissionMgr  tools.ToolPermissionManager
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Model:          "qwen2.5-coder:3b",
		MaxTokens:      4000,
		Temperature:    0.7,
		OllamaURL:      "http://localhost:11434/api",
		PromptTemplate: DefaultPromptTemplate(),
		Logger:         logger.DefaultLogger(),
	}
}

// agent implements the Agent interface
type agent struct {
	config        *Config
	context       *Context
	ollamaClient  ollama.Client
	toolRegistry  tools.ToolRegistry
	logger        *logger.Logger
	permissionMgr tools.ToolPermissionManager
}

// NewAgent creates a new agent with the given configuration
func NewAgent(config *Config) Agent {
	if config == nil {
		config = DefaultConfig()
	}

	if config.Logger == nil {
		config.Logger = logger.DefaultLogger()
	}

	var ollamaOpts []func(*ollama.ClientOptions)
	if config.OllamaURL != "" {
		ollamaOpts = append(ollamaOpts, ollama.WithBaseURL(config.OllamaURL))
	}

	ollamaClient := ollama.NewClient(ollamaOpts...)

	// If no permission manager is provided, create one with a default callback that always allows execution
	// This will be replaced by the CLI with a proper interactive callback
	if config.PermissionMgr == nil {
		config.PermissionMgr = tools.NewPermissionManager(func(ctx context.Context, request tools.PermissionRequest) (tools.PermissionResponse, error) {
			// Default behavior: grant permission but don't remember
			return tools.PermissionResponse{Granted: true, RememberMe: false}, nil
		})
	}

	agent := &agent{
		config:        config,
		context:       NewContext(config.MaxTokens),
		ollamaClient:  ollamaClient,
		toolRegistry:  config.ToolRegistry,
		logger:        config.Logger,
		permissionMgr: config.PermissionMgr,
	}

	// Add initial system message if provided
	if config.SystemPrompt != "" {
		// Format system prompt with tool information
		var toolSpecs []tools.ToolSpec
		if config.ToolRegistry != nil {
			toolSpecs = config.ToolRegistry.GetToolSpecs()
		}

		formattedPrompt := FormatSystemPrompt(config.SystemPrompt, toolSpecs)
		agent.AddSystemMessage(formattedPrompt)
	}

	return agent
}

// ProcessMessage processes a user message and returns the agent's response
func (a *agent) ProcessMessage(ctx context.Context, message string) (string, error) {
	a.logger.Debug("Processing message", "message", message)

	// Add user message to context
	a.AddUserMessage(message)

	// Generate response
	a.logger.Debug("Generating initial response")
	response, err := a.generateResponse(ctx)
	if err != nil {
		a.logger.Error("Failed to generate response", "error", err)
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	a.logger.Debug("Checking for tool calls in response")

	var finalResponse = response
	var remainingText string

	// Loop to handle recursive tool calls until we reach a final response with no tools
	maxIterations := 10 // Safety limit to prevent infinite loops
	iterations := 0

	for iterations < maxIterations {
		iterations++

		// Check for tool usage in response
		toolCall, newRemainingText, hasTool := a.extractToolCall(finalResponse)
		if !hasTool {
			a.logger.Debug("No more tool calls detected, reached final response",
				"iterations", iterations)
			break
		}

		remainingText = newRemainingText

		a.logger.Debug("Tool call detected in iteration",
			"iteration", iterations,
			"tool", toolCall.ToolName,
			"params", fmt.Sprintf("%v", toolCall.Params))

		// Add tool call to context
		a.context.AddToolCallMessage(toolCall.ToolName, toolCall.Params)

		// Execute tool
		a.logger.Debug("Executing tool", "tool", toolCall.ToolName)
		result, err := a.ExecuteTool(ctx, toolCall.ToolName, toolCall.Params)

		if err != nil {
			a.logger.Error("Tool execution failed", "tool", toolCall.ToolName, "error", err)
		} else {
			a.logger.Debug("Tool execution succeeded", "tool", toolCall.ToolName)
		}

		// Add tool result to context
		a.context.AddToolResultMessage(result, err)

		// Generate follow-up response
		a.logger.Debug("Generating follow-up response after tool execution",
			"iteration", iterations)
		followUpResponse, followUpErr := a.generateResponse(ctx)
		if followUpErr != nil {
			a.logger.Error("Failed to generate follow-up response", "error", followUpErr,
				"iteration", iterations)
			// If we can't get a follow-up, use what we have so far
			break
		}

		a.logger.Debug("Received follow-up response",
			"iteration", iterations,
			"hasRemainingText", remainingText != "",
			"followUpLength", len(followUpResponse))

		// Combine remaining text with follow-up
		if remainingText != "" {
			finalResponse = remainingText + "\n\n" + followUpResponse
		} else {
			finalResponse = followUpResponse
		}
	}

	if iterations >= maxIterations {
		a.logger.Warn("Reached maximum number of tool call iterations",
			"maxIterations", maxIterations)
	}

	// Add assistant response to context
	a.AddAssistantMessage(finalResponse)

	return finalResponse, nil
}

// generateResponse generates a response from the LLM using the Generate endpoint
func (a *agent) generateResponse(ctx context.Context) (string, error) {
	// Get formatted messages for the LLM
	messages := a.context.GetFormattedMessages()

	a.logger.Debug("Preparing to send request to Ollama", "messageCount", len(messages))

	// Combine messages into a single prompt
	var systemPrompt string
	var userPrompt strings.Builder

	// Check if we have any messages to process
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages in context to generate a response")
	}

	// Extract the tools information for the system prompt
	var toolsInfo string
	if a.toolRegistry != nil && len(a.toolRegistry.ListTools()) > 0 {
		toolsInfo = "You have access to the following tools:\n\n"
		for _, tool := range a.toolRegistry.ListTools() {
			toolsInfo += fmt.Sprintf("- %s: %s\n", tool.Name(), tool.Description())
		}
		toolsInfo += "\nWhen you need to use a tool, format your response like this:\n"
		toolsInfo += "<tool>\n{\n  \"name\": \"toolName\",\n  \"params\": {\n    \"param1\": \"value1\",\n    \"param2\": \"value2\"\n  }\n}\n</tool>\n\n"
	}

	// First process system messages
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)

		if role == "system" {
			if systemPrompt == "" {
				systemPrompt = content
			} else {
				systemPrompt += "\n\n" + content
			}
		}
	}

	// Add tools information to system prompt if not already included
	if toolsInfo != "" && !strings.Contains(systemPrompt, "You have access to the following tools") {
		systemPrompt = systemPrompt + "\n\n" + toolsInfo
	}

	// Track if we have any user/assistant messages
	hasConversation := false

	// Then add user/assistant messages as a conversation
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)

		if content == "" {
			continue // Skip empty messages
		}

		if role == "system" {
			continue // Already processed
		} else if role == "user" {
			hasConversation = true
			userPrompt.WriteString("User: ")
			userPrompt.WriteString(content)
			userPrompt.WriteString("\n\n")
		} else if role == "assistant" {
			hasConversation = true
			userPrompt.WriteString("Assistant: ")
			userPrompt.WriteString(content)
			userPrompt.WriteString("\n\n")
		} else if role == "tool" {
			hasConversation = true
			userPrompt.WriteString("Tool Result: ")
			userPrompt.WriteString(content)
			userPrompt.WriteString("\n\n")
		}
	}

	// Make sure we have a user prompt
	if !hasConversation {
		// If somehow we have no conversation, create a default prompt
		userPrompt.WriteString("User: Hello\n\n")
	}

	// Add final prompt for the assistant to respond
	userPrompt.WriteString("Assistant: ")

	// Create generate request
	request := ollama.GenerateRequest{
		Model:  a.config.Model,
		Prompt: userPrompt.String(),
		System: systemPrompt,
		Stream: false, // Ensure stream is false for non-streaming responses
		Options: map[string]interface{}{
			"temperature": a.config.Temperature,
		},
	}

	a.logger.Debug("Sending Generate request to Ollama",
		"model", a.config.Model,
		"temperature", a.config.Temperature,
		"systemPromptLength", len(systemPrompt),
		"userPromptLength", userPrompt.Len())

	// Send request to Ollama
	startTime := time.Now()
	response, err := a.ollamaClient.Generate(ctx, request)
	duration := time.Since(startTime)

	if err != nil {
		a.logger.Error("Failed to get response from Ollama Generate API",
			"error", err,
			"duration", duration.String())
		return "", fmt.Errorf("failed to get response from Ollama Generate API: %w", err)
	}

	a.logger.Debug("Received response from Ollama Generate API",
		"responseLength", len(response.Response),
		"duration", duration.String(),
		"model", response.Model)

	// Process the response if needed
	cleanResponse := response.Response

	// Some models might prepend "Assistant:" to their responses when using the Generate API
	// Let's remove it if present
	if strings.HasPrefix(cleanResponse, "Assistant:") {
		cleanResponse = strings.TrimPrefix(cleanResponse, "Assistant:")
		cleanResponse = strings.TrimSpace(cleanResponse)
	}

	// If the response is empty, provide a fallback
	if cleanResponse == "" {
		a.logger.Warn("Empty response from model, using fallback")
		cleanResponse = "I'm sorry, I wasn't able to generate a proper response. Could you please try again or rephrase your question?"
	}

	return cleanResponse, nil
}

// extractToolCall extracts a tool call from the response
func (a *agent) extractToolCall(response string) (*ToolCall, string, bool) {
	// Log for debugging purposes
	a.logger.Debug("Checking for tool calls in response", "responseLength", len(response))

	// Pattern to match <tool>...</tool> sections, more flexible with whitespace and formatting
	pattern := regexp.MustCompile(`(?s)<tool>[\s\n]*(.*?)[\s\n]*</tool>`)

	// Find the first match
	matches := pattern.FindStringSubmatch(response)
	if len(matches) < 2 {
		// Try alternative pattern with backticks that might be used by LLMs
		altPattern := regexp.MustCompile("(?s)```json[\\s\\n]*(.*?)[\\s\\n]*```")
		matches = altPattern.FindStringSubmatch(response)

		if len(matches) < 2 {
			a.logger.Debug("No tool call patterns found in response")
			return nil, response, false
		}
	}

	// Extract tool JSON
	toolJSON := matches[1]
	a.logger.Debug("Found potential tool call", "json", toolJSON)

	// Clean up the JSON - remove any leading/trailing backticks or JSON formatting
	toolJSON = strings.TrimPrefix(toolJSON, "```json")
	toolJSON = strings.TrimSuffix(toolJSON, "```")
	toolJSON = strings.TrimSpace(toolJSON)

	// Parse the tool call
	var toolCall struct {
		Name   string                 `json:"name"`
		Params map[string]interface{} `json:"params"`
	}

	err := json.Unmarshal([]byte(toolJSON), &toolCall)
	if err != nil {
		a.logger.Error("Failed to parse tool call", "error", err, "json", toolJSON)

		// Try with more preprocessing of the JSON
		toolJSON = strings.ReplaceAll(toolJSON, "\n", "")
		toolJSON = strings.ReplaceAll(toolJSON, "\r", "")

		// Try again with cleaned JSON
		err = json.Unmarshal([]byte(toolJSON), &toolCall)
		if err != nil {
			a.logger.Error("Still failed to parse tool call after cleaning", "error", err)
			return nil, response, false
		}
	}

	// Validate the tool call has required fields
	if toolCall.Name == "" {
		a.logger.Error("Tool call missing name field", "json", toolJSON)
		return nil, response, false
	}

	if toolCall.Params == nil {
		a.logger.Error("Tool call missing params field", "json", toolJSON)
		return nil, response, false
	}

	// Create the ToolCall object
	result := &ToolCall{
		ToolName: toolCall.Name,
		Params:   toolCall.Params,
	}

	a.logger.Debug("Successfully extracted tool call",
		"toolName", result.ToolName,
		"paramsCount", len(result.Params))

	// Remove the tool section from the response
	remainingText := pattern.ReplaceAllString(response, "")
	remainingText = strings.TrimSpace(remainingText)

	return result, remainingText, true
}

// ExecuteTool executes a tool with the given parameters
func (a *agent) ExecuteTool(ctx context.Context, toolName string, params map[string]interface{}) (interface{}, error) {
	if a.toolRegistry == nil {
		return nil, ErrToolNotFound
	}

	// Get the tool
	tool, found := a.toolRegistry.GetTool(toolName)
	if !found {
		a.logger.Error("Tool not found", "tool", toolName)
		fmt.Fprintf(os.Stderr, "\n==== TOOL NOT FOUND ====\n")
		fmt.Fprintf(os.Stderr, "Tool name: %s\n", toolName)
		fmt.Fprintf(os.Stderr, "=======================\n\n")
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, toolName)
	}

	// Log tool execution start
	fmt.Fprintf(os.Stderr, "\n==== EXECUTING TOOL ====\n")
	fmt.Fprintf(os.Stderr, "Tool: %s\n", toolName)
	fmt.Fprintf(os.Stderr, "Description: %s\n", tool.Description())

	// Format parameters for logging
	paramsJSON, _ := json.MarshalIndent(params, "", "  ")
	fmt.Fprintf(os.Stderr, "Parameters:\n%s\n", string(paramsJSON))
	fmt.Fprintf(os.Stderr, "Start time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "=======================\n\n")

	a.logger.Info("Executing tool", "tool", toolName, "params", params)

	// Validate parameters
	err := tools.ValidateToolParams(tool, params)
	if err != nil {
		a.logger.Error("Invalid tool parameters", "tool", toolName, "error", err)
		fmt.Fprintf(os.Stderr, "\n==== TOOL VALIDATION ERROR ====\n")
		fmt.Fprintf(os.Stderr, "Tool: %s\n", toolName)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "==============================\n\n")
		return nil, err
	}

	// Request execution permission
	if a.permissionMgr != nil {
		a.logger.Debug("Requesting tool execution permission", "tool", toolName)

		fmt.Fprintf(os.Stderr, "\n==== PERMISSION REQUEST ====\n")
		fmt.Fprintf(os.Stderr, "Tool: %s\n", toolName)

		// Request permission
		granted, err := a.permissionMgr.RequestPermission(ctx, toolName, params, tool)
		if err != nil {
			a.logger.Error("Permission request failed", "tool", toolName, "error", err)
			fmt.Fprintf(os.Stderr, "Permission request error: %v\n", err)
			fmt.Fprintf(os.Stderr, "============================\n\n")
			return nil, fmt.Errorf("failed to request permission: %w", err)
		}

		if !granted {
			a.logger.Info("Permission denied for tool execution", "tool", toolName)
			fmt.Fprintf(os.Stderr, "Permission denied by user\n")
			fmt.Fprintf(os.Stderr, "============================\n\n")
			return nil, tools.ErrPermissionDenied
		}

		fmt.Fprintf(os.Stderr, "Permission granted\n")
		fmt.Fprintf(os.Stderr, "============================\n\n")
		a.logger.Debug("Permission granted for tool execution", "tool", toolName)
	}

	// Execute the tool
	startTime := time.Now()
	result, err := tool.Execute(ctx, params)
	duration := time.Since(startTime)

	if err != nil {
		// Log tool execution failure
		a.logger.Error("Tool execution failed", "tool", toolName, "error", err, "duration", duration.String())
		fmt.Fprintf(os.Stderr, "\n==== TOOL EXECUTION FAILED ====\n")
		fmt.Fprintf(os.Stderr, "Tool: %s\n", toolName)
		fmt.Fprintf(os.Stderr, "Duration: %s\n", duration.String())
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "==============================\n\n")
		return nil, fmt.Errorf("%w: %s: %v", ErrToolExecutionFailed, toolName, err)
	}

	// Log tool execution success
	fmt.Fprintf(os.Stderr, "\n==== TOOL EXECUTION COMPLETED ====\n")
	fmt.Fprintf(os.Stderr, "Tool: %s\n", toolName)
	fmt.Fprintf(os.Stderr, "Duration: %s\n", duration.String())

	// Format result for logging
	var resultOutput string
	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err == nil {
		resultOutput = string(resultJSON)
	} else {
		resultOutput = fmt.Sprintf("%v", result)
	}

	// Truncate very large results for the log
	if len(resultOutput) > 500 {
		fmt.Fprintf(os.Stderr, "Result: %s...\n[result truncated, total length: %d bytes]\n",
			resultOutput[:500], len(resultOutput))
	} else {
		fmt.Fprintf(os.Stderr, "Result: %s\n", resultOutput)
	}

	fmt.Fprintf(os.Stderr, "Finish time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "================================\n\n")

	a.logger.Info("Tool executed successfully",
		"tool", toolName,
		"duration", duration.String(),
		"resultSize", len(resultOutput))

	return result, nil
}

// AddSystemMessage adds a system message to the context
func (a *agent) AddSystemMessage(message string) {
	a.context.AddSystemMessage(message)
}

// AddUserMessage adds a user message to the context
func (a *agent) AddUserMessage(message string) {
	a.context.AddUserMessage(message)
}

// AddAssistantMessage adds an assistant message to the context
func (a *agent) AddAssistantMessage(message string) {
	a.context.AddAssistantMessage(message)
}

// ClearContext clears all non-system messages from the conversation context
func (a *agent) ClearContext() {
	a.logger.Info("Clearing conversation context (keeping system messages)")

	// Display to stderr for visibility
	fmt.Fprintf(os.Stderr, "\n==== CLEARING CONVERSATION CONTEXT ====\n")
	fmt.Fprintf(os.Stderr, "Time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "Keeping system messages\n")
	fmt.Fprintf(os.Stderr, "======================================\n\n")

	// Call the context's ClearContext method
	a.context.ClearContext()
}

// SetModel changes the active model used by the agent
func (a *agent) SetModel(model string) {
	a.logger.Info("Changing model", "from", a.config.Model, "to", model)

	// Display to stderr for visibility
	fmt.Fprintf(os.Stderr, "\n==== CHANGING MODEL ====\n")
	fmt.Fprintf(os.Stderr, "From: %s\n", a.config.Model)
	fmt.Fprintf(os.Stderr, "To: %s\n", model)
	fmt.Fprintf(os.Stderr, "Time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "=======================\n\n")

	// Update the model in the config
	a.config.Model = model
}

// SetTemperature changes the temperature setting
func (a *agent) SetTemperature(temperature float64) {
	a.logger.Info("Changing temperature", "from", a.config.Temperature, "to", temperature)
	a.config.Temperature = temperature
}

// SetMaxTokens changes the max tokens setting
func (a *agent) SetMaxTokens(maxTokens int) {
	a.logger.Info("Changing max tokens", "from", a.config.MaxTokens, "to", maxTokens)
	a.config.MaxTokens = maxTokens
}
