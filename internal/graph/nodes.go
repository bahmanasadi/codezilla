package graph

import (
	"bufio"
	"codezilla/internal/ollama"
	"codezilla/internal/tools"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func New() *Graph {
	return &Graph{
		Start: "get_user_input",
		Nodes: map[string]*Node{
			"get_user_input": {
				Function: func(state map[string]interface{}) (string, error) {
					fmt.Print("Enter your prompt: ")
					reader := bufio.NewReader(os.Stdin)
					input, err := reader.ReadString('\n')
					if err != nil {
						return "", err
					}
					state["user"] = strings.TrimSpace(input)
					return "route_tool", nil
				},
			},
			"route_tool": {
				Function: func(state map[string]interface{}) (string, error) {
					input := state["user"].(string)

					switch {
					case strings.HasPrefix(input, "calculate"):
						state["tool"] = "calculator"
						state["query"] = strings.TrimPrefix(input, "calculate ")
						return "use_tool", nil
					case strings.HasPrefix(input, "search"):
						state["tool"] = "websearch"
						state["query"] = strings.TrimPrefix(input, "search ")
						return "use_tool", nil
					case strings.HasPrefix(input, "time") || strings.Contains(input, "date"):
						state["tool"] = "datetime"
						return "use_tool", nil
					case strings.HasPrefix(input, "read "):
						state["tool"] = "fileread"
						// Format expected: read filename [query] [context_lines]
						args := strings.SplitN(strings.TrimPrefix(input, "read "), " ", 3)
						state["path"] = args[0]

						if len(args) > 1 {
							state["query"] = args[1]
						}

						if len(args) > 2 {
							state["context_lines"] = args[2]
						}

						return "use_tool", nil
					default:
						return "ask_ollama", nil
					}
				},
			},
			"use_tool": {
				Function: func(state map[string]interface{}) (string, error) {
					tool := state["tool"].(string)
					var output string
					var err error

					switch tool {
					case "calculator":
						output, err = tools.Calculate(state["query"].(string))
					case "websearch":
						output = tools.WebSearch(state["query"].(string))
					case "datetime":
						if query, ok := state["query"].(string); ok && query != "" {
							output = tools.GetDateTime(query)
						} else {
							output = tools.GetDateTime()
						}
					case "fileread":
						path := state["path"].(string)
						query := ""
						contextLines := 2 // Default context lines

						if q, ok := state["query"].(string); ok {
							query = q
						}

						if c, ok := state["context_lines"].(string); ok {
							if cl, err := strconv.Atoi(c); err == nil {
								contextLines = cl
							}
						}

						output, err = tools.ReadFile(path, query, contextLines)
					}

					if err != nil {
						state["response"] = "Tool error: " + err.Error()
					} else {
						state["response"] = output
					}
					return "print_response", nil
				},
			},
			"ask_ollama": {
				Function: func(state map[string]interface{}) (string, error) {
					system := `You are a helpful assistant with access to the following tools:

TOOLS = [
  {
    "name": "calculator",
    "description": "Perform mathematical calculations and evaluate expressions",
    "parameters": {
      "type": "object",
      "properties": {
        "expression": {
          "type": "string",
          "description": "The mathematical expression to evaluate (e.g. '2+2', '(5*10)/2', 'sqrt(16)')"
        }
      },
      "required": ["expression"]
    }
  },
  {
    "name": "websearch",
    "description": "Search the web for up-to-date information on any topic",
    "parameters": {
      "type": "object",
      "properties": {
        "query": {
          "type": "string",
          "description": "The search query to find information about"
        }
      },
      "required": ["query"]
    }
  },
  {
    "name": "datetime",
    "description": "Get the current date and time in a formatted string",
    "parameters": {
      "type": "object",
      "properties": {
        "format": {
          "type": "string",
          "description": "Optional format specification (defaults to standard format)",
          "enum": ["standard", "iso", "unix"]
        }
      }
    }
  },
  {
    "name": "fileread",
    "description": "Read a file to provide its content as context",
    "parameters": {
      "type": "object",
      "properties": {
        "path": {
          "type": "string",
          "description": "The path to the file to read"
        },
        "query": {
          "type": "string",
          "description": "Optional search query to filter content (if empty, returns whole file)"
        },
        "context_lines": {
          "type": "integer",
          "description": "Number of context lines to include before and after matches (default: 2)"
        }
      },
      "required": ["path"]
    }
  }
]

When you need to use a tool, respond with a JSON object in the following format:
{
  "tool": "toolName",
  "query": "yourQuery"
}

Examples:
1. For calculator: {"tool": "calculator", "query": "2+2*5"}
2. For web search: {"tool": "websearch", "query": "weather in London"}
3. For date/time: {"tool": "datetime"} or {"tool": "datetime", "query": "iso"}
4. For file reading: {"tool": "fileread", "path": "/path/to/file", "query": "optional search term", "context_lines": 3}

PRIORITISE tool use to give an answer. For direct responses without using tools, simply answer normally without JSON formatting.`
					prompt := fmt.Sprintf("%s\n\nUser: %s", system, state["user"])
					response, err := ollama.AskOllama(prompt, true) // true enables tools
					if err != nil {
						return "", err
					}

					// Try to parse the response as JSON to see if a tool is being requested
					var toolRequest struct {
						Tool         string `json:"tool"`
						Query        string `json:"query"`
						Path         string `json:"path"`
						ContextLines int    `json:"context_lines"`
					}

					// Check if the response is in JSON format
					if strings.HasPrefix(strings.TrimSpace(response), "{") && strings.HasSuffix(strings.TrimSpace(response), "}") {
						err := json.Unmarshal([]byte(response), &toolRequest)
						if err == nil && toolRequest.Tool != "" {
							// Valid tool request found, set up for tool use
							switch toolRequest.Tool {
							case "calculator":
								state["tool"] = "calculator"
								state["query"] = toolRequest.Query
								return "use_tool", nil
							case "websearch":
								state["tool"] = "websearch"
								state["query"] = toolRequest.Query
								return "use_tool", nil
							case "datetime":
								state["tool"] = "datetime"
								state["query"] = toolRequest.Query // This could be empty or contain the format
								return "use_tool", nil
							case "fileread":
								state["tool"] = "fileread"
								state["path"] = toolRequest.Path
								state["query"] = toolRequest.Query
								if toolRequest.ContextLines > 0 {
									state["context_lines"] = strconv.Itoa(toolRequest.ContextLines)
								}
								return "use_tool", nil
							}
						}
					}

					// If we get here, either it wasn't JSON or wasn't a recognized tool
					state["response"] = response
					return "print_response", nil
				},
			},
			"print_response": {
				Function: func(state map[string]interface{}) (string, error) {
					fmt.Println("\nAssistant:", state["response"])
					return "", nil
				},
			},
		},
	}
}
