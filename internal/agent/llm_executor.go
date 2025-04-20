package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"codezilla/pkg/logger"
)

// OllamaExecutor is an implementation of Executor that uses Ollama as a backend
type OllamaExecutor struct {
	BasicExecutor
	model          string
	baseURL        string
	systemPrompt   string
	temperature    float64
	maxTokens      int
	responseFormat string
	httpClient     *http.Client
}

// OllamaRequest represents a request to the Ollama API
type OllamaRequest struct {
	Model    string         `json:"model"`
	Prompt   string         `json:"prompt"`
	System   string         `json:"system,omitempty"`
	Template string         `json:"template,omitempty"`
	Context  []int          `json:"context,omitempty"`
	Options  *OllamaOptions `json:"options,omitempty"`
	Format   string         `json:"format,omitempty"`
	Stream   bool           `json:"stream,omitempty"`
}

// OllamaOptions represents options for the Ollama API
type OllamaOptions struct {
	Temperature      float64 `json:"temperature,omitempty"`
	TopP             float64 `json:"top_p,omitempty"`
	TopK             int     `json:"top_k,omitempty"`
	NumPredict       int     `json:"num_predict,omitempty"`
	NumKeep          int     `json:"num_keep,omitempty"`
	Seed             int     `json:"seed,omitempty"`
	FrequencyPenalty float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64 `json:"presence_penalty,omitempty"`
	Mirostat         int     `json:"mirostat,omitempty"`
	MirostatEta      float64 `json:"mirostat_eta,omitempty"`
	MirostatTau      float64 `json:"mirostat_tau,omitempty"`
	RepeatPenalty    float64 `json:"repeat_penalty,omitempty"`
	RepeatLastN      int     `json:"repeat_last_n,omitempty"`
	Tfs_z            float64 `json:"tfs_z,omitempty"`
	Typical_p        float64 `json:"typical_p,omitempty"`
}

// OllamaResponse represents a response from the Ollama API
type OllamaResponse struct {
	Model              string `json:"model"`
	Response           string `json:"response"`
	CreatedAt          string `json:"created_at,omitempty"`
	Done               bool   `json:"done"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
}

// NewOllamaExecutor creates a new executor that uses Ollama
func NewOllamaExecutor(model string, baseURL string) *OllamaExecutor {
	// Create the basic executor first to inherit all standard tools
	basic := NewBasicExecutor()

	// Create an HTTP client with reasonable timeouts
	httpClient := &http.Client{
		Timeout: 120 * time.Second, // 2-minute timeout for model calls
	}

	executor := &OllamaExecutor{
		BasicExecutor:  *basic,
		model:          model,
		baseURL:        baseURL,
		systemPrompt:   "You are a helpful AI assistant with access to various tools. Always follow instructions exactly.",
		temperature:    0.7,
		maxTokens:      4096,
		responseFormat: "text", // or "json"
		httpClient:     httpClient,
	}

	// Register LLM-specific actions
	executor.RegisterAction("ask", executor.handleAsk)
	executor.RegisterAction("think", executor.handleThink)
	executor.RegisterAction("analyze", executor.handleAnalyze)
	executor.RegisterAction("summarize", executor.handleSummarize)
	executor.RegisterAction("setSystemPrompt", executor.handleSetSystemPrompt)
	executor.RegisterAction("setModel", executor.handleSetModel)
	executor.RegisterAction("getConfig", executor.handleGetConfig)

	return executor
}

// handleAsk processes a user query through the LLM
func (e *OllamaExecutor) handleAsk(input string) (string, error) {
	prompt := fmt.Sprintf("User query: %s\nPlease respond directly to the query.", input)
	return e.callOllama(prompt)
}

// handleThink uses the LLM to reason about a problem
func (e *OllamaExecutor) handleThink(input string) (string, error) {
	prompt := fmt.Sprintf("I need to reason about the following problem or situation:\n\n%s\n\nPlease think step-by-step about this situation and provide insights.", input)
	return e.callOllama(prompt)
}

// handleAnalyze uses the LLM to analyze data or text
func (e *OllamaExecutor) handleAnalyze(input string) (string, error) {
	prompt := fmt.Sprintf("Please analyze the following data or text:\n\n%s\n\nProvide a detailed analysis including key points, patterns, and insights.", input)
	return e.callOllama(prompt)
}

// handleSummarize uses the LLM to summarize text
func (e *OllamaExecutor) handleSummarize(input string) (string, error) {
	prompt := fmt.Sprintf("Please summarize the following text concisely while preserving the key information:\n\n%s", input)
	return e.callOllama(prompt)
}

// handleSetSystemPrompt changes the system prompt
func (e *OllamaExecutor) handleSetSystemPrompt(input string) (string, error) {
	e.systemPrompt = input
	return fmt.Sprintf("System prompt updated to: %s", input), nil
}

// handleSetModel changes the model used by the executor
func (e *OllamaExecutor) handleSetModel(input string) (string, error) {
	// Check if the model exists by making a small test query
	oldModel := e.model
	e.model = input

	_, err := e.callOllama("Test query to verify model exists")
	if err != nil {
		// Revert to the old model if the new one doesn't work
		e.model = oldModel
		return "", fmt.Errorf("failed to switch to model '%s': %v", input, err)
	}

	return fmt.Sprintf("Model switched to: %s", input), nil
}

// handleGetConfig returns the current configuration
func (e *OllamaExecutor) handleGetConfig(input string) (string, error) {
	config := fmt.Sprintf(`Current Configuration:
Model: %s
Base URL: %s
System Prompt: %s
Temperature: %.1f
Max Tokens: %d
Response Format: %s`,
		e.model, e.baseURL, e.systemPrompt, e.temperature, e.maxTokens, e.responseFormat)

	return config, nil
}

// callOllama makes a request to the Ollama API
func (e *OllamaExecutor) callOllama(prompt string) (string, error) {
	logger.Debug("Calling Ollama",
		"model", e.model,
		"url", e.baseURL,
		"prompt_length", len(prompt))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Prepare the request
	request := OllamaRequest{
		Model:  e.model,
		Prompt: prompt,
		System: e.systemPrompt,
		Options: &OllamaOptions{
			Temperature: e.temperature,
			NumPredict:  e.maxTokens,
		},
		Format: e.responseFormat,
		Stream: false,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/generate", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	startTime := time.Now()
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call Ollama API: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama API returned error status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var ollamaResp OllamaResponse
	err = json.Unmarshal(respBody, &ollamaResp)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}

	duration := time.Since(startTime)
	logger.Debug("Ollama response received",
		"duration_ms", duration.Milliseconds(),
		"model", ollamaResp.Model,
		"eval_count", ollamaResp.EvalCount,
		"response_length", len(ollamaResp.Response))

	// Clean up the response
	response := strings.TrimSpace(ollamaResp.Response)

	return response, nil
}
