package agent

import (
	"codezilla/internal/tools"
	"codezilla/pkg/fsutil"
	"codezilla/pkg/logger"
	"fmt"
	"strings"
	"time"
)

// BasicExecutor implements Executor with common tools
type BasicExecutor struct {
	availableActions map[string]func(string) (string, error)
	fileCtrl         *fsutil.Controller
}

// NewBasicExecutor creates a new executor with standard tools
func NewBasicExecutor() *BasicExecutor {
	fileCtrl := fsutil.NewController(fsutil.DefaultControllerConfig())
	err := fileCtrl.Initialize()
	if err != nil {
		logger.Error("Failed to initialize file controller", "error", err)
	}

	executor := &BasicExecutor{
		availableActions: make(map[string]func(string) (string, error)),
		fileCtrl:         fileCtrl,
	}

	// Register basic actions
	executor.RegisterAction("calculate", func(input string) (string, error) {
		return tools.Calculate(input)
	})

	executor.RegisterAction("datetime", func(input string) (string, error) {
		format := input
		if format == "" {
			format = "2006-01-02 15:04:05"
		}
		return tools.GetDateTime(format), nil
	})

	executor.RegisterAction("search_web", func(input string) (string, error) {
		return tools.WebSearch(input), nil
	})

	executor.RegisterAction("read_file", func(input string) (string, error) {
		parts := strings.SplitN(input, "|", 3)
		if len(parts) < 1 {
			return "", fmt.Errorf("missing file path")
		}

		filePath := strings.TrimSpace(parts[0])
		query := ""
		contextLines := 5

		if len(parts) >= 2 {
			query = strings.TrimSpace(parts[1])
		}

		return tools.ReadFile(filePath, query, contextLines)
	})

	executor.RegisterAction("search_files", func(input string) (string, error) {
		parts := strings.SplitN(input, "|", 3)
		if len(parts) < 1 {
			return "", fmt.Errorf("missing pattern")
		}

		pattern := strings.TrimSpace(parts[0])
		isRegex := false
		caseSensitive := false

		if len(parts) >= 2 {
			isRegex = strings.TrimSpace(parts[1]) == "true"
		}

		if len(parts) >= 3 {
			caseSensitive = strings.TrimSpace(parts[2]) == "true"
		}

		results, err := executor.fileCtrl.SearchFiles(pattern, isRegex, caseSensitive)
		if err != nil {
			return "", err
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d files matching pattern '%s':\n", len(results), pattern))
		for i, result := range results {
			sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, result.FilePath))
		}
		return sb.String(), nil
	})

	executor.RegisterAction("wait", func(input string) (string, error) {
		seconds := 1
		fmt.Sscanf(input, "%d", &seconds)
		if seconds <= 0 {
			seconds = 1
		}
		if seconds > 30 {
			seconds = 30 // Cap at 30 seconds for safety
		}

		time.Sleep(time.Duration(seconds) * time.Second)
		return fmt.Sprintf("Waited for %d seconds", seconds), nil
	})

	return executor
}

// RegisterAction adds a new action to the executor
func (e *BasicExecutor) RegisterAction(name string, handler func(string) (string, error)) {
	e.availableActions[name] = handler
}

// ExecuteAction executes the specified action with the given input
func (e *BasicExecutor) ExecuteAction(action string, input string) (string, error) {
	logger.Debug("Executing action", "action", action, "input", input)

	// Check if action exists
	handler, exists := e.availableActions[action]
	if !exists {
		availableActions := strings.Join(e.GetAvailableActions(), ", ")
		return "", fmt.Errorf("unknown action '%s', available actions: %s", action, availableActions)
	}

	// Execute the action
	result, err := handler(input)
	if err != nil {
		logger.Error("Action execution failed", "action", action, "input", input, "error", err)
		return "", err
	}

	logger.Debug("Action executed successfully", "action", action, "input", input)
	return result, nil
}

// GetAvailableActions returns a list of all available actions
func (e *BasicExecutor) GetAvailableActions() []string {
	actions := make([]string, 0, len(e.availableActions))
	for action := range e.availableActions {
		actions = append(actions, action)
	}
	return actions
}
