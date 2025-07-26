package dynamodb

import (
	"context"
	"testing"
	"time"
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
		err := lock.Lock(ctx)
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}

		// Test successful unlock
		err = lock.Unlock(ctx)
		if err != nil {
			t.Fatalf("Failed to release lock: %v", err)
		}
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
		err := lock1.Lock(ctx)
		if err != nil {
			t.Fatalf("Failed to acquire first lock: %v", err)
		}

		// Second lock should fail due to contention
		err = lock2.Lock(ctx)
		if err == nil {
			t.Fatal("Expected second lock to fail, but it succeeded")
		}

		if err != ErrLockAlreadyHeld {
			t.Fatalf("Expected ErrLockAlreadyHeld, got: %v", err)
		}

		// Release first lock
		err = lock1.Unlock(ctx)
		if err != nil {
			t.Fatalf("Failed to release first lock: %v", err)
		}

		// Now second lock should succeed
		err = lock2.Lock(ctx)
		if err != nil {
			t.Fatalf("Failed to acquire second lock after first was released: %v", err)
		}

		// Clean up
		err = lock2.Unlock(ctx)
		if err != nil {
			t.Fatalf("Failed to release second lock: %v", err)
		}
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
		err := lock.Lock(ctx)
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}

		// Wait for TTL to expire (a bit longer than TTL)
		time.Sleep(3 * time.Second)

		// Create a new lock instance with the same ID
		newLock := NewLock(infra.Client, tableName, "test-lock", 30*time.Second,
			WithBackoffMaxTries(3),
			WithBackoffMaxTime(5*time.Second))

		// Should be able to acquire lock after TTL expiration
		err = newLock.Lock(ctx)
		if err != nil {
			t.Fatalf("Failed to acquire lock after TTL expiration: %v", err)
		}

		// Clean up
		err = newLock.Unlock(ctx)
		if err != nil {
			t.Fatalf("Failed to release lock: %v", err)
		}
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
		err := lock.Unlock(ctx)
		// Should not return an error (graceful handling)
		if err != nil {
			t.Fatalf("Unexpected error when unlocking non-existent lock: %v", err)
		}
	})

	t.Run("UnlockWrongOwner", func(t *testing.T) {
		tableName := "test-lock-table-wrongowner"
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
		err := lock1.Lock(ctx)
		if err != nil {
			t.Fatalf("Failed to acquire first lock: %v", err)
		}

		// Second lock tries to unlock (wrong owner)
		err = lock2.Unlock(ctx)
		// Should not return an error (graceful handling)
		if err != nil {
			t.Fatalf("Unexpected error when unlocking with wrong owner: %v", err)
		}

		// First lock should still be able to unlock
		err = lock1.Unlock(ctx)
		if err != nil {
			t.Fatalf("Failed to release lock with correct owner: %v", err)
		}
	})
}
