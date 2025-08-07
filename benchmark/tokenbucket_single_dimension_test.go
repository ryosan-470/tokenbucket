// This test case benchmarks the performance of a single dimension token bucket.
// The bucket has a capacity of 1000 tokens and a refill rate of 100 tokens per second.
package benchmark

import (
	"context"
	"testing"

	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/benchmark/storage"
	"github.com/ryosan-470/tokenbucket/storage/memory"
	"github.com/stretchr/testify/require"
)

const (
	capacityForSingleDimension   = 1000
	refillRateForSingleDimension = 100

	parallels = 10
)

// BenchmarkSingleDimension tests the performance of a single dimension token bucket
func BenchmarkSingleDimension(b *testing.B) {
	provider := storage.BenchmarkSetup(b)

	for _, tt := range []struct {
		name   string
		bucket func() *tokenbucket.Bucket
	}{
		{
			name: "WithMemoryBackend",
			bucket: func() *tokenbucket.Bucket {
				backend := memory.NewBackend()
				bucket, err := provider.CreateBucket(capacityForSingleDimension, refillRateForSingleDimension, "bench-with-memory-backend", backend)
				require.NoError(b, err, "Failed to create bucket: %v", err)
				return bucket
			},
		},
		{
			name: "WithDynamoDBBackend",
			bucket: func() *tokenbucket.Bucket {
				ddbCfg := provider.CreateBucketConfig("bench-with-dynamodb-backend")
				backend, err := ddbCfg.NewCustomBackend(context.Background(), "bench-with-dynamodb-backend")
				require.NoError(b, err, "Failed to create backend: %v", err)
				bucket, err := provider.CreateBucket(capacityForSingleDimension, refillRateForSingleDimension, "bench-with-dynamodb-backend", backend)
				require.NoError(b, err, "Failed to create bucket: %v", err)
				return bucket
			},
		},
	} {
		b.Run(tt.name, func(b *testing.B) {
			ctx := context.Background()
			bucket := tt.bucket()
			b.ResetTimer()

			b.SetParallelism(parallels)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = bucket.Take(ctx)
				}
			})
		})
	}
}
