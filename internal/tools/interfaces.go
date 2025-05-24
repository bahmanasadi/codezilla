package tools

import (
	"context"
)

// LLMClient interface for LLM operations
type LLMClient interface {
	GenerateResponse(ctx context.Context, messages []LLMMessage) (string, error)
}

// LLMMessage represents a message in the LLM conversation
type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
