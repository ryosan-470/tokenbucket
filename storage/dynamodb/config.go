package dynamodb

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/mennanov/limiters"
)

type BucketBackendConfig struct {
	client    *dynamodb.Client
	tableName string
	dimension string
}

func NewBucketBackendConfig(client *dynamodb.Client, tableName, dimension string) *BucketBackendConfig {
	return &BucketBackendConfig{
		client:    client,
		tableName: tableName,
		dimension: dimension,
	}
}

// NewLimitersBackend creates a new TokenBucketDynamoDB instance implemented by the limiters package.
func (b *BucketBackendConfig) NewLimitersBackend(ctx context.Context, enableRaceHandling bool) (BucketBackendInterface, error) {
	props, err := limiters.LoadDynamoDBTableProperties(ctx, b.client, b.tableName)
	if err != nil {
		return nil, err
	}

	return limiters.NewTokenBucketDynamoDB(
		b.client,
		b.dimension,
		props,
		time.Duration(1*time.Second), // Default fill rate
		enableRaceHandling,
	), nil
}

// NewCustomBackend creates a new Bucket instance implemented by this package.
func (b *BucketBackendConfig) NewCustomBackend(ctx context.Context) (BucketBackendInterface, error) {
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

type LockBackendConfig struct {
	client        *dynamodb.Client
	lockTableName string
	lockID        string
	lockTTL       time.Duration
	lockMaxTries  uint
	lockMaxTime   time.Duration
}

func NewLockBackendConfig(client *dynamodb.Client, lockTableName string, lockTTL time.Duration, lockMaxTries uint, lockMaxTime time.Duration) *LockBackendConfig {
	return &LockBackendConfig{
		client:        client,
		lockTableName: lockTableName,
		lockTTL:       lockTTL,
		lockMaxTries:  lockMaxTries,
		lockMaxTime:   lockMaxTime,
	}
}

func (b *LockBackendConfig) NewLockBackend(lockID string) *Lock {
	return NewLock(
		b.client,
		b.lockTableName,
		lockID,
		b.lockTTL,
		WithBackoffMaxTries(b.lockMaxTries),
		WithBackoffMaxTime(b.lockMaxTime),
	)
}
