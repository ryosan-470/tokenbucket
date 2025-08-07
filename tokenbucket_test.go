package tokenbucket_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/internal/testutils"
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
				name string
				repo tokenbucket.TokenBucketStateRepository
			}{
				{name: "DynamoDB", repo: ddbBackend},
				{name: "Memory", repo: memoryBackend},
			} {
				t.Run(tt.name, func(t *testing.T) {
					dimension := uuid.NewString()
					bucket, err := tokenbucket.NewBucket(capacity, fillRate, dimension, tt.repo)
					require.NoError(t, err)

					state, err := bucket.GetState(context.Background())
					require.NoError(t, err)
					assert.Equal(t, capacity, bucket.Capacity)
					assert.Equal(t, fillRate, bucket.FillRate)
					assert.Equal(t, dimension, bucket.Dimension)
					assert.Equal(t, int64(0), state.Available)
					assert.Equal(t, int64(0), state.LastUpdated)
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

			state, err := bucket.GetState(context.Background())
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.Equal(t, int64(0), state.Available)
			assert.Equal(t, int64(0), state.LastUpdated)
		})

		t.Run("ZeroCapacity", func(t *testing.T) {
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(0, fillRate, dimension, ddbBackend)
			require.NoError(t, err)

			state, err := bucket.GetState(context.Background())
			require.NoError(t, err)
			assert.Equal(t, int64(0), state.Available)
			assert.Equal(t, int64(0), bucket.Capacity)
			assert.Equal(t, fillRate, bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.Equal(t, int64(0), state.LastUpdated)
		})

		t.Run("ZeroFillRate", func(t *testing.T) {
			dimension := uuid.NewString()
			bucket, err := tokenbucket.NewBucket(capacity, 0, dimension, ddbBackend)
			require.NoError(t, err)

			state, err := bucket.GetState(context.Background())
			require.NoError(t, err)
			assert.Equal(t, capacity, bucket.Capacity)
			assert.Equal(t, int64(0), bucket.FillRate)
			assert.Equal(t, dimension, bucket.Dimension)
			assert.Equal(t, int64(0), state.Available)
			assert.Equal(t, int64(0), state.LastUpdated)
		})
	})

	t.Run("Take", func(t *testing.T) {
		ctx := context.Background()
		bucketTable := infra.CreateTokenBucketTable(t, uuid.NewString())
		defer infra.DeleteTable(t, bucketTable.TableName)

		bucketCfg := dynamodb.NewBucketBackendConfig(infra.Client, bucketTable.TableName)

		for _, tt := range []struct {
			name    string
			prepare func(ctx context.Context) (string, tokenbucket.TokenBucketStateRepository)
		}{
			// TODO: fix DynamoDB backend
			// {
			// 	name: "DynamoDB Backend",
			// 	prepare: func(ctx context.Context) (string, storage.Storage) {
			// 		dimension := uuid.NewString()
			// 		backend, err := bucketCfg.NewCustomBackend(ctx, dimension)
			// 		require.NoError(t, err)
			// 		return dimension, backend
			// 	},
			// },
			{
				name: "Memory Backend",
				prepare: func(ctx context.Context) (string, tokenbucket.TokenBucketStateRepository) {
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

					state, err := bucket.GetState(ctx)
					require.NoError(t, err)
					assert.Equal(t, int64(9), state.Available)
					assert.NotEqual(t, int64(0), state.LastUpdated)
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
					state, err := bucket.GetState(ctx)
					require.NoError(t, err)
					assert.Equal(t, int64(0), state.Available)

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
					state, err := bucket.GetState(ctx)
					require.NoError(t, err)
					assert.Equal(t, int64(0), state.Available)

					// Next take should fail
					assert.ErrorIs(t, bucket.Take(ctx), tokenbucket.ErrNoTokensAvailable)

					// Advance time by 1 second (should add 1 token)
					mockClock.Advance(1000 * time.Millisecond)

					// Now take should succeed
					require.NoError(t, bucket.Take(ctx))
					state, err = bucket.GetState(ctx)
					require.NoError(t, err)
					assert.Equal(t, int64(0), state.Available)
				})

				t.Run("ConcurrentTake", func(t *testing.T) {
					dimension, backend := tt.prepare(ctx)
					capacity := int64(10)
					bucket, err := tokenbucket.NewBucket(capacity, 1, dimension, backend)
					require.NoError(t, err)

					// Test concurrent takes equal to capacity
					numGoroutines := int(capacity)
					var wg sync.WaitGroup
					errors := make(chan error, numGoroutines)

					// Launch goroutines to take tokens concurrently
					for i := 0; i < numGoroutines; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							if err := bucket.Take(ctx); err != nil {
								errors <- err
							}
						}()
					}

					// Wait for all goroutines to complete
					wg.Wait()
					close(errors)

					// Check that no errors occurred
					for err := range errors {
						require.NoError(t, err)
					}

					// Verify final state - all tokens should be consumed
					state, err := bucket.GetState(ctx)
					require.NoError(t, err)
					assert.Equal(t, int64(0), state.Available)

					// Additional take should fail
					assert.ErrorIs(t, bucket.Take(ctx), tokenbucket.ErrNoTokensAvailable)
				})

				t.Run("ConcurrentTakeExceedsCapacity", func(t *testing.T) {
					dimension, backend := tt.prepare(ctx)
					capacity := int64(5)
					numGoroutines := 10 // More goroutines than capacity
					bucket, err := tokenbucket.NewBucket(capacity, 1, dimension, backend)
					require.NoError(t, err)

					var wg sync.WaitGroup
					successCount := int64(0)
					errorCount := int64(0)
					results := make(chan error, numGoroutines)

					// Launch more goroutines than available tokens
					for i := 0; i < numGoroutines; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							err := bucket.Take(ctx)
							results <- err
						}()
					}

					// Wait for all goroutines to complete
					wg.Wait()
					close(results)

					// Count successes and failures
					for result := range results {
						if result == nil {
							successCount++
						} else if errors.Is(result, tokenbucket.ErrNoTokensAvailable) {
							errorCount++
						} else {
							require.NoError(t, result) // Unexpected error
						}
					}

					// Verify that exactly 'capacity' operations succeeded
					assert.Equal(t, capacity, successCount, "Expected %d successful takes", capacity)
					assert.Equal(t, int64(numGoroutines)-capacity, errorCount, "Expected %d failed takes", numGoroutines-int(capacity))

					// Verify final state - all tokens should be consumed
					state, err := bucket.GetState(ctx)
					require.NoError(t, err)
					assert.Equal(t, int64(0), state.Available)
				})

				t.Run("ConcurrentTakeAndGet", func(t *testing.T) {
					dimension, backend := tt.prepare(ctx)
					capacity := int64(10)
					bucket, err := tokenbucket.NewBucket(capacity, 1, dimension, backend)
					require.NoError(t, err)

					numTakeGoroutines := 5
					numGetGoroutines := 5
					var wg sync.WaitGroup

					takeResults := make(chan error, numTakeGoroutines)
					getResults := make(chan error, numGetGoroutines)
					getValues := make(chan int64, numGetGoroutines)

					// Launch Take goroutines
					for i := 0; i < numTakeGoroutines; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							err := bucket.Take(ctx)
							takeResults <- err
						}()
					}

					// Launch Get goroutines
					for i := 0; i < numGetGoroutines; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							state, err := bucket.GetState(ctx)
							if err != nil {
								getResults <- err
								return
							}
							getValues <- state.Available
							getResults <- nil
						}()
					}

					// Wait for all goroutines to complete
					wg.Wait()
					close(takeResults)
					close(getResults)
					close(getValues)

					// Verify Take operations
					takeSuccessCount := 0
					takeErrorCount := 0
					for takeResult := range takeResults {
						if takeResult == nil {
							takeSuccessCount++
						} else if errors.Is(takeResult, tokenbucket.ErrNoTokensAvailable) {
							takeErrorCount++
						} else {
							require.NoError(t, takeResult) // Unexpected error
						}
					}

					// Verify Get operations - should all succeed without errors
					for getResult := range getResults {
						require.NoError(t, getResult)
					}

					// Verify that all Get values are within valid range
					for available := range getValues {
						assert.GreaterOrEqual(t, available, int64(0), "Available tokens should not be negative")
						assert.LessOrEqual(t, available, capacity, "Available tokens should not exceed capacity")
					}

					// Verify that exactly the expected number of Take operations succeeded
					assert.Equal(t, numTakeGoroutines, takeSuccessCount, "All Take operations should succeed with sufficient tokens")
					assert.Equal(t, 0, takeErrorCount, "No Take operations should fail with sufficient tokens")

					// Verify final state
					state, err := bucket.GetState(ctx)
					require.NoError(t, err)
					expectedRemaining := capacity - int64(numTakeGoroutines)
					assert.Equal(t, expectedRemaining, state.Available)
				})

				t.Run("ConcurrentWithTokenRefill", func(t *testing.T) {
					now := time.Now()
					mockClock := testutils.NewMockClock(now)
					dimension, backend := tt.prepare(ctx)
					capacity := int64(5)
					fillRate := int64(2) // 2 tokens per second
					bucket, err := tokenbucket.NewBucket(capacity, fillRate, dimension, backend, tokenbucket.WithClock(mockClock))
					require.NoError(t, err)

					// First, exhaust all tokens
					for i := 0; i < int(capacity); i++ {
						require.NoError(t, bucket.Take(ctx))
					}
					state, err := bucket.GetState(ctx)
					require.NoError(t, err)
					assert.Equal(t, int64(0), state.Available)

					// Advance time to add some tokens back
					mockClock.Advance(1500 * time.Millisecond) // 1.5 seconds = 3 tokens

					numGoroutines := 6 // More than available tokens (3)
					var wg sync.WaitGroup
					results := make(chan error, numGoroutines)

					// Launch goroutines concurrently to compete for the refilled tokens
					for i := 0; i < numGoroutines; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							err := bucket.Take(ctx)
							results <- err
						}()
					}

					// Wait for all goroutines to complete
					wg.Wait()
					close(results)

					// Count results
					successCount := 0
					errorCount := 0
					for result := range results {
						if result == nil {
							successCount++
						} else if errors.Is(result, tokenbucket.ErrNoTokensAvailable) {
							errorCount++
						} else {
							require.NoError(t, result) // Unexpected error
						}
					}

					// After 1.5 seconds with 2 tokens/sec rate, we should have 3 tokens available
					// So exactly 3 operations should succeed, 3 should fail
					assert.Equal(t, 3, successCount, "Exactly 3 takes should succeed after token refill")
					assert.Equal(t, 3, errorCount, "Exactly 3 takes should fail when tokens are exhausted")

					// Verify final state is consistent
					state, err = bucket.GetState(ctx)
					require.NoError(t, err)
					assert.Equal(t, int64(0), state.Available, "All refilled tokens should be consumed")
				})

				t.Run("HighConcurrencyStress", func(t *testing.T) {
					dimension, backend := tt.prepare(ctx)
					capacity := int64(20)
					bucket, err := tokenbucket.NewBucket(capacity, 1, dimension, backend)
					require.NoError(t, err)

					numGoroutines := 100 // High concurrency
					timeout := 10 * time.Second
					ctx, cancel := context.WithTimeout(ctx, timeout)
					defer cancel()

					var wg sync.WaitGroup
					takeResults := make(chan error, numGoroutines/2)
					getResults := make(chan error, numGoroutines/2)
					getValues := make(chan int64, numGoroutines/2)

					// Launch Take goroutines (50)
					for i := 0; i < numGoroutines/2; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							err := bucket.Take(ctx)
							takeResults <- err
						}()
					}

					// Launch Get goroutines (50)
					for i := 0; i < numGoroutines/2; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							state, err := bucket.GetState(ctx)
							if err != nil {
								getResults <- err
								return
							}
							getValues <- state.Available
							getResults <- nil
						}()
					}

					// Wait for all goroutines to complete with timeout protection
					done := make(chan bool)
					go func() {
						wg.Wait()
						done <- true
					}()

					select {
					case <-done:
						// All goroutines completed successfully
					case <-ctx.Done():
						t.Fatal("Test timed out - possible deadlock detected")
					}

					close(takeResults)
					close(getResults)
					close(getValues)

					// Count Take results
					takeSuccessCount := 0
					takeErrorCount := 0
					for takeResult := range takeResults {
						if takeResult == nil {
							takeSuccessCount++
						} else if errors.Is(takeResult, tokenbucket.ErrNoTokensAvailable) {
							takeErrorCount++
						} else {
							require.NoError(t, takeResult) // Unexpected error
						}
					}

					// Verify Get operations - should all succeed without errors
					for getResult := range getResults {
						require.NoError(t, getResult)
					}

					// Verify that all Get values are within valid range
					for available := range getValues {
						assert.GreaterOrEqual(t, available, int64(0), "Available tokens should not be negative")
						assert.LessOrEqual(t, available, capacity, "Available tokens should not exceed capacity")
					}

					// High-level consistency checks
					assert.Equal(t, numGoroutines/2, takeSuccessCount+takeErrorCount, "All Take operations should complete")
					assert.LessOrEqual(t, takeSuccessCount, int(capacity), "Success count should not exceed capacity")
					assert.GreaterOrEqual(t, takeSuccessCount, 1, "At least some Take operations should succeed")

					// Verify final state is consistent
					state, err := bucket.GetState(ctx)
					require.NoError(t, err)
					expectedRemaining := capacity - int64(takeSuccessCount)
					assert.Equal(t, expectedRemaining, state.Available, "Final state should be consistent")
					assert.GreaterOrEqual(t, state.Available, int64(0), "Available tokens should not be negative")
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
			prepare func(ctx context.Context) (string, tokenbucket.TokenBucketStateRepository)
		}{
			{
				name: "DynamoDB Backend",
				prepare: func(ctx context.Context) (string, tokenbucket.TokenBucketStateRepository) {
					dimension := uuid.NewString()
					backend, err := bucketCfg.NewCustomBackend(ctx, dimension)
					require.NoError(t, err)
					return dimension, backend
				},
			},
			{
				name: "Memory Backend",
				prepare: func(ctx context.Context) (string, tokenbucket.TokenBucketStateRepository) {
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

				state, err := bucket.GetState(ctx)
				require.NoError(t, err)
				assert.Equal(t, capacity, bucket.Capacity)
				assert.Equal(t, fillRate, bucket.FillRate)
				assert.Equal(t, dimension, bucket.Dimension)
				assert.Equal(t, int64(0), state.Available)
				assert.Equal(t, int64(0), state.LastUpdated)
			})
		}
	})
}
