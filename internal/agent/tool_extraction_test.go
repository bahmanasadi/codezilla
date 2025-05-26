package agent

import (
	"codezilla/pkg/logger"
	"testing"
)

func TestExtractToolCallFormats(t *testing.T) {
	// Create a test logger
	log, _ := logger.New(logger.Config{Silent: true})

	// Create a minimal agent for testing
	a := &agent{
		logger: log,
	}

	tests := []struct {
		name       string
		response   string
		expectTool bool
		toolName   string
		paramName  string
		paramValue interface{}
	}{
		{
			name: "XML format",
			response: `Here's the file content:
<tool>
  <name>fileRead</name>
  <params>
    <file_path>/etc/hosts</file_path>
  </params>
</tool>`,
			expectTool: true,
			toolName:   "fileRead",
			paramName:  "file_path",
			paramValue: "/etc/hosts",
		},
		{
			name: "JSON format",
			response: `Let me read that file:
` + "```json\n{\n  \"tool\": \"fileRead\",\n  \"params\": {\n    \"file_path\": \"/etc/hosts\"\n  }\n}\n```",
			expectTool: true,
			toolName:   "fileRead",
			paramName:  "file_path",
			paramValue: "/etc/hosts",
		},
		{
			name: "JSON with 'name' field",
			response: `Reading the file:
` + "```json\n{\n  \"name\": \"fileWrite\",\n  \"params\": {\n    \"file_path\": \"/tmp/test.txt\",\n    \"content\": \"Hello World\"\n  }\n}\n```",
			expectTool: true,
			toolName:   "fileWrite",
			paramName:  "file_path",
			paramValue: "/tmp/test.txt",
		},
		{
			name: "Bash code block",
			response: `Let me list the files:
` + "```bash\nls -la /tmp\n```",
			expectTool: true,
			toolName:   "execute",
			paramName:  "command",
			paramValue: "ls -la /tmp",
		},
		{
			name: "Shell code block",
			response: `Checking disk usage:
` + "```shell\ndf -h\n```",
			expectTool: true,
			toolName:   "execute",
			paramName:  "command",
			paramValue: "df -h",
		},
		{
			name: "Sh code block",
			response: `Running the script:
` + "```sh\necho \"Hello from shell\"\n```",
			expectTool: true,
			toolName:   "execute",
			paramName:  "command",
			paramValue: "echo \"Hello from shell\"",
		},
		{
			name:       "No tool call",
			response:   `This is just a regular response with no tool calls.`,
			expectTool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCall, remaining, hasTool := a.extractToolCall(tt.response)

			if hasTool != tt.expectTool {
				t.Errorf("hasTool = %v, want %v", hasTool, tt.expectTool)
			}

			if tt.expectTool {
				if toolCall == nil {
					t.Fatal("Expected tool call but got nil")
				}

				if toolCall.ToolName != tt.toolName {
					t.Errorf("ToolName = %q, want %q", toolCall.ToolName, tt.toolName)
				}

				if tt.paramName != "" {
					val, ok := toolCall.Params[tt.paramName]
					if !ok {
						t.Errorf("Parameter %q not found, available params: %v", tt.paramName, toolCall.Params)
					} else if val != tt.paramValue {
						t.Errorf("Parameter %q = %v, want %v", tt.paramName, val, tt.paramValue)
					}
				}

				// Check that tool call was removed from remaining text
				if tt.toolName == "execute" && remaining == tt.response {
					t.Error("Tool call was not removed from response")
				}
			}
		})
	}
}

func TestExtractToolCallPriority(t *testing.T) {
	// Create a test logger
	log, _ := logger.New(logger.Config{Silent: true})

	// Create a minimal agent for testing
	a := &agent{
		logger: log,
	}

	// Test that JSON takes priority over XML when both are present
	response := `Here's both formats:
` + "```json\n{\n  \"tool\": \"fileRead\",\n  \"params\": {\n    \"path\": \"/from/json\"\n  }\n}\n```" + `
<tool>
  <name>fileWrite</name>
  <params>
    <path>/from/xml</path>
  </params>
</tool>`

	toolCall, _, hasTool := a.extractToolCall(response)

	if !hasTool {
		t.Error("Expected to find tool call")
	}

	if toolCall == nil {
		t.Fatal("Expected tool call but got nil")
	}

	// JSON should be extracted first
	if toolCall.ToolName != "fileRead" {
		t.Errorf("Expected fileRead (JSON) to be extracted first, got %s", toolCall.ToolName)
	}

	if path, ok := toolCall.Params["path"].(string); !ok || path != "/from/json" {
		t.Errorf("Expected path from JSON (/from/json), got %v", toolCall.Params["path"])
	}
}

func TestExtractMultipleToolCalls(t *testing.T) {
	// Create a test logger
	log, _ := logger.New(logger.Config{Silent: true})

	// Create a minimal agent for testing
	a := &agent{
		logger: log,
	}

	tests := []struct {
		name          string
		response      string
		expectedTools []string
	}{
		{
			name: "Multiple JSON tool calls",
			response: `Let me read both files:
` + "```json\n{\n  \"tool\": \"fileRead\",\n  \"params\": {\n    \"file_path\": \"/file1.txt\"\n  }\n}\n```" + `
And then:
` + "```json\n{\n  \"tool\": \"fileRead\",\n  \"params\": {\n    \"file_path\": \"/file2.txt\"\n  }\n}\n```",
			expectedTools: []string{"fileRead", "fileRead"},
		},
		{
			name: "Mixed JSON and bash",
			response: `First, let me check the directory:
` + "```bash\nls -la\n```" + `
Then read the config:
` + "```json\n{\n  \"tool\": \"fileRead\",\n  \"params\": {\n    \"file_path\": \"config.json\"\n  }\n}\n```",
			expectedTools: []string{"execute", "fileRead"},
		},
		{
			name: "Multiple XML tool calls",
			response: `Here are the operations:
<tool>
  <name>fileRead</name>
  <params>
    <file_path>/etc/hosts</file_path>
  </params>
</tool>
And also:
<tool>
  <name>fileWrite</name>
  <params>
    <file_path>/tmp/output.txt</file_path>
    <content>Hello World</content>
  </params>
</tool>`,
			expectedTools: []string{"fileRead", "fileWrite"},
		},
		{
			name: "Three bash commands",
			response: `Running multiple commands:
` + "```bash\npwd\n```" + `
` + "```sh\nls -la\n```" + `
` + "```shell\necho \"Done\"\n```",
			expectedTools: []string{"execute", "execute", "execute"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCalls := a.extractAllToolCalls(tt.response)
			hasTools := len(toolCalls) > 0
			var remaining string
			if hasTools {
				remaining = toolCalls[len(toolCalls)-1].remainingText
			}

			if !hasTools {
				t.Error("Expected to find tool calls")
			}

			if len(toolCalls) != len(tt.expectedTools) {
				t.Errorf("Expected %d tool calls, got %d", len(tt.expectedTools), len(toolCalls))
				if len(toolCalls) > 0 {
					t.Logf("Last remaining text: %q", toolCalls[len(toolCalls)-1].remainingText)
				}
			}

			for i, expectedTool := range tt.expectedTools {
				if i >= len(toolCalls) {
					break
				}
				if toolCalls[i].toolCall.ToolName != expectedTool {
					t.Errorf("Tool %d: expected %q, got %q", i, expectedTool, toolCalls[i].toolCall.ToolName)
				}
			}

			// Verify tool calls were removed from remaining text
			if len(remaining) >= len(tt.response) {
				t.Error("Tool calls were not properly removed from response")
			}
		})
	}
}
