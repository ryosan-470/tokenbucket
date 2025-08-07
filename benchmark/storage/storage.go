package storage

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/storage"
	dynamodbstorage "github.com/ryosan-470/tokenbucket/storage/dynamodb"
)

// Provider provides TokenBucket instances for benchmarking
type Provider interface {
	Setup(ctx context.Context) error
	Cleanup(ctx context.Context) error
	CreateBucket(capacity, fillRate int64, dimension string, backend storage.Storage, opts ...tokenbucket.Option) (*tokenbucket.Bucket, error)
	CreateBucketConfig(dimension string) *dynamodbstorage.BucketBackendConfig
	CreateLockBackendConfig() *dynamodbstorage.LockBackendConfig
}

func createLockBackendConfig(client *dynamodb.Client, lockTableName string) *dynamodbstorage.LockBackendConfig {
	return dynamodbstorage.NewLockBackendConfig(
		client,
		lockTableName,
		1*time.Second,        // Default TTL for locks
		3,                    // Max tries to acquire lock
		900*time.Microsecond, // Max time to hold lock
	)
}
