package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

// TodoPersistence handles saving and loading todo plans
type TodoPersistence struct {
	mu       sync.Mutex
	dataDir  string
	fileName string
}

// NewTodoPersistence creates a new persistence handler
func NewTodoPersistence(dataDir string) *TodoPersistence {
	return &TodoPersistence{
		dataDir:  dataDir,
		fileName: "todo_plans.json",
	}
}

// TodoState represents the complete state of all todo plans
type TodoState struct {
	Plans         map[string]*TodoPlan `json:"plans"`
	CurrentPlanID string               `json:"current_plan_id"`
}

// Save persists the current todo state to disk
func (tp *TodoPersistence) Save(manager *TodoManager) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Ensure data directory exists
	if err := os.MkdirAll(tp.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	manager.mu.RLock()
	state := TodoState{
		Plans:         manager.plans,
		CurrentPlanID: manager.currentPlanID,
	}
	manager.mu.RUnlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal todo state: %w", err)
	}

	filePath := filepath.Join(tp.dataDir, tp.fileName)
	if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write todo state: %w", err)
	}

	return nil
}

// Load restores todo state from disk
func (tp *TodoPersistence) Load(manager *TodoManager) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	filePath := filepath.Join(tp.dataDir, tp.fileName)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// No saved state, this is fine
		return nil
	}

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read todo state: %w", err)
	}

	var state TodoState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to unmarshal todo state: %w", err)
	}

	manager.mu.Lock()
	manager.plans = state.Plans
	manager.currentPlanID = state.CurrentPlanID
	manager.mu.Unlock()

	return nil
}

// AutoSave creates a function that automatically saves after operations
func (tp *TodoPersistence) AutoSave(manager *TodoManager) func() {
	return func() {
		if err := tp.Save(manager); err != nil {
			// Log error but don't fail the operation
			fmt.Fprintf(os.Stderr, "Warning: failed to save todo state: %v\n", err)
		}
	}
}

// Initialize global persistence
var todoPersistence *TodoPersistence

func initTodoPersistence() {
	// Use a hidden directory in the project root for persistence
	workDir, _ := os.Getwd()
	dataDir := filepath.Join(workDir, ".codezilla", "todos")
	todoPersistence = NewTodoPersistence(dataDir)

	// Load existing state
	if err := todoPersistence.Load(globalTodoManager); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load todo state: %v\n", err)
	}
}

// Update todo tools to auto-save after modifications
func wrapWithAutoSave(original func(map[string]interface{}) (string, error)) func(map[string]interface{}) (string, error) {
	return func(params map[string]interface{}) (string, error) {
		result, err := original(params)
		if err == nil && todoPersistence != nil {
			todoPersistence.AutoSave(globalTodoManager)()
		}
		return result, err
	}
}

// TodoClearTool clears completed tasks from a plan
type TodoClearTool struct{}

func (t TodoClearTool) Name() string {
	return "todo_clear"
}

func (t TodoClearTool) Description() string {
	return "Clear completed or cancelled tasks from a todo plan"
}

func (t TodoClearTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"plan_id": {Type: "string", Description: "Plan ID to clear (optional, uses current plan)"},
			"status":  {Type: "string", Enum: []interface{}{"completed", "cancelled", "both"}, Default: "completed"},
		},
	}
}

func (t TodoClearTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	globalTodoManager.mu.Lock()
	defer globalTodoManager.mu.Unlock()

	planID := globalTodoManager.currentPlanID
	if pid, ok := params["plan_id"].(string); ok {
		planID = pid
	}

	plan, exists := globalTodoManager.plans[planID]
	if !exists {
		return "", fmt.Errorf("plan not found: %s", planID)
	}

	statusToClear := "completed"
	if s, ok := params["status"].(string); ok {
		statusToClear = s
	}

	var remaining []TodoItem
	var cleared int

	for _, item := range plan.Items {
		shouldClear := false
		switch statusToClear {
		case "completed":
			shouldClear = item.Status == "completed"
		case "cancelled":
			shouldClear = item.Status == "cancelled"
		case "both":
			shouldClear = item.Status == "completed" || item.Status == "cancelled"
		}

		if shouldClear {
			cleared++
		} else {
			remaining = append(remaining, item)
		}
	}

	plan.Items = remaining

	if todoPersistence != nil {
		todoPersistence.AutoSave(globalTodoManager)()
	}

	return fmt.Sprintf("Cleared %d %s tasks from plan '%s'. %d tasks remaining.",
		cleared, statusToClear, plan.Name, len(remaining)), nil
}

func init() {
	// Initialize persistence
	initTodoPersistence()
}

// GetTodoClearTool returns the todo clear tool
func GetTodoClearTool() Tool {
	return TodoClearTool{}
}
