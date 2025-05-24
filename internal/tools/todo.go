package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// TodoItem represents a single todo task
type TodoItem struct {
	ID           string     `json:"id"`
	Content      string     `json:"content"`
	Status       string     `json:"status"`   // pending, in_progress, completed, cancelled
	Priority     string     `json:"priority"` // high, medium, low
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Dependencies []string   `json:"dependencies,omitempty"` // IDs of tasks that must be completed first
}

// TodoPlan represents a collection of todo items with planning metadata
type TodoPlan struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Items       []TodoItem `json:"items"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TodoManager manages todo lists and plans
type TodoManager struct {
	mu            sync.RWMutex
	plans         map[string]*TodoPlan
	currentPlanID string
}

// NewTodoManager creates a new todo manager
func NewTodoManager() *TodoManager {
	return &TodoManager{
		plans: make(map[string]*TodoPlan),
	}
}

// Global todo manager instance
var globalTodoManager = NewTodoManager()

// TodoCreateTool creates new todo plans
type TodoCreateTool struct{}

func (t TodoCreateTool) Name() string {
	return "todo_create"
}

func (t TodoCreateTool) Description() string {
	return "Create a new todo plan with tasks"
}

func (t TodoCreateTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"name":        {Type: "string", Description: "Name of the todo plan"},
			"description": {Type: "string", Description: "Description of what this plan aims to achieve"},
			"items": {
				Type:        "array",
				Description: "List of todo items",
				Items: &JSONSchema{
					Type: "object",
					Properties: map[string]JSONSchema{
						"content":  {Type: "string", Description: "Task description"},
						"priority": {Type: "string", Enum: []interface{}{"high", "medium", "low"}, Default: "medium"},
						"dependencies": {
							Type:        "array",
							Items:       &JSONSchema{Type: "string"},
							Description: "IDs of tasks that must be completed first",
						},
					},
					Required: []string{"content"},
				},
			},
		},
		Required: []string{"name", "items"},
	}
}

func (t TodoCreateTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	name, _ := params["name"].(string)
	description, _ := params["description"].(string)

	plan := &TodoPlan{
		ID:          fmt.Sprintf("plan_%d", time.Now().UnixNano()),
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Items:       []TodoItem{},
	}

	if items, ok := params["items"].([]interface{}); ok {
		for i, item := range items {
			if itemMap, ok := item.(map[string]interface{}); ok {
				todoItem := TodoItem{
					ID:        fmt.Sprintf("task_%d_%d", time.Now().UnixNano(), i),
					Content:   itemMap["content"].(string),
					Status:    "pending",
					Priority:  "medium",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}

				if priority, ok := itemMap["priority"].(string); ok {
					todoItem.Priority = priority
				}

				if deps, ok := itemMap["dependencies"].([]interface{}); ok {
					for _, dep := range deps {
						if depStr, ok := dep.(string); ok {
							todoItem.Dependencies = append(todoItem.Dependencies, depStr)
						}
					}
				}

				plan.Items = append(plan.Items, todoItem)
			}
		}
	}

	globalTodoManager.mu.Lock()
	globalTodoManager.plans[plan.ID] = plan
	globalTodoManager.currentPlanID = plan.ID
	globalTodoManager.mu.Unlock()

	result, _ := json.MarshalIndent(plan, "", "  ")
	return fmt.Sprintf("Created todo plan:\n%s", string(result)), nil
}

// TodoUpdateTool updates todo item status
type TodoUpdateTool struct{}

func (t TodoUpdateTool) Name() string {
	return "todo_update"
}

func (t TodoUpdateTool) Description() string {
	return "Update the status of todo items"
}

func (t TodoUpdateTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"plan_id": {Type: "string", Description: "Plan ID (optional, uses current plan if not specified)"},
			"task_id": {Type: "string", Description: "Task ID to update"},
			"status":  {Type: "string", Enum: []interface{}{"pending", "in_progress", "completed", "cancelled"}},
			"content": {Type: "string", Description: "Updated task content (optional)"},
		},
		Required: []string{"task_id", "status"},
	}
}

func (t TodoUpdateTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	taskID, _ := params["task_id"].(string)
	status, _ := params["status"].(string)

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

	for i := range plan.Items {
		if plan.Items[i].ID == taskID {
			plan.Items[i].Status = status
			plan.Items[i].UpdatedAt = time.Now()

			if status == "completed" {
				now := time.Now()
				plan.Items[i].CompletedAt = &now
			}

			if content, ok := params["content"].(string); ok {
				plan.Items[i].Content = content
			}

			plan.UpdatedAt = time.Now()
			return fmt.Sprintf("Updated task %s to status: %s", taskID, status), nil
		}
	}

	return "", fmt.Errorf("task not found: %s", taskID)
}

// TodoListTool lists current todo plans and items
type TodoListTool struct{}

func (t TodoListTool) Name() string {
	return "todo_list"
}

func (t TodoListTool) Description() string {
	return "List todo plans and their items"
}

func (t TodoListTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"plan_id":       {Type: "string", Description: "Specific plan ID to list (optional)"},
			"status_filter": {Type: "string", Enum: []interface{}{"all", "pending", "in_progress", "completed", "cancelled"}, Default: "all"},
		},
	}
}

func (t TodoListTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	globalTodoManager.mu.RLock()
	defer globalTodoManager.mu.RUnlock()

	statusFilter := "all"
	if filter, ok := params["status_filter"].(string); ok {
		statusFilter = filter
	}

	var output string

	if planID, ok := params["plan_id"].(string); ok {
		// List specific plan
		plan, exists := globalTodoManager.plans[planID]
		if !exists {
			return "", fmt.Errorf("plan not found: %s", planID)
		}
		output = formatPlan(plan, statusFilter)
	} else {
		// List all plans
		if len(globalTodoManager.plans) == 0 {
			return "No todo plans created yet.", nil
		}

		output = "# Todo Plans\n\n"
		for _, plan := range globalTodoManager.plans {
			output += formatPlan(plan, statusFilter) + "\n---\n\n"
		}
	}

	return output, nil
}

func formatPlan(plan *TodoPlan, statusFilter string) string {
	output := fmt.Sprintf("## %s\n", plan.Name)
	if plan.Description != "" {
		output += fmt.Sprintf("*%s*\n\n", plan.Description)
	}
	output += fmt.Sprintf("ID: %s | Created: %s\n\n", plan.ID, plan.CreatedAt.Format("2006-01-02 15:04"))

	// Group tasks by status
	statusGroups := map[string][]TodoItem{
		"pending":     {},
		"in_progress": {},
		"completed":   {},
		"cancelled":   {},
	}

	for _, item := range plan.Items {
		if statusFilter == "all" || item.Status == statusFilter {
			statusGroups[item.Status] = append(statusGroups[item.Status], item)
		}
	}

	// Display tasks by status
	statusOrder := []string{"in_progress", "pending", "completed", "cancelled"}
	statusIcons := map[string]string{
		"pending":     "⏳",
		"in_progress": "🔄",
		"completed":   "✅",
		"cancelled":   "❌",
	}

	for _, status := range statusOrder {
		items := statusGroups[status]
		if len(items) > 0 {
			output += fmt.Sprintf("### %s %s (%d)\n\n", statusIcons[status], status, len(items))
			for _, item := range items {
				priorityIcon := ""
				switch item.Priority {
				case "high":
					priorityIcon = "🔴"
				case "medium":
					priorityIcon = "🟡"
				case "low":
					priorityIcon = "🟢"
				}

				output += fmt.Sprintf("- [%s] %s %s (ID: %s)\n",
					item.ID, priorityIcon, item.Content, item.ID)

				if len(item.Dependencies) > 0 {
					output += fmt.Sprintf("  Dependencies: %v\n", item.Dependencies)
				}
			}
			output += "\n"
		}
	}

	// Show progress
	total := len(plan.Items)
	completed := len(statusGroups["completed"])
	if total > 0 {
		progress := float64(completed) / float64(total) * 100
		output += fmt.Sprintf("**Progress: %d/%d (%.0f%%)**\n", completed, total, progress)
	}

	return output
}

// TodoAnalyzeTool analyzes the current plan and suggests next actions
type TodoAnalyzeTool struct{}

func (t TodoAnalyzeTool) Name() string {
	return "todo_analyze"
}

func (t TodoAnalyzeTool) Description() string {
	return "Analyze todo plan and suggest next actions based on dependencies and priorities"
}

func (t TodoAnalyzeTool) ParameterSchema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"plan_id": {Type: "string", Description: "Plan ID to analyze (optional, uses current plan)"},
		},
	}
}

func (t TodoAnalyzeTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	globalTodoManager.mu.RLock()
	defer globalTodoManager.mu.RUnlock()

	planID := globalTodoManager.currentPlanID
	if pid, ok := params["plan_id"].(string); ok {
		planID = pid
	}

	plan, exists := globalTodoManager.plans[planID]
	if !exists {
		return "", fmt.Errorf("plan not found: %s", planID)
	}

	// Build dependency map
	depMap := make(map[string][]string)
	taskMap := make(map[string]*TodoItem)
	for i := range plan.Items {
		item := &plan.Items[i]
		taskMap[item.ID] = item
		for _, dep := range item.Dependencies {
			depMap[dep] = append(depMap[dep], item.ID)
		}
	}

	// Find actionable tasks (no incomplete dependencies)
	var actionable []TodoItem
	var blocked []TodoItem
	var inProgress []TodoItem

	for _, item := range plan.Items {
		if item.Status == "completed" || item.Status == "cancelled" {
			continue
		}

		if item.Status == "in_progress" {
			inProgress = append(inProgress, item)
			continue
		}

		// Check if all dependencies are complete
		canStart := true
		for _, depID := range item.Dependencies {
			if dep, exists := taskMap[depID]; exists {
				if dep.Status != "completed" {
					canStart = false
					break
				}
			}
		}

		if canStart {
			actionable = append(actionable, item)
		} else {
			blocked = append(blocked, item)
		}
	}

	// Sort actionable by priority
	priorityOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
	for i := 0; i < len(actionable)-1; i++ {
		for j := i + 1; j < len(actionable); j++ {
			if priorityOrder[actionable[i].Priority] > priorityOrder[actionable[j].Priority] {
				actionable[i], actionable[j] = actionable[j], actionable[i]
			}
		}
	}

	// Generate analysis
	output := fmt.Sprintf("# Todo Plan Analysis: %s\n\n", plan.Name)

	if len(inProgress) > 0 {
		output += "## 🔄 Currently In Progress\n"
		for _, item := range inProgress {
			output += fmt.Sprintf("- %s (ID: %s)\n", item.Content, item.ID)
		}
		output += "\n"
	}

	if len(actionable) > 0 {
		output += "## ✅ Ready to Start\n"
		output += "These tasks have no blocking dependencies:\n\n"
		for _, item := range actionable {
			priorityIcon := map[string]string{"high": "🔴", "medium": "🟡", "low": "🟢"}[item.Priority]
			output += fmt.Sprintf("- %s %s (ID: %s)\n", priorityIcon, item.Content, item.ID)

			// Show what tasks this will unlock
			if deps := depMap[item.ID]; len(deps) > 0 {
				output += "  Completing this will unlock:\n"
				for _, depID := range deps {
					if dep := taskMap[depID]; dep != nil {
						output += fmt.Sprintf("    - %s\n", dep.Content)
					}
				}
			}
		}
		output += "\n"
	}

	if len(blocked) > 0 {
		output += "## 🚫 Blocked Tasks\n"
		for _, item := range blocked {
			output += fmt.Sprintf("- %s (ID: %s)\n", item.Content, item.ID)
			output += "  Waiting for:\n"
			for _, depID := range item.Dependencies {
				if dep := taskMap[depID]; dep != nil && dep.Status != "completed" {
					output += fmt.Sprintf("    - %s (Status: %s)\n", dep.Content, dep.Status)
				}
			}
		}
		output += "\n"
	}

	// Recommendations
	output += "## 📋 Recommendations\n\n"
	if len(inProgress) > 0 {
		output += "1. Focus on completing the in-progress tasks first\n"
	}
	if len(actionable) > 0 {
		if len(actionable) > 0 && actionable[0].Priority == "high" {
			output += fmt.Sprintf("2. Start with high-priority task: %s (ID: %s)\n",
				actionable[0].Content, actionable[0].ID)
		} else {
			output += fmt.Sprintf("2. Next recommended task: %s (ID: %s)\n",
				actionable[0].Content, actionable[0].ID)
		}
	}
	if len(blocked) > 0 {
		output += fmt.Sprintf("3. %d tasks are blocked by dependencies\n", len(blocked))
	}

	return output, nil
}

// GetTodoTools returns all todo management tools
func GetTodoTools() []Tool {
	return []Tool{
		TodoCreateTool{},
		TodoUpdateTool{},
		TodoListTool{},
		TodoAnalyzeTool{},
	}
}
