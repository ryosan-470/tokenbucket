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
