# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go library implementing a token bucket algorithm for rate limiting, using DynamoDB as the storage backend for distributed environments. The library provides both token bucket functionality and distributed locking capabilities.

## Architecture

The codebase is organized as follows:

- **Root package (`tokenbucket`)**: Main interface and implementation
  - `TokenBucket` interface with `Take()` and `Get()` methods
  - `Bucket` struct containing capacity, fill rate, available tokens, and backend
  - Uses the `github.com/mennanov/limiters` library as the underlying implementation

- **Storage layer (`storage/dynamodb/`)**: DynamoDB-specific implementations
  - `config.go`: Backend configuration for DynamoDB token bucket storage
  - `lock.go`: Distributed locking implementation using DynamoDB with TTL and backoff
  - `state.go`: Constants for DynamoDB attribute names and structure

## Key Design Patterns

- **Dependency Injection**: Backend storage is injected via `BucketBackendConfig`
- **Interface Segregation**: Clean separation between token bucket interface and storage backend
- **Distributed Locking**: Custom DynamoDB-based lock implementation with automatic cleanup via TTL
- **Error Handling**: Custom errors defined in `errors.go`

## Development Commands

This is a standard Go module. Common commands:

```bash
# Build the module
go build ./...

# Run tests (if any exist)
go test ./...

# Get dependencies
go mod tidy

# Format code
go fmt ./...

# Vet code
go vet ./...
```

## Dependencies

- **AWS SDK v2**: For DynamoDB operations
- **github.com/mennanov/limiters**: Core token bucket and rate limiting logic
- **github.com/cenkalti/backoff/v5**: Retry logic with exponential backoff
- **github.com/google/uuid**: UUID generation for lock ownership

## DynamoDB Schema Requirements

The library expects DynamoDB tables with specific attributes:
- **Token Bucket Table**: Uses partition key `PK`, sort key `SK`, and TTL attribute `_TTL`
- **Lock Table**: Uses `LockID` as primary key, `OwnerID` for ownership, and `TTL` for automatic cleanup

## Usage Pattern

Typical usage involves:
1. Creating a `BucketBackendConfig` with DynamoDB client and table configuration
2. Instantiating a `Bucket` with capacity, fill rate, and dimension
3. Using `Take()` to consume tokens and `Get()` to check current state
4. The library handles distributed coordination automatically via DynamoDB

## Localization Guidelines

- **Communication Guidelines**:
  - 私と会話する際は日本語で対応してください。ただしコード上のコメントなどは全て英語でお願いします。