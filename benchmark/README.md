# TokenBucket Benchmark Suite

This directory contains benchmark tests for performance evaluation of the TokenBucket library.

## Structure

- `harness.go` - DynamoDB Local setup and common configuration
- `metrics.go` - Metrics collection and analysis
- `tokenbucket_test.go` - Go standard benchmark tests
- `cmd/benchmark/main.go` - CLI for 60-second sustained load tests
- `results/` - Benchmark results storage directory

## Benchmark Scenarios

### 1. Single Dimension High Load Test
- High-frequency token acquisition against a single dimension
- Concurrency levels: 1, 10, 50, 100 goroutines
- Configuration: Capacity=1000, FillRate=100

### 2. Multi Dimension Distributed Load Test
- Distributed token acquisition across multiple dimensions (10-50)
- Each dimension: Capacity=100, FillRate=10
- Token acquisition at moderate intervals

### 3. Lock vs No-Lock Comparison Test
- Performance comparison between WithLock/WithoutLock under identical conditions

### 4. 60-Second Sustained Load Test
- Time-series metrics collection
- Analysis of throughput and latency variations

## Execution

### Go Standard Benchmarks
```bash
# Run all benchmarks
go test -bench=. -benchtime=30s

# Run specific benchmark
go test -bench=BenchmarkSingleDimension -benchtime=60s

# Run with CPU profiling
go test -bench=. -cpuprofile=cpu.prof

# Performance measurement by concurrency level
go test -bench=BenchmarkSingleDimensionConcurrent -benchtime=60s

# Lock vs no-lock comparison
go test -bench=BenchmarkLockComparison -benchtime=30s

# Benchmark with detailed metrics
go test -bench=BenchmarkWithMetrics -benchtime=30s -v
```

### 60-Second Sustained Tests
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

## Metrics

- **Throughput**: Operations per second
- **Latency**: P50, P95, P99, Max, Min
- **Success Rate**: Percentage of successful Take operations
- **Time Series Data**: Snapshots at 1-second intervals

## Result Interpretation

- **Single Dimension**: Measurement of maximum throughput and scalability
- **Multi Dimension**: Performance evaluation under realistic usage patterns
- **Lock Comparison**: Selection criteria based on distributed coordination requirements
- **Sustained Test**: Stability confirmation during long-term operation