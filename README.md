# Codezilla

A CLI agent tool powered by Ollama, inspired by Claude Code. Codezilla provides a command-line interface to interact with local LLMs and execute local tools.

## Features

- Interactive CLI interface for chatting with the LLM
- Integration with Ollama for local LLM inference
- Tool execution system for file operations and shell commands
- Context management to maintain conversation history
- Configurable settings for model, logging, and behavior

## Prerequisites

- Go 1.23 or higher
- Ollama installed and running locally
- A compatible Ollama model (default: qwen2.5-coder:3b)

## Installation

1. Make sure you have Go installed
2. Clone the repository
3. Build the application:

```bash
make build
```

4. Run the application:

```bash
make run
```

## Usage

Once running, Codezilla provides an interactive CLI where you can chat with the LLM assistant. The assistant has access to various tools to help with tasks:

- Reading and writing files
- Executing shell commands
- And more that can be added via the tool system

### Commands

- `/help` - Show help message
- `/models` - List available models
- `/model <name>` - Switch to a different model
- `/tools` - List available tools
- `/clear` - Clear the screen
- `/version` - Show version information
- `/config` - Show current configuration
- `/config <key> <value>` - Change configuration
- `exit` or `quit` - Exit the application

## Configuration

Configuration can be provided through a config file or command-line flags:

```bash
./build/codezilla -config path/to/config.json -model model-name -log path/to/log.log
```

### Available flags:

- `-config` - Path to config file
- `-log` - Path to log file
- `-log-level` - Log level (debug, info, warn, error)
- `-log-silent` - Disable console logging
- `-model` - Model name to use
- `-ollama-url` - Ollama API URL

## Development

### Project Structure

- `cmd/codezilla/` - Main application executable
- `internal/agent/` - Agent implementation and context management
- `internal/cli/` - Command-line interface code
- `internal/tools/` - Tool implementations
- `llm/ollama/` - Ollama API client
- `pkg/logger/` - Logging utilities
- `pkg/style/` - Terminal styling utilities

### Make Commands

- `make build` - Build the application
- `make run` - Run the application
- `make run-debug` - Run with debug logging
- `make test` - Run tests
- `make lint` - Run linter
- `make fmt` - Format code
- `make check` - Run code quality checks
- `make clean` - Clean build artifacts

## Example

```
$ make run

=====================
      Codezilla      
=====================
A CLI agent powered by Ollama
Type '/help' to see available commands
Current model: qwen2.5-coder:3b

user> What files are in the current directory?

assistant> I'll help you list the files in the current directory. Let me use a tool for that.

<tool>
{
  "name": "execute",
  "params": {
    "command": "ls -la"
  }
}
</tool>

Here are the files in the current directory:

total 24
drwxr-xr-x  7 user user 4096 Sep 5 12:34 .
drwxr-xr-x 32 user user 4096 Sep 5 12:30 ..
drwxr-xr-x  2 user user 4096 Sep 5 12:34 cmd
-rw-r--r--  1 user user  268 Sep 5 12:31 go.mod
-rw-r--r--  1 user user 1825 Sep 5 12:31 go.sum
drwxr-xr-x  3 user user 4096 Sep 5 12:32 internal
drwxr-xr-x  3 user user 4096 Sep 5 12:32 llm
drwxr-xr-x  2 user user 4096 Sep 5 12:33 logs
drwxr-xr-x  3 user user 4096 Sep 5 12:33 pkg
-rw-r--r--  1 user user 2835 Sep 5 12:34 README.md

user> exit

Thank you for using Codezilla! Goodbye.
```

## License

MIT License