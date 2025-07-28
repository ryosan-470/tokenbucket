# TokenBucket Benchmark Suite

This directory contains benchmark tests for performance evaluation of the TokenBucket library.

## Backend Support

The benchmark suite supports two DynamoDB backends:

- **Local** (default): Uses DynamoDB Local via testcontainers for isolated testing
- **AWS**: Uses real AWS DynamoDB with AWS_PROFILE authentication

## Benchmark Scenarios

### 1. Single Dimension Tests
- **Memory Backend**: High-frequency token acquisition using in-memory backend
- **Without Lock**: Performance testing without distributed locking
- **With Lock**: Performance testing with distributed locking enabled
- Configuration: Capacity=1000, FillRate=100 tokens/second
- Parallelism: 10 goroutines

### 2. Multiple Dimension Tests
- **Memory Backend**: Performance across 10 dimensions using in-memory backend
- **Without Lock**: Multi-dimension testing without distributed locking
- **With Lock**: Multi-dimension testing with distributed locking
- Configuration per dimension: Capacity=1000, FillRate=100 tokens/second
- Round-robin token acquisition across dimensions
- Parallelism: 10 goroutines

### 3. CLI Load Testing Tool (cmd/benchmark)
The command-line tool provides comprehensive performance testing with real-time metrics:

**Available Scenarios:**
- `single`: Single dimension high-frequency token acquisition
- `multi`: Multiple dimension round-robin testing

**Backend Types:**
- `custom`: Default custom backend implementation
- `memory`: In-memory backend for testing
- `limiters`: mennanov/limiters backend with race checking

**Provider Types:**
- `local`: DynamoDB Local via testcontainers (default)
- `aws`: AWS DynamoDB with AWS profile authentication

**Features:**
- Real-time metrics with 1-second snapshots
- Configurable concurrency, duration, and bucket parameters
- Distributed locking support
- Report generation and optional file output

## Execution

### Go Standard Benchmarks
```bash
# Run all benchmarks
go test -bench=. -benchtime=30s

# Run using Makefile
make benchmark

# Single dimension benchmarks
go test -bench=BenchmarkSingleDimension -benchtime=60s

# Multiple dimension benchmarks
go test -bench=BenchmarkMultipleDimension -benchtime=30s

# Memory backend tests
go test -bench=WithMemoryBackend -benchtime=30s

# Lock vs no-lock comparison
go test -bench=WithoutLock -benchtime=30s
go test -bench=WithLock -benchtime=30s

# Run with CPU profiling
go test -bench=. -cpuprofile=cpu.prof

# Benchmark with memory profiling
go test -bench=. -memprofile=mem.prof
```

### CLI Load Testing Tool

#### Basic Usage
```bash
cd cmd/benchmark

# Single dimension test (matches BenchmarkSingleDimension configuration)
go run main.go -scenario=single -concurrency=10 -capacity=1000 -fill-rate=100 -duration=60s

# Multi dimension test (10 dimensions with round-robin)
go run main.go -scenario=multi -dimensions=10 -concurrency=10 -capacity=1000 -fill-rate=100 -duration=60s

# Memory backend test
go run main.go -scenario=single -backend-type=memory -concurrency=10 -duration=60s

# Test with distributed locking
go run main.go -scenario=single -with-lock -concurrency=10 -duration=60s
```

#### Backend Configurations
```bash
# Custom backend (default)
go run main.go -scenario=single -backend-type=custom -concurrency=10

# Limiters backend with race checking
go run main.go -scenario=single -backend-type=limiters -race-check -concurrency=10

# Memory backend for single-node testing
go run main.go -scenario=single -backend-type=memory -concurrency=10
```

#### Provider Types
```bash
# Local DynamoDB (default)
go run main.go -provider-type=local -scenario=single

# AWS DynamoDB (requires AWS_PROFILE)
export AWS_PROFILE=your-aws-profile
go run main.go -provider-type=aws -scenario=single

# Save results to file
go run main.go -scenario=single -output=results.json
```

#### Complete Example Commands
```bash
# High-load single dimension test matching benchmark configuration
go run main.go \
  -scenario=single \
  -concurrency=10 \
  -capacity=1000 \
  -fill-rate=100 \
  -backend-type=custom \
  -duration=60s

# Multi-dimension distributed test with locking
go run main.go \
  -scenario=multi \
  -dimensions=10 \
  -concurrency=20 \
  -capacity=200 \
  -fill-rate=20 \
  -with-lock \
  -duration=60s

# AWS backend performance test
export AWS_PROFILE=your-profile
go run main.go \
  -provider-type=aws \
  -scenario=single \
  -backend-type=limiters \
  -race-check \
  -concurrency=10 \
  -duration=120s
```

**Note**: The AWS backend will automatically create the required DynamoDB tables with fixed names if they don't exist:
- Bucket table: `tokenbucket-benchmark-bucket` (PK: String, SK: String, TTL: `_TTL`)
- Lock table: `tokenbucket-benchmark-lock` (LockID: String, TTL: `_TTL`)

Both tables are created with Pay-Per-Request billing mode for cost efficiency.

## Metrics

- **Throughput**: Operations per second
- **Latency**: P50, P95, P99, Max, Min
- **Success Rate**: Percentage of successful Take operations
- **Time Series Data**: Snapshots at 1-second intervals

## Result Interpretation

- **Single Dimension**: Measurement of maximum throughput with high-frequency access patterns
- **Multiple Dimension**: Performance evaluation with distributed load across multiple buckets
- **Memory vs DynamoDB**: Backend performance comparison for different deployment scenarios
- **Lock vs No-Lock**: Trade-offs between consistency and performance
- **Sustained Test**: Long-term stability and performance characteristics
