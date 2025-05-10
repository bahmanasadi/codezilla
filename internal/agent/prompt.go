package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"codezilla/internal/tools"
)

// PromptTemplate contains templates for different prompt components
type PromptTemplate struct {
	SystemTemplate    string
	UserTemplate      string
	AssistantTemplate string
	ToolTemplate      string
}

// DefaultPromptTemplate returns the default prompt template
func DefaultPromptTemplate() *PromptTemplate {
	return &PromptTemplate{
		SystemTemplate: `You are a helpful AI assistant with access to a set of tools. When you need to use a tool, format your response like this:
<tool>
{
  "name": "toolName",
  "params": {
    "param1": "value1",
    "param2": "value2"
  }
}
</tool>

Wait for the tool response before continuing the conversation. The available tools are:

{{tools}}

Remember:
1. Think through problems step by step
2. Use tools when needed to gather information or perform actions
3. Don't make up information - use tools to get accurate data
4. Always reply in markdown format
5. Be concise and helpful`,

		UserTemplate: `{{content}}`,

		AssistantTemplate: `{{content}}`,

		ToolTemplate: `Tool result: {{result}}`,
	}
}

// FormatSystemPrompt formats the system prompt with tool specifications
func FormatSystemPrompt(template string, toolSpecs []tools.ToolSpec) string {
	// Convert tool specs to a readable format
	toolsDescription := formatToolSpecsForPrompt(toolSpecs)

	// Replace the {{tools}} placeholder with the tool descriptions
	return strings.Replace(template, "{{tools}}", toolsDescription, 1)
}

// formatToolSpecsForPrompt formats tool specifications in a readable way for the prompt
func formatToolSpecsForPrompt(specs []tools.ToolSpec) string {
	var builder strings.Builder

	for _, spec := range specs {
		builder.WriteString(fmt.Sprintf("## %s\n", spec.Name))
		builder.WriteString(fmt.Sprintf("Description: %s\n", spec.Description))

		// Format parameters
		builder.WriteString("Parameters:\n")

		if spec.ParameterSchema.Properties != nil {
			for paramName, paramSchema := range spec.ParameterSchema.Properties {
				required := ""
				if contains(spec.ParameterSchema.Required, paramName) {
					required = " (required)"
				}

				builder.WriteString(fmt.Sprintf("- %s: %s%s", paramName, paramSchema.Description, required))

				// Add type information
				builder.WriteString(fmt.Sprintf(" [%s]", paramSchema.Type))

				// Add default value if present
				if paramSchema.Default != nil {
					builder.WriteString(fmt.Sprintf(" (default: %v)", paramSchema.Default))
				}

				builder.WriteString("\n")
			}
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

// FormatToolCallPrompt formats a tool call message for the LLM
func FormatToolCallPrompt(toolCall *ToolCall) string {
	// Format the tool call as JSON
	paramsJSON, err := json.MarshalIndent(toolCall.Params, "  ", "  ")
	if err != nil {
		return fmt.Sprintf("<tool>\n{\n  \"name\": %q,\n  \"params\": {}\n}\n</tool>", toolCall.ToolName)
	}

	return fmt.Sprintf("<tool>\n{\n  \"name\": %q,\n  \"params\": %s\n}\n</tool>",
		toolCall.ToolName, string(paramsJSON))
}

// FormatToolResultPrompt formats a tool result message for the LLM
func FormatToolResultPrompt(result interface{}, err error) string {
	if err != nil {
		return fmt.Sprintf("<tool-result>\nError: %s\n</tool-result>", err.Error())
	}

	// Format the result based on its type
	var resultStr string
	switch v := result.(type) {
	case string:
		resultStr = v
	case []byte:
		resultStr = string(v)
	default:
		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			resultStr = fmt.Sprintf("%v", result)
		} else {
			resultStr = string(resultJSON)
		}
	}

	return fmt.Sprintf("<tool-result>\n%s\n</tool-result>", resultStr)
}

// Helper function to check if a string slice contains a value
func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}
