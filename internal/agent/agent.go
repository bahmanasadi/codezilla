package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"codezilla/pkg/logger"
)

// Step represents a single step in the agent's execution
type Step struct {
	Thought string `json:"thought,omitempty"`
	Action  string `json:"action,omitempty"`
	Input   string `json:"input,omitempty"`
	Result  string `json:"result,omitempty"`
}

// Executor is an interface for executing actions
type Executor interface {
	ExecuteAction(action string, input string) (string, error)
	GetAvailableActions() []string
}

// Agent manages the interaction using thought-action cycles
type Agent struct {
	Executor Executor
	Steps    []Step
	MaxSteps int
}

// New creates a new agent with the provided executor
func New(executor Executor) *Agent {
	return &Agent{
		Executor: executor,
		Steps:    []Step{},
		MaxSteps: 10, // Default max steps
	}
}

// SetMaxSteps sets the maximum number of steps allowed for the agent
func (a *Agent) SetMaxSteps(max int) {
	a.MaxSteps = max
}

// GetHistory returns the step history as a formatted string
func (a *Agent) GetHistory() string {
	var sb strings.Builder

	for i, step := range a.Steps {
		sb.WriteString(fmt.Sprintf("Step %d:\n", i+1))
		if step.Thought != "" {
			sb.WriteString(fmt.Sprintf("  Thought: %s\n", step.Thought))
		}
		if step.Action != "" && step.Input != "" {
			sb.WriteString(fmt.Sprintf("  Action: %s(%s)\n", step.Action, step.Input))
		}
		if step.Result != "" {
			sb.WriteString(fmt.Sprintf("  Result: %s\n", step.Result))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// GetHistoryJSON returns the step history as a JSON string
func (a *Agent) GetHistoryJSON() (string, error) {
	historyJSON, err := json.Marshal(a.Steps)
	if err != nil {
		return "", err
	}
	return string(historyJSON), nil
}

// Execute executes a single step with the provided thought, action, and input
func (a *Agent) Execute(thought, action, input string) (string, error) {
	if len(a.Steps) >= a.MaxSteps {
		return "", fmt.Errorf("maximum number of steps (%d) exceeded", a.MaxSteps)
	}

	logger.Debug("Executing agent step",
		"thought", thought,
		"action", action,
		"input", input,
		"step", len(a.Steps)+1)

	// Create a new step
	step := Step{
		Thought: thought,
		Action:  action,
		Input:   input,
	}

	// Execute the action
	result, err := a.Executor.ExecuteAction(action, input)
	if err != nil {
		// Add to steps even if error occurred
		step.Result = fmt.Sprintf("Error: %s", err.Error())
		a.Steps = append(a.Steps, step)
		return "", err
	}

	// Record the result
	step.Result = result
	a.Steps = append(a.Steps, step)

	return result, nil
}

// Reset clears the agent's step history
func (a *Agent) Reset() {
	a.Steps = []Step{}
}
