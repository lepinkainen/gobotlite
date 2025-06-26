# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gobotlite is a lightweight IRC bot written in Go that acts as a shim layer, delegating actual command processing and URL title parsing to AWS Lambda functions. The bot connects to multiple IRC networks simultaneously and handles both dot-prefixed commands (`.command`) and automatic URL title fetching.

## Architecture

### Core Components

- **main.go**: Entry point with configuration loading, multi-network IRC connection management, and event handling
- **command.go**: Command processing logic that forwards IRC commands to Lambda functions
- **title.go**: URL title fetching via Lambda function integration
- **config.yaml**: YAML configuration file defining networks, channels, nicknames, and Lambda endpoints

### Key Architecture Patterns

- **Multi-network support**: Uses goroutines to connect to multiple IRC networks concurrently
- **Lambda integration**: All business logic is handled by external AWS Lambda functions via HTTP POST requests
- **Event-driven processing**: Uses the go-ircevent library's callback system for IRC event handling
- **Retry logic**: Implements exponential backoff for IRC connection failures

### Configuration Structure

The bot is configured via `config.yaml` with the following key sections:

- `networks`: Map of network configurations (server, channels, TLS, port)
- `nickname`: IRC nickname to use across all networks
- `lambdatitle`: Lambda endpoint for URL title parsing
- `lambdacommand`: Lambda endpoint for command processing
- Security and logging configuration

## Development Commands

### Build System (Task)

The project uses Task (Taskfile.yml) for build automation:

```bash
# Default build (includes tests and clean)
task

# Build for current platform
task build

# Build for Linux deployment
task build-linux

# Run tests with coverage
task test

# Run linter
task lint

# Clean build artifacts
task clean

# Deploy to remote server (requires DESTINATION env var)
task publish

# Upgrade dependencies
task upgrade-deps

# Build for CI environments
task build-ci

# Run tests for CI environments
task test-ci
```

### Go Commands

```bash
# Build manually
go build -o build/gobotlite

# Run tests
go test -v -race ./...

# Run with coverage
go test -v -race -coverprofile=coverage/coverage.out ./...
go tool cover -html=coverage/coverage.out -o coverage/coverage.html

# Run linter (requires golangci-lint)
golangci-lint run ./...

# Tidy dependencies
go mod tidy
```

## Testing

- Tests are run automatically before builds via Task
- Coverage reports are generated in `coverage/` directory
- Use `go test -v` for verbose test output
- No specific test files exist yet, but the framework expects standard Go testing patterns

## Dependencies

- **github.com/thoj/go-ircevent**: IRC client library
- **github.com/spf13/viper**: Configuration management
- Standard Go libraries for HTTP, TLS, concurrency, and structured logging (log/slog)

## Configuration Notes

- The bot expects `config.yaml` in the working directory
- Configuration is loaded using Viper for flexible config management
- API keys for Lambda functions are stored in the configuration file
- TLS is configured per network with InsecureSkipVerify enabled
- Channels are auto-prefixed with `#` if not present
- Default IRC port is 6667, override with `port` in network config
- Structured logging using log/slog for better observability

## Lambda Integration

The bot sends structured JSON payloads to Lambda functions:

- **Commands**: `{command, args, channel, user}`
- **URLs**: `{url, channel, user}`

Responses are expected in format:

- **Commands**: `{result, errorMessage}`
- **URLs**: `{title, errorMessage}`
