package tokenbucket_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mennanov/limiters"
	"github.com/ryosan-470/tokenbucket/internal/testutils"

	ddbbackend "github.com/ryosan-470/tokenbucket/storage/dynamodb"
)

var backend *testutils.DynamoDBTestBackend

func TestMain(m *testing.M) {
	var err error
	backend, err = testutils.SetupDynamoDBTestBackend(context.Background())
	if err != nil {
		fmt.Printf("Failed to set up DynamoDB test backend: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	if backend.Container != nil {
		if err := backend.Container.Terminate(backend.Ctx); err != nil {
			fmt.Printf("Failed to terminate DynamoDB Local container: %v\n", err)
		}
	}

	os.Exit(code)
}

type TestInfrastructure struct {
	Client *dynamodb.Client
	Ctx    context.Context
}

func GetTestInfrastructure() *TestInfrastructure {
	return &TestInfrastructure{
		Client: backend.Client,
		Ctx:    backend.Ctx,
	}
}

// CreateLockTable creates a DynamoDB table for lock testing
func (ti *TestInfrastructure) CreateLockTable(t *testing.T, tableName string) limiters.DynamoDBTableProperties {
	_, err := ti.Client.CreateTable(backend.Ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String(ddbbackend.AttributeNameLockID),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String(ddbbackend.AttributeNameLockID),
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
	if err := waiter.Wait(backend.Ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 2*time.Minute); err != nil {
		t.Fatalf("Failed to wait for table to be active: %v", err)
	}

	// Enable TTL on the table
	_, err = ti.Client.UpdateTimeToLive(backend.Ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String(ddbbackend.AttributeNameTTL),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		t.Fatalf("Failed to enable TTL on table: %v", err)
	}

	// Load table properties using limiters library
	props, err := limiters.LoadDynamoDBTableProperties(backend.Ctx, ti.Client, tableName)
	if err != nil {
		t.Fatalf("Failed to load DynamoDB table properties: %v", err)
	}

	return props
}

// CreateTokenBucketTable creates a DynamoDB table for token bucket testing
func (ti *TestInfrastructure) CreateTokenBucketTable(t *testing.T, tableName string) limiters.DynamoDBTableProperties {
	// Create table with standard limiters schema
	_, err := ti.Client.CreateTable(backend.Ctx, &dynamodb.CreateTableInput{
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
	if err := waiter.Wait(backend.Ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 2*time.Minute); err != nil {
		t.Fatalf("Failed to wait for table to be active: %v", err)
	}

	// Load table properties using limiters library
	props, err := limiters.LoadDynamoDBTableProperties(backend.Ctx, ti.Client, tableName)
	if err != nil {
		t.Fatalf("Failed to load DynamoDB table properties: %v", err)
	}

	return props
}

// DeleteTable removes a table (useful for test cleanup)
func (ti *TestInfrastructure) DeleteTable(t *testing.T, tableName string) {
	_, err := ti.Client.DeleteTable(backend.Ctx, &dynamodb.DeleteTableInput{
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
