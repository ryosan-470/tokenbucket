package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/testcontainers/testcontainers-go"
	dynamodbcontainer "github.com/testcontainers/testcontainers-go/modules/dynamodb"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ryosan-470/tokenbucket"
	dynamodbstorage "github.com/ryosan-470/tokenbucket/storage/dynamodb"
)

// LocalProvider provides TokenBucket benchmarks using DynamoDB Local
type LocalProvider struct {
	client          *dynamodb.Client
	container       testcontainers.Container
	bucketTableName string
	lockTableName   string
	mu              sync.Mutex
	initialized     bool
}

var (
	globalLocalProvider *LocalProvider
	setupOnce           sync.Once
)

// GetLocalProvider returns a singleton LocalProvider instance for reuse across benchmarks
func GetLocalProvider() *LocalProvider {
	setupOnce.Do(func() {
		globalLocalProvider = &LocalProvider{
			bucketTableName: "benchmark-token-bucket",
			lockTableName:   "benchmark-lock-table",
		}
	})
	return globalLocalProvider
}

// Setup initializes DynamoDB Local container and creates tables
func (h *LocalProvider) Setup(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.initialized {
		return nil
	}

	// Start DynamoDB Local container
	waitStrategy := wait.ForExposedPort().
		WithStartupTimeout(2 * time.Minute).
		WithPollInterval(1 * time.Second)

	container, err := dynamodbcontainer.Run(ctx, "amazon/dynamodb-local:latest",
		testcontainers.WithWaitStrategy(waitStrategy),
	)
	if err != nil {
		return err
	}
	h.container = container

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		return err
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
		return err
	}

	h.client = dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
	})

	// Create tables
	if err := h.createTokenBucketTable(ctx); err != nil {
		return err
	}
	if err := h.createLockTable(ctx); err != nil {
		return err
	}

	h.initialized = true
	return nil
}

// Cleanup terminates the DynamoDB container
func (h *LocalProvider) Cleanup(ctx context.Context) error {
	if h.container != nil {
		return h.container.Terminate(ctx)
	}
	return nil
}

// CreateBucketConfig creates a bucket configuration for the given dimension
func (h *LocalProvider) CreateBucketConfig(dimension string) *dynamodbstorage.BucketBackendConfig {
	return dynamodbstorage.NewBucketBackendConfig(
		h.client,
		h.bucketTableName,
		dimension,
		h.lockTableName,
		30*time.Second,
		3,
		5*time.Second,
	)
}

// CreateBucket creates a TokenBucket with the specified configuration
func (h *LocalProvider) CreateBucket(capacity, fillRate int64, dimension string, opts ...tokenbucket.Option) (*tokenbucket.Bucket, error) {
	cfg := h.CreateBucketConfig(dimension)
	return tokenbucket.NewBucket(capacity, fillRate, dimension, cfg, opts...)
}

func (h *LocalProvider) createTokenBucketTable(ctx context.Context) error {
	_, err := h.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(h.bucketTableName),
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
		return err
	}

	waiter := dynamodb.NewTableExistsWaiter(h.client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(h.bucketTableName),
	}, 2*time.Minute); err != nil {
		return err
	}

	_, err = h.client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(h.bucketTableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("_TTL"),
			Enabled:       aws.Bool(true),
		},
	})
	return err
}

func (h *LocalProvider) createLockTable(ctx context.Context) error {
	_, err := h.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(h.lockTableName),
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
		return err
	}

	waiter := dynamodb.NewTableExistsWaiter(h.client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(h.lockTableName),
	}, 2*time.Minute); err != nil {
		return err
	}

	_, err = h.client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(h.lockTableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("TTL"),
			Enabled:       aws.Bool(true),
		},
	})
	return err
}

// BenchmarkSetup is a helper function for benchmark setup
func BenchmarkSetup(b *testing.B) *LocalProvider {
	b.Helper()
	
	ctx := context.Background()
	provider := GetLocalProvider()
	
	if err := provider.Setup(ctx); err != nil {
		b.Fatalf("Failed to setup benchmark provider: %v", err)
	}
	
	return provider
}