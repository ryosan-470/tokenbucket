package dynamodb

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/testcontainers/testcontainers-go"
	dynamodbcontainer "github.com/testcontainers/testcontainers-go/modules/dynamodb"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupDynamoDBLocal(t *testing.T) (*dynamodb.Client, func()) {
	ctx := context.Background()

	// Create wait strategy focusing on port availability and service readiness
	waitStrategy := wait.ForExposedPort().
		WithStartupTimeout(2 * time.Minute).
		WithPollInterval(1 * time.Second)

	container, err := dynamodbcontainer.Run(ctx, "amazon/dynamodb-local:latest",
		testcontainers.WithWaitStrategy(waitStrategy),
	)
	if err != nil {
		t.Fatalf("Failed to start DynamoDB Local container: %v", err)
	}

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("Failed to get DynamoDB Local endpoint: %v", err)
	}

	// Add http:// prefix to endpoint
	endpointURL := "http://" + endpoint

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     "test",
				SecretAccessKey: "test",
			}, nil
		})),
	)
	if err != nil {
		t.Fatalf("Failed to load AWS config: %v", err)
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
	})

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate DynamoDB Local container: %v", err)
		}
	}

	return client, cleanup
}

func createLockTable(t *testing.T, client *dynamodb.Client, tableName string) {
	ctx := context.Background()

	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String(attributeNameLockID),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String(attributeNameLockID),
				KeyType:       types.KeyTypeHash,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		t.Fatalf("Failed to create lock table: %v", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 2*time.Minute); err != nil {
		t.Fatalf("Failed to wait for table to be active: %v", err)
	}

	// Enable TTL on the table
	_, err = client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String(attributeNameTTL),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		t.Fatalf("Failed to enable TTL on table: %v", err)
	}
}

func TestLock_BasicLockUnlock(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	tableName := "test-lock-table"
	createLockTable(t, client, tableName)

	lock := NewLock(client, tableName, "test-lock", 30*time.Second, 3, 5*time.Second)

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
}

func TestLock_ConcurrentLock(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	tableName := "test-lock-table"
	createLockTable(t, client, tableName)

	lock1 := NewLock(client, tableName, "test-lock", 30*time.Second, 1, 1*time.Second)
	lock2 := NewLock(client, tableName, "test-lock", 30*time.Second, 1, 1*time.Second)

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
}

func TestLock_TTLExpiration(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	tableName := "test-lock-table"
	createLockTable(t, client, tableName)

	// Create lock with very short TTL
	lock := NewLock(client, tableName, "test-lock", 1*time.Second, 3, 5*time.Second)

	ctx := context.Background()

	// Acquire lock
	err := lock.Lock(ctx)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Wait for TTL to expire (a bit longer than TTL)
	time.Sleep(3 * time.Second)

	// Create a new lock instance with the same ID
	newLock := NewLock(client, tableName, "test-lock", 30*time.Second, 3, 5*time.Second)

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
}

func TestLock_UnlockNonExistentLock(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	tableName := "test-lock-table"
	createLockTable(t, client, tableName)

	lock := NewLock(client, tableName, "non-existent-lock", 30*time.Second, 3, 5*time.Second)

	ctx := context.Background()

	// Try to unlock a lock that was never acquired
	err := lock.Unlock(ctx)
	// Should not return an error (graceful handling)
	if err != nil {
		t.Fatalf("Unexpected error when unlocking non-existent lock: %v", err)
	}
}

func TestLock_UnlockWrongOwner(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	tableName := "test-lock-table"
	createLockTable(t, client, tableName)

	lock1 := NewLock(client, tableName, "test-lock", 30*time.Second, 3, 5*time.Second)
	lock2 := NewLock(client, tableName, "test-lock", 30*time.Second, 3, 5*time.Second)

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
}
