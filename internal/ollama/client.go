package ollama

import (
	"bytes"
	"codezilla/internal/tools"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type ollamaRequest struct {
	Model  string                   `json:"model"`
	Prompt string                   `json:"prompt"`
	Stream bool                     `json:"stream"`
	Tools  []map[string]interface{} `json:"tools,omitempty"`
	Tool   map[string]interface{}   `json:"tool,omitempty"`
}

type ollamaResponse struct {
	Response string                 `json:"response"`
	Done     bool                   `json:"done"`
	Tool     map[string]interface{} `json:"tool,omitempty"`
}

// AskOllama sends a prompt to the Ollama API and returns the response
// If withTools is true, it will include tool definitions that the model can use
func AskOllama(prompt string, withTools bool) (string, error) {
	payload := ollamaRequest{
		Model:  "dolphin3:8b", // Make sure the model is running in Ollama
		Prompt: prompt,
		Stream: true,
	}

	if withTools {
		payload.Tools = defineTools()
	}

	data, _ := json.Marshal(payload)
	resp, err := http.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var reply string
	var toolCall map[string]interface{}

	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var chunk ollamaResponse
		if err := decoder.Decode(&chunk); err != nil {
			break
		}

		// Check if this chunk contains a tool call
		if len(chunk.Tool) > 0 {
			toolCall = chunk.Tool
		}

		reply += chunk.Response
		if chunk.Done {
			break
		}
	}

	// If a tool was called, execute it and append the result
	if toolCall != nil {
		toolResult, err := executeToolCall(toolCall)
		if err != nil {
			return reply, err
		}
		reply += "\n\nTool result: " + toolResult
	}

	return reply, nil
}

// defineTools creates tool definitions for the Ollama API
func defineTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "calculate",
				"description": "Perform mathematical calculations and evaluate expressions",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"expression": map[string]interface{}{
							"type":        "string",
							"description": "The mathematical expression to evaluate (e.g. '2+2', '(5*10)/2', 'sqrt(16)')",
						},
					},
					"required": []string{"expression"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "web_search",
				"description": "Search the web for up-to-date information on any topic",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "The search query to find information about",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_date_time",
				"description": "Get the current date and time in a formatted string",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"format": map[string]interface{}{
							"type":        "string",
							"description": "Optional format specification (defaults to standard format)",
							"enum":        []string{"standard", "iso", "unix"},
						},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "read_file",
				"description": "Read a file to provide its content as context",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "The path to the file to read",
						},
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Optional search query to filter content (if empty, returns whole file)",
						},
						"context_lines": map[string]interface{}{
							"type":        "integer",
							"description": "Number of context lines to include before and after matches (default: 2)",
						},
					},
					"required": []string{"path"},
				},
			},
		},
	}
}

// executeToolCall executes a tool call and returns the result
func executeToolCall(toolCall map[string]interface{}) (string, error) {
	// Extract tool name and parameters
	if toolCall["type"] != "function" {
		return "", errors.New("unsupported tool type")
	}

	function, ok := toolCall["function"].(map[string]interface{})
	if !ok {
		return "", errors.New("invalid function format")
	}

	name, ok := function["name"].(string)
	if !ok {
		return "", errors.New("missing function name")
	}

	arguments := "{}"
	if args, ok := function["arguments"].(string); ok {
		arguments = args
	}

	// Execute the appropriate tool based on the name
	switch name {
	case "calculate":
		var params struct {
			Expression string `json:"expression"`
		}
		if err := json.Unmarshal([]byte(arguments), &params); err != nil {
			return "", fmt.Errorf("invalid calculator parameters: %v", err)
		}
		return tools.Calculate(params.Expression)

	case "web_search":
		var params struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(arguments), &params); err != nil {
			return "", fmt.Errorf("invalid web search parameters: %v", err)
		}
		return tools.WebSearch(params.Query), nil

	case "get_date_time":
		var params struct {
			Format string `json:"format"`
		}
		if err := json.Unmarshal([]byte(arguments), &params); err != nil {
			// If there's an error parsing, just use the default format
			return tools.GetDateTime(), nil
		}
		return tools.GetDateTime(params.Format), nil

	case "read_file":
		var params struct {
			Path         string `json:"path"`
			Query        string `json:"query"`
			ContextLines int    `json:"context_lines"`
		}
		if err := json.Unmarshal([]byte(arguments), &params); err != nil {
			return "", fmt.Errorf("invalid file read parameters: %v", err)
		}
		return tools.ReadFile(params.Path, params.Query, params.ContextLines)

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}
