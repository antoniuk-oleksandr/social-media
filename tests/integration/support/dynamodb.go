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
	"github.com/testcontainers/testcontainers-go"
	tcdynamodb "github.com/testcontainers/testcontainers-go/modules/dynamodb"
	"github.com/testcontainers/testcontainers-go/network"
)

type DynamoDBSuite struct {
	Container    *tcdynamodb.DynamoDBContainer
	Client       *dynamodb.Client
	UsersTable   string
	UniquesTable string
	RefreshTable string
}

func StartDynamoDB(ctx context.Context, prefix string) (*DynamoDBSuite, error) {
	return startDynamoDB(ctx, prefix)
}

// StartDynamoDBOnNetwork starts DynamoDB Local on an existing Docker network.
// It is used by local SAM E2E tests so Lambda containers can resolve the
// database through the stable "dynamodb-local" network alias.
func StartDynamoDBOnNetwork(ctx context.Context, prefix, networkName string) (*DynamoDBSuite, error) {
	return startDynamoDB(ctx, prefix,
		network.WithNetworkName([]string{"dynamodb-local"}, networkName),
		testcontainers.WithCmdArgs("-sharedDb"),
	)
}

func startDynamoDB(ctx context.Context, prefix string, options ...testcontainers.ContainerCustomizer) (*DynamoDBSuite, error) {
	container, err := runDynamoDB(ctx, options...)
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
		Container: container,
		// DynamoDB Local emits checksum headers that do not match the response
		// body expected by the AWS SDK. Disable validation only for this local
		// Testcontainers client; production AWS clients keep validation enabled.
		Client: dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
			options.DisableValidateResponseChecksum = true
		}),
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

func runDynamoDB(ctx context.Context, options ...testcontainers.ContainerCustomizer) (container *tcdynamodb.DynamoDBContainer, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("start DynamoDB Local container: %v", recovered)
		}
	}()
	return tcdynamodb.Run(ctx, "amazon/dynamodb-local:2.6.1", append(options, tcdynamodb.WithDisableTelemetry())...)
}

func CreateTable(ctx context.Context, client *dynamodb.Client, tableName, hashKey string) error {
	return createUserTable(ctx, client, tableName, hashKey, types.ScalarAttributeTypeS)
}

func createUserTable(ctx context.Context, client *dynamodb.Client, tableName, hashKey string, keyType types.ScalarAttributeType) error {
	input := &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{{
			AttributeName: aws.String(hashKey), AttributeType: keyType,
		}},
		KeySchema: []types.KeySchemaElement{{
			AttributeName: aws.String(hashKey), KeyType: types.KeyTypeHash,
		}},
		BillingMode: types.BillingModePayPerRequest,
	}
	if hashKey == "userId" {
		input.AttributeDefinitions = append(input.AttributeDefinitions,
			types.AttributeDefinition{AttributeName: aws.String("email"), AttributeType: types.ScalarAttributeTypeS},
			types.AttributeDefinition{AttributeName: aws.String("username"), AttributeType: types.ScalarAttributeTypeS},
		)
		input.GlobalSecondaryIndexes = []types.GlobalSecondaryIndex{
			{IndexName: aws.String("EmailIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("email"), KeyType: types.KeyTypeHash}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
			{IndexName: aws.String("UsernameIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("username"), KeyType: types.KeyTypeHash}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
		}
	}
	if hashKey == "tokenHash" {
		input.AttributeDefinitions = append(input.AttributeDefinitions,
			types.AttributeDefinition{AttributeName: aws.String("sessionId"), AttributeType: types.ScalarAttributeTypeS},
		)
		input.GlobalSecondaryIndexes = []types.GlobalSecondaryIndex{
			{IndexName: aws.String("SessionIdIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("sessionId"), KeyType: types.KeyTypeHash}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
		}
	}
	_, err := client.CreateTable(ctx, input)
	return err
}

func (s *DynamoDBSuite) Close(ctx context.Context) error {
	return s.Container.Terminate(ctx)
}
