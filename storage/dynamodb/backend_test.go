package dynamodb

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ryosan-470/tokenbucket/storage"
)

const (
	testBackendTableNamePrefix = "test-bucket-"
	testBackendPartitionKey    = "pk"
)

func TestBackend(t *testing.T) {
	infra := GetTestInfrastructure()

	t.Run("NewBackend", func(t *testing.T) {
		props := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, props.TableName)
		backend := NewBackend(infra.Client, testBackendPartitionKey, props, 1*time.Hour)
		require.NotNil(t, backend)
	})

	t.Run("State when item does not exist", func(t *testing.T) {
		props := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, props.TableName)
		backend := NewBackend(infra.Client, "non-existent-key", props, 1*time.Hour)
		state, err := backend.State(context.Background())
		require.NoError(t, err)
		assert.Equal(t, storage.State{}, state)
	})

	t.Run("SetState and State", func(t *testing.T) {
		ctx := context.Background()
		props := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, props.TableName)
		backend := NewBackend(infra.Client, testBackendPartitionKey, props, 1*time.Hour)

		initialState := storage.State{
			Last:      123,
			Available: 456,
		}

		err := backend.SetState(ctx, initialState)
		require.NoError(t, err)

		state, err := backend.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, initialState.Last, state.Last)
		assert.Equal(t, initialState.Available, state.Available)
	})

	t.Run("Reset", func(t *testing.T) {
		ctx := context.Background()
		props := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, props.TableName)
		backend := NewBackend(infra.Client, testBackendPartitionKey, props, 1*time.Hour)

		// Set some initial state
		initialState := storage.State{
			Last:      123,
			Available: 456,
		}
		err := backend.SetState(ctx, initialState)
		require.NoError(t, err)

		// Now reset it
		err = backend.Reset(ctx)
		require.NoError(t, err)

		// Verify state in DB is reset
		state, err := backend.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), state.Last)
		assert.Equal(t, int64(0), state.Available)
	})

	t.Run("SetState with optimistic locking", func(t *testing.T) {
		ctx := context.Background()
		props := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, props.TableName)
		// Create two backends pointing to the same item
		backend1 := NewBackend(infra.Client, testBackendPartitionKey, props, 1*time.Hour)
		backend2 := NewBackend(infra.Client, testBackendPartitionKey, props, 1*time.Hour)

		// Reset state to ensure a clean slate
		err := backend1.Reset(ctx)
		require.NoError(t, err)

		// Both backends read the same initial state (version 0)
		_, err = backend1.State(ctx)
		require.NoError(t, err)
		_, err = backend2.State(ctx)
		require.NoError(t, err)

		// Backend1 updates the state successfully
		err = backend1.SetState(ctx, storage.State{Last: 1, Available: 1})
		require.NoError(t, err)

		// Backend2 tries to update with an old version (0), which should fail
		err = backend2.SetState(ctx, storage.State{Last: 2, Available: 2})
		require.Error(t, err, "expected an error due to conditional check failure")

		// Verify the state in DB is still from backend1's update
		state, err := backend2.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), state.Last)
		assert.Equal(t, int64(1), state.Available)
	})

	t.Run("SetState concurrent", func(t *testing.T) {
		ctx := context.Background()
		props := infra.CreateTokenBucketTable(t, fmt.Sprintf("%s%s", testBackendTableNamePrefix, uuid.NewString()))
		defer infra.DeleteTable(t, props.TableName)

		// Reset state
		be := NewBackend(infra.Client, testBackendPartitionKey, props, 1*time.Hour)
		err := be.Reset(ctx)
		require.NoError(t, err)

		numGoroutines := 5
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		successCount := 0
		var mu sync.Mutex

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				// Each goroutine gets its own backend instance but points to the same item
				backend := NewBackend(infra.Client, testBackendPartitionKey, props, 1*time.Hour)
				// All load the same initial state
				_, err := backend.State(ctx)
				if err != nil {
					t.Logf("failed to get state: %v", err)
					return
				}

				// Try to update the state
				err = backend.SetState(ctx, storage.State{Last: 1, Available: 1})
				if err == nil {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}()
		}

		wg.Wait()
		assert.Equal(t, 1, successCount, "only one concurrent SetState should succeed")
	})
}
