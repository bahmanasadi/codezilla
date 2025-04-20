# Agent Examples

This document provides examples of using the agent pattern with the Codezilla CLI.

Codezilla provides three types of agents:
1. Basic Agent - Simple tools like calculation, file operations, etc.
2. Advanced Agent - Adds memory and chaining capabilities
3. Ollama Agent - Connects to local LLMs via Ollama (see [Ollama Guide](./ollama_guide.md) for details)

## Basic Agent Example

Here's a walkthrough of using the agent framework for a simple task:

```
# Start the Agent shell
> agent

# List available actions
Agent> list
Available Agent Actions:
1. calculate
2. datetime
3. read_file
4. search_files
5. search_web
6. wait

# Execute a simple calculation
Agent> run I need to calculate the result of 2 + 2|calculate|2 + 2
Agent Execution Result:
Result: 4

# Check execution history
Agent> history
Agent Execution History:
Step 1:
  Thought: I need to calculate the result of 2 + 2
  Action: calculate(2 + 2)
  Result: Result: 4

# Get the current date and time
Agent> run I need to know the current date and time|datetime|
Agent Execution Result:
2023-04-15 10:30:45

# See updated history
Agent> history
Agent Execution History:
Step 1:
  Thought: I need to calculate the result of 2 + 2
  Action: calculate(2 + 2)
  Result: Result: 4

Step 2:
  Thought: I need to know the current date and time
  Action: datetime()
  Result: 2023-04-15 10:30:45

# Reset history and start fresh
Agent> reset
Agent history has been reset.

# Verify history is cleared
Agent> history
No agent execution history available.

# Exit Agent shell
Agent> exit
Exiting agent shell
```

## Advanced Agent Example with Advanced Executor

The advanced agent executor adds memory capabilities and action chaining:

```
# Start the Advanced Agent shell
> agent:advanced

# List available actions
Agent> list
Available Agent Actions:
1. calculate
2. chain
3. datetime
4. forget
5. list_memories
6. read_file
7. recall
8. reflect
9. remember
10. search_files
11. search_web
12. wait

# Store information in memory
Agent> run I want to remember the project name|remember|project_name|Codezilla
Agent Execution Result:
Memory stored with key 'project_name'

# Recall stored information
Agent> run I need to retrieve the project name|recall|project_name
Agent Execution Result:
Codezilla

# List all stored memories
Agent> run I want to see all my stored memories|list_memories|
Agent Execution Result:
Found 1 memories:
1. project_name: Codezilla

# Execute a chain of actions
Agent> run I want to execute multiple steps together|chain|calculate: 5 * 10
datetime: 
remember: result|The calculation result is 50 and the current time

Agent Execution Result:
Step 1 (calculate): Result: 50

Step 2 (datetime): 2023-04-15 10:35:22

Step 3 (remember): Memory stored with key 'result'

# Self-reflection
Agent> run I want the agent to reflect on its current state|reflect|What am I capable of?
Agent Execution Result:
Agent Reflection:
- Timestamp: 2023-04-15T10:35:30Z
- Memory Size: 2 items
- Available Actions: 12

Reflection Prompt: What am I capable of?

Agent Self-Assessment:
- The agent is capable of executing a variety of tools
- The agent has memory capabilities to store and retrieve information
- The agent can chain multiple actions together
- Current focus: What am I capable of?

# Exit advanced Agent shell
Agent> exit
Exiting agent shell
```

## Ollama LLM-Powered Agent Example

The Ollama agent integrates with local LLM models via Ollama:

```
# Start the Ollama-powered Agent shell with a specific model
> agent:ollama llama3

# List available actions, which now include LLM-specific operations
Agent> list
Available Agent Actions:
1. analyze
2. ask
3. calculate
4. datetime
5. getConfig
6. read_file
7. search_files
8. search_web
9. setModel
10. setSystemPrompt
11. summarize
12. think
13. wait

# Ask the LLM a question
Agent> run I want to ask the LLM|ask|What are the key principles of software architecture?
Agent Execution Result:
The key principles of software architecture include:

1. **Modularity**: Breaking down systems into well-defined, independent components.

2. **Abstraction**: Hiding complexity and exposing only necessary details.

3. **Encapsulation**: Bundling data and methods that work on that data together.

4. **Separation of Concerns**: Dividing a system into distinct sections that address separate concerns.

5. **Single Responsibility**: Each component should have one responsibility or reason to change.

6. **Open/Closed Principle**: Components should be open for extension but closed for modification.

7. **Interface Segregation**: Clients should not depend on interfaces they don't use.

8. **Dependency Inversion**: High-level modules should not depend on low-level modules.

9. **Scalability**: Ability to handle growing amounts of work.

10. **Maintainability**: Ease of making changes and fixes.

11. **Performance Efficiency**: Optimal use of resources.

12. **Security**: Protection against vulnerabilities and threats.

13. **Testability**: Ease of testing components in isolation.

14. **Reusability**: Ability to use components in different contexts.

15. **Interoperability**: Ability to exchange and use information between systems.

These principles guide architects in creating robust, flexible, and sustainable software systems.

# Get model configuration
Agent> run I want to check the model settings|getConfig|
Agent Execution Result:
Current Configuration:
Model: llama3
Base URL: http://localhost:11434
System Prompt: You are a helpful AI assistant with access to various tools. Always follow instructions exactly.
Temperature: 0.7
Max Tokens: 4096
Response Format: text

# Use LLM to analyze code
Agent> run I want the LLM to analyze file content|analyze|
func main() {
    if len(os.Args) < 2 {
        fmt.Println("Missing required argument")
        return
    }
    processFile(os.Args[1])
}
Agent Execution Result:
This code is a simple Go program that analyzes command-line arguments. Here's a detailed analysis:

1. **Function Purpose**: 
   - This is a `main()` function, which serves as the entry point for a Go application.

2. **Command-Line Argument Handling**:
   - The code checks if there are enough command-line arguments using `len(os.Args) < 2`.
   - `os.Args` is a slice where:
     - `os.Args[0]` is the program name itself
     - `os.Args[1]` is the first actual argument provided by the user

3. **Error Handling**:
   - If no arguments are provided (beyond the program name), it prints "Missing required argument".
   - The program then terminates with `return` (exits the main function).

4. **Function Call**:
   - If at least one argument is provided, it calls `processFile()` with `os.Args[1]` as the parameter.
   - This suggests the program expects a file path as its first argument.

5. **Potential Issues**:
   - No error code is returned when exiting due to missing arguments. Consider using `os.Exit(1)` for proper error signaling.
   - There's no validation on the argument beyond its existence.
   - The error message doesn't explain what kind of argument is required.

6. **Dependencies**:
   - The code imports `fmt` for printing and `os` for accessing command-line arguments.
   - A `processFile` function must be defined elsewhere in the codebase.

7. **Suggested Improvements**:
   - Add a more descriptive error message (e.g., "Missing filename argument")
   - Include usage instructions when argument validation fails
   - Implement proper error code return with `os.Exit(1)` for error conditions
   - Consider validating that the file exists before passing to `processFile()`

# Exit the Ollama agent shell
Agent> exit
Exiting agent shell
```

## Integration with External Tools

Agents can be used to integrate with external tools and APIs:

```
# Example of integrating with file system
Agent> run I want to find all Go files in the project|search_files|*.go|true|false
Agent Execution Result:
Found 15 files matching pattern '*.go':
[1] /Users/username/project/cmd/codezilla/main.go
[2] /Users/username/project/internal/cli/cli.go
...

# Example of reading file content
Agent> run I want to read the content of a specific file|read_file|/Users/username/project/go.mod
Agent Execution Result:
module codezilla

go 1.23.0

toolchain go1.23.8

require (
    github.com/Knetic/govaluate v3.0.0+incompatible
    github.com/sergi/go-diff v1.3.1
    github.com/tmc/langchaingo v0.1.13
    golang.org/x/term v0.31.0
)
...
```