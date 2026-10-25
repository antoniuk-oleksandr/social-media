package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/antoniuk-oleksandr/social-media/internal/post"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type API interface {
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type Repository struct {
	Client    API
	TableName string
}

func NewRepository(client *dynamodb.Client, tableName string) post.Repository {
	return &Repository{Client: client, TableName: tableName}
}

func (r *Repository) Create(ctx context.Context, value post.Post) error {
	record := toRecord(value)
	item := map[string]types.AttributeValue{
		"postId": &types.AttributeValueMemberS{Value: record.ID}, "entityType": &types.AttributeValueMemberS{Value: record.EntityType},
		"authorId": &types.AttributeValueMemberS{Value: record.AuthorID}, "content": &types.AttributeValueMemberS{Value: record.Content},
		"status": &types.AttributeValueMemberS{Value: record.Status}, "createdAt": &types.AttributeValueMemberS{Value: record.CreatedAt},
		"createdAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(record.CreatedAtEpoch)}, "feedPartition": &types.AttributeValueMemberS{Value: record.FeedPartition},
	}
	_, err := r.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(r.TableName), Item: item, ConditionExpression: aws.String("attribute_not_exists(postId)")})
	if err != nil {
		return post.ErrDependencyUnavailable
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (post.Post, error) {
	result, err := r.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(r.TableName),
		Key:            map[string]types.AttributeValue{"postId": &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return post.Post{}, post.ErrDependencyUnavailable
	}
	if len(result.Item) == 0 {
		return post.Post{}, post.ErrNotFound
	}
	record := PostRecord{
		ID: attributeString(result.Item, "postId"), EntityType: attributeString(result.Item, "entityType"),
		AuthorID: attributeString(result.Item, "authorId"), Content: attributeString(result.Item, "content"),
		Status: attributeString(result.Item, "status"), CreatedAt: attributeString(result.Item, "createdAt"),
		FeedPartition: attributeString(result.Item, "feedPartition"),
	}
	if record.Status != "active" {
		return post.Post{}, post.ErrNotFound
	}
	createdAtEpoch, ok := attributeNumber(result.Item, "createdAtEpoch")
	if !ok {
		return post.Post{}, post.ErrDependencyUnavailable
	}
	record.CreatedAtEpoch = createdAtEpoch
	value, err := toDomain(record, id)
	if err != nil {
		return post.Post{}, post.ErrDependencyUnavailable
	}
	return value, nil
}

func (r *Repository) Delete(ctx context.Context, id, ownerID string) error {
	_, err := r.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:                aws.String(r.TableName),
		Key:                      map[string]types.AttributeValue{"postId": &types.AttributeValueMemberS{Value: id}},
		ConditionExpression:      aws.String("attribute_exists(postId) AND authorId = :ownerId AND #postStatus = :active"),
		ExpressionAttributeNames: map[string]string{"#postStatus": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":ownerId": &types.AttributeValueMemberS{Value: ownerID},
			":active":  &types.AttributeValueMemberS{Value: "active"},
		},
	})
	if err == nil {
		return nil
	}
	var conditionalFailure *types.ConditionalCheckFailedException
	if errors.As(err, &conditionalFailure) {
		return post.ErrNotFound
	}
	return post.ErrDependencyUnavailable
}

func attributeString(item map[string]types.AttributeValue, key string) string {
	value, ok := item[key].(*types.AttributeValueMemberS)
	if !ok {
		return ""
	}
	return value.Value
}

func attributeNumber(item map[string]types.AttributeValue, key string) (int64, bool) {
	value, ok := item[key].(*types.AttributeValueMemberN)
	if !ok {
		return 0, false
	}
	if value.Value == "" || value.Value[0] < '1' || value.Value[0] > '9' {
		return 0, false
	}
	for _, character := range value.Value[1:] {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	result, err := strconv.ParseInt(value.Value, 10, 64)
	if err != nil || result <= 0 {
		return 0, false
	}
	return result, true
}
