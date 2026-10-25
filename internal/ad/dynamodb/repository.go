package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
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

func NewRepository(client *dynamodb.Client, tableName string) ad.Repository {
	return &Repository{Client: client, TableName: tableName}
}
func (r *Repository) Create(ctx context.Context, value ad.Advertisement) error {
	_, err := r.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(r.TableName), Item: map[string]types.AttributeValue{
		"adId": &types.AttributeValueMemberS{Value: value.ID}, "entityType": &types.AttributeValueMemberS{Value: value.Type}, "advertiser": &types.AttributeValueMemberS{Value: value.Advertiser}, "content": &types.AttributeValueMemberS{Value: value.Content}, "targetUrl": &types.AttributeValueMemberS{Value: value.TargetURL}, "startsAt": &types.AttributeValueMemberS{Value: value.StartsAt.Format("2006-01-02T15:04:05Z07:00")}, "endsAt": &types.AttributeValueMemberS{Value: value.EndsAt.Format("2006-01-02T15:04:05Z07:00")}, "startsAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(value.StartsAt.UnixMilli())}, "endsAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(value.EndsAt.UnixMilli())}, "status": &types.AttributeValueMemberS{Value: string(value.Status)}, "createdBy": &types.AttributeValueMemberS{Value: value.CreatedBy}, "createdAt": &types.AttributeValueMemberS{Value: value.CreatedAt.Format(time.RFC3339Nano)}, "createdAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(value.CreatedAtEpoch)},
	}, ConditionExpression: aws.String("attribute_not_exists(adId)")})
	if err != nil {
		return ad.ErrDependencyUnavailable
	}
	return nil
}
