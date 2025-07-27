package testutils

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/testcontainers/testcontainers-go"
	dynamodbcontainer "github.com/testcontainers/testcontainers-go/modules/dynamodb"
	"github.com/testcontainers/testcontainers-go/wait"
)

type DynamoDBTestBackend struct {
	Client    *dynamodb.Client
	Container testcontainers.Container
	Ctx       context.Context
}

func SetupDynamoDBTestBackend(ctx context.Context) (*DynamoDBTestBackend, error) {
	waitStrategy := wait.ForHTTP("/").
		WithPort("8000/tcp").
		WithStatusCodeMatcher(func(status int) bool {
			return status == http.StatusBadRequest
		}).
		WithStartupTimeout(2 * time.Minute).
		WithPollInterval(1 * time.Second)

	container, err := dynamodbcontainer.Run(
		ctx,
		"amazon/dynamodb-local:latest",
		testcontainers.WithWaitStrategy(waitStrategy),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start DynamoDB Local container: %w", err)
	}

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get DynamoDB Local endpoint: %w", err)
	}

	// Add http:// prefix to endpoint
	endpointURL := "http://" + endpoint

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     "test",
				SecretAccessKey: "test",
			}, nil
		})),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
	})

	return &DynamoDBTestBackend{
		Client:    client,
		Container: container,
		Ctx:       ctx,
	}, nil
}
