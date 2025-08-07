package dynamodb

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/mennanov/limiters"

	"github.com/ryosan-470/tokenbucket"
)

type BucketBackendConfig struct {
	client    *dynamodb.Client
	tableName string
}

func NewBucketBackendConfig(client *dynamodb.Client, tableName string) *BucketBackendConfig {
	return &BucketBackendConfig{
		client:    client,
		tableName: tableName,
	}
}

// NewCustomBackend creates a new Bucket instance implemented by this package.
func (b *BucketBackendConfig) NewCustomBackend(ctx context.Context, dimension string) (tokenbucket.TokenBucketStateRepository, error) {
	props, err := limiters.LoadDynamoDBTableProperties(ctx, b.client, b.tableName)
	if err != nil {
		return nil, err
	}

	return NewBackend(
		b.client,
		dimension,
		props,
		time.Duration(1*time.Second), // Default fill rate
	), nil
}

type LockBackendConfig struct {
	client        *dynamodb.Client
	lockTableName string
	lockTTL       time.Duration
	lockMaxTries  uint
	lockMaxTime   time.Duration
}

func NewLockBackendConfig(client *dynamodb.Client, tableName string, ttl time.Duration, maxTries uint, maxTime time.Duration) *LockBackendConfig {
	return &LockBackendConfig{
		client:        client,
		lockTableName: tableName,
		lockTTL:       ttl,
		lockMaxTries:  maxTries,
		lockMaxTime:   maxTime,
	}
}

func (l *LockBackendConfig) NewLockBackend(lockID string) limiters.DistLocker {
	return NewLock(
		l.client,
		l.lockTableName,
		lockID,
		l.lockTTL,
		WithBackoffMaxTime(l.lockMaxTime),
		WithBackoffMaxTries(l.lockMaxTries),
	)
}
