package dynamodb

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/mennanov/limiters"
)

type BucketBackendConfig struct {
	client        *dynamodb.Client
	tableName     string
	dimension     string
	lockTableName string
	lockTTL       time.Duration
	lockMaxTries  uint
	lockMaxTime   time.Duration
}

func NewBucketBackendConfig(client *dynamodb.Client, tableName, dimension, lockTableName string, lockTTL time.Duration, lockMaxTries uint, lockMaxTime time.Duration) *BucketBackendConfig {
	return &BucketBackendConfig{
		client:        client,
		tableName:     tableName,
		dimension:     dimension,
		lockTableName: lockTableName,
		lockTTL:       lockTTL,
		lockMaxTries:  lockMaxTries,
		lockMaxTime:   lockMaxTime,
	}
}

// NewTokenBucketDynamoDB creates a new TokenBucketDynamoDB instance implemented by the limiters package.
func (b *BucketBackendConfig) NewTokenBucketDynamoDB(ctx context.Context, race bool) (BucketBackendInterface, error) {
	props, err := limiters.LoadDynamoDBTableProperties(ctx, b.client, b.tableName)
	if err != nil {
		return nil, err
	}

	return limiters.NewTokenBucketDynamoDB(
		b.client,
		b.dimension,
		props,
		time.Duration(1*time.Second), // Default fill rate
		race,
	), nil
}

// NewBucket creates a new Bucket instance instance implemented by this package.
func (b *BucketBackendConfig) NewBucket(ctx context.Context) (BucketBackendInterface, error) {
	props, err := limiters.LoadDynamoDBTableProperties(ctx, b.client, b.tableName)
	if err != nil {
		return nil, err
	}

	return NewBackend(
		b.client,
		b.dimension,
		props,
		time.Duration(1*time.Second), // Default fill rate
	), nil
}

func (b *BucketBackendConfig) NewLock(lockID string) *Lock {
	return NewLock(
		b.client,
		b.lockTableName,
		lockID,
		b.lockTTL,
		WithBackoffMaxTries(b.lockMaxTries),
		WithBackoffMaxTime(b.lockMaxTime),
	)
}
