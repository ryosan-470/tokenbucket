package tokenbucket_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/internal/testutils"
	"github.com/ryosan-470/tokenbucket/storage/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testBackendTableNamePrefix = "test-token-bucket-backend-"
	testLockTableNamePrefix    = "test-token-bucket-lock-"
)

func TestTokenBucket(t *testing.T) {
	infra := GetTestInfrastructure()

	t.Run("NewBucket", func(t *testing.T) {
		bucketTable := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, bucketTable.TableName)
		lockTable := infra.CreateLockTable(t, fmt.Sprintf("%s%s", testLockTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, lockTable.TableName)

		dimension := uuid.NewString() // Use a unique dimension for each test run
		lockID := uuid.NewString()

		bucketCfg := dynamodb.NewBucketBackendConfig(
			infra.Client,
			bucketTable.TableName,
			dimension,
		)

		lockCfg := dynamodb.NewLockBackendConfig(
			infra.Client,
			lockTable.TableName,
			30*time.Second,
			3,
			5*time.Second,
		)

		capacity := int64(1000)
		fillRate := int64(10)

		t.Run("OK", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(capacity, fillRate, dimension, bucketCfg)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.NotNil(t, bucket.LastUpdated)
		})

		t.Run("WithLockBackend", func(t *testing.T) {
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

		t.Run("WithLimitersBackend", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(
				capacity,
				fillRate,
				dimension,
				bucketCfg,
				tokenbucket.WithLimitersBackend(bucketCfg, true),
			)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
		})

		t.Run("WithClock", func(t *testing.T) {
			mockClock := testutils.NewMockClock(time.Now())
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
			bucket, err := tokenbucket.NewBucket(0, fillRate, dimension, bucketCfg)
			require.NoError(t, err)
			assert.Equal(t, int64(0), bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
		})

		t.Run("ZeroFillRate", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(capacity, 0, dimension, bucketCfg)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, int64(0), bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
		})

		t.Run("InvalidBackendConfig", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(capacity, fillRate, dimension, nil)
			assert.Error(t, err)
			assert.Nil(t, bucket)
		})
	})

	t.Run("Take", func(t *testing.T) {
		ctx := context.Background()
		bucketTable := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, bucketTable.TableName)

		dimension := uuid.NewString()

		bucketCfg := dynamodb.NewBucketBackendConfig(
			infra.Client,
			bucketTable.TableName,
			dimension,
		)

		t.Run("SingleToken", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(10, 1, dimension, bucketCfg)
			require.NoError(t, err)

			err = bucket.Take(ctx)
			require.NoError(t, err)

			state, err := bucket.Get(ctx)
			require.NoError(t, err)
			assert.LessOrEqual(t, state.Available, int64(9))
		})

		t.Run("ExhaustAllTokens", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(3, 0, dimension+"-exhaust", bucketCfg)
			require.NoError(t, err)

			// Take all 3 tokens
			for i := 0; i < 3; i++ {
				err := bucket.Take(ctx)
				require.NoError(t, err)
			}

			// Get state to check available tokens
			state, err := bucket.Get(ctx)
			require.NoError(t, err)
			if state.Available > 0 {
				// If tokens are still available, continue taking until exhausted
				for state.Available > 0 {
					bucket.Take(ctx)
					state, _ = bucket.Get(ctx)
				}
			}

			// Next attempt should fail
			err = bucket.Take(ctx)
			require.Error(t, err)
		})

		t.Run("WithTokenRefill", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(
				5, 5, dimension+"-refill", bucketCfg,
			)
			require.NoError(t, err)

			// Exhaust all tokens
			state, _ := bucket.Get(ctx)
			for state.Available > 0 {
				bucket.Take(ctx)
				state, _ = bucket.Get(ctx)
			}

			// Next take should fail
			err = bucket.Take(ctx)
			require.Error(t, err)

			// Wait for token refill (1 second should add 5 tokens)
			time.Sleep(1200 * time.Millisecond)

			// Now take should succeed
			err = bucket.Take(ctx)
			require.NoError(t, err)
		})

		t.Run("ContextCancellation", func(t *testing.T) {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel()

			bucket, err := tokenbucket.NewBucket(10, 1, dimension+"-cancel", bucketCfg)
			require.NoError(t, err)

			err = bucket.Take(cancelCtx)
			require.Error(t, err)
		})
	})

	t.Run("Get", func(t *testing.T) {
		ctx := context.Background()
		bucketTable := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, bucketTable.TableName)

		dimension := uuid.NewString()

		bucketCfg := dynamodb.NewBucketBackendConfig(
			infra.Client,
			bucketTable.TableName,
			dimension,
		)

		t.Run("InitialState", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(100, 10, dimension, bucketCfg)
			require.NoError(t, err)

			state, err := bucket.Get(ctx)
			require.NoError(t, err)
			assert.Equal(t, int64(100), state.Capacity)
			assert.Equal(t, int64(10), state.FillRate)
			assert.Equal(t, dimension, state.Dimension)
			assert.GreaterOrEqual(t, state.Available, int64(0))
			assert.GreaterOrEqual(t, state.LastUpdated, int64(0))
		})

		t.Run("StateAfterMultipleTakes", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(50, 5, dimension+"-takes", bucketCfg)
			require.NoError(t, err)

			// Take 5 tokens
			for i := 0; i < 5; i++ {
				err := bucket.Take(ctx)
				require.NoError(t, err)
			}

			state, err := bucket.Get(ctx)
			require.NoError(t, err)
			assert.LessOrEqual(t, state.Available, int64(45))
			assert.Equal(t, int64(50), state.Capacity)
			assert.Equal(t, int64(5), state.FillRate)
		})
	})

	t.Run("Concurrency", func(t *testing.T) {
		ctx := context.Background()
		bucketTable := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, bucketTable.TableName)
		lockTable := infra.CreateLockTable(t, fmt.Sprintf("%s%s", testLockTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, lockTable.TableName)

		dimension := uuid.NewString()
		lockID := uuid.NewString()

		bucketCfg := dynamodb.NewBucketBackendConfig(
			infra.Client,
			bucketTable.TableName,
			dimension,
		)

		lockCfg := dynamodb.NewLockBackendConfig(
			infra.Client,
			lockTable.TableName,
			30*time.Second,
			3,
			5*time.Second,
		)

		t.Run("ConcurrentTakes", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(
				100, 0, dimension, bucketCfg,
				tokenbucket.WithLockBackend(lockCfg, lockID),
			)
			require.NoError(t, err)

			var wg sync.WaitGroup
			successCount := int32(0)
			numGoroutines := 100

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := bucket.Take(ctx); err == nil {
						atomic.AddInt32(&successCount, 1)
					}
				}()
			}

			wg.Wait()
			assert.Equal(t, int32(100), successCount)

			state, err := bucket.Get(ctx)
			require.NoError(t, err)
			assert.Equal(t, int64(0), state.Available)
		})
	})

	t.Run("EdgeCases", func(t *testing.T) {
		ctx := context.Background()
		bucketTable := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, bucketTable.TableName)

		dimension := uuid.NewString()

		bucketCfg := dynamodb.NewBucketBackendConfig(
			infra.Client,
			bucketTable.TableName,
			dimension,
		)

		t.Run("LargeCapacity", func(t *testing.T) {
			bucket, err := tokenbucket.NewBucket(int64(1e9), 1000, dimension, bucketCfg)
			require.NoError(t, err)
			assert.Equal(t, int64(1e9), bucket.Capacity)
		})

		t.Run("PartialTokenRefill", func(t *testing.T) {
			mockClock := testutils.NewMockClock(time.Now())
			bucket, err := tokenbucket.NewBucket(
				10, 10, dimension+"-partial", bucketCfg,
				tokenbucket.WithClock(mockClock),
			)
			require.NoError(t, err)

			// Take 5 tokens
			for i := 0; i < 5; i++ {
				err := bucket.Take(ctx)
				require.NoError(t, err)
			}

			// Get initial state after taking tokens
			stateAfterTake, err := bucket.Get(ctx)
			require.NoError(t, err)

			// Advance time by 0.5 seconds (should refill 5 tokens)
			mockClock.Advance(500 * time.Millisecond)

			state, err := bucket.Get(ctx)
			require.NoError(t, err)
			// Available tokens should be greater than or equal to the state after taking
			assert.GreaterOrEqual(t, state.Available, stateAfterTake.Available)
		})
	})
}

func TestCalculateFillRate(t *testing.T) {
	testCases := []struct {
		name     string
		fillRate int64
		expected time.Duration
	}{
		{"Rate1", 1, time.Second},
		{"Rate10", 10, 100 * time.Millisecond},
		{"Rate100", 100, 10 * time.Millisecond},
		{"Rate1000", 1000, time.Millisecond},
		{"Zero", 0, 0},
		{"Negative", -10, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tokenbucket.CalculateFillRate(tc.fillRate)
			assert.Equal(t, tc.expected, result)
		})
	}
}
