# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go library implementing a token bucket algorithm for rate limiting, using DynamoDB as the storage backend for distributed environments. The library provides both token bucket functionality and distributed locking capabilities.

## Architecture

The codebase is organized as follows:

- **Root package (`tokenbucket`)**: Main interface and implementation
  - `tokenbucket.go`: `TokenBucket` interface with `Take()` and `Get()` methods and `Bucket` struct implementation
  - `tokenbucket_test.go`: Unit tests for the main tokenbucket functionality
  - `main_test.go`: Test utilities and setup for the root package
  - Uses the `github.com/mennanov/limiters` library as the underlying implementation
  - `errors.go`: Custom error definitions for the library

- **Internal utilities (`internal/`)**: Shared utilities
  - `testutils/`: Testing utilities
    - `dynamodb.go`: DynamoDB test setup and helper functions
    - `time.go`: Time-related test utilities
  - `clock/`: Clock abstraction for testable time operations
    - `clock.go`: Clock interface and system clock implementation

- **Storage layer (`storage/`)**: Storage backends and interfaces
  - `state.go`: Common storage interface and state structure
  - `memory/`: In-memory storage backend
    - `backend.go`: Memory-based storage implementation
    - `backend_test.go`: Tests for memory backend
  - `dynamodb/`: DynamoDB-specific implementations
    - `adapter.go`: Adapter for limiters library integration
    - `backend.go`: DynamoDB backend implementation for token bucket storage
    - `backend_test.go`: Tests for DynamoDB backend functionality
    - `config.go`: Backend configuration for DynamoDB token bucket storage and lock settings
    - `lock.go`: Distributed locking implementation using DynamoDB with TTL and backoff
    - `lock_test.go`: Comprehensive test suite for lock functionality
    - `main_test.go`: Test setup and teardown utilities

- **Benchmark package (`benchmark/`)**: Performance testing and metrics
  - `cmd/benchmark/main.go`: Command-line benchmarking tool
  - `metrics.go`: Performance metrics collection
  - `storage/`: Storage implementations for benchmarking
    - `aws.go`: AWS DynamoDB storage for benchmarks
    - `local.go`: Local/in-memory storage for benchmarks
    - `storage.go`: Common storage interfaces
  - `tokenbucket_single_dimension_test.go`: Single dimension benchmark tests
  - `tokenbucket_multiple_dimension_test.go`: Multiple dimension benchmark tests

## Key Design Patterns

- **Interface Segregation**: Clean separation between token bucket interface and storage backend via `storage.Storage` interface
- **Adapter Pattern**: Seamless integration with `limiters` library through adapter layer
- **Dependency Injection**: Backend storage is injected via configuration
- **Distributed Locking**: Custom DynamoDB-based lock implementation with automatic cleanup via TTL
- **Clock Abstraction**: Testable time operations through `internal/clock` interface
- **Error Handling**: Custom errors defined in `errors.go`
- **Options Pattern**: Functional options for configuring bucket behavior
- **Testcontainers**: Isolated testing with DynamoDB Local containers

## Development Commands

This is a standard Go module. Common commands:

```bash
# Build the module
go build ./...

# Run tests
make test
# or
go test ./...

# Run tests with verbose output
go test -v ./...

# Run specific test
go test -v ./storage/dynamodb/ -run TestLock_BasicLockUnlock

# Run benchmark tests
make benchmark
# or
go test -bench=. -benchmem ./benchmark

# Run CLI load testing tool
cd benchmark/cmd/benchmark
go run main.go -scenario=single -concurrency=10 -duration=60s

# Get dependencies
go mod tidy

# Format code
go fmt ./...

# Vet code
go vet ./...
```

## Testing

The project includes comprehensive test coverage:

### Lock Implementation Tests (`storage/dynamodb/lock_test.go`)
- **Basic lock/unlock functionality**: Tests successful acquisition and release of locks
- **Concurrent access**: Tests that multiple processes cannot hold the same lock simultaneously
- **TTL expiration**: Tests that locks are automatically released after TTL expires
- **Error handling**: Tests graceful handling of edge cases like unlocking non-existent locks
- **Wrong owner scenarios**: Tests that locks can only be released by their owners

### Infrastructure
- Uses **testcontainers** with DynamoDB Local containers for isolated test environments
- Automatic table creation and TTL configuration in tests
- Proper wait strategies for container startup and table readiness

### Benchmark Suite
- **Single Dimension Tests** (`tokenbucket_single_dimension_test.go`): Performance testing with high load on a single dimension
- **Multiple Dimension Tests** (`tokenbucket_multiple_dimension_test.go`): Multi-dimension round-robin testing
- **CLI Load Testing Tool** (`cmd/benchmark/main.go`): Interactive command-line tool for sustained load testing
  - Real-time metrics with 1-second snapshots
  - Backend types: `memory` (default), `custom` (DynamoDB)
  - Provider types: `local` (DynamoDB Local), `aws` (AWS DynamoDB)
  - Available scenarios: `single`, `multi`
- Configuration: Capacity=1000, FillRate=100 tokens/second, 10 goroutines (default)
- Metrics collection for throughput and latency analysis

## Dependencies

- **AWS SDK v2**: For DynamoDB operations (`github.com/aws/aws-sdk-go-v2/service/dynamodb`)
- **github.com/mennanov/limiters**: Core token bucket and rate limiting logic
- **github.com/cenkalti/backoff/v5**: Retry logic with exponential backoff
- **github.com/google/uuid**: UUID generation for lock ownership
- **testcontainers**: For isolated testing with DynamoDB Local (`github.com/testcontainers/testcontainers-go`)

## DynamoDB Schema Requirements

The library expects DynamoDB tables with specific attributes:

### Token Bucket Table
- Uses attributes defined by the `limiters` library
- Requires proper table properties loaded via `LoadDynamoDBTableProperties`

### Lock Table
- **Primary Key**: `LockID` (String) - Unique identifier for the lock
- **Attributes**:
  - `OwnerID` (String) - UUID of the lock owner
  - `_TTL` (Number) - Unix timestamp for automatic lock expiration
- **TTL Configuration**: Must have TTL enabled on the `_TTL` attribute

## Usage Pattern

Typical usage involves:

1. **Configuration**: Creating a `BucketBackendConfig` with:
   - DynamoDB client
   - Table names for token bucket and locks
   - Lock settings (TTL, retry parameters)

2. **Bucket Creation**: Instantiating a `Bucket` with:
   - Capacity (maximum tokens)
   - Fill rate (tokens per second)
   - Dimension (unique identifier)
   - Optional parameters (clock, logger, lock behavior)

3. **Operations**:
   - `Take()`: Consume tokens from the bucket
   - `Get()`: Check current bucket state

4. **Distributed Coordination**: The library handles distributed locking automatically via DynamoDB

## Configuration Options

### Lock Configuration
- `lockTTL`: Time-to-live for locks (automatic cleanup)
- `lockMaxTries`: Maximum retry attempts for lock acquisition
- `lockMaxTime`: Maximum time to spend retrying lock acquisition

### Bucket Options
- `WithClock()`: Custom clock implementation for testing (uses `internal/clock` interface)
- `WithLogger()`: Custom logger for debugging
- `WithoutLock()`: Disable distributed locking (for single-node deployments)
- `WithMemoryBackend()`: Use in-memory backend via `storage/memory` package

## Localization Guidelines

- **Communication Guidelines**:
  - 私と会話する際は日本語で対応してください。ただしコード上のコメントなどは全て英語でお願いします。