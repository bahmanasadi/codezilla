# Using Ollama with Codezilla

This guide explains how to use the Ollama integration in Codezilla to power your agents with local LLMs.

## Prerequisites

1. [Ollama](https://ollama.ai/) must be installed and running on your machine
2. You should have at least one model downloaded in Ollama
3. Ollama should be running on the default port (11434)

## Getting Started

To use the Ollama-powered agent, use the `agent:ollama` command followed by an optional model name:

```
> agent:ollama
```

This launches an agent shell using the default "llama3" model, or:

```
> agent:ollama llama3:8b
```

This launches an agent shell with a specified model ("llama3:8b" in this example).

## Available LLM Actions

The Ollama agent adds several LLM-specific actions to the basic agent:

1. **ask** - Ask the LLM a direct question
   ```
   Agent> run I want to ask a question|ask|What is the capital of France?
   ```

2. **think** - Use the LLM to reason step-by-step about a problem
   ```
   Agent> run I need to solve a complex problem|think|How can I architect a microservice system with proper error handling?
   ```

3. **analyze** - Analyze data or text with the LLM
   ```
   Agent> run I want to analyze this data|analyze|[your data or text here]
   ```

4. **summarize** - Summarize text using the LLM
   ```
   Agent> run I need a summary of this text|summarize|[your text here]
   ```

5. **setSystemPrompt** - Change the system prompt for the LLM
   ```
   Agent> run I want to change the LLM behavior|setSystemPrompt|You are an expert software developer with deep knowledge of Go programming.
   ```

6. **setModel** - Switch to a different Ollama model
   ```
   Agent> run I want to use a different model|setModel|codellama:7b-instruct
   ```

7. **getConfig** - Show the current LLM configuration
   ```
   Agent> run I want to see the current configuration|getConfig|
   ```

## Combining LLM with Other Tools

The real power comes when combining LLM capabilities with other tools:

```
# Searching files and analyzing results
Agent> run I want to find and understand Go files|search_files|*.go|false|false
Agent> run I want to analyze the search results|analyze|[paste search results here]

# Reading file content and summarizing it
Agent> run I want to read a file|read_file|/path/to/file.go
Agent> run I want to summarize this code|summarize|[paste file content here]
```

## Example Usage: Building a Simple Workflow

Here's an example of using an Ollama-powered agent for a development workflow:

```
# Start the Ollama agent
> agent:ollama codellama:13b-instruct

# First, get config to check settings
Agent> run I want to see the configuration|getConfig|

# Set a better system prompt for code-related tasks
Agent> run I need a coding-focused system prompt|setSystemPrompt|You are an expert Go developer helping analyze and generate code.

# Search for main Go files
Agent> run I want to find main files|search_files|**/main.go|false|false

# Analyze a specific file
Agent> run I need to read this file|read_file|/Users/username/project/cmd/codezilla/main.go

# Ask for an analysis of the code architecture
Agent> run I want to understand the architecture|analyze|[paste file content]

# Get suggestions for improving the code
Agent> run Help me improve this code|think|How could I refactor the main.go file to make it more maintainable?
```

## Adjusting the Ollama Model Parameters

You can adjust the system prompt directly from the agent shell. Currently, other parameters like temperature are fixed in the code, but future versions will provide more control over the model's behavior.

If you want to reset all memory, simply use the `reset` command to clear the agent's history.

## Troubleshooting

If you encounter issues with the Ollama integration:

1. Ensure Ollama is running (`ollama serve` command)
2. Verify you have the model downloaded (`ollama list` to check)
3. Check your model name is correct (some models use colon syntax: `model:variant`)
4. Ensure port 11434 is not blocked by a firewall
5. For high-quality responses, try larger models (e.g., 13B or 34B parameter models)