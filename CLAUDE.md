# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

APIC is a Go library for building API clients that abstracts HTTP and WebSocket communications. The library provides two main client types:

- **HTTPClient**: HTTP client with JSON encoding/decoding, rate limiting, authentication hooks, and configurable error handling
- **WSClient**: WebSocket client with automatic reconnection, ping/pong handling, stale connection detection, and rate limiting

## Common Commands

Since this is a Go library (not an application), there are no build or run commands. Standard Go commands apply:

```bash
go test ./...        # Run tests
go mod tidy          # Clean up dependencies  
go build ./...       # Verify compilation
go fmt ./...         # Format code
go vet ./...         # Run static analysis
```

## Architecture

### Core Components

- **http.go**: Main HTTPClient implementation with GET/POST/PUT/PATCH/DELETE methods
- **ws.go**: WSClient implementation with connection management and message handling
- **encode.go**: JSON encoder/decoder interfaces and defaults
- **error.go**: Custom error types (ResponseError, DecodeError)
- **logger.go**: Simple logging interface with no-op default
- **http_option.go**: Functional options for HTTP client configuration
- **ws_option.go**: Functional options for WebSocket client configuration

### HTTP Client Architecture

The HTTPClient uses a functional options pattern for configuration. Key features:
- Rate limiting via golang.org/x/time/rate
- Request/response logging with sensitive header redaction
- Configurable encoders/decoders (defaults to JSON)
- Before-request hooks for authentication
- Response status code validation
- Header manipulation support

### WebSocket Client Architecture

The WSClient manages connection lifecycle with:
- Automatic reconnection with exponential backoff
- Ping/pong handling for keepalive
- Stale connection detection and recovery
- Message handler callbacks
- Connection open/close lifecycle hooks
- Rate-limited write operations

### Dependencies

- `golang.org/x/time`: Rate limiting functionality
- `nhooyr.io/websocket`: WebSocket implementation

### Key Patterns

Both clients use:
- Functional options pattern for configuration
- Interface-based logging for flexibility
- Context-aware operations
- Graceful error handling with custom error types