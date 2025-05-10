package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ExecuteTool allows executing shell commands
type ExecuteTool struct {
	// Max execution time before timeout
	Timeout time.Duration
}

// NewExecuteTool creates a new execute tool with the given timeout
func NewExecuteTool(timeout time.Duration) *ExecuteTool {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &ExecuteTool{
		Timeout: timeout,
	}
}

// Name returns the tool name
func (t *ExecuteTool) Name() string {
	return "execute"
}

// Description returns the tool description
func (t *ExecuteTool) Description() string {
	return "Executes a shell command and returns its output"
}

// ParameterSchema returns the JSON schema for this tool's parameters
func (t *ExecuteTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"command": {
				Type:        "string",
				Description: "The shell command to execute",
			},
			"timeout_ms": {
				Type:        "integer",
				Description: fmt.Sprintf("Timeout in milliseconds (default: %d)", t.Timeout.Milliseconds()),
				Minimum:     ptr(float64(100)),
				Maximum:     ptr(float64(120000)), // 2 minutes max
			},
		},
		Required: []string{"command"},
	}
}

// Execute runs the shell command and returns its output
func (t *ExecuteTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Validate parameters
	if err := ValidateToolParams(t, params); err != nil {
		return nil, err
	}

	// Extract command
	cmdStr, ok := params["command"].(string)
	if !ok {
		return nil, &ErrInvalidToolParams{
			ToolName: t.Name(),
			Message:  "command must be a string",
		}
	}

	// Extract timeout if provided
	timeout := t.Timeout
	if timeoutMs, ok := params["timeout_ms"].(float64); ok {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	// Create a context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(execCtx, "sh", "-c", cmdStr)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Prepare result
	result := map[string]interface{}{
		"command":     cmdStr,
		"stdout":      stdout.String(),
		"stderr":      stderr.String(),
		"duration_ms": duration.Milliseconds(),
	}

	// Handle errors
	if err != nil {
		// Check if it was a timeout
		if execCtx.Err() == context.DeadlineExceeded {
			result["error"] = fmt.Sprintf("command timed out after %s", timeout)
			result["timed_out"] = true
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			// Command ran but returned non-zero exit code
			result["exit_code"] = exitErr.ExitCode()
			result["error"] = fmt.Sprintf("command exited with code %d", exitErr.ExitCode())
		} else {
			// Other error
			result["error"] = err.Error()
		}
		return result, nil
	}

	// Success
	result["exit_code"] = 0
	result["success"] = true

	// Trim trailing newlines from stdout for cleaner output
	result["stdout"] = strings.TrimRight(result["stdout"].(string), "\n")

	return result, nil
}

// Helper function to create pointer to float64
func ptr(v float64) *float64 {
	return &v
}
