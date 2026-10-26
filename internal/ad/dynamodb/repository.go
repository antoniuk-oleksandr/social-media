package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
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

func NewRepository(client *dynamodb.Client, tableName string) ad.Repository {
	return &Repository{Client: client, TableName: tableName}
}
func (r *Repository) Create(ctx context.Context, value ad.Advertisement) error {
	_, err := r.Client.PutItem(ctx, putInput(r.TableName, value, "attribute_not_exists(adId)"))
	if err != nil {
		return ad.ErrDependencyUnavailable
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, value ad.Advertisement) error {
	_, err := r.Client.PutItem(ctx, putInput(r.TableName, value, "attribute_exists(adId)"))
	if err != nil {
		var conditionErr *types.ConditionalCheckFailedException
		if errors.As(err, &conditionErr) {
			return ad.ErrNotFound
		}
		return ad.ErrDependencyUnavailable
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(r.TableName),
		Key:                 map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: id}},
		ConditionExpression: aws.String("attribute_exists(adId)"),
	})
	if err != nil {
		var conditionErr *types.ConditionalCheckFailedException
		if errors.As(err, &conditionErr) {
			return ad.ErrNotFound
		}
		return ad.ErrDependencyUnavailable
	}
	return nil
}

func putInput(tableName string, value ad.Advertisement, condition string) *dynamodb.PutItemInput {
	return &dynamodb.PutItemInput{TableName: aws.String(tableName), Item: map[string]types.AttributeValue{
		"adId": &types.AttributeValueMemberS{Value: value.ID}, "entityType": &types.AttributeValueMemberS{Value: value.Type}, "advertiser": &types.AttributeValueMemberS{Value: value.Advertiser}, "content": &types.AttributeValueMemberS{Value: value.Content}, "targetUrl": &types.AttributeValueMemberS{Value: value.TargetURL}, "startsAt": &types.AttributeValueMemberS{Value: value.StartsAt.Format(time.RFC3339Nano)}, "endsAt": &types.AttributeValueMemberS{Value: value.EndsAt.Format(time.RFC3339Nano)}, "startsAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(value.StartsAt.UnixMilli())}, "endsAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(value.EndsAt.UnixMilli())}, "status": &types.AttributeValueMemberS{Value: string(value.Status)}, "createdBy": &types.AttributeValueMemberS{Value: value.CreatedBy}, "createdAt": &types.AttributeValueMemberS{Value: value.CreatedAt.Format(time.RFC3339Nano)}, "createdAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(value.CreatedAtEpoch)},
	}, ConditionExpression: aws.String(condition)}
}

func (r *Repository) GetByID(ctx context.Context, id string) (ad.Advertisement, error) {
	output, err := r.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(r.TableName), Key: map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: id}}})
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	if len(output.Item) == 0 {
		return ad.Advertisement{}, ad.ErrNotFound
	}
	item := output.Item
	getString := func(name string) (string, error) {
		value, ok := item[name].(*types.AttributeValueMemberS)
		if !ok {
			return "", fmt.Errorf("missing string attribute %s", name)
		}
		return value.Value, nil
	}
	getNumber := func(name string) (int64, error) {
		value, ok := item[name].(*types.AttributeValueMemberN)
		if !ok {
			return 0, fmt.Errorf("missing number attribute %s", name)
		}
		var result int64
		if _, err := fmt.Sscan(value.Value, &result); err != nil {
			return 0, err
		}
		return result, nil
	}
	idValue, err := getString("adId")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	typeValue, err := getString("entityType")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	advertiser, err := getString("advertiser")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	content, err := getString("content")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	targetURL, err := getString("targetUrl")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	startsAt, err := getString("startsAt")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	endsAt, err := getString("endsAt")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	status, err := getString("status")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	createdBy, err := getString("createdBy")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	createdAt, err := getString("createdAt")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	starts, err := time.Parse(time.RFC3339Nano, startsAt)
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	ends, err := time.Parse(time.RFC3339Nano, endsAt)
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	createdEpoch, err := getNumber("createdAtEpoch")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	return ad.Advertisement{ID: idValue, Type: typeValue, Advertiser: advertiser, Content: content, TargetURL: targetURL, StartsAt: starts, EndsAt: ends, Status: ad.Status(status), CreatedBy: createdBy, CreatedAt: created, CreatedAtEpoch: createdEpoch}, nil
}
