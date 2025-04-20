package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"codezilla/pkg/logger"
)

// AdvancedExecutor is a more advanced implementation of Executor
// with memory, chaining capabilities, and specialized tools
type AdvancedExecutor struct {
	BasicExecutor
	memory map[string]string
}

// NewAdvancedExecutor creates a new advanced executor
func NewAdvancedExecutor() *AdvancedExecutor {
	basic := NewBasicExecutor()
	advanced := &AdvancedExecutor{
		BasicExecutor: *basic,
		memory:        make(map[string]string),
	}

	// Register additional specialized actions
	advanced.RegisterAction("remember", advanced.handleRemember)
	advanced.RegisterAction("recall", advanced.handleRecall)
	advanced.RegisterAction("forget", advanced.handleForget)
	advanced.RegisterAction("list_memories", advanced.handleListMemories)
	advanced.RegisterAction("chain", advanced.handleChain)
	advanced.RegisterAction("reflect", advanced.handleReflect)

	return advanced
}

// handleRemember stores a key-value pair in memory
func (e *AdvancedExecutor) handleRemember(input string) (string, error) {
	parts := strings.SplitN(input, "|", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("remember requires key|value format")
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	if key == "" {
		return "", fmt.Errorf("key cannot be empty")
	}

	e.memory[key] = value
	logger.Debug("Stored memory", "key", key, "value_length", len(value))

	return fmt.Sprintf("Memory stored with key '%s'", key), nil
}

// handleRecall retrieves a value from memory by key
func (e *AdvancedExecutor) handleRecall(input string) (string, error) {
	key := strings.TrimSpace(input)
	if key == "" {
		return "", fmt.Errorf("key cannot be empty")
	}

	value, exists := e.memory[key]
	if !exists {
		return "", fmt.Errorf("no memory found with key '%s'", key)
	}

	logger.Debug("Retrieved memory", "key", key, "value_length", len(value))
	return value, nil
}

// handleForget removes a key-value pair from memory
func (e *AdvancedExecutor) handleForget(input string) (string, error) {
	key := strings.TrimSpace(input)
	if key == "" {
		return "", fmt.Errorf("key cannot be empty")
	}

	if key == "*" {
		count := len(e.memory)
		e.memory = make(map[string]string)
		return fmt.Sprintf("Cleared all %d memories", count), nil
	}

	_, exists := e.memory[key]
	if !exists {
		return "", fmt.Errorf("no memory found with key '%s'", key)
	}

	delete(e.memory, key)
	logger.Debug("Removed memory", "key", key)

	return fmt.Sprintf("Memory with key '%s' has been forgotten", key), nil
}

// handleListMemories lists all keys in memory
func (e *AdvancedExecutor) handleListMemories(input string) (string, error) {
	if len(e.memory) == 0 {
		return "No memories stored", nil
	}

	keys := make([]string, 0, len(e.memory))
	for key := range e.memory {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories:\n", len(keys)))
	for i, key := range keys {
		value := e.memory[key]
		preview := value
		if len(preview) > 30 {
			preview = preview[:27] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, key, preview))
	}

	return sb.String(), nil
}

// handleChain executes a sequence of actions
func (e *AdvancedExecutor) handleChain(input string) (string, error) {
	lines := strings.Split(input, "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("chain requires a sequence of actions")
	}

	var results []string
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid action format at line %d, expected 'action: input'", i+1)
		}

		action := strings.TrimSpace(parts[0])
		actionInput := strings.TrimSpace(parts[1])

		logger.Debug("Chain executing action", "action", action, "input", actionInput)

		result, err := e.ExecuteAction(action, actionInput)
		if err != nil {
			return "", fmt.Errorf("error at line %d (%s): %s", i+1, action, err.Error())
		}

		results = append(results, fmt.Sprintf("Step %d (%s): %s", i+1, action, result))
	}

	return strings.Join(results, "\n\n"), nil
}

// handleReflect performs meta-reasoning about the agent's state
func (e *AdvancedExecutor) handleReflect(input string) (string, error) {
	var sb strings.Builder

	sb.WriteString("Agent Reflection:\n")
	sb.WriteString(fmt.Sprintf("- Timestamp: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- Memory Size: %d items\n", len(e.memory)))
	sb.WriteString(fmt.Sprintf("- Available Actions: %d\n", len(e.availableActions)))

	if input != "" {
		sb.WriteString(fmt.Sprintf("\nReflection Prompt: %s\n", input))
		sb.WriteString("\nAgent Self-Assessment:\n")
		sb.WriteString("- The agent is capable of executing a variety of tools\n")
		sb.WriteString("- The agent has memory capabilities to store and retrieve information\n")
		sb.WriteString("- The agent can chain multiple actions together\n")
		sb.WriteString(fmt.Sprintf("- Current focus: %s\n", input))
	}

	return sb.String(), nil
}
