package core

import (
	"codezilla/internal/tools"
	"codezilla/llm/ollama"
	"context"
)

// LLMClientAdapter adapts ollama.Client to tools.LLMClient
type LLMClientAdapter struct {
	client ollama.Client
}

// NewLLMClientAdapter creates a new adapter
func NewLLMClientAdapter(client ollama.Client) *LLMClientAdapter {
	return &LLMClientAdapter{client: client}
}

// GenerateResponse adapts the GenerateResponse call
func (a *LLMClientAdapter) GenerateResponse(ctx context.Context, messages []tools.LLMMessage) (string, error) {
	// For now, we'll use a simple approach - concatenate messages into a single prompt
	// In a real implementation, we'd want to use the Ollama chat API
	var prompt string
	for _, msg := range messages {
		if msg.Role == "system" {
			prompt += "System: " + msg.Content + "\n\n"
		} else if msg.Role == "user" {
			prompt += "User: " + msg.Content + "\n\n"
		}
	}

	// Use the default model for analysis
	resp, err := a.client.Generate(ctx, ollama.GenerateRequest{
		Model:  "qwen3:14b",
		Prompt: prompt,
		Stream: false,
	})

	if err != nil {
		return "", err
	}

	return resp.Response, nil
}
