package benchmark

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/benchmark/storage"
)

// BenchmarkSingleDimensionSequential tests single-threaded performance
func BenchmarkSingleDimensionSequential(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	
	bucket, err := provider.CreateBucket(1000, 100, "bench-sequential")
	if err != nil {
		b.Fatalf("Failed to create bucket: %v", err)
	}
	
	ctx := context.Background()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = bucket.Take(ctx)
	}
}

// BenchmarkSingleDimensionConcurrent tests concurrent performance with varying goroutine counts
func BenchmarkSingleDimensionConcurrent(b *testing.B) {
	concurrencyLevels := []int{1, 10, 50, 100}
	
	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrent-%d", concurrency), func(b *testing.B) {
			provider := storage.BenchmarkSetup(b)
			
			bucket, err := provider.CreateBucket(1000, 100, fmt.Sprintf("bench-concurrent-%d", concurrency))
			if err != nil {
				b.Fatalf("Failed to create bucket: %v", err)
			}
			
			ctx := context.Background()
			b.ResetTimer()
			
			b.SetParallelism(concurrency)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = bucket.Take(ctx)
				}
			})
		})
	}
}

// BenchmarkSingleDimensionWithLock tests performance with distributed locking
func BenchmarkSingleDimensionWithLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	
	bucket, err := provider.CreateBucket(1000, 100, "bench-with-lock")
	if err != nil {
		b.Fatalf("Failed to create bucket: %v", err)
	}
	
	ctx := context.Background()
	b.ResetTimer()
	
	b.SetParallelism(10)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = bucket.Take(ctx)
		}
	})
}

// BenchmarkSingleDimensionWithoutLock tests performance without distributed locking
func BenchmarkSingleDimensionWithoutLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	
	bucket, err := provider.CreateBucket(1000, 100, "bench-without-lock", tokenbucket.WithoutLock())
	if err != nil {
		b.Fatalf("Failed to create bucket: %v", err)
	}
	
	ctx := context.Background()
	b.ResetTimer()
	
	b.SetParallelism(10)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = bucket.Take(ctx)
		}
	})
}

// BenchmarkMultiDimensionDistributed tests performance across multiple dimensions
func BenchmarkMultiDimensionDistributed(b *testing.B) {
	dimensionCounts := []int{10, 30, 50}
	
	for _, dimCount := range dimensionCounts {
		b.Run(fmt.Sprintf("Dimensions-%d", dimCount), func(b *testing.B) {
			provider := storage.BenchmarkSetup(b)
			
			// Create buckets for each dimension
			buckets := make([]*tokenbucket.Bucket, dimCount)
			for i := 0; i < dimCount; i++ {
				bucket, err := provider.CreateBucket(100, 10, fmt.Sprintf("bench-multi-%d-%d", dimCount, i))
				if err != nil {
					b.Fatalf("Failed to create bucket %d: %v", i, err)
				}
				buckets[i] = bucket
			}
			
			ctx := context.Background()
			var dimensionIndex int64
			
			b.ResetTimer()
			
			b.SetParallelism(dimCount * 2) // 2 goroutines per dimension
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					// Round-robin across dimensions
					idx := atomic.AddInt64(&dimensionIndex, 1) % int64(dimCount)
					_ = buckets[idx].Take(ctx)
				}
			})
		})
	}
}

// BenchmarkLockComparison directly compares lock vs no-lock performance
func BenchmarkLockComparison(b *testing.B) {
	scenarios := []struct {
		name    string
		withLock bool
	}{
		{"WithLock", true},
		{"WithoutLock", false},
	}
	
	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			provider := storage.BenchmarkSetup(b)
			
			var bucket *tokenbucket.Bucket
			var err error
			
			if scenario.withLock {
				bucket, err = provider.CreateBucket(500, 50, fmt.Sprintf("bench-lock-comp-%s", scenario.name))
			} else {
				bucket, err = provider.CreateBucket(500, 50, fmt.Sprintf("bench-lock-comp-%s", scenario.name), tokenbucket.WithoutLock())
			}
			
			if err != nil {
				b.Fatalf("Failed to create bucket: %v", err)
			}
			
			ctx := context.Background()
			b.ResetTimer()
			
			b.SetParallelism(20)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = bucket.Take(ctx)
				}
			})
		})
	}
}

// BenchmarkWithMetrics demonstrates how to collect detailed metrics during benchmarks
func BenchmarkWithMetrics(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	metrics := NewMetrics()
	
	bucket, err := provider.CreateBucket(1000, 100, "bench-with-metrics")
	if err != nil {
		b.Fatalf("Failed to create bucket: %v", err)
	}
	
	ctx := context.Background()
	
	// Start metric collection
	var wg sync.WaitGroup
	done := make(chan struct{})
	
	// Periodic snapshot collection
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				metrics.TakeSnapshot()
			case <-done:
				return
			}
		}
	}()
	
	b.ResetTimer()
	
	b.SetParallelism(10)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			start := time.Now()
			err := bucket.Take(ctx)
			latency := time.Since(start)
			
			metrics.RecordTake(latency, err == nil)
		}
	})
	
	// Stop metrics collection
	close(done)
	wg.Wait()
	
	// Generate and log report
	report := metrics.GenerateReport()
	b.Logf("Metrics Report:")
	b.Logf("  Total Operations: %d", report.TotalOperations)
	b.Logf("  Success Rate: %.2f%%", report.SuccessRate)
	b.Logf("  Avg Ops/Sec: %.2f", report.AvgOpsPerSecond)
	b.Logf("  Latency P50: %v", report.LatencyP50)
	b.Logf("  Latency P95: %v", report.LatencyP95)
	b.Logf("  Latency P99: %v", report.LatencyP99)
}

// BenchmarkRateLimitedScenario simulates a realistic rate-limited scenario
func BenchmarkRateLimitedScenario(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	
	// Small bucket that will frequently exhaust tokens
	bucket, err := provider.CreateBucket(10, 5, "bench-rate-limited")
	if err != nil {
		b.Fatalf("Failed to create bucket: %v", err)
	}
	
	ctx := context.Background()
	b.ResetTimer()
	
	b.SetParallelism(50) // High contention
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = bucket.Take(ctx)
			// Small delay to simulate real-world usage
			time.Sleep(10 * time.Millisecond)
		}
	})
}