package cli

import (
	"fmt"
	"strings"
)

// SimpleContextManager provides basic context management
type SimpleContextManager struct {
	messages []Message
	maxSize  int
}

// Message represents a conversation message
type Message struct {
	Role    string
	Content string
}

// NewSimpleContextManager creates a new context manager
func NewSimpleContextManager(maxSize int) *SimpleContextManager {
	if maxSize == 0 {
		maxSize = 10
	}
	return &SimpleContextManager{
		messages: make([]Message, 0, maxSize*2),
		maxSize:  maxSize * 2, // Store both user and assistant messages
	}
}

// AddMessage adds a message to the context
func (cm *SimpleContextManager) AddMessage(role, content string) {
	cm.messages = append(cm.messages, Message{
		Role:    role,
		Content: content,
	})

	// Trim if we exceed max size
	if len(cm.messages) > cm.maxSize {
		cm.messages = cm.messages[len(cm.messages)-cm.maxSize:]
	}
}

// GetContext returns the conversation context as a string
func (cm *SimpleContextManager) GetContext() string {
	if len(cm.messages) == 0 {
		return ""
	}

	var parts []string
	for _, msg := range cm.messages {
		parts = append(parts, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
	}

	return "Previous conversation:\n" + strings.Join(parts, "\n")
}

// Clear clears all messages
func (cm *SimpleContextManager) Clear() {
	cm.messages = cm.messages[:0]
}

// Size returns the number of messages
func (cm *SimpleContextManager) Size() int {
	return len(cm.messages)
}
