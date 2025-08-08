package dynamodb

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mennanov/limiters"

	"github.com/ryosan-470/tokenbucket"
)

type DynamoDBBackend struct {
	client        *dynamodb.Client
	tableProps    limiters.DynamoDBTableProperties
	partitionKey  string
	ttl           time.Duration
	latestVersion int64
	keys          map[string]types.AttributeValue
}

const (
	attributeNameBackendLast      = "Last"
	attributeNameBackendVersion   = "Version"
	attributeNameBackendAvailable = "Available"
)

var _ tokenbucket.TokenBucketStateRepository = (*DynamoDBBackend)(nil)

func NewBackend(client *dynamodb.Client, partitionKey string, tableProps limiters.DynamoDBTableProperties, ttl time.Duration) *DynamoDBBackend {
	keys := map[string]types.AttributeValue{
		tableProps.PartitionKeyName: &types.AttributeValueMemberS{Value: partitionKey},
	}

	if tableProps.SortKeyUsed {
		keys[tableProps.SortKeyName] = &types.AttributeValueMemberS{Value: partitionKey}
	}

	return &DynamoDBBackend{
		client:        client,
		tableProps:    tableProps,
		partitionKey:  partitionKey,
		ttl:           ttl,
		latestVersion: 0,
		keys:          keys,
	}
}

func (b *DynamoDBBackend) State(ctx context.Context) (tokenbucket.State, error) {
	res, err := b.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(b.tableProps.TableName),
		Key:       b.keys,
	})
	if err != nil {
		return tokenbucket.NewState(0, 0), err
	}

	if res.Item == nil {
		return tokenbucket.NewState(0, 0), nil
	}

	return b.makeTokenBucketState(res.Item)
}

func (b *DynamoDBBackend) makeTokenBucketState(item map[string]types.AttributeValue) (tokenbucket.State, error) {
	state := tokenbucket.NewState(0, 0)

	if err := attributevalue.Unmarshal(item[attributeNameBackendLast], &state.LastUpdated); err != nil {
		return tokenbucket.NewState(0, 0), err
	}

	if err := attributevalue.Unmarshal(item[attributeNameBackendAvailable], &state.Available); err != nil {
		return tokenbucket.NewState(0, 0), err
	}

	if err := attributevalue.Unmarshal(item[attributeNameBackendVersion], &b.latestVersion); err != nil {
		return tokenbucket.NewState(0, 0), err
	}

	return state, nil
}

func (b *DynamoDBBackend) SetState(ctx context.Context, state tokenbucket.State) error {
	updateExpr := makeUpdateExpressionBuilder(state, b.latestVersion+1)
	cond := expression.Name(attributeNameBackendVersion).AttributeNotExists().Or(
		expression.Name(attributeNameBackendVersion).Equal(expression.Value(b.latestVersion)),
	)

	expr, err := expression.NewBuilder().WithUpdate(updateExpr).WithCondition(cond).Build()
	if err != nil {
		return err
	}

	input := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(b.tableProps.TableName),
		Key:                       b.keys,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ReturnValues:              types.ReturnValueNone,
	}

	_, err = b.client.UpdateItem(ctx, input)
	if err != nil {
		return err
	}
	b.latestVersion++
	return nil
}

func (b *DynamoDBBackend) Reset(ctx context.Context) error {
	updateExpr := makeUpdateExpressionBuilder(tokenbucket.NewState(0, 0), 0)

	expr, err := expression.NewBuilder().WithUpdate(updateExpr).Build()
	if err != nil {
		return err
	}

	input := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(b.tableProps.TableName),
		Key:                       b.keys,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
		ReturnValues:              types.ReturnValueNone,
	}

	if _, err := b.client.UpdateItem(ctx, input); err != nil {
		return err
	}

	b.latestVersion = 0
	return nil
}

func makeUpdateExpressionBuilder(state tokenbucket.State, latestVersion int64) expression.UpdateBuilder {
	return expression.Set(
		expression.Name(attributeNameBackendLast), expression.Value(state.LastUpdated),
	).Set(
		expression.Name(attributeNameBackendAvailable), expression.Value(state.Available),
	).Set(
		expression.Name(attributeNameBackendVersion), expression.Value(latestVersion),
	)
}
