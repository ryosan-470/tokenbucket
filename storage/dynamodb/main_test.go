package dynamodb

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mennanov/limiters"
	"github.com/testcontainers/testcontainers-go"
	dynamodbcontainer "github.com/testcontainers/testcontainers-go/modules/dynamodb"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Global test infrastructure - shared across all tests
var (
	testClient    *dynamodb.Client
	testContainer testcontainers.Container
	testCtx       = context.Background()
)

// TestMain sets up and tears down the shared test infrastructure
func TestMain(m *testing.M) {
	// Setup
	var err error
	testClient, testContainer, err = setupSharedDynamoDBLocal()
	if err != nil {
		fmt.Printf("Failed to set up DynamoDB Local container: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	if testContainer != nil {
		if err := testContainer.Terminate(testCtx); err != nil {
			fmt.Printf("Failed to terminate DynamoDB Local container: %v\n", err)
		}
	}

	os.Exit(code)
}

// setupSharedDynamoDBLocal creates a single DynamoDB Local container for all tests
func setupSharedDynamoDBLocal() (*dynamodb.Client, testcontainers.Container, error) {
	// Create wait strategy focusing on port availability and service readiness
	waitStrategy := wait.ForHTTP("/").
		WithPort("8000/tcp").
		WithStatusCodeMatcher(func(status int) bool {
			return status == http.StatusBadRequest
		}).
		WithStartupTimeout(2 * time.Minute).
		WithPollInterval(1 * time.Second)

	container, err := dynamodbcontainer.Run(testCtx, "amazon/dynamodb-local:latest",
		testcontainers.WithWaitStrategy(waitStrategy),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start DynamoDB Local container: %w", err)
	}

	endpoint, err := container.ConnectionString(testCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get DynamoDB Local endpoint: %w", err)
	}

	// Add http:// prefix to endpoint
	endpointURL := "http://" + endpoint

	cfg, err := config.LoadDefaultConfig(testCtx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     "test",
				SecretAccessKey: "test",
			}, nil
		})),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
	})

	return client, container, nil
}

// TestInfrastructure provides convenient access to shared test resources
type TestInfrastructure struct {
	Client *dynamodb.Client
}

// GetTestInfrastructure returns the shared test infrastructure
func GetTestInfrastructure() *TestInfrastructure {
	return &TestInfrastructure{
		Client: testClient,
	}
}

// CreateLockTable creates a DynamoDB table for lock testing
func (ti *TestInfrastructure) CreateLockTable(t *testing.T, tableName string) {
	_, err := ti.Client.CreateTable(testCtx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String(AttributeNameLockID),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String(AttributeNameLockID),
				KeyType:       types.KeyTypeHash,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		t.Fatalf("Failed to create lock table: %v", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(ti.Client)
	if err := waiter.Wait(testCtx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 2*time.Minute); err != nil {
		t.Fatalf("Failed to wait for table to be active: %v", err)
	}

	// Enable TTL on the table
	_, err = ti.Client.UpdateTimeToLive(testCtx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String(AttributeNameTTL),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		t.Fatalf("Failed to enable TTL on table: %v", err)
	}
}

// CreateTokenBucketTable creates a DynamoDB table for token bucket testing
func (ti *TestInfrastructure) CreateTokenBucketTable(t *testing.T, tableName string) limiters.DynamoDBTableProperties {
	// Create table with standard limiters schema
	_, err := ti.Client.CreateTable(testCtx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("PartitionKey"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("PartitionKey"),
				KeyType:       types.KeyTypeHash,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		t.Fatalf("Failed to create token bucket table: %v", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(ti.Client)
	if err := waiter.Wait(testCtx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 2*time.Minute); err != nil {
		t.Fatalf("Failed to wait for table to be active: %v", err)
	}

	// Load table properties using limiters library
	props, err := limiters.LoadDynamoDBTableProperties(testCtx, ti.Client, tableName)
	if err != nil {
		t.Fatalf("Failed to load DynamoDB table properties: %v", err)
	}

	return props
}

// DeleteTable removes a table (useful for test cleanup)
func (ti *TestInfrastructure) DeleteTable(t *testing.T, tableName string) {
	_, err := ti.Client.DeleteTable(testCtx, &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		// Don't fail test if table doesn't exist
		var notFound *types.ResourceNotFoundException
		if err, ok := err.(*types.ResourceNotFoundException); ok {
			_ = notFound
			_ = err
			return
		}
		t.Logf("Warning: Failed to delete table %s: %v", tableName, err)
	}
}
