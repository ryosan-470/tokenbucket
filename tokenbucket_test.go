package tokenbucket_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/internal/testutils"
	"github.com/ryosan-470/tokenbucket/storage"
	"github.com/ryosan-470/tokenbucket/storage/dynamodb"
	"github.com/ryosan-470/tokenbucket/storage/memory"
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

		memoryBackend := memory.NewBackend()
		bucketCfg := dynamodb.NewBucketBackendConfig(infra.Client, bucketTable.TableName)
		ddbBackend, err := bucketCfg.NewCustomBackend(context.Background(), uuid.NewString())
		require.NoError(t, err)

		capacity := int64(1000)
		fillRate := int64(10)
		now := time.Now()

		t.Run("Backend", func(t *testing.T) {
			for _, tt := range []struct {
				name    string
				backend storage.Storage
			}{
				{name: "DynamoDB", backend: ddbBackend},
				{name: "Memory", backend: memoryBackend},
			} {
				t.Run(tt.name, func(t *testing.T) {
					dimension := uuid.NewString()
					bucket, err := tokenbucket.NewBucket(capacity, fillRate, dimension, tt.backend)
					require.NoError(t, err)
					assert.Equal(t, capacity, bucket.Available)
					assert.Equal(t, capacity, bucket.Capacity)
					assert.Equal(t, fillRate, bucket.FillRate)
					assert.Equal(t, dimension, bucket.Dimension)
					assert.Equal(t, int64(0), bucket.LastUpdated)
				})
			}
		})

		t.Run("WithClock", func(t *testing.T) {
			dimension := uuid.NewString()
			mockClock := testutils.NewMockClock(now)

			bucket, err := tokenbucket.NewBucket(
				capacity,
				fillRate,
				dimension,
				ddbBackend,
				tokenbucket.WithClock(mockClock),
			)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Available)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.Equal(t, int64(0), bucket.LastUpdated)
		})

		t.Run("ZeroCapacity", func(t *testing.T) {
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(0, fillRate, dimension, ddbBackend)
			require.NoError(t, err)
			assert.Equal(t, int64(0), bucket.Available)
			assert.Equal(t, int64(0), bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.Equal(t, int64(0), bucket.LastUpdated)
		})

		t.Run("ZeroFillRate", func(t *testing.T) {
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(capacity, 0, dimension, ddbBackend)
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Available)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, int64(0), bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.Equal(t, int64(0), bucket.LastUpdated)
		})
	})

	t.Run("Take", func(t *testing.T) {
		ctx := context.Background()
		bucketTable := infra.CreateTokenBucketTable(t, uuid.NewString())
		defer infra.DeleteTable(t, bucketTable.TableName)

		bucketCfg := dynamodb.NewBucketBackendConfig(infra.Client, bucketTable.TableName)

		for _, tt := range []struct {
			name    string
			prepare func(ctx context.Context) (string, storage.Storage)
		}{
			{
				name: "DynamoDB Backend",
				prepare: func(ctx context.Context) (string, storage.Storage) {
					dimension := uuid.NewString()
					backend, err := bucketCfg.NewCustomBackend(ctx, dimension)
					require.NoError(t, err)
					return dimension, backend
				},
			},
			{
				name: "Memory Backend",
				prepare: func(ctx context.Context) (string, storage.Storage) {
					dimension := uuid.NewString()
					backend := memory.NewBackend()
					return dimension, backend
				},
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Run("Take 1 token", func(t *testing.T) {
					dimension, backend := tt.prepare(ctx)
					bucket, err := tokenbucket.NewBucket(10, 1, dimension, backend)
					require.NoError(t, err)

					require.NoError(t, bucket.Take(ctx))
					assert.Equal(t, int64(9), bucket.Available)
					assert.NotEqual(t, int64(0), bucket.LastUpdated)
				})

				t.Run("ExhaustAllTokens", func(t *testing.T) {
					dimension, backend := tt.prepare(ctx)
					bucket, err := tokenbucket.NewBucket(3, 1, dimension, backend)
					require.NoError(t, err)

					// Take all 3 tokens
					for i := 0; i < 3; i++ {
						require.NoError(t, bucket.Take(ctx))
					}

					// Get state to check available tokens
					assert.Equal(t, int64(0), bucket.Available)

					// Next attempt should fail
					assert.ErrorIs(t, bucket.Take(ctx), tokenbucket.ErrNoTokensAvailable)
				})

				t.Run("WithTokenRefill", func(t *testing.T) {
					now := time.Now()
					mockClock := testutils.NewMockClock(now)
					dimension, backend := tt.prepare(ctx)
					bucket, err := tokenbucket.NewBucket(5, 1, dimension, backend, tokenbucket.WithClock(mockClock))
					require.NoError(t, err)

					// Take 5 tokens
					for i := 0; i < 5; i++ {
						require.NoError(t, bucket.Take(ctx))
					}
					// Get state to check available tokens
					assert.Equal(t, int64(0), bucket.Available)

					// Next take should fail
					assert.ErrorIs(t, bucket.Take(ctx), tokenbucket.ErrNoTokensAvailable)

					// Advance time by 1 second (should add 1 token)
					mockClock.Advance(1000 * time.Millisecond)

					// Now take should succeed
					require.NoError(t, bucket.Take(ctx))
					assert.Equal(t, int64(0), bucket.Available)
				})
			})
		}

		t.Run("ContextCancellation with DynamoDB Backend", func(t *testing.T) {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel()

			dimension := uuid.NewString()
			backend, err := bucketCfg.NewCustomBackend(context.Background(), dimension)
			require.NoError(t, err)
			bucket, err := tokenbucket.NewBucket(10, 1, dimension, backend)
			require.NoError(t, err)
			err = bucket.Take(cancelCtx)
			require.Error(t, err)
		})
	})

	t.Run("Get", func(t *testing.T) {
		ctx := context.Background()
		bucketTable := infra.CreateTokenBucketTable(t, uuid.NewString())
		defer infra.DeleteTable(t, bucketTable.TableName)

		bucketCfg := dynamodb.NewBucketBackendConfig(infra.Client, bucketTable.TableName)

		capacity := int64(1000)
		fillRate := int64(10)

		for _, tt := range []struct {
			name    string
			prepare func(ctx context.Context) (string, storage.Storage)
		}{
			{
				name: "DynamoDB Backend",
				prepare: func(ctx context.Context) (string, storage.Storage) {
					dimension := uuid.NewString()
					backend, err := bucketCfg.NewCustomBackend(ctx, dimension)
					require.NoError(t, err)
					return dimension, backend
				},
			},
			{
				name: "Memory Backend",
				prepare: func(ctx context.Context) (string, storage.Storage) {
					dimension := uuid.NewString()
					backend := memory.NewBackend()
					return dimension, backend
				},
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				dimension, backend := tt.prepare(ctx)
				bucket, err := tokenbucket.NewBucket(capacity, fillRate, dimension, backend)
				require.NoError(t, err)

				require.NoError(t, bucket.Get(ctx))
				assert.Equal(t, capacity, bucket.Capacity)
				assert.Equal(t, fillRate, bucket.FillRate)
				assert.Equal(t, dimension, bucket.Dimension)
				assert.Equal(t, int64(0), bucket.LastUpdated)
			})
		}
	})
}
