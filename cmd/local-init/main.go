package main

import (
	"context"
	"errors"
	"fmt"
	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/fx"
	"log"
	"os"
)

type configLoader func(context.Context, string) (aws.Config, error)
type tableCreator interface {
	CreateTable(context.Context, *dynamodb.CreateTableInput, ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
}

func loadConfig(ctx context.Context, region string) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(region))
}
func run(ctx context.Context, load configLoader, makeClient func(aws.Config) tableCreator, print func(string, ...any) (int, error)) error {
	cfg, err := load(ctx, env("AWS_REGION", "us-east-1"))
	if err != nil {
		return err
	}
	if endpoint := os.Getenv("DYNAMODB_ENDPOINT"); endpoint != "" {
		cfg.BaseEndpoint = &endpoint
	}
	return initTables(ctx, makeClient(cfg), print)
}

func initTables(ctx context.Context, client tableCreator, print func(string, ...any) (int, error)) error {
	users := env("USERS_TABLE", "social-media-local-users")
	uniques := env("UNIQUES_TABLE", "social-media-local-uniques")
	refresh := env("REFRESH_TABLE", "social-media-local-refresh-sessions")
	posts := env("POSTS_TABLE", "social-media-local-posts")
	ads := env("ADS_TABLE", "social-media-local-ads")
	for _, table := range []struct {
		name                 string
		key                  string
		attributeDefinitions []types.AttributeDefinition
		indexes              []types.GlobalSecondaryIndex
	}{
		{name: users, key: "userId", attributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("userId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("email"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("username"), AttributeType: types.ScalarAttributeTypeS},
		}, indexes: []types.GlobalSecondaryIndex{
			{IndexName: aws.String("EmailIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("email"), KeyType: types.KeyTypeHash}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
			{IndexName: aws.String("UsernameIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("username"), KeyType: types.KeyTypeHash}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
		}},
		{name: uniques, key: "uniqueKey", attributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("uniqueKey"), AttributeType: types.ScalarAttributeTypeS}}},
		{name: refresh, key: "tokenHash", attributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("tokenHash"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sessionId"), AttributeType: types.ScalarAttributeTypeS},
		}, indexes: []types.GlobalSecondaryIndex{
			{IndexName: aws.String("SessionIdIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("sessionId"), KeyType: types.KeyTypeHash}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
		}},
		{name: posts, key: "postId", attributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("postId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("feedPartition"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("createdAtEpoch"), AttributeType: types.ScalarAttributeTypeN},
		}, indexes: []types.GlobalSecondaryIndex{
			{IndexName: aws.String("FeedPostsIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("feedPartition"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("createdAtEpoch"), KeyType: types.KeyTypeRange}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
		}},
		{name: ads, key: "adId", attributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("adId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("status"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("createdAtEpoch"), AttributeType: types.ScalarAttributeTypeN},
		}, indexes: []types.GlobalSecondaryIndex{
			{IndexName: aws.String("ActiveAdsIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("status"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("createdAtEpoch"), KeyType: types.KeyTypeRange}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
		}},
	} {
		_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{TableName: aws.String(table.name), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String(table.key), KeyType: types.KeyTypeHash}}, AttributeDefinitions: table.attributeDefinitions, BillingMode: types.BillingModePayPerRequest, GlobalSecondaryIndexes: table.indexes})
		var already *types.ResourceInUseException
		if err != nil && !errorAs(err, &already) {
			return err
		}
		if _, e := print("ready %s\n", table.name); e != nil {
			return e
		}
	}
	return nil
}

func main() {
	app := fx.New(
		fx.Provide(
			func() context.Context { return context.Background() },
			platformfx.LoadAWSConfig,
			fx.Annotate(platformfx.NewDynamoDBClient, fx.As(new(tableCreator))),
			func() func(string, ...any) (int, error) {
				return func(format string, args ...any) (int, error) { return fmt.Printf(format, args...) }
			},
		),
		fx.Invoke(func(ctx context.Context, client tableCreator, print func(string, ...any) (int, error)) error {
			return initTables(ctx, client, print)
		}),
	)
	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		panic(err)
	}
	if err := app.Stop(ctx); err != nil {
		log.Panic(err)
	}
}
func errorAs(err error, target any) bool { return errors.As(err, target) }
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
