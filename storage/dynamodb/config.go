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

func (b *BucketBackendConfig) NewTokenBucketDynamoDB(ctx context.Context) (*limiters.TokenBucketDynamoDB, error) {
	props, err := limiters.LoadDynamoDBTableProperties(ctx, b.client, b.tableName)
	if err != nil {
		return nil, err
	}

	return limiters.NewTokenBucketDynamoDB(
		b.client,
		b.dimension,
		props,
		time.Duration(1*time.Second), // Default fill rate
		true,                         // Enable race-checking
	), nil
}

func (b *BucketBackendConfig) NewLock(lockID string) *Lock {
	return NewLock(
		b.client,
		b.lockTableName,
		lockID,
		b.lockTTL,
		b.lockMaxTries,
		b.lockMaxTime,
	)
}
