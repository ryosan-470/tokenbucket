package tokenbucket_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/internal/testutils"
	"github.com/ryosan-470/tokenbucket/storage/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenBucket(t *testing.T) {
	infra := GetTestInfrastructure()

	t.Run("NewBucket", func(t *testing.T) {
		bucketTable := infra.CreateTokenBucketTable(t, uuid.NewString())
		defer infra.DeleteTable(t, bucketTable.TableName)
		lockTable := infra.CreateLockTable(t, uuid.NewString())
		defer infra.DeleteTable(t, lockTable.TableName)

		bucketCfg := dynamodb.NewBucketBackendConfig(infra.Client, bucketTable.TableName)
		lockCfg := dynamodb.NewLockBackendConfig(infra.Client, lockTable.TableName, 30*time.Second, 3, 5*time.Second)

		capacity := int64(1000)
		fillRate := int64(10)
		now := time.Now()

		t.Run("OK", func(t *testing.T) {
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(capacity, fillRate, dimension, bucketCfg)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.NotNil(t, bucket.LastUpdated)
		})

		t.Run("WithLockBackend", func(t *testing.T) {
			dimension := uuid.NewString()
			lockID := fmt.Sprintf("lock-%s", dimension)

			bucket, err := tokenbucket.NewBucket(
				capacity,
				fillRate,
				dimension,
				bucketCfg,
				tokenbucket.WithLockBackend(lockCfg, lockID),
			)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
		})

		t.Run("WithMemoryBackend", func(t *testing.T) {
			dimension := uuid.NewString()

			bucket, err := tokenbucket.NewBucket(
				capacity,
				fillRate,
				dimension,
				nil,
				tokenbucket.WithMemoryBackend(),
			)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
		})

		t.Run("WithClock", func(t *testing.T) {
			dimension := uuid.NewString()
			mockClock := testutils.NewMockClock(now)

			bucket, err := tokenbucket.NewBucket(
				capacity,
				fillRate,
				dimension,
				bucketCfg,
				tokenbucket.WithClock(mockClock),
			)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.NotNil(t, bucket)
		})

		t.Run("ZeroCapacity", func(t *testing.T) {
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(0, fillRate, dimension, bucketCfg)
			require.NoError(t, err)
			assert.Equal(t, int64(0), bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
		})

		t.Run("ZeroFillRate", func(t *testing.T) {
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(capacity, 0, dimension, bucketCfg)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, int64(0), bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
		})
	})

	t.Run("Take", func(t *testing.T) {
		ctx := context.Background()
		bucketTable := infra.CreateTokenBucketTable(t, uuid.NewString())
		defer infra.DeleteTable(t, bucketTable.TableName)

		bucketCfg := dynamodb.NewBucketBackendConfig(infra.Client, bucketTable.TableName)

		t.Run("SingleToken", func(t *testing.T) {
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(10, 1, dimension, bucketCfg)
			require.NoError(t, err)

			require.NoError(t, bucket.Take(ctx))
			require.NoError(t, bucket.Get(ctx))
			assert.Equal(t, int64(9), bucket.Available)
		})

		t.Run("ExhaustAllTokens", func(t *testing.T) {
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(3, 1, dimension, bucketCfg)
			require.NoError(t, err)

			// Take all 3 tokens
			for i := 0; i < 3; i++ {
				require.NoError(t, bucket.Take(ctx))
			}

			// Get state to check available tokens
			require.NoError(t, bucket.Get(ctx))
			assert.Equal(t, int64(0), bucket.Available)

			// Next attempt should fail
			require.Error(t, bucket.Take(ctx))
		})

		t.Run("WithTokenRefill", func(t *testing.T) {
			now := time.Now()
			mockClock := testutils.NewMockClock(now)
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(5, 1, dimension, bucketCfg, tokenbucket.WithClock(mockClock))
			require.NoError(t, err)

			// Take 5 tokens
			for i := 0; i < 5; i++ {
				require.NoError(t, bucket.Take(ctx))
			}
			// Get state to check available tokens
			require.NoError(t, bucket.Get(ctx))
			assert.Equal(t, int64(0), bucket.Available)

			// Next take should fail
			require.Error(t, bucket.Take(ctx))

			// Advance time by 1 second (should add 1 token)
			mockClock.Advance(1000 * time.Millisecond)

			// Now take should succeed
			require.NoError(t, bucket.Take(ctx))
			// Get state to check available tokens
			require.NoError(t, bucket.Get(ctx))
			assert.Equal(t, int64(0), bucket.Available)
		})

		t.Run("ContextCancellation", func(t *testing.T) {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel()

			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(10, 1, dimension, bucketCfg)
			require.NoError(t, err)
			require.Error(t, bucket.Take(cancelCtx))
		})
	})

	t.Run("Get", func(t *testing.T) {
		bucketTable := infra.CreateTokenBucketTable(t, uuid.NewString())
		defer infra.DeleteTable(t, bucketTable.TableName)

		bucketCfg := dynamodb.NewBucketBackendConfig(infra.Client, bucketTable.TableName)

		capacity := int64(1000)
		fillRate := int64(10)

		dimension := uuid.NewString()
		bucket, err := tokenbucket.NewBucket(capacity, fillRate, dimension, bucketCfg)
		require.NoError(t, err)

		t.Run("OK", func(t *testing.T) {
			require.NoError(t, bucket.Get(context.Background()))
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.NotNil(t, bucket.LastUpdated)
		})
	})
}
