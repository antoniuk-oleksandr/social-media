package dynamodb

import (
	"context"
	"errors"
	"strings"
	"time"

	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type API interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

type updateAPI interface {
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

type transactAPI interface {
	TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

type Repository struct {
	Client          API
	TableName       string
	UniqueTableName string
	Clock           interface{ Now() time.Time }
}

func NewRepository(client *dynamodb.Client, tableName, uniqueTableName string) user.Repository {
	return &Repository{Client: client, TableName: tableName, UniqueTableName: uniqueTableName, Clock: platformclock.Real{}}
}

func NewConfiguredRepository(client *dynamodb.Client, usersTable, uniqueTable string) user.Repository {
	return &Repository{Client: client, TableName: usersTable, UniqueTableName: uniqueTable, Clock: platformclock.Real{}}
}

func NewConfiguredRepositoryWithClock(client *dynamodb.Client, usersTable, uniqueTable string, clk interface{ Now() time.Time }) user.Repository {
	return &Repository{Client: client, TableName: usersTable, UniqueTableName: uniqueTable, Clock: clk}
}

func (r *Repository) GetByID(ctx context.Context, id string) (user.User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.Version() != 7 || parsed.String() != id {
		return user.User{}, user.ErrInvalidID
	}
	result, err := r.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(r.TableName), Key: map[string]types.AttributeValue{"userId": &types.AttributeValueMemberS{Value: id}}, ConsistentRead: aws.Bool(true)})
	if err != nil {
		return user.User{}, user.ErrDependencyUnavailable
	}
	if len(result.Item) == 0 {
		return user.User{}, user.ErrNotFound
	}
	record := UserRecord{
		ID:           attributeString(result.Item, "userId"),
		Username:     attributeString(result.Item, "username"),
		Email:        attributeString(result.Item, "email"),
		PasswordHash: attributeString(result.Item, "passwordHash"),
		DisplayName:  attributeString(result.Item, "displayName"),
		Role:         attributeString(result.Item, "role"),
		Status:       attributeString(result.Item, "status"),
		CreatedAt:    attributeString(result.Item, "createdAt"),
		UpdatedAt:    attributeString(result.Item, "updatedAt"),
	}
	value, err := toDomain(record)
	if err != nil {
		return user.User{}, user.ErrDependencyUnavailable
	}
	return value, nil
}

func (r *Repository) UpdateProfile(ctx context.Context, command user.UpdateProfileCommand) (user.User, error) {
	current, err := r.GetByID(ctx, command.UserID)
	if err != nil {
		return user.User{}, err
	}
	client, ok := r.Client.(updateAPI)
	if !ok {
		return user.User{}, user.ErrDependencyUnavailable
	}
	sets := []string{"#updatedAt = :updatedAt"}
	values := map[string]types.AttributeValue{}
	names := map[string]string{"#updatedAt": "updatedAt"}
	updatedAt := time.Now().UTC()
	if r.Clock != nil {
		updatedAt = r.Clock.Now().UTC()
	}
	values[":updatedAt"] = &types.AttributeValueMemberS{Value: updatedAt.Format(time.RFC3339)}
	if command.Username != nil {
		sets = append(sets, "#username = :username")
		names["#username"] = "username"
		values[":username"] = &types.AttributeValueMemberS{Value: *command.Username}
	}
	if command.DisplayName != nil {
		sets = append(sets, "#displayName = :displayName")
		names["#displayName"] = "displayName"
		values[":displayName"] = &types.AttributeValueMemberS{Value: *command.DisplayName}
	}
	update := &types.Update{
		TableName:                aws.String(r.TableName),
		Key:                      map[string]types.AttributeValue{"userId": &types.AttributeValueMemberS{Value: command.UserID}},
		UpdateExpression:         aws.String("SET " + strings.Join(sets, ", ")),
		ExpressionAttributeNames: names, ExpressionAttributeValues: values,
		ConditionExpression: aws.String("attribute_exists(userId)"),
	}
	if command.Username != nil && *command.Username != current.Username {
		transactionClient, ok := r.Client.(transactAPI)
		if !ok {
			return user.User{}, user.ErrDependencyUnavailable
		}
		_, err = transactionClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
			{Delete: &types.Delete{TableName: aws.String(r.UniqueTableName), Key: map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#" + current.Username}}, ConditionExpression: aws.String("userId = :owner"), ExpressionAttributeValues: map[string]types.AttributeValue{":owner": &types.AttributeValueMemberS{Value: command.UserID}}}},
			{Put: &types.Put{TableName: aws.String(r.UniqueTableName), Item: map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#" + *command.Username}, "userId": &types.AttributeValueMemberS{Value: command.UserID}}, ConditionExpression: aws.String("attribute_not_exists(uniqueKey)")}},
			{Update: update},
		}})
	} else {
		_, err = client.UpdateItem(ctx, &dynamodb.UpdateItemInput{TableName: update.TableName, Key: update.Key, UpdateExpression: update.UpdateExpression, ExpressionAttributeNames: update.ExpressionAttributeNames, ExpressionAttributeValues: update.ExpressionAttributeValues, ConditionExpression: update.ConditionExpression})
	}
	if err != nil {
		if command.Username != nil && *command.Username != current.Username && isUsernameConflict(err) {
			return user.User{}, user.ErrUsernameTaken
		}
		return user.User{}, user.ErrDependencyUnavailable
	}
	return r.GetByID(ctx, command.UserID)
}

func isUsernameConflict(err error) bool {
	var canceled *types.TransactionCanceledException
	if !errors.As(err, &canceled) || len(canceled.CancellationReasons) < 2 {
		return false
	}
	return canceled.CancellationReasons[1].Code != nil && *canceled.CancellationReasons[1].Code == "ConditionalCheckFailed"
}

func attributeString(item map[string]types.AttributeValue, key string) string {
	value, ok := item[key].(*types.AttributeValueMemberS)
	if !ok {
		return ""
	}
	return value.Value
}
