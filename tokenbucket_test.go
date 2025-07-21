package tokenbucket

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

	dynamodbstorage "github.com/ryosan-470/tokenbucket/storage/dynamodb"
)

func setupDynamoDBLocal(t *testing.T) (*dynamodb.Client, func()) {
	ctx := context.Background()

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

func createTokenBucketTable(t *testing.T, client *dynamodb.Client, tableName string) {
	ctx := context.Background()

	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("PK"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("SK"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("PK"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("SK"),
				KeyType:       types.KeyTypeRange,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		t.Fatalf("Failed to create token bucket table: %v", err)
	}

	waiter := dynamodb.NewTableExistsWaiter(client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 2*time.Minute); err != nil {
		t.Fatalf("Failed to wait for table to be active: %v", err)
	}

	_, err = client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("_TTL"),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		t.Fatalf("Failed to enable TTL on token bucket table: %v", err)
	}
}

func createLockTable(t *testing.T, client *dynamodb.Client, tableName string) {
	ctx := context.Background()

	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("LockID"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("LockID"),
				KeyType:       types.KeyTypeHash,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		t.Fatalf("Failed to create lock table: %v", err)
	}

	waiter := dynamodb.NewTableExistsWaiter(client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 2*time.Minute); err != nil {
		t.Fatalf("Failed to wait for lock table to be active: %v", err)
	}

	_, err = client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("TTL"),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		t.Fatalf("Failed to enable TTL on lock table: %v", err)
	}
}

func TestTokenBucket_BasicOperations(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	bucketTableName := "test-token-bucket"
	lockTableName := "test-lock-table"

	createTokenBucketTable(t, client, bucketTableName)
	createLockTable(t, client, lockTableName)

	cfg := dynamodbstorage.NewBucketBackendConfig(
		client,
		bucketTableName,
		"test-dimension",
		lockTableName,
		30*time.Second,
		3,
		5*time.Second,
	)

	bucket, err := NewBucket(10, 1, "test-dimension", cfg)
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	ctx := context.Background()

	// Test initial state
	state, err := bucket.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get bucket state: %v", err)
	}

	if state.Capacity != 10 {
		t.Errorf("Expected capacity 10, got %d", state.Capacity)
	}

	if state.FillRate != 1 {
		t.Errorf("Expected fill rate 1, got %d", state.FillRate)
	}

	if state.Dimension != "test-dimension" {
		t.Errorf("Expected dimension 'test-dimension', got %s", state.Dimension)
	}

	// Test taking tokens
	for i := 0; i < 5; i++ {
		if err := bucket.Take(ctx); err != nil {
			t.Fatalf("Failed to take token %d: %v", i, err)
		}
	}

	// Check state after taking tokens
	state, err = bucket.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get bucket state after taking tokens: %v", err)
	}

	if state.Available != 5 {
		t.Errorf("Expected 5 available tokens, got %d", state.Available)
	}
}

func TestTokenBucket_RateLimiting(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	bucketTableName := "test-token-bucket-rate"
	lockTableName := "test-lock-table-rate"

	createTokenBucketTable(t, client, bucketTableName)
	createLockTable(t, client, lockTableName)

	cfg := dynamodbstorage.NewBucketBackendConfig(
		client,
		bucketTableName,
		"test-rate-limit",
		lockTableName,
		30*time.Second,
		3,
		5*time.Second,
	)

	// Create bucket with capacity 3 and fill rate 1 token/second
	bucket, err := NewBucket(3, 1, "test-rate-limit", cfg)
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	ctx := context.Background()

	// Exhaust all tokens
	for i := 0; i < 3; i++ {
		if err := bucket.Take(ctx); err != nil {
			t.Fatalf("Failed to take token %d: %v", i, err)
		}
	}

	// Next take should fail (rate limited)
	if err := bucket.Take(ctx); err == nil {
		t.Fatal("Expected rate limiting, but take succeeded")
	}

	// Check that bucket state shows 0 available tokens
	state, err := bucket.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get bucket state: %v", err)
	}

	if state.Available != 0 {
		t.Errorf("Expected 0 available tokens after exhaustion, got %d", state.Available)
	}

	// Wait for refill and test that bucket can provide tokens again
	time.Sleep(1500 * time.Millisecond) // Wait a bit more than 1 second

	// Should be able to take at least 1 token now (possibly more due to refill)
	if err := bucket.Take(ctx); err != nil {
		t.Fatalf("Failed to take token after refill: %v", err)
	}

	// Verify that state is being tracked correctly
	state, err = bucket.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get bucket state after refill: %v", err)
	}

	if state.Available < 0 || state.Available > 3 {
		t.Errorf("Expected available tokens to be between 0 and 3, got %d", state.Available)
	}
}

func TestTokenBucket_DistributedCoordination(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	bucketTableName := "test-token-bucket-distributed"
	lockTableName := "test-lock-table-distributed"

	createTokenBucketTable(t, client, bucketTableName)
	createLockTable(t, client, lockTableName)

	cfg := dynamodbstorage.NewBucketBackendConfig(
		client,
		bucketTableName,
		"distributed-test",
		lockTableName,
		30*time.Second,
		3,
		5*time.Second,
	)

	// Create two bucket instances with same dimension
	bucket1, err := NewBucket(5, 1, "distributed-test", cfg)
	if err != nil {
		t.Fatalf("Failed to create bucket1: %v", err)
	}

	bucket2, err := NewBucket(5, 1, "distributed-test", cfg)
	if err != nil {
		t.Fatalf("Failed to create bucket2: %v", err)
	}

	ctx := context.Background()

	// Take 3 tokens from bucket1
	for i := 0; i < 3; i++ {
		if err := bucket1.Take(ctx); err != nil {
			t.Fatalf("Failed to take token from bucket1: %v", err)
		}
	}

	// Check state from bucket2 should reflect bucket1's consumption
	state, err := bucket2.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get state from bucket2: %v", err)
	}

	if state.Available != 2 {
		t.Errorf("Expected 2 available tokens in bucket2, got %d", state.Available)
	}

	// Take remaining 2 tokens from bucket2
	for i := 0; i < 2; i++ {
		if err := bucket2.Take(ctx); err != nil {
			t.Fatalf("Failed to take remaining token from bucket2: %v", err)
		}
	}

	// Both buckets should now be empty
	if err := bucket1.Take(ctx); err == nil {
		t.Fatal("Expected bucket1 to be empty, but take succeeded")
	}

	if err := bucket2.Take(ctx); err == nil {
		t.Fatal("Expected bucket2 to be empty, but take succeeded")
	}
}

func TestTokenBucket_WithoutLock(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	bucketTableName := "test-token-bucket-nolock"
	lockTableName := "test-lock-table-nolock"

	createTokenBucketTable(t, client, bucketTableName)
	createLockTable(t, client, lockTableName)

	cfg := dynamodbstorage.NewBucketBackendConfig(
		client,
		bucketTableName,
		"no-lock-test",
		lockTableName,
		30*time.Second,
		3,
		5*time.Second,
	)

	// Test bucket creation without lock (should succeed)
	bucket, err := NewBucket(5, 1, "no-lock-test", cfg, WithoutLock())
	if err != nil {
		t.Fatalf("Failed to create bucket without lock: %v", err)
	}

	// Verify the bucket was created with correct configuration
	if bucket.Capacity != 5 {
		t.Errorf("Expected capacity 5, got %d", bucket.Capacity)
	}

	if bucket.FillRate != 1 {
		t.Errorf("Expected fill rate 1, got %d", bucket.FillRate)
	}

	if bucket.Dimension != "no-lock-test" {
		t.Errorf("Expected dimension 'no-lock-test', got %s", bucket.Dimension)
	}

	// Test that lock field is nil when WithoutLock is used
	if bucket.lock != nil {
		t.Error("Expected lock to be nil when WithoutLock option is used")
	}
}

func TestTokenBucket_StatePersistence(t *testing.T) {
	client, cleanup := setupDynamoDBLocal(t)
	defer cleanup()

	bucketTableName := "test-token-bucket-persistence"
	lockTableName := "test-lock-table-persistence"

	createTokenBucketTable(t, client, bucketTableName)
	createLockTable(t, client, lockTableName)

	cfg := dynamodbstorage.NewBucketBackendConfig(
		client,
		bucketTableName,
		"persistence-test",
		lockTableName,
		30*time.Second,
		3,
		5*time.Second,
	)

	ctx := context.Background()

	// Create first bucket instance and consume tokens
	{
		bucket, err := NewBucket(10, 1, "persistence-test", cfg)
		if err != nil {
			t.Fatalf("Failed to create first bucket: %v", err)
		}

		// Take 7 tokens
		for i := 0; i < 7; i++ {
			if err := bucket.Take(ctx); err != nil {
				t.Fatalf("Failed to take token %d: %v", i, err)
			}
		}
	}

	// Create second bucket instance with same dimension
	bucket2, err := NewBucket(10, 1, "persistence-test", cfg)
	if err != nil {
		t.Fatalf("Failed to create second bucket: %v", err)
	}

	// State should be preserved from first instance
	state, err := bucket2.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get state from second bucket: %v", err)
	}

	if state.Available != 3 {
		t.Errorf("Expected 3 available tokens in recreated bucket, got %d", state.Available)
	}

	// Should be able to take the remaining 3 tokens
	for i := 0; i < 3; i++ {
		if err := bucket2.Take(ctx); err != nil {
			t.Fatalf("Failed to take remaining token %d: %v", i, err)
		}
	}

	// Should be empty now
	if err := bucket2.Take(ctx); err == nil {
		t.Fatal("Expected bucket to be empty, but take succeeded")
	}
}
