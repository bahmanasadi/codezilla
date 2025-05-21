package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codezilla/llm/ollama"
	"codezilla/pkg/logger"
)

// ContextAnalyzer is the interface for analyzing whether a prompt requires context
type ContextAnalyzer interface {
	// NeedsContext analyzes a prompt and returns true if it requires previous context
	NeedsContext(ctx context.Context, prompt string) (bool, error)
}

// LLMContextAnalyzer uses an LLM to determine if a prompt requires context
type LLMContextAnalyzer struct {
	client  ollama.Client
	model   string
	logger  *logger.Logger
	timeout time.Duration
}

// NewLLMContextAnalyzer creates a new context analyzer
func NewLLMContextAnalyzer(client ollama.Client, model string, logger *logger.Logger) *LLMContextAnalyzer {
	return &LLMContextAnalyzer{
		client:  client,
		model:   model,
		logger:  logger,
		timeout: 5 * time.Second, // Default timeout of 5 seconds
	}
}

// NeedsContext determines if a prompt requires previous conversation context
func (a *LLMContextAnalyzer) NeedsContext(ctx context.Context, prompt string) (bool, error) {
	// Create a timeout context to ensure the analysis doesn't take too long
	timeoutCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	a.logger.Debug("Analyzing context need for prompt", "prompt", prompt, "using_model", a.model)

	// Check for common patterns that indicate a need for context before using the LLM
	// This can save time for obvious cases
	if containsContextIndicators(prompt) {
		a.logger.Debug("Pattern matching detected context requirement", "prompt", prompt)
		return true, nil
	}

	// The system prompt specifically for context analysis - simplified for smaller models
	systemPrompt := `You are analyzing if a prompt requires previous conversation context.
Answer YES if the prompt:
- References previous messages
- Uses pronouns without clear referents (it, this, that, these, those)
- Refers to something not mentioned in the prompt
- Is a follow-up question
- Is a command to continue or tell more
Answer NO if the prompt is standalone and complete by itself.
Output ONLY YES or NO.`

	// Prepare the prompt for analysis - keep it simple and short for small models
	userPrompt := fmt.Sprintf("Does this prompt need context to be understood? Prompt: %s", prompt)

	// Create generate request with minimal parameters
	request := ollama.GenerateRequest{
		Model:  a.model,
		Prompt: userPrompt,
		System: systemPrompt,
		Stream: false,
		Options: map[string]interface{}{
			"temperature": 0.1, // Low temperature for more deterministic responses
			"num_predict": 10,  // We only need a few tokens for YES/NO
		},
	}

	// Send request to Ollama
	startTime := time.Now()
	response, err := a.client.Generate(timeoutCtx, request)
	duration := time.Since(startTime)

	if err != nil {
		a.logger.Error("Failed to get response from Ollama for context analysis",
			"error", err,
			"duration", duration.String())
		return true, fmt.Errorf("failed to analyze context need: %w", err)
	}

	a.logger.Debug("Received context analysis response",
		"responseLength", len(response.Response),
		"response", response.Response,
		"duration", duration.String())

	// Process the response - looking for a clear YES or NO
	cleanResponse := strings.TrimSpace(response.Response)
	needsContext := false

	if strings.Contains(strings.ToUpper(cleanResponse), "YES") {
		needsContext = true
	}

	a.logger.Info("Context analysis result",
		"prompt", prompt,
		"needsContext", needsContext,
		"model", a.model,
		"duration", duration.String())

	return needsContext, nil
}

// containsContextIndicators checks for common patterns that suggest context dependence
func containsContextIndicators(prompt string) bool {
	prompt = strings.ToLower(prompt)

	// Common context-dependent phrases
	contextPhrases := []string{
		"as mentioned", "as you said", "as i said", "as we discussed",
		"continue", "go on", "tell me more", "what else", "and then", "proceed",
		"next steps", "next step", "previous", "above", "earlier",
		"you told me", "you mentioned", "you said", "you suggested",
	}

	// Common demonstrative pronouns without clear referents
	demonstratives := []string{
		" it ", " its ", " it's ", " it.", " it,", " it?", " it!",
		" this ", " this.", " this,", " this?", " this!",
		" that ", " that.", " that,", " that?", " that!",
		" these ", " these.", " these,", " these?", " these!",
		" those ", " those.", " those,", " those?", " those!",
		" they ", " them ", " their ", " theirs ",
		" he ", " him ", " his ", " she ", " her ", " hers ",
	}

	// Phrases that typically start followup questions
	followups := []string{
		"and what about", "what if", "why is that", "how about",
		"could you explain", "do you mean", "but why", "and why",
	}

	// Check for any known context indicators
	paddedPrompt := " " + prompt + " " // Pad for better matching

	for _, phrase := range contextPhrases {
		if strings.Contains(paddedPrompt, phrase) {
			return true
		}
	}

	for _, pronoun := range demonstratives {
		if strings.Contains(paddedPrompt, pronoun) {
			return true
		}
	}

	for _, followup := range followups {
		if strings.Contains(paddedPrompt, followup) {
			return true
		}
	}

	return false
}
