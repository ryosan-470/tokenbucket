package storage

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ryosan-470/tokenbucket"
	dynamodbstorage "github.com/ryosan-470/tokenbucket/storage/dynamodb"
)

// AWSProvider provides TokenBucket benchmarks using real AWS DynamoDB
type AWSProvider struct {
	client          *dynamodb.Client
	bucketTableName string
	lockTableName   string
	mu              sync.Mutex
	initialized     bool
}

var (
	globalAWSProvider *AWSProvider
	awsSetupOnce      sync.Once
)

// GetAWSProvider returns a singleton AWSProvider instance for reuse across benchmarks
func GetAWSProvider() *AWSProvider {
	awsSetupOnce.Do(func() {
		globalAWSProvider = &AWSProvider{
			bucketTableName: "tokenbucket-benchmark-bucket",
			lockTableName:   "tokenbucket-benchmark-lock",
		}
	})
	return globalAWSProvider
}

// Setup initializes AWS DynamoDB client using AWS_PROFILE environment variable
func (h *AWSProvider) Setup(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.initialized {
		return nil
	}

	// Load AWS configuration from AWS_PROFILE environment variable
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}

	h.client = dynamodb.NewFromConfig(cfg)

	// Create tables if they don't exist
	if err := h.createTokenBucketTableIfNotExists(ctx); err != nil {
		return err
	}
	if err := h.createLockTableIfNotExists(ctx); err != nil {
		return err
	}

	h.initialized = true
	return nil
}

// Cleanup is a no-op for AWS provider since we don't manage any resources
func (h *AWSProvider) Cleanup(ctx context.Context) error {
	// Nothing to cleanup for AWS harness
	return nil
}

// CreateBucketConfig creates a bucket configuration for the given dimension
func (h *AWSProvider) CreateBucketConfig(dimension string) *dynamodbstorage.BucketBackendConfig {
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
func (h *AWSProvider) CreateBucket(capacity, fillRate int64, dimension string, opts ...tokenbucket.Option) (*tokenbucket.Bucket, error) {
	cfg := h.CreateBucketConfig(dimension)
	return tokenbucket.NewBucket(capacity, fillRate, dimension, cfg, opts...)
}

func (h *AWSProvider) createTokenBucketTableIfNotExists(ctx context.Context) error {
	// Check if table exists
	_, err := h.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(h.bucketTableName),
	})

	if err == nil {
		log.Printf("Token bucket table '%s' already exists", h.bucketTableName)
		return nil
	}

	// Table doesn't exist, create it
	log.Printf("Creating token bucket table '%s'...", h.bucketTableName)

	_, err = h.client.CreateTable(ctx, &dynamodb.CreateTableInput{
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

	// Wait for table to be created
	waiter := dynamodb.NewTableExistsWaiter(h.client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(h.bucketTableName),
	}, 5*time.Minute); err != nil {
		return err
	}

	// Enable TTL
	_, err = h.client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(h.bucketTableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("_TTL"),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		return err
	}

	log.Printf("Token bucket table '%s' created successfully", h.bucketTableName)
	return nil
}

func (h *AWSProvider) createLockTableIfNotExists(ctx context.Context) error {
	// Check if table exists
	_, err := h.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(h.lockTableName),
	})

	if err == nil {
		log.Printf("Lock table '%s' already exists", h.lockTableName)
		return nil
	}

	// Table doesn't exist, create it
	log.Printf("Creating lock table '%s'...", h.lockTableName)

	_, err = h.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(h.lockTableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String(dynamodbstorage.AttributeNameLockID),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String(dynamodbstorage.AttributeNameLockID),
				KeyType:       types.KeyTypeHash,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		return err
	}

	// Wait for table to be created
	waiter := dynamodb.NewTableExistsWaiter(h.client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(h.lockTableName),
	}, 5*time.Minute); err != nil {
		return err
	}

	// Enable TTL
	_, err = h.client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(h.lockTableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String(dynamodbstorage.AttributeNameTTL),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		return err
	}

	log.Printf("Lock table '%s' created successfully", h.lockTableName)
	return nil
}
