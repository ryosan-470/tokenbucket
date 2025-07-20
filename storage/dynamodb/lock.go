package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/cenkalti/backoff/v5"
	"github.com/google/uuid"
	"github.com/mennanov/limiters"
)

var _ limiters.DistLocker = (*Lock)(nil)

type Lock struct {
	client          *dynamodb.Client
	tableName       string
	lockID          string
	ownerID         string
	ttl             time.Duration
	backoffMaxTries uint
	backoffMaxTime  time.Duration
}

// NewLock creates a new instance of Lock for distributed locking using DynamoDB.
func NewLock(client *dynamodb.Client, tableName, lockID string, ttl time.Duration, maxTries uint, maxTime time.Duration) *Lock {
	return &Lock{
		client:          client,
		tableName:       tableName,
		lockID:          lockID,
		ttl:             ttl,
		backoffMaxTries: maxTries,
		backoffMaxTime:  maxTime,
	}
}

const (
	conditionExpressionForPutItem    = "attribute_not_exists(LockID) OR (#ttl < :current_time)"
	conditionExpressionForDeleteItem = "attribute_exists(LockID) AND OwnerID = :owner_id"

	attributeNameLockID  = "LockID"
	attributeNameOwnerID = "OwnerID"
	attributeNameTTL     = "TTL"

	expressionAttributeNameOwnerID     = ":owner_id"
	expressionAttributeNameCurrentTime = ":current_time"
	expressionAttributeNameTTL         = "#ttl"
)

var ErrLockAlreadyHeld = errors.New("lock is already held by another process")

// Lock attempts to acquire the lock. If the lock is already held, it will retry until it succeeds or the context is canceled.
func (l *Lock) Lock(ctx context.Context) error {
	l.ownerID = uuid.New().String()
	ttl := time.Now().Add(l.ttl).Unix()
	currentTime := time.Now().Unix()

	input := &dynamodb.PutItemInput{
		TableName: aws.String(l.tableName),
		Item: map[string]types.AttributeValue{
			attributeNameLockID:  &types.AttributeValueMemberS{Value: l.lockID},
			attributeNameOwnerID: &types.AttributeValueMemberS{Value: l.ownerID},
			attributeNameTTL:     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", ttl)},
		},
		ConditionExpression: aws.String(conditionExpressionForPutItem),
		ExpressionAttributeNames: map[string]string{
			expressionAttributeNameTTL: attributeNameTTL,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			expressionAttributeNameCurrentTime: &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", currentTime)},
		},
	}

	operation := func() (string, error) {
		_, err := l.client.PutItem(ctx, input)
		if err != nil {
			var condFailed *types.ConditionalCheckFailedException
			if errors.As(err, &condFailed) {
				return "", ErrLockAlreadyHeld
			}
			return "", err
		}
		return "", nil
	}

	_, err := backoff.Retry(ctx, operation, backoff.WithMaxTries(l.backoffMaxTries), backoff.WithMaxElapsedTime(l.backoffMaxTime))

	return err
}

// Unlock releases the lock. If the lock is not held by this owner, it will return an error.
func (l *Lock) Unlock(ctx context.Context) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(l.tableName),
		Key: map[string]types.AttributeValue{
			attributeNameLockID: &types.AttributeValueMemberS{Value: l.lockID},
		},
		ConditionExpression: aws.String(conditionExpressionForDeleteItem),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			expressionAttributeNameOwnerID: &types.AttributeValueMemberS{Value: l.ownerID},
		},
	}

	_, err := l.client.DeleteItem(ctx, input)
	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		// If the lock was not held by this owner, we can ignore the error
		// because it means the lock was already released or held by another owner.
		if errors.As(err, &condFailed) {
			return nil
		}
	}

	return err
}
