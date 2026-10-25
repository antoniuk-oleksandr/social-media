package dynamodb

import (
	"context"

	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type API interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

type Repository struct {
	Client    API
	TableName string
}

func NewRepository(client *dynamodb.Client, tableName string) user.Repository {
	return &Repository{Client: client, TableName: tableName}
}

func NewConfiguredRepository(client *dynamodb.Client, usersTable string) user.Repository {
	return &Repository{Client: client, TableName: usersTable}
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

func attributeString(item map[string]types.AttributeValue, key string) string {
	value, ok := item[key].(*types.AttributeValueMemberS)
	if !ok {
		return ""
	}
	return value.Value
}
