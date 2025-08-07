// This test case benchmarks the performance of a multiple dimension token bucket.
// There are 10 dimensions, each with a capacity of 200 tokens and a refill rate of 20 tokens per second.
package benchmark

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/ryosan-470/tokenbucket"
	benchmarkstorage "github.com/ryosan-470/tokenbucket/benchmark/storage"
	"github.com/ryosan-470/tokenbucket/storage/memory"
	"github.com/stretchr/testify/require"
)

const (
	dimensions = 10

	capacityForMultipleDimension   = 200
	refillRateForMultipleDimension = 20
)

// BenchmarkMultipleDimension tests the performance of a multiple dimension token bucket
func BenchmarkMultipleDimension(b *testing.B) {
	provider := benchmarkstorage.BenchmarkSetup(b)

	for _, tt := range []struct {
		name    string
		buckets func() []*tokenbucket.Bucket
	}{
		{
			name: "WithMemoryBackend",
			buckets: func() []*tokenbucket.Bucket {
				buckets := make([]*tokenbucket.Bucket, dimensions)
				for i := 0; i < dimensions; i++ {
					dimension := fmt.Sprintf("bench-multiple-dimension-%d", i)
					backend := memory.NewBackend()
					bucket, err := provider.CreateBucket(capacityForMultipleDimension, refillRateForMultipleDimension, dimension, backend)
					require.NoError(b, err, "Failed to create bucket: %v", err)
					buckets[i] = bucket
				}
				return buckets
			},
		},
		{
			name: "WithDynamoDBBackend",
			buckets: func() []*tokenbucket.Bucket {
				buckets := make([]*tokenbucket.Bucket, dimensions)
				for i := 0; i < dimensions; i++ {
					dimension := fmt.Sprintf("bench-multiple-dimension-%d", i)
					ddbCfg := provider.CreateBucketConfig(dimension)
					backend, err := ddbCfg.NewCustomBackend(context.Background(), dimension)
					require.NoError(b, err, "Failed to create backend: %v", err)
					bucket, err := provider.CreateBucket(capacityForMultipleDimension, refillRateForMultipleDimension, dimension, backend)
					require.NoError(b, err, "Failed to create bucket: %v", err)
					buckets[i] = bucket
				}
				return buckets
			},
		},
	} {
		b.Run(tt.name, func(b *testing.B) {
			ctx := context.Background()
			buckets := tt.buckets()
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
		})
	}
}
