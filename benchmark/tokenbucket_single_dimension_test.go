// This test case benchmarks the performance of a single dimension token bucket.
// The bucket has a capacity of 1000 tokens and a refill rate of 100 tokens per second.
package benchmark

import (
	"context"
	"testing"

	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/benchmark/storage"
	"github.com/stretchr/testify/require"
)

const (
	capacityForSingleDimension   = 1000
	refillRateForSingleDimension = 100

	parallels = 10
)

// BenchmarkSingleDimension_WithMemoryBackend tests the performance of a single dimension token bucket
func BenchmarkSingleDimension_WithMemoryBackend(b *testing.B) {
	provider := storage.BenchmarkSetup(b)

	bucket, err := provider.CreateBucket(
		capacityForSingleDimension,
		refillRateForSingleDimension,
		"bench-with-memory-backend",
		tokenbucket.WithMemoryBackend(),
	)
	require.NoError(b, err, "Failed to create bucket: %v", err)

	ctx := context.Background()
	b.ResetTimer()

	b.SetParallelism(parallels)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = bucket.Take(ctx)
		}
	})
}

// BenchmarkSingleDimension_WithoutLock tests the performance of a single dimension token bucket without locks
func BenchmarkSingleDimension_WithoutLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	dimension := "bench-without-lock"

	bucket, err := provider.CreateBucket(
		capacityForSingleDimension,
		refillRateForSingleDimension,
		dimension,
		tokenbucket.WithLimitersBackend(provider.CreateBucketConfig(dimension), dimension, false),
	)
	require.NoError(b, err, "Failed to create bucket: %v", err)

	ctx := context.Background()
	b.ResetTimer()
	b.SetParallelism(parallels)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bucket.Take(ctx)
		}
	})
}

// BenchmarkSingleDimension_WithLock tests the performance of a single dimension token bucket with locks
func BenchmarkSingleDimension_WithLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	dimension := "bench-with-lock"

	bucket, err := provider.CreateBucket(
		capacityForSingleDimension,
		refillRateForSingleDimension,
		dimension,
		tokenbucket.WithLimitersBackend(provider.CreateBucketConfig(dimension), dimension, false),
		tokenbucket.WithLockBackend(provider.CreateLockBackendConfig(), dimension),
	)
	require.NoError(b, err, "Failed to create bucket: %v", err)

	ctx := context.Background()
	b.ResetTimer()
	b.SetParallelism(parallels)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bucket.Take(ctx)
		}
	})
}

// BenchmarkSingleDimension_OptimisticLock tests the performance of a single dimension token bucket with optimistic locking
func BenchmarkSingleDimension_OptimisticLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	dimension := "bench-optimistic-lock"

	for _, tc := range []struct {
		message string
		opts    []tokenbucket.Option
	}{
		{
			message: "WithCustomBackend",
			opts:    []tokenbucket.Option{},
		},
		{
			message: "WithLimitersBackend",
			opts: []tokenbucket.Option{
				tokenbucket.WithLimitersBackend(provider.CreateBucketConfig(dimension), dimension, true),
			},
		},
	} {
		b.Run(tc.message, func(b *testing.B) {
			bucket, err := provider.CreateBucket(capacityForSingleDimension, refillRateForSingleDimension, dimension, tc.opts...)
			require.NoError(b, err, "Failed to create bucket: %v", err)

			ctx := context.Background()
			b.ResetTimer()
			b.SetParallelism(parallels)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					bucket.Take(ctx)
				}
			})
		})
	}
}

// BenchmarkSingleDimension_PessimisticLock tests the performance of a single dimension token bucket with pessimistic locking
func BenchmarkSingleDimension_PessimisticLock(b *testing.B) {
	provider := storage.BenchmarkSetup(b)
	dimension := "bench-pessimistic-lock"

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
		{
			message: "WithLimitersBackend",
			opts: []tokenbucket.Option{
				tokenbucket.WithLimitersBackend(provider.CreateBucketConfig(dimension), dimension, true),
				lockBackend,
			},
		},
	} {
		b.Run(tc.message, func(b *testing.B) {
			bucket, err := provider.CreateBucket(capacityForSingleDimension, refillRateForSingleDimension, dimension, tc.opts...)
			require.NoError(b, err, "Failed to create bucket: %v", err)

			ctx := context.Background()
			b.ResetTimer()
			b.SetParallelism(parallels)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					bucket.Take(ctx)
				}
			})
		})
	}
}
