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
)

type Backend struct {
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

var _ limiters.TokenBucketStateBackend = (*Backend)(nil)

func NewBackend(client *dynamodb.Client, partitionKey string, tableProps limiters.DynamoDBTableProperties, ttl time.Duration) *Backend {
	keys := map[string]types.AttributeValue{
		tableProps.PartitionKeyName: &types.AttributeValueMemberS{Value: partitionKey},
	}

	if tableProps.SortKeyUsed {
		keys[tableProps.SortKeyName] = &types.AttributeValueMemberS{Value: partitionKey}
	}

	return &Backend{
		client:        client,
		tableProps:    tableProps,
		partitionKey:  partitionKey,
		ttl:           ttl,
		latestVersion: 0,
		keys:          keys,
	}
}

func (b *Backend) State(ctx context.Context) (limiters.TokenBucketState, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(b.tableProps.TableName),
		Key:       b.keys,
	}

	res, err := b.client.GetItem(ctx, input)
	if err != nil {
		return limiters.TokenBucketState{}, err
	}

	if res.Item == nil {
		return limiters.TokenBucketState{}, nil
	}

	return b.makeTokenBucketState(res.Item)
}

func (b *Backend) makeTokenBucketState(item map[string]types.AttributeValue) (limiters.TokenBucketState, error) {
	state := limiters.TokenBucketState{}

	if err := attributevalue.Unmarshal(item[attributeNameBackendLast], &state.Last); err != nil {
		return limiters.TokenBucketState{}, err
	}

	if err := attributevalue.Unmarshal(item[attributeNameBackendAvailable], &state.Available); err != nil {
		return limiters.TokenBucketState{}, err
	}

	if err := attributevalue.Unmarshal(item[attributeNameBackendVersion], &b.latestVersion); err != nil {
		return limiters.TokenBucketState{}, err
	}

	return state, nil
}

func (b *Backend) SetState(ctx context.Context, state limiters.TokenBucketState) error {
	updateExpr := makeUpdateExpressionBuilder(state, b.latestVersion+1)
	cond := expression.Name(attributeNameBackendVersion).Equal(expression.Value(b.latestVersion))

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
	return err
}

func (b *Backend) Reset(ctx context.Context) error {
	state := limiters.TokenBucketState{
		Last:      0,
		Available: 0,
	}

	if err := b.SetState(ctx, state); err != nil {
		return err
	}

	b.latestVersion = 0
	return nil
}

func makeUpdateExpressionBuilder(state limiters.TokenBucketState, latestVersion int64) expression.UpdateBuilder {
	return expression.Set(
		expression.Name(attributeNameBackendLast), expression.Value(state.Last),
	).Set(
		expression.Name(attributeNameBackendAvailable), expression.Value(state.Available),
	).Set(
		expression.Name(attributeNameBackendVersion), expression.Value(latestVersion),
	)
}
