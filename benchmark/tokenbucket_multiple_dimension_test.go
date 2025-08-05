// This test case benchmarks the performance of a multiple dimension token bucket.
// There are 10 dimensions, each with a capacity of 200 tokens and a refill rate of 20 tokens per second.
package benchmark

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/benchmark/storage"
	"github.com/stretchr/testify/require"
)

const (
	dimensions = 10

	capacityForMultipleDimension   = 200
	refillRateForMultipleDimension = 20
)

// BenchmarkMultipleDimension_WithMemoryBackend tests the performance of a multiple dimension token bucket
func BenchmarkMultipleDimension_WithMemoryBackend(b *testing.B) {
	provider := storage.BenchmarkSetup(b)

	buckets := make([]*tokenbucket.Bucket, dimensions)
	for i := 0; i < dimensions; i++ {
		dimension := fmt.Sprintf("bench-with-memory-backend-%d", i)
		bucket, err := provider.CreateBucket(
			capacityForMultipleDimension,
			refillRateForMultipleDimension,
			dimension,
			tokenbucket.WithMemoryBackend(),
		)
		require.NoError(b, err, "Failed to create bucket: %v", err)
		buckets[i] = bucket
	}

	ctx := context.Background()
	var dimensionIdx int64
	b.ResetTimer()

	b.SetParallelism(dimensions * 2) // 2 goroutines per dimension
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Round-robin across dimensions
			idx := atomic.AddInt64(&dimensionIdx, 1) % int64(dimensions)

			_ = buckets[idx].Take(ctx)
		}
	})
}

// BenchmarkMultipleDimension_WithoutLock tests the performance of a multiple dimension token bucket without locks
func BenchmarkMultipleDimension_WithoutLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)

	buckets := make([]*tokenbucket.Bucket, dimensions)
	for i := 0; i < dimensions; i++ {
		dimension := fmt.Sprintf("bench-without-lock-%d", i)
		bucket, err := provider.CreateBucket(
			capacityForMultipleDimension,
			refillRateForMultipleDimension,
			dimension,
		)
		require.NoError(b, err, "Failed to create bucket: %v", err)
		buckets[i] = bucket
	}

	ctx := context.Background()
	var dimensionIdx int64
	b.ResetTimer()

	b.SetParallelism(dimensions * 2) // 2 goroutines per dimension
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Round-robin across dimensions
			idx := atomic.AddInt64(&dimensionIdx, 1) % int64(dimensions)

			buckets[idx].Take(ctx)
		}
	})
}

// BenchmarkMultipleDimension_WithLock tests the performance of a multiple dimension token bucket with locks
func BenchmarkMultipleDimension_WithLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)

	buckets := make([]*tokenbucket.Bucket, dimensions)
	for i := 0; i < dimensions; i++ {
		dimension := fmt.Sprintf("bench-with-lock-%d", i)
		bucket, err := provider.CreateBucket(
			capacityForMultipleDimension,
			refillRateForMultipleDimension,
			dimension,
			tokenbucket.WithLockBackend(provider.CreateLockBackendConfig(), dimension),
		)
		require.NoError(b, err, "Failed to create bucket: %v", err)
		buckets[i] = bucket
	}

	ctx := context.Background()
	var dimensionIdx int64
	b.ResetTimer()

	b.SetParallelism(dimensions * 2) // 2 goroutines per dimension
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Round-robin across dimensions
			idx := atomic.AddInt64(&dimensionIdx, 1) % int64(dimensions)

			buckets[idx].Take(ctx)
		}
	})
}

// BenchmarkMultipleDimension_OptimisticLock tests the performance of a multiple dimension token bucket with optimistic locking
func BenchmarkMultipleDimension_OptimisticLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)

	for _, tc := range []struct {
		message string
		opts    []tokenbucket.Option
	}{
		{
			message: "WithCustomBackend",
			opts:    []tokenbucket.Option{},
		},
	} {
		b.Run(tc.message, func(b *testing.B) {
			buckets := make([]*tokenbucket.Bucket, dimensions)
			for i := 0; i < dimensions; i++ {
				dimension := fmt.Sprintf("bench-optimistic-lock-%d", i)
				bucket, err := provider.CreateBucket(capacityForMultipleDimension, refillRateForMultipleDimension, dimension, tc.opts...)
				require.NoError(b, err, "Failed to create bucket: %v", err)
				buckets[i] = bucket
			}

			ctx := context.Background()
			var dimensionIdx int64
			b.ResetTimer()

			b.SetParallelism(dimensions * 2) // 2 goroutines per dimension
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					// Round-robin across dimensions
					idx := atomic.AddInt64(&dimensionIdx, 1) % int64(dimensions)

					buckets[idx].Take(ctx)
				}
			})
		})
	}
}

// BenchmarkMultipleDimension_PessimisticLock tests the performance of a multiple dimension token bucket with pessimistic locking
func BenchmarkMultipleDimension_PessimisticLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)

	lockCfg := provider.CreateLockBackendConfig()
	lockBackend := tokenbucket.WithLockBackend(
		lockCfg,
		"bench-pessimistic-lock",
	)

	for _, tc := range []struct {
		message string
		opts    []tokenbucket.Option
	}{
		{
			message: "WithCustomBackend",
			opts:    []tokenbucket.Option{lockBackend},
		},
	} {
		b.Run(tc.message, func(b *testing.B) {
			buckets := make([]*tokenbucket.Bucket, dimensions)
			for i := 0; i < dimensions; i++ {
				dimension := fmt.Sprintf("bench-pessimistic-lock-%d", i)
				bucket, err := provider.CreateBucket(capacityForMultipleDimension, refillRateForMultipleDimension, dimension, tc.opts...)
				require.NoError(b, err, "Failed to create bucket: %v", err)
				buckets[i] = bucket
			}

			ctx := context.Background()
			var dimensionIdx int64
			b.ResetTimer()

			b.SetParallelism(dimensions * 2) // 2 goroutines per dimension
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					// Round-robin across dimensions
					idx := atomic.AddInt64(&dimensionIdx, 1) % int64(dimensions)

					buckets[idx].Take(ctx)
				}
			})
		})
	}
}
