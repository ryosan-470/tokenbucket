# TokenBucket Benchmark Suite

This directory contains benchmark tests for performance evaluation of the TokenBucket library.

## Structure

- `storage/` - Backend storage configurations
  - `storage.go` - Common storage interface
  - `local.go` - DynamoDB Local setup via testcontainers
  - `aws.go` - AWS DynamoDB setup and configuration
- `metrics.go` - Metrics collection and analysis
- `tokenbucket_single_dimension_test.go` - Single dimension benchmark tests
- `tokenbucket_multiple_dimension_test.go` - Multiple dimension benchmark tests
- `cmd/benchmark/main.go` - CLI for sustained load tests
- `results/` - Benchmark results storage directory

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
- Configuration per dimension: Capacity=200, FillRate=20 tokens/second
- Round-robin token acquisition across dimensions
- Parallelism: 20 goroutines (2 per dimension)

### 3. 60-Second Sustained Load Test (CLI)
- Time-series metrics collection
- Analysis of throughput and latency variations
- Configurable scenarios and backends

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

### Sustained Load Tests

#### DynamoDB Local (Default)
```bash
cd cmd/benchmark

# Single dimension high load test
go run main.go -scenario=single -concurrency=50 -duration=60s

# Multi dimension distributed test
go run main.go -scenario=multi -dimensions=30 -concurrency=60 -duration=60s

# Lock vs no-lock performance comparison
go run main.go -scenario=lock-comparison -concurrency=20 -duration=60s

# Test with custom configuration
go run main.go -scenario=single -capacity=500 -fill-rate=50 -concurrency=25 -duration=120s
```

#### AWS DynamoDB
```bash
cd cmd/benchmark

# Set AWS profile (required for AWS backend)
export AWS_PROFILE=your-aws-profile

# Single dimension test with AWS backend
go run main.go -backend=aws -scenario=single -concurrency=50 -duration=60s

# Multi dimension test with AWS backend
go run main.go -backend=aws -scenario=multi -dimensions=30 -concurrency=60 -duration=60s

# Lock comparison test on AWS
go run main.go -backend=aws -scenario=lock-comparison -concurrency=20 -duration=60s
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