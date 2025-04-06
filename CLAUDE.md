# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build/Lint/Test Commands

- Build: `make build` or `go build ./cmd/codezilla`
- Run: `make run` or `./build/codezilla`
- Run with logging: `./build/codezilla -log ./logs/app.log -log-level debug -log-silent`
- Format code: `make fmt` or `go fmt ./...`
- Run tests: `make test` or `go test ./...`
- Run single test: `go test -run TestName ./path/to/package` (replace TestName and path)
- Lint code: `make lint` (uses golangci-lint)
- Tidy dependencies: `make tidy` or `go mod tidy`
- Check all: `make check` (runs tidy, fmt, vet, and lint)
- Build with race detection: `go build -race ./cmd/codezilla`
- Install: `make install`
- Clean: `make clean`

## Code Style Guidelines

- **Package Structure**:
  - `cmd/codezilla`: Main application executable
  - `internal/tags`: Core tag parsing and indexing functionality 
  - `internal/cli`: Command-line interface code
  - `internal/ui`: Terminal UI components
  - `pkg/style`: Styling utilities
  - `pkg/util`: General utilities
  - `pkg/logger`: Structured logging utilities

- **Imports**: Standard library imports first, 3rd party packages second, local packages third. Each group separated by a blank line.
- **Formatting**: Follow standard Go formatting (use `go fmt`).
- **Types**: Prefer explicit typing. Use meaningful type names.
- **Naming**: 
  - Use CamelCase for exported identifiers, camelCase for unexported.
  - Use descriptive variable names.
- **Error Handling**: Always check errors. Use descriptive error messages with context.
- **Logging**: Use structured logging with `pkg/logger` which uses Go's `slog` package.
- **Comments**: Document all exported functions, types, and constants.
- **Linting**: All code should pass golangci-lint checks defined in .golangci.yml.