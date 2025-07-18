# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go library for building API clients with support for both HTTP and WebSocket connections. The library uses functional options pattern for configuration and provides a clean, interface-based design.

## Commands

### Development Commands
```bash
# Build the library
go build ./...

# Run tests (note: no tests currently exist)
go test ./...

# Format code
go fmt ./...

# Run static analysis
go vet ./...

# Manage dependencies
go mod tidy

# Check for compilation errors
go build -v ./...
```

## Architecture

### Core Components

1. **HTTP Client** (`http.go`, `http_option.go`):
   - Configurable HTTP client using functional options
   - Features: rate limiting, logging, auth hooks, custom encoders/decoders
   - Key method: `Do(ctx, method, endpoint, body, response) error`

2. **WebSocket Client** (`ws.go`, `ws_option.go`):
   - WebSocket client with automatic reconnection
   - Features: message handlers, connection callbacks, rate limiting, reconnection policies
   - Key methods: `Start()`, `Stop()`, `Send(ctx, msg) error`

3. **Supporting Types**:
   - `Logger` interface (`logger.go`): Simple logging abstraction
   - `Encoder`/`Decoder` interfaces (`encode.go`): Pluggable serialization (defaults to JSON)
   - Error types (`error.go`): `ResponseError`, `DecodeError`, `MaxAttemptsError`

### Design Patterns

- **Functional Options**: All configuration uses `WithXxx()` functions
- **Interface-based**: Logger, Encoder, Decoder are interfaces for flexibility
- **Context-aware**: All operations accept context.Context for cancellation
- **Builder Pattern**: Clients are built with New() functions and configured with options

### Key Implementation Details

- HTTP client logs requests/responses, filtering sensitive headers
- WebSocket client implements exponential backoff for reconnection
- Both clients support rate limiting via `golang.org/x/time/rate`
- Default encoder/decoder uses `encoding/json`
- WebSocket uses `nhooyr.io/websocket` as the underlying implementation

## Important Notes

- No test files exist yet - when adding features, create corresponding test files
- The library is designed for simplicity - avoid adding complex features unless necessary
- Maintain the functional options pattern for any new configuration
- Keep external dependencies minimal (currently only 2 direct dependencies)
- Follow the existing error handling patterns with typed errors