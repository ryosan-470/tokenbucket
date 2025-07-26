package dynamodb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStorageLockTable(t *testing.T) {
	infra := GetTestInfrastructure()

	t.Run("BasicLockUnlock", func(t *testing.T) {
		tableName := "test-lock-table-basic"
		infra.CreateLockTable(t, tableName)
		defer infra.DeleteTable(t, tableName)

		lock := NewLock(infra.Client, tableName, "test-lock", 30*time.Second,
			WithBackoffMaxTries(3),
			WithBackoffMaxTime(5*time.Second))

		ctx := context.Background()

		// Test successful lock acquisition
		require.NoError(t, lock.Lock(ctx), "Failed to acquire lock")

		// Test successful unlock
		require.NoError(t, lock.Unlock(ctx), "Failed to release lock")
	})

	t.Run("ConcurrentLock", func(t *testing.T) {
		tableName := "test-lock-table-concurrent"
		infra.CreateLockTable(t, tableName)
		defer infra.DeleteTable(t, tableName)

		lock1 := NewLock(infra.Client, tableName, "test-lock", 30*time.Second,
			WithBackoffMaxTries(1),
			WithBackoffMaxTime(1*time.Second))
		lock2 := NewLock(infra.Client, tableName, "test-lock", 30*time.Second,
			WithBackoffMaxTries(1),
			WithBackoffMaxTime(1*time.Second))

		ctx := context.Background()

		// First lock should succeed
		require.NoError(t, lock1.Lock(ctx), "Failed to acquire first lock")

		// Second lock should fail due to contention
		err := lock2.Lock(ctx)
		require.Error(t, err, "Expected second lock to fail, but it succeeded")
		require.Equal(t, ErrLockAlreadyHeld, err, "Expected ErrLockAlreadyHeld")

		// Release first lock
		require.NoError(t, lock1.Unlock(ctx), "Failed to release first lock")

		// Now second lock should succeed
		require.NoError(t, lock2.Lock(ctx), "Failed to acquire second lock after first was released")

		// Clean up
		require.NoError(t, lock2.Unlock(ctx), "Failed to release second lock")
	})

	t.Run("TTLExpiration", func(t *testing.T) {
		tableName := "test-lock-table-ttl"
		infra.CreateLockTable(t, tableName)
		defer infra.DeleteTable(t, tableName)

		// Create lock with very short TTL
		lock := NewLock(infra.Client, tableName, "test-lock", 1*time.Second,
			WithBackoffMaxTries(3),
			WithBackoffMaxTime(5*time.Second))

		ctx := context.Background()

		// Acquire lock
		require.NoError(t, lock.Lock(ctx), "Failed to acquire lock")

		// Wait for TTL to expire (a bit longer than TTL)
		time.Sleep(3 * time.Second)

		// Create a new lock instance with the same ID
		newLock := NewLock(infra.Client, tableName, "test-lock", 30*time.Second,
			WithBackoffMaxTries(3),
			WithBackoffMaxTime(5*time.Second))

		// Should be able to acquire lock after TTL expiration
		require.NoError(t, newLock.Lock(ctx), "Failed to acquire lock after TTL expiration")

		// Clean up
		require.NoError(t, newLock.Unlock(ctx), "Failed to release lock")
	})

	t.Run("UnlockNonExistentLock", func(t *testing.T) {
		tableName := "test-lock-table-nonexistent"
		infra.CreateLockTable(t, tableName)
		defer infra.DeleteTable(t, tableName)

		lock := NewLock(infra.Client, tableName, "non-existent-lock", 30*time.Second,
			WithBackoffMaxTries(3),
			WithBackoffMaxTime(5*time.Second))

		ctx := context.Background()

		// Try to unlock a lock that was never acquired
		require.NoError(t, lock.Unlock(ctx), "Unexpected error when unlocking non-existent lock")
	})

	t.Run("UnlockWrongOwner", func(t *testing.T) {
		tableName := "test-lock-table-wrong-owner"
		infra.CreateLockTable(t, tableName)
		defer infra.DeleteTable(t, tableName)

		lock1 := NewLock(infra.Client, tableName, "test-lock", 30*time.Second,
			WithBackoffMaxTries(3),
			WithBackoffMaxTime(5*time.Second))
		lock2 := NewLock(infra.Client, tableName, "test-lock", 30*time.Second,
			WithBackoffMaxTries(3),
			WithBackoffMaxTime(5*time.Second))

		ctx := context.Background()

		// First lock acquires
		require.NoError(t, lock1.Lock(ctx), "Failed to acquire first lock")

		// Second lock tries to unlock (wrong owner)
		require.NoError(t, lock2.Unlock(ctx), "Unexpected error when unlocking with wrong owner")

		// First lock should still be able to unlock
		require.NoError(t, lock1.Unlock(ctx), "Failed to release lock with correct owner")
	})
}
