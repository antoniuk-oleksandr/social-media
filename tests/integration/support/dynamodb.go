package support

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	tcdynamodb "github.com/testcontainers/testcontainers-go/modules/dynamodb"
)

type DynamoDBSuite struct {
	Container    *tcdynamodb.DynamoDBContainer
	Client       *dynamodb.Client
	UsersTable   string
	UniquesTable string
	RefreshTable string
}

func StartDynamoDB(ctx context.Context, prefix string) (*DynamoDBSuite, error) {
	container, err := runDynamoDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("start DynamoDB Local container: %w", err)
	}
	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		return nil, fmt.Errorf("get DynamoDB Local endpoint: %w", cleanupContainer(ctx, container, err))
	}
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithBaseEndpoint(dynamoDBEndpoint(endpoint)),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load DynamoDB Local config: %w", cleanupContainer(ctx, container, err))
	}
	suffix := time.Now().UTC().Format("20060102-150405.000000000")
	suite := &DynamoDBSuite{
		Container:    container,
		Client:       dynamodb.NewFromConfig(cfg),
		UsersTable:   fmt.Sprintf("%s-users-%s", prefix, suffix),
		UniquesTable: fmt.Sprintf("%s-uniques-%s", prefix, suffix),
		RefreshTable: fmt.Sprintf("%s-refresh-%s", prefix, suffix),
	}
	for _, table := range []struct {
		name string
		key  string
	}{
		{name: suite.UsersTable, key: "userId"},
		{name: suite.UniquesTable, key: "uniqueKey"},
		{name: suite.RefreshTable, key: "tokenHash"},
	} {
		if err := CreateTable(ctx, suite.Client, table.name, table.key); err != nil {
			return nil, fmt.Errorf("create table %q: %w", table.name, cleanupContainer(ctx, container, err))
		}
	}
	return suite, nil
}

func cleanupContainer(ctx context.Context, container *tcdynamodb.DynamoDBContainer, cause error) error {
	if cleanupErr := container.Terminate(ctx); cleanupErr != nil {
		return errors.Join(cause, fmt.Errorf("terminate container: %w", cleanupErr))
	}
	return cause
}

func dynamoDBEndpoint(hostPort string) string {
	return (&url.URL{Scheme: "http", Host: hostPort}).String()
}

func runDynamoDB(ctx context.Context) (container *tcdynamodb.DynamoDBContainer, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("start DynamoDB Local container: %v", recovered)
		}
	}()
	return tcdynamodb.Run(ctx, "amazon/dynamodb-local:2.6.1", tcdynamodb.WithDisableTelemetry())
}

func CreateTable(ctx context.Context, client *dynamodb.Client, tableName, hashKey string) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{{
			AttributeName: aws.String(hashKey), AttributeType: types.ScalarAttributeTypeS,
		}},
		KeySchema: []types.KeySchemaElement{{
			AttributeName: aws.String(hashKey), KeyType: types.KeyTypeHash,
		}},
		BillingMode: types.BillingModePayPerRequest,
	})
	return err
}

func (s *DynamoDBSuite) Close(ctx context.Context) error {
	return s.Container.Terminate(ctx)
}
