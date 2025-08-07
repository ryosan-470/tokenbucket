# TokenBucket Benchmark Suite

Performance evaluation tools for the TokenBucket library supporting various backends and test scenarios.

## Test Types

### Go Benchmark Tests
Standard Go benchmarks located in test files:
- `tokenbucket_single_dimension_test.go`: Single dimension performance testing
- `tokenbucket_multiple_dimension_test.go`: Multi-dimension round-robin testing

### CLI Load Testing Tool
Interactive command-line tool at `cmd/benchmark/main.go` for sustained load testing with real-time metrics.

## Backend Support

**Backend Types:**
- **Memory**: In-memory backend for single-node testing (default)
- **Custom**: DynamoDB backend implementation

**Provider Types (for DynamoDB backends):**
- **Local**: DynamoDB Local via testcontainers (default)
- **AWS**: Real AWS DynamoDB with AWS_PROFILE authentication

## Usage

### Go Benchmark Tests
```bash
# Run all benchmarks
make benchmark
# or
go test -bench=. -benchmem ./benchmark

# Specific scenarios
go test -bench=BenchmarkSingleDimension -benchtime=60s ./benchmark
go test -bench=BenchmarkMultipleDimension -benchtime=30s ./benchmark
```

### CLI Load Testing Tool
```bash
cd benchmark/cmd/benchmark

# Basic usage
go run main.go -scenario=single -concurrency=10 -duration=60s
go run main.go -scenario=multi -dimensions=10 -concurrency=10 -duration=60s

# Backend types
go run main.go -backend-type=memory    # In-memory backend (default)
go run main.go -backend-type=custom    # DynamoDB backend

# Provider types
go run main.go -provider-type=local    # DynamoDB Local (default)
export AWS_PROFILE=your-profile
go run main.go -provider-type=aws      # AWS DynamoDB
```

## Configuration

Test parameters can be customized:
- **Capacity**: Maximum tokens in bucket (default: 1000)
- **Fill Rate**: Token refill rate per second (default: 100)
- **Concurrency**: Number of goroutines (default: 10)
- **Duration**: Test duration (default: 60s)
- **Dimensions**: Number of buckets for multi-dimension tests (default: 10)

## Metrics

CLI tool provides real-time metrics:
- **Throughput**: Operations per second
- **Latency**: P50, P95, P99 percentiles
- **Success Rate**: Successful token acquisitions
- **Time Series**: 1-second interval snapshots
