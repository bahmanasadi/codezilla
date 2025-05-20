package agent

import (
	"fmt"
	"sort"
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
		SystemTemplate: `You are a helpful AI assistant with access to a set of tools. When you need to use a tool, you MUST format your response using XML format like this:
<tool>
  <name>toolName</name>
  <params>
    <param1>value1</param1>
    <param2>value2</param2>
  </params>
</tool>

IMPORTANT: Always use the XML format shown above, NEVER use JSON format inside the tool tags. XML is required for proper tool execution.

Wait for the tool response before continuing the conversation. The available tools are:

{{tools}}

Remember:
1. Think through problems step by step
2. Use tools when needed to gather information or perform actions
3. Don't make up information - use tools to get accurate data
4. Always reply in markdown format
5. Be concise and helpful
6. ALWAYS use XML format for tool calls, not JSON`,

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
	// Format the tool call as XML
	var builder strings.Builder
	builder.WriteString("<tool>\n")
	builder.WriteString(fmt.Sprintf("  <name>%s</name>\n", escapeXML(toolCall.ToolName)))
	builder.WriteString("  <params>\n")

	// Add parameters
	for paramName, paramValue := range toolCall.Params {
		// Convert parameter value to string
		var valueStr string
		switch v := paramValue.(type) {
		case string:
			valueStr = escapeXML(v)
		case []byte:
			valueStr = escapeXML(string(v))
		default:
			valueStr = fmt.Sprintf("%v", paramValue)
		}

		builder.WriteString(fmt.Sprintf("    <%s>%s</%s>\n",
			paramName, valueStr, paramName))
	}

	builder.WriteString("  </params>\n")
	builder.WriteString("</tool>")

	return builder.String()
}

// FormatToolResultPrompt formats a tool result message for the LLM
func FormatToolResultPrompt(result interface{}, err error) string {
	var builder strings.Builder
	builder.WriteString("<tool-result>\n")

	if err != nil {
		builder.WriteString(fmt.Sprintf("  <error>%s</error>\n", escapeXML(err.Error())))
	} else {
		// Format the result based on its type
		switch v := result.(type) {
		case string:
			builder.WriteString(fmt.Sprintf("  <content>%s</content>\n", escapeXML(v)))
		case []byte:
			builder.WriteString(fmt.Sprintf("  <content>%s</content>\n", escapeXML(string(v))))
		case map[string]interface{}:
			// Sort keys for consistent output
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			// Add each field as an XML element
			for _, k := range keys {
				valueStr := fmt.Sprintf("%v", v[k])
				builder.WriteString(fmt.Sprintf("  <%s>%s</%s>\n",
					k, escapeXML(valueStr), k))
			}
		default:
			builder.WriteString(fmt.Sprintf("  <value>%v</value>\n", v))
		}
	}

	builder.WriteString("</tool-result>")
	return builder.String()
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

// uses the escapeXML function from context.go
