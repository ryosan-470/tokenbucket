package benchmark

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
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

// BenchmarkSingleDimensionWithMemoryBackend tests performance with in-memory backend
func BenchmarkSingleDimensionWithMemoryBackend(b *testing.B) {
	provider := storage.BenchmarkSetup(b)

	bucket, err := provider.CreateBucket(
		1000,
		100,
		"bench-with-memory-backend",
		tokenbucket.WithMemoryBackend(),
	)
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

// BenchmarkSingleDimensionWithLock tests performance with distributed locking
func BenchmarkSingleDimensionWithLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)

	bucket, err := provider.CreateBucket(
		1000,
		100,
		"bench-with-lock",
		tokenbucket.WithLockBackend(
			provider.CreateLockBackendConfig(),
			uuid.NewString(),
		),
	)
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

// BenchmarkSingleDimensionWithLimitersBackend tests performance with Limiters backend
func BenchmarkSingleDimensionWithLimitersBackend(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	dimension := "bench-with-limiters-backend"

	bucket, err := provider.CreateBucket(
		1000,
		100,
		dimension,
		tokenbucket.WithLimitersBackend(
			provider.CreateBucketConfig(dimension),
			uuid.NewString(),
			true, // enable race condition checking
		),
	)
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
