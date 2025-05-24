package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ExecuteTool allows executing shell commands
type ExecuteTool struct {
	// Max execution time before timeout
	Timeout time.Duration
	// AllowedCommands is a whitelist of allowed command names (optional)
	AllowedCommands []string
	// WorkingDir restricts command execution to this directory (optional)
	WorkingDir string
	// DisableShell prevents shell execution entirely
	DisableShell bool
}

// NewExecuteTool creates a new execute tool with the given timeout
func NewExecuteTool(timeout time.Duration) *ExecuteTool {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &ExecuteTool{
		Timeout:      timeout,
		DisableShell: true, // Safe by default
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

	// Validate command for safety
	if err := t.validateCommand(cmdStr); err != nil {
		return nil, &ErrInvalidToolParams{
			ToolName: t.Name(),
			Message:  err.Error(),
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
	var cmd *exec.Cmd
	if t.DisableShell {
		// Parse command safely without shell
		args := parseCommandArgs(cmdStr)
		if len(args) == 0 {
			return nil, &ErrInvalidToolParams{
				ToolName: t.Name(),
				Message:  "empty command",
			}
		}
		cmd = exec.CommandContext(execCtx, args[0], args[1:]...)
	} else {
		// Use shell execution (less safe, but sometimes necessary)
		cmd = exec.CommandContext(execCtx, "sh", "-c", cmdStr)
	}

	// Set working directory if specified
	if t.WorkingDir != "" {
		cmd.Dir = t.WorkingDir
	}

	// Set clean environment to prevent injection via env vars
	cmd.Env = getCleanEnvironment()

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

// validateCommand checks if the command is safe to execute
func (t *ExecuteTool) validateCommand(cmdStr string) error {
	// Check for common dangerous patterns
	dangerousPatterns := []string{
		`;\\s*rm\\s+-rf`,
		`&&\\s*rm\\s+-rf`,
		`\\|\\s*rm\\s+-rf`,
		`>\\s*/dev/s`,
		`dd\\s+if=/dev/zero`,
		`mkfs`,
		`:\(\)\{ :\|:& \};:`, // Fork bomb
	}

	for _, pattern := range dangerousPatterns {
		if matched, _ := regexp.MatchString(pattern, cmdStr); matched {
			return fmt.Errorf("potentially dangerous command pattern detected")
		}
	}

	// If whitelist is configured, check against it
	if len(t.AllowedCommands) > 0 {
		args := parseCommandArgs(cmdStr)
		if len(args) == 0 {
			return fmt.Errorf("empty command")
		}

		cmdName := filepath.Base(args[0])
		allowed := false
		for _, allowedCmd := range t.AllowedCommands {
			if cmdName == allowedCmd {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("command '%s' is not in the allowed list", cmdName)
		}
	}

	return nil
}

// parseCommandArgs safely parses command arguments without shell interpretation
func parseCommandArgs(cmdStr string) []string {
	// Simple argument parsing that handles quoted strings
	var args []string
	var current []rune
	var inQuote rune
	var escaped bool

	for _, r := range cmdStr {
		if escaped {
			current = append(current, r)
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			continue
		}

		if inQuote != 0 {
			if r == inQuote {
				inQuote = 0
			} else {
				current = append(current, r)
			}
			continue
		}

		if r == '"' || r == '\'' {
			inQuote = r
			continue
		}

		if r == ' ' || r == '\t' {
			if len(current) > 0 {
				args = append(args, string(current))
				current = nil
			}
			continue
		}

		current = append(current, r)
	}

	if len(current) > 0 {
		args = append(args, string(current))
	}

	return args
}

// getCleanEnvironment returns a minimal safe environment for command execution
func getCleanEnvironment() []string {
	// Start with a minimal set of safe environment variables
	safeVars := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"USER=" + os.Getenv("USER"),
		"LANG=" + os.Getenv("LANG"),
		"LC_ALL=" + os.Getenv("LC_ALL"),
		"TERM=" + os.Getenv("TERM"),
	}

	// Filter out empty values
	var env []string
	for _, v := range safeVars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 && parts[1] != "" {
			env = append(env, v)
		}
	}

	return env
}
