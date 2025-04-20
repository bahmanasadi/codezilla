package agent

import (
	"fmt"
	"strings"
	"time"

	"codezilla/pkg/logger"
)

// AutoAgent is an agent that uses an LLM to autonomously select and use tools
// to solve user queries through a chain of reasoning and tool use
type AutoAgent struct {
	Agent
	MaxIterations int
	UserQuery     string
	Thinking      []string
	ToolResults   map[string]string
}

// NewAutoAgent creates a new autonomous agent that uses LLMs to pick tools
func NewAutoAgent(executor Executor) *AutoAgent {
	baseAgent := New(executor)

	// Create auto agent with reasonable defaults
	autoAgent := &AutoAgent{
		Agent:         *baseAgent,
		MaxIterations: 10,
		Thinking:      []string{},
		ToolResults:   make(map[string]string),
	}

	return autoAgent
}

// SystemPrompt returns the system prompt for the auto-agent
func (a *AutoAgent) SystemPrompt() string {
	// Get available tools from the executor
	tools := a.Executor.GetAvailableActions()
	toolsList := strings.Join(tools, ", ")

	return fmt.Sprintf(`You are a helpful autonomous agent with access to various tools.
Your task is to help users by understanding their queries and using tools to find answers.

Available tools: %s

For each step, follow this process:
1. THINK: Consider what you know, what the user is asking, and what you need to find out
2. TOOL: Select the most appropriate tool to use
3. INPUT: Determine the correct input for the tool
4. ANALYZE: Analyze the results from the tool
5. NEXT: Decide what to do next (use another tool or present final answer)

First, analyze the user's query carefully. Then use tools to gather information.
Continue using tools until you have enough information to fully answer the user's query.
If you need to use multiple tools, use them in a logical sequence.

When you have a final answer, preface it with "FINAL ANSWER:"
`, toolsList)
}

// ProcessQuery processes a user query using autonomous reasoning and tool use
func (a *AutoAgent) ProcessQuery(query string) (string, error) {
	a.UserQuery = query
	a.Thinking = []string{}
	a.ToolResults = make(map[string]string)
	a.Reset() // Clear previous agent steps

	// Create the first LLM prompt to analyze the query
	initialPrompt := fmt.Sprintf(`USER QUERY: %s

Analyze this query. What tools might you need to answer it?
Think through the steps you'll need to take to answer this query completely.
`, query)

	// Start the reasoning cycle
	logger.Info("Starting autonomous agent processing", "query", query)

	currentContext := initialPrompt
	iteration := 0
	var finalAnswer string

	for iteration < a.MaxIterations {
		iteration++
		logger.Debug("Running agent iteration", "iteration", iteration, "context_length", len(currentContext))

		// Ask the LLM to think about the next step
		thinking, err := a.executeThinking(currentContext)
		if err != nil {
			return "", fmt.Errorf("thinking error: %v", err)
		}
		a.Thinking = append(a.Thinking, thinking)

		// Parse thinking to extract tool use
		toolName, toolInput, isFinalAnswer := a.parseThinking(thinking)

		// If we have a final answer, we're done
		if isFinalAnswer {
			finalAnswer = a.extractFinalAnswer(thinking)
			break
		}

		// If no tool was selected, but no final answer either, add more guidance
		if toolName == "" {
			currentContext += "\n\nYou didn't select a specific tool. Please select one of the available tools and provide input for it, or provide a final answer if you have enough information."
			continue
		}

		// Execute the tool
		toolResult, err := a.executeToolUse(toolName, toolInput)
		if err != nil {
			// Tool execution failed, include the error in context and continue
			errorMsg := fmt.Sprintf("\nTool '%s' failed with error: %v\n", toolName, err)
			currentContext += "\n" + thinking + errorMsg
			currentContext += "\nPlease try a different approach or tool."
			continue
		}

		// Record tool results for history
		resultKey := fmt.Sprintf("%s(%s)", toolName, toolInput)
		a.ToolResults[resultKey] = toolResult

		// Update context with results
		currentContext += "\n\nYou thought: " + thinking
		currentContext += fmt.Sprintf("\n\nYou used tool: %s with input: %s", toolName, toolInput)
		currentContext += "\nTool Result: " + toolResult
		currentContext += "\n\nBased on this result, what will you do next? Choose another tool or provide a final answer."
	}

	// If we reached max iterations without a final answer
	if finalAnswer == "" {
		finalAnswer = "I've explored multiple approaches but couldn't reach a definitive answer within the iteration limit. Here's what I found:\n\n"
		for i, thinking := range a.Thinking {
			finalAnswer += fmt.Sprintf("Step %d: %s\n\n", i+1, thinking)
		}
	}

	return finalAnswer, nil
}

// executeThinking asks the LLM to think about what to do next
func (a *AutoAgent) executeThinking(context string) (string, error) {
	// Use system prompt for context
	systemPrompt := a.SystemPrompt()

	// Set system prompt if using OllamaExecutor
	if ollamaExec, ok := a.Executor.(*OllamaExecutor); ok {
		ollamaExec.systemPrompt = systemPrompt
	}

	// Ask the LLM to think about the next step
	result, err := a.Executor.ExecuteAction("think", context)
	if err != nil {
		return "", err
	}

	return result, nil
}

// parseThinking extracts tool name, tool input, and whether this is a final answer
func (a *AutoAgent) parseThinking(thinking string) (string, string, bool) {
	thinking = strings.TrimSpace(thinking)

	// Check if this is a final answer
	if strings.Contains(strings.ToUpper(thinking), "FINAL ANSWER:") {
		return "", "", true
	}

	// Look for tool selection patterns
	toolPatterns := []string{
		"I'll use the `",
		"I should use the `",
		"Let's use the `",
		"Use tool: `",
		"Tool: `",
		"TOOL: `",
	}

	var toolName, toolInput string

	// Try to find a tool name
	for _, pattern := range toolPatterns {
		if strings.Contains(thinking, pattern) {
			parts := strings.Split(thinking, pattern)
			if len(parts) > 1 {
				// Extract the tool name
				toolPart := parts[1]
				endIdx := strings.Index(toolPart, "`")
				if endIdx > 0 {
					toolName = toolPart[:endIdx]
					break
				}
			}
		}
	}

	// If no tool name found using patterns, look for any of the available tool names
	if toolName == "" {
		availableTools := a.Executor.GetAvailableActions()
		for _, tool := range availableTools {
			// Look for the tool name with indicators like "use" or "tool"
			searchTerms := []string{
				"use " + tool,
				"using " + tool,
				"tool " + tool,
				"tool: " + tool,
				"TOOL: " + tool,
			}

			for _, term := range searchTerms {
				if strings.Contains(strings.ToLower(thinking), strings.ToLower(term)) {
					toolName = tool
					break
				}
			}

			if toolName != "" {
				break
			}
		}
	}

	// If still no tool found, return empty
	if toolName == "" {
		return "", "", false
	}

	// Look for tool input patterns
	inputPatterns := []string{
		"with input `",
		"input: `",
		"INPUT: `",
		"with the query `",
		"with parameter `",
		"parameter: `",
	}

	// Try to find the input
	for _, pattern := range inputPatterns {
		if strings.Contains(thinking, pattern) {
			parts := strings.Split(thinking, pattern)
			if len(parts) > 1 {
				// Extract the input
				inputPart := parts[1]
				endIdx := strings.Index(inputPart, "`")
				if endIdx > 0 {
					toolInput = inputPart[:endIdx]
					break
				}
			}
		}
	}

	// If no structured input found, look for descriptions after the tool name
	if toolInput == "" {
		// Look for sentences mentioning the tool
		sentences := strings.Split(thinking, ".")
		for _, sentence := range sentences {
			if strings.Contains(strings.ToLower(sentence), strings.ToLower(toolName)) {
				// Extract anything that might be input-like after the tool name
				parts := strings.SplitN(sentence, toolName, 2)
				if len(parts) > 1 {
					// Clean up the potential input
					candidate := strings.TrimSpace(parts[1])
					candidate = strings.Trim(candidate, "` \t\n:,")
					if candidate != "" {
						toolInput = candidate
						break
					}
				}
			}
		}
	}

	// If we have a tool but no input, try to infer from context
	if toolInput == "" && toolName != "" {
		// Look through the thinking for what might be relevant input
		// Default to the last few sentences if can't find anything specific
		sentences := strings.Split(thinking, ".")
		if len(sentences) > 0 {
			// Use the last non-empty sentence as a fallback input
			for i := len(sentences) - 1; i >= 0; i-- {
				candidate := strings.TrimSpace(sentences[i])
				if candidate != "" && !strings.Contains(strings.ToLower(candidate), "tool") {
					toolInput = candidate
					break
				}
			}
		}
	}

	// If still no input, use a portion of user query as default
	if toolInput == "" && toolName != "" {
		toolInput = a.UserQuery
		// Truncate if too long
		if len(toolInput) > 100 {
			toolInput = toolInput[:100] + "..."
		}
	}

	return toolName, toolInput, false
}

// executeToolUse executes a tool with the provided input
func (a *AutoAgent) executeToolUse(toolName, toolInput string) (string, error) {
	startTime := time.Now()
	logger.Debug("Executing tool", "tool", toolName, "input", toolInput)

	// Execute the tool
	result, err := a.Execute("Auto-agent using tool", toolName, toolInput)
	if err != nil {
		return "", err
	}

	logger.Debug("Tool execution completed",
		"tool", toolName,
		"duration_ms", time.Since(startTime).Milliseconds(),
		"result_length", len(result))

	return result, nil
}

// extractFinalAnswer extracts the final answer from thinking
func (a *AutoAgent) extractFinalAnswer(thinking string) string {
	thinking = strings.TrimSpace(thinking)

	// Look for "FINAL ANSWER:" marker
	if idx := strings.Index(strings.ToUpper(thinking), "FINAL ANSWER:"); idx >= 0 {
		answer := thinking[idx+len("FINAL ANSWER:"):]
		return strings.TrimSpace(answer)
	}

	// If no marker found, return the whole thinking
	return thinking
}

// GetSummary returns a summary of the agent's processing
func (a *AutoAgent) GetSummary() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Query: %s\n\n", a.UserQuery))

	// Add thinking steps
	sb.WriteString("Reasoning Steps:\n")
	for i, thinking := range a.Thinking {
		sb.WriteString(fmt.Sprintf("Step %d: %s\n\n", i+1, thinking))
	}

	// Add tool use history
	sb.WriteString("Tools Used:\n")
	for tool, result := range a.ToolResults {
		sb.WriteString(fmt.Sprintf("- %s\n", tool))
		if len(result) > 200 {
			result = result[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("  Result: %s\n\n", result))
	}

	return sb.String()
}
