package dynamodb

import (
	"context"
	"fmt"

	"github.com/antoniuk-oleksandr/social-media/internal/post"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type API interface {
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
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
