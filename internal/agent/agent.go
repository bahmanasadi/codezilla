package agent

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
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
		toolsInfo += "\nWhen you need to use a tool, you can format your response in one of these ways:\n\n"
		toolsInfo += "1. XML format:\n"
		toolsInfo += "<tool>\n  <name>toolName</name>\n  <params>\n    <param1>value1</param1>\n    <param2>value2</param2>\n  </params>\n</tool>\n\n"
		toolsInfo += "2. JSON format:\n"
		toolsInfo += "```json\n{\n  \"tool\": \"toolName\",\n  \"params\": {\n    \"param1\": \"value1\",\n    \"param2\": \"value2\"\n  }\n}\n```\n\n"
		toolsInfo += "3. For bash/shell commands, use code blocks:\n"
		toolsInfo += "```bash\ncommand here\n```\n\n"
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

// XMLToolCall represents the XML structure for tool calls
type XMLToolCall struct {
	Name   string    `xml:"name"` // Accept standard name tag
	Params XMLParams `xml:"params"`
}

// XMLParams represents the dynamic parameters in XML
type XMLParams struct {
	XMLData []byte `xml:",innerxml"`
}

// extractToolCall extracts a tool call from the response
func (a *agent) extractToolCall(response string) (*ToolCall, string, bool) {
	// Log for debugging purposes
	a.logger.Debug("Checking for tool calls in response", "responseLength", len(response))

	// Add more debug logging when fixing tool detection
	a.logger.Debug("Full response for tool detection", "response", response)

	// Try JSON format first (most structured)
	jsonPattern := regexp.MustCompile("(?s)```json\\s*\\n(.*?)\\n?```")
	jsonMatches := jsonPattern.FindStringSubmatch(response)
	if len(jsonMatches) >= 2 {
		jsonContent := strings.TrimSpace(jsonMatches[1])
		a.logger.Debug("Found JSON code block", "content", jsonContent)

		// Try to parse as JSON tool call
		var jsonToolCall struct {
			Tool   string                 `json:"tool"`
			Name   string                 `json:"name"` // Alternative field name
			Params map[string]interface{} `json:"params"`
		}

		if err := json.Unmarshal([]byte(jsonContent), &jsonToolCall); err == nil {
			toolName := jsonToolCall.Tool
			if toolName == "" {
				toolName = jsonToolCall.Name
			}

			if toolName != "" && jsonToolCall.Params != nil {
				a.logger.Debug("Successfully parsed JSON tool call", "toolName", toolName)

				result := &ToolCall{
					ToolName: toolName,
					Params:   jsonToolCall.Params,
				}

				// Remove the JSON block from response
				loc := jsonPattern.FindStringIndex(response)
				remainingText := response[:loc[0]] + response[loc[1]:]
				remainingText = strings.TrimSpace(remainingText)

				return result, remainingText, true
			}
		}
	}

	// Check for bash/shell code blocks
	bashPattern := regexp.MustCompile("(?s)```(bash|sh|shell|terminal|console)\\s*\\n(.*?)\\n?```")
	bashMatches := bashPattern.FindStringSubmatch(response)
	if len(bashMatches) >= 3 {
		command := strings.TrimSpace(bashMatches[2])
		a.logger.Debug("Found bash code block", "language", bashMatches[1], "command", command)

		// Create tool call for bash execution
		result := &ToolCall{
			ToolName: "execute",
			Params: map[string]interface{}{
				"command": command,
			},
		}

		// Remove the bash block from response
		loc := bashPattern.FindStringIndex(response)
		remainingText := response[:loc[0]] + response[loc[1]:]
		remainingText = strings.TrimSpace(remainingText)

		return result, remainingText, true
	}

	// Pattern to match <tool>...</tool> sections, more flexible with whitespace and formatting
	pattern := regexp.MustCompile(`(?s)<tool>[\s\n]*(.*?)[\s\n]*</tool>`)

	// Find the first match
	matches := pattern.FindStringSubmatch(response)
	if len(matches) < 2 {
		// Try alternative pattern with backticks that might be used by LLMs
		altPattern := regexp.MustCompile("(?s)```xml[\\s\\n]*(.*?)[\\s\\n]*```")
		matches = altPattern.FindStringSubmatch(response)

		if len(matches) < 2 {
			// If still no matches, look for any <n> tag for backward compatibility
			directToolPattern := regexp.MustCompile(`(?s)<n>\s*(.*?)\s*</n>`)
			matches = directToolPattern.FindStringSubmatch(response)

			if len(matches) >= 2 {
				// Found a direct tool call, wrap it in a tool tag structure
				toolName := matches[1]
				a.logger.Debug("Found direct <n> tag", "toolName", toolName)

				// Find the parameters section
				paramsMatch := regexp.MustCompile(`(?s)<params>(.*?)</params>`).FindStringSubmatch(response)
				if len(paramsMatch) >= 2 {
					// Reconstruct into proper tool XML format
					toolXML := fmt.Sprintf("<n>%s</n>\n<params>%s</params>",
						toolName, paramsMatch[1])
					matches[1] = toolXML
					a.logger.Debug("Reconstructed tool call from direct tag", "toolXML", toolXML)
				} else {
					a.logger.Debug("No params section found for direct tool tag")
					return nil, response, false
				}
			} else {
				a.logger.Debug("No tool call patterns found in response")
				return nil, response, false
			}
		}
	}

	// Extract tool XML content
	toolXML := matches[0]
	a.logger.Debug("Found potential tool call", "xml", toolXML)

	// Clean up the XML - remove any leading/trailing backticks or formatting
	toolXML = strings.TrimPrefix(toolXML, "```xml")
	toolXML = strings.TrimSuffix(toolXML, "```")
	toolXML = strings.TrimSpace(toolXML)

	// Ensure XML has root element
	if !strings.HasPrefix(toolXML, "<") {
		toolXML = "<tool>" + toolXML + "</tool>"
	}

	// Try standard XML parsing first
	var xmlToolCall XMLToolCall
	err := xml.Unmarshal([]byte(toolXML), &xmlToolCall)

	// Add more debug logging for XML parsing
	a.logger.Debug("Attempting to parse XML", "toolXML", toolXML, "error", err,
		"extractedName", xmlToolCall.Name)

	if err == nil && xmlToolCall.Name != "" {
		// Successfully parsed XML
		a.logger.Debug("Successfully parsed tool call with standard XML parser",
			"toolName", xmlToolCall.Name)

		// Parse the parameters from inner XML
		params, err := parseXMLParams(xmlToolCall.Params.XMLData, a.logger)
		if err != nil {
			a.logger.Error("Failed to parse parameters", "error", err)
			return nil, response, false
		}

		// Create the ToolCall object
		result := &ToolCall{
			ToolName: xmlToolCall.Name,
			Params:   params,
		}

		a.logger.Debug("Successfully extracted tool call",
			"toolName", result.ToolName,
			"paramsCount", len(result.Params))

		// Remove the tool section from the response
		remainingText := pattern.ReplaceAllString(response, "")
		remainingText = strings.TrimSpace(remainingText)

		return result, remainingText, true
	}

	// If standard parsing failed, try fallback methods
	a.logger.Debug("Standard XML parsing failed, trying fallback methods", "error", err)

	// Try legacy approach
	toolName := extractXMLElement(toolXML, "name")
	if toolName == "" {
		// We should not see missing name field errors anymore with our improved extraction
		// but we'll still log it for debugging purposes
		a.logger.Debug("Tool name extracted as empty, this should not happen with improved extraction", "xml", toolXML)

		// Try to handle legacy JSON format for backward compatibility
		if strings.Contains(toolXML, "\"name\"") {
			a.logger.Debug("Detected legacy JSON format, trying to parse as JSON")
			return extractLegacyJSONToolCall(a, toolXML, response, pattern)
		}

		return nil, response, false
	}

	// Extract params section
	paramsSection := extractXMLElement(toolXML, "params")
	if paramsSection == "" {
		a.logger.Error("Tool call missing params section", "xml", toolXML)
		return nil, response, false
	}

	// Parse parameters from params section
	params := extractXMLParams(paramsSection, a.logger)
	if params == nil {
		a.logger.Error("Failed to extract parameters from tool call", "xml", toolXML)
		return nil, response, false
	}

	// Create the ToolCall object
	result := &ToolCall{
		ToolName: toolName,
		Params:   params,
	}

	a.logger.Debug("Successfully extracted tool call using fallback method",
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

	// Log tool execution start in XML format
	fmt.Fprintf(os.Stderr, "\n==== EXECUTING TOOL ====\n")
	fmt.Fprintf(os.Stderr, "<tool_execution>\n")
	fmt.Fprintf(os.Stderr, "  <tool_name>%s</tool_name>\n", agentEscapeXML(toolName))
	fmt.Fprintf(os.Stderr, "  <description>%s</description>\n", agentEscapeXML(tool.Description()))

	// Format parameters as XML
	fmt.Fprintf(os.Stderr, "  <parameters>\n")

	// Sort parameters for consistent output
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Add each parameter as XML
	for _, k := range keys {
		v := params[k]
		fmt.Fprintf(os.Stderr, "    <%s>%v</%s>\n", k, agentFormatXMLValue(v), k)
	}

	fmt.Fprintf(os.Stderr, "  </parameters>\n")
	fmt.Fprintf(os.Stderr, "  <start_time>%s</start_time>\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "</tool_execution>\n")
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
	fmt.Fprintf(os.Stderr, "<tool_result>\n")
	fmt.Fprintf(os.Stderr, "  <tool_name>%s</tool_name>\n", agentEscapeXML(toolName))
	fmt.Fprintf(os.Stderr, "  <duration>%s</duration>\n", duration.String())

	// Format result as XML inline
	xmlOutput := formatToolResultAsXML(result, toolName)

	// Truncate very large results for the log
	if len(xmlOutput) > 500 {
		fmt.Fprintf(os.Stderr, "  <result_truncated length=\"%d\">\n%s...\n  </result_truncated>\n",
			len(xmlOutput), xmlOutput[:500])
	} else {
		fmt.Fprintf(os.Stderr, "  <result>\n%s\n  </result>\n", xmlOutput)
	}

	fmt.Fprintf(os.Stderr, "  <finish_time>%s</finish_time>\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "</tool_result>\n")
	fmt.Fprintf(os.Stderr, "================================\n\n")

	a.logger.Info("Tool executed successfully",
		"tool", toolName,
		"duration", duration.String(),
		"resultSize", len(xmlOutput))

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

// formatToolResultAsXML formats a tool result as XML for display
func formatToolResultAsXML(result interface{}, toolName string) string {
	var builder strings.Builder

	switch v := result.(type) {
	case string:
		builder.WriteString(agentEscapeXML(v))
	case []byte:
		builder.WriteString(agentEscapeXML(string(v)))
	case map[string]interface{}:
		// Sort the keys for consistent output
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Add each field as an XML element
		for _, k := range keys {
			builder.WriteString(fmt.Sprintf("    <%s>%v</%s>\n", k, agentFormatXMLValue(v[k]), k))
		}
	default:
		// For other types, just convert to string
		builder.WriteString(fmt.Sprintf("    <value>%v</value>\n", v))
	}

	return builder.String()
}

// extractXMLElement extracts a specific element from an XML string
func extractXMLElement(xmlStr string, elementName string) string {
	// First try the requested element name
	result := tryExtractXMLElement(xmlStr, elementName)

	// If we're looking for "name" and didn't find it, try "n" as an alternative
	if result == "" && elementName == "name" {
		result = tryExtractXMLElement(xmlStr, "n")
	}

	return result
}

// tryExtractXMLElement attempts to extract a specific XML element by name
func tryExtractXMLElement(xmlStr string, elementName string) string {
	// Create patterns for opening and closing tags
	openTag := regexp.MustCompile(fmt.Sprintf(`<%s[^>]*>`, regexp.QuoteMeta(elementName)))
	closeTag := regexp.MustCompile(fmt.Sprintf(`</%s>`, regexp.QuoteMeta(elementName)))

	// First check if it's a self-closing tag
	selfClosingPattern := regexp.MustCompile(fmt.Sprintf(`<%s[^>]*/?>`, regexp.QuoteMeta(elementName)))
	if selfClosingPattern.MatchString(xmlStr) {
		return "" // Self-closing tag has no content
	}

	// Find positions of opening and closing tags
	openMatches := openTag.FindStringIndex(xmlStr)
	closeMatches := closeTag.FindStringIndex(xmlStr)

	// Check if we found both tags
	if openMatches == nil || closeMatches == nil {
		return "" // Element not found
	}

	// Extract content between tags
	openEnd := openMatches[1]     // End position of opening tag
	closeStart := closeMatches[0] // Start position of closing tag

	// Validate positions
	if openEnd >= closeStart || openEnd >= len(xmlStr) || closeStart > len(xmlStr) {
		return "" // Invalid positions
	}

	// Return the content between opening and closing tags, trimmed
	return strings.TrimSpace(xmlStr[openEnd:closeStart])
}

// parseXMLParams parses parameters from XML data using Go's standard XML package
func parseXMLParams(xmlData []byte, logger *logger.Logger) (map[string]interface{}, error) {
	params := make(map[string]interface{})

	// Create a decoder for the XML data
	decoder := xml.NewDecoder(strings.NewReader(string(xmlData)))

	var currentElement string

	// Process XML tokens
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Error("Error parsing XML parameters", "error", err)
			return nil, err
		}

		switch t := token.(type) {
		case xml.StartElement:
			// We're starting a new element
			currentElement = t.Name.Local

		case xml.EndElement:
			// Element is closing, clear current element if it matches
			if t.Name.Local == currentElement {
				currentElement = ""
			}

		case xml.CharData:
			// We have character data (the parameter value)
			if currentElement != "" && currentElement != "params" {
				// Convert value to string and trim whitespace
				paramValue := strings.TrimSpace(string(t))

				// Skip empty values
				if paramValue == "" {
					continue
				}

				// Try to convert to appropriate types
				switch {
				case paramValue == "true" || paramValue == "false":
					// Boolean
					params[currentElement] = paramValue == "true"
					logger.Debug("Parsed XML parameter as boolean", "name", currentElement, "value", params[currentElement])

				case regexp.MustCompile(`^-?\d+$`).MatchString(paramValue):
					// Integer
					intVal, err := strconv.Atoi(paramValue)
					if err == nil {
						params[currentElement] = intVal
						logger.Debug("Parsed XML parameter as integer", "name", currentElement, "value", params[currentElement])
					} else {
						params[currentElement] = paramValue
						logger.Debug("Failed to parse numeric value, using as string", "name", currentElement, "value", paramValue)
					}

				case regexp.MustCompile(`^-?\d+\.\d+$`).MatchString(paramValue):
					// Float
					floatVal, err := strconv.ParseFloat(paramValue, 64)
					if err == nil {
						params[currentElement] = floatVal
						logger.Debug("Parsed XML parameter as float", "name", currentElement, "value", params[currentElement])
					} else {
						params[currentElement] = paramValue
						logger.Debug("Failed to parse float value, using as string", "name", currentElement, "value", paramValue)
					}

				default:
					// String
					params[currentElement] = paramValue
					logger.Debug("Parsed XML parameter as string", "name", currentElement, "value", paramValue)
				}
			}
		}
	}

	return params, nil
}

// extractXMLParams is the fallback method for parsing parameters when standard XML parsing fails
func extractXMLParams(paramsXML string, logger *logger.Logger) map[string]interface{} {
	params := make(map[string]interface{})

	// Simple name pattern for XML element names
	namePattern := regexp.MustCompile(`<([a-zA-Z0-9_-]+)[^>]*>`)

	// Parse parameters using a more robust approach without backreferences
	// Get all potential parameter names first
	potentialNames := namePattern.FindAllStringSubmatch(paramsXML, -1)
	if len(potentialNames) == 0 {
		return nil
	}

	// Process each potential parameter
	for _, nameMatch := range potentialNames {
		if len(nameMatch) < 2 {
			continue
		}

		paramName := nameMatch[1]

		// Skip if this is not a direct child element of params (could be nested)
		// or if we already processed this parameter
		if _, exists := params[paramName]; exists || paramName == "params" {
			continue
		}

		// Create patterns specific to this parameter name
		openTag := regexp.MustCompile(fmt.Sprintf(`<%s[^>]*>`, regexp.QuoteMeta(paramName)))
		closeTag := regexp.MustCompile(fmt.Sprintf(`</%s>`, regexp.QuoteMeta(paramName)))

		// Find the positions of opening and closing tags
		openMatches := openTag.FindAllStringIndex(paramsXML, -1)
		closeMatches := closeTag.FindAllStringIndex(paramsXML, -1)

		// Skip if we can't find a matching pair
		if len(openMatches) == 0 || len(closeMatches) == 0 {
			continue
		}

		// Take the first occurrence for simplicity
		openPos := openMatches[0][1]   // End position of opening tag
		closePos := closeMatches[0][0] // Start position of closing tag

		// Check if we have valid positions for extraction
		if openPos >= closePos || openPos >= len(paramsXML) || closePos > len(paramsXML) {
			continue
		}

		// Extract the parameter value
		paramValue := strings.TrimSpace(paramsXML[openPos:closePos])

		// Try to convert to appropriate types (boolean, number, etc.)
		switch {
		case paramValue == "true" || paramValue == "false":
			// Boolean
			params[paramName] = paramValue == "true"
			logger.Debug("Parsed XML parameter as boolean", "name", paramName, "value", params[paramName])

		case regexp.MustCompile(`^-?\d+$`).MatchString(paramValue):
			// Integer
			intVal, err := strconv.Atoi(paramValue)
			if err == nil {
				params[paramName] = intVal
				logger.Debug("Parsed XML parameter as integer", "name", paramName, "value", params[paramName])
			} else {
				params[paramName] = paramValue
				logger.Debug("Failed to parse numeric value, using as string", "name", paramName, "value", paramValue)
			}

		case regexp.MustCompile(`^-?\d+\.\d+$`).MatchString(paramValue):
			// Float
			floatVal, err := strconv.ParseFloat(paramValue, 64)
			if err == nil {
				params[paramName] = floatVal
				logger.Debug("Parsed XML parameter as float", "name", paramName, "value", params[paramName])
			} else {
				params[paramName] = paramValue
				logger.Debug("Failed to parse float value, using as string", "name", paramName, "value", paramValue)
			}

		default:
			// String
			params[paramName] = paramValue
			logger.Debug("Parsed XML parameter as string", "name", paramName, "value", paramValue)
		}
	}

	return params
}

// formatXMLValue formats a value for inclusion in XML
func agentFormatXMLValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		// Escape XML special characters
		return agentEscapeXML(v)
	case []interface{}:
		// Format arrays as nested elements
		var builder strings.Builder
		builder.WriteString("\n")
		for i, item := range v {
			builder.WriteString(fmt.Sprintf("    <item index=\"%d\">%v</item>\n", i, agentFormatXMLValue(item)))
		}
		builder.WriteString("  ")
		return builder.String()
	case map[string]interface{}:
		// Format nested maps as nested XML
		var builder strings.Builder
		builder.WriteString("\n")

		// Sort the keys for consistent output
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			builder.WriteString(fmt.Sprintf("    <%s>%v</%s>\n", k, agentFormatXMLValue(v[k]), k))
		}
		builder.WriteString("  ")
		return builder.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// extractLegacyJSONToolCall handles legacy JSON format for backward compatibility
func extractLegacyJSONToolCall(a *agent, toolJSON string, response string, pattern *regexp.Regexp) (*ToolCall, string, bool) {
	a.logger.Debug("Attempting to parse legacy JSON tool call", "json", toolJSON)

	// Parse the tool call
	var toolCall struct {
		Name   string                 `json:"name"`
		Params map[string]interface{} `json:"params"`
	}

	err := json.Unmarshal([]byte(toolJSON), &toolCall)
	if err != nil {
		a.logger.Error("Failed to parse legacy JSON tool call", "error", err, "json", toolJSON)

		// Try with preprocessing
		toolJSON = strings.ReplaceAll(toolJSON, "\n", "")
		toolJSON = strings.ReplaceAll(toolJSON, "\r", "")

		// Try again with cleaned JSON
		err = json.Unmarshal([]byte(toolJSON), &toolCall)
		if err != nil {
			a.logger.Error("Failed to parse legacy JSON after cleaning", "error", err)
			return nil, response, false
		}
	}

	// Validate the tool call has required fields
	if toolCall.Name == "" {
		a.logger.Error("Legacy JSON tool call missing name field", "json", toolJSON)
		return nil, response, false
	}

	if toolCall.Params == nil {
		a.logger.Error("Legacy JSON tool call missing params field", "json", toolJSON)
		return nil, response, false
	}

	// Create the ToolCall object
	result := &ToolCall{
		ToolName: toolCall.Name,
		Params:   toolCall.Params,
	}

	a.logger.Debug("Successfully extracted legacy JSON tool call",
		"toolName", result.ToolName,
		"paramsCount", len(result.Params))

	// Remove the tool section from the response
	remainingText := pattern.ReplaceAllString(response, "")
	remainingText = strings.TrimSpace(remainingText)

	return result, remainingText, true
}

// escapeXML escapes XML special characters
func agentEscapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
