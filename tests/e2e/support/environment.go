//go:build e2e

package support

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/password"
	"github.com/antoniuk-oleksandr/social-media/tests/fixtures"
	"github.com/antoniuk-oleksandr/social-media/tests/integration/support"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

type Environment struct {
	Suite            *support.DynamoDBSuite
	Network          *testcontainers.DockerNetwork
	DynamoDBEndpoint string
	PostsTable       string
	AdsTable         string
}

func (e *Environment) SeedAdmin(ctx context.Context, id string, credentials fixtures.Credentials) error {
	hash, err := (password.Bcrypt{}).Hash(credentials.Password)
	if err != nil {
		return fmt.Errorf("hash E2E admin password: %w", err)
	}
	_, err = e.Suite.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(e.Suite.UsersTable),
		Item: map[string]types.AttributeValue{
			"userId":       &types.AttributeValueMemberS{Value: id},
			"username":     &types.AttributeValueMemberS{Value: credentials.Username},
			"email":        &types.AttributeValueMemberS{Value: credentials.Email},
			"passwordHash": &types.AttributeValueMemberS{Value: hash},
			"displayName":  &types.AttributeValueMemberS{Value: credentials.DisplayName},
			"role":         &types.AttributeValueMemberS{Value: "admin"},
			"status":       &types.AttributeValueMemberS{Value: "active"},
			"createdAt":    &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
	})
	return err
}

func Start(ctx context.Context) (*Environment, error) {
	nw, err := network.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create E2E Docker network: %w", err)
	}
	suite, err := support.StartDynamoDBOnNetwork(ctx, "tests-e2e", nw.Name)
	if err != nil {
		_ = nw.Remove(ctx)
		return nil, err
	}
	connection, err := suite.Container.ConnectionString(ctx)
	if err != nil {
		_ = suite.Close(ctx)
		_ = nw.Remove(ctx)
		return nil, fmt.Errorf("get E2E DynamoDB endpoint: %w", err)
	}
	_, port, err := net.SplitHostPort(connection)
	if err != nil {
		_ = suite.Close(ctx)
		_ = nw.Remove(ctx)
		return nil, fmt.Errorf("parse E2E DynamoDB endpoint %q: %w", connection, err)
	}
	environment := &Environment{
		Suite:            suite,
		Network:          nw,
		DynamoDBEndpoint: "http://host.docker.internal:" + port,
		PostsTable:       "tests-e2e-posts",
		AdsTable:         "tests-e2e-ads",
	}
	// SAM's local template uses these stable names. The entire E2E run owns
	// this fresh DynamoDB Local container, so stable names remain isolated.
	suite.UsersTable = "social-media-local-users"
	suite.UniquesTable = "social-media-local-uniques"
	suite.RefreshTable = "social-media-local-refresh-sessions"
	if err := createUsersTable(ctx, suite.Client, suite.UsersTable); err != nil {
		_ = environment.Close(ctx)
		return nil, fmt.Errorf("create E2E users table: %w", err)
	}
	if err := createUniquesTable(ctx, suite.Client, suite.UniquesTable); err != nil {
		_ = environment.Close(ctx)
		return nil, fmt.Errorf("create E2E uniques table: %w", err)
	}
	if err := createRefreshTable(ctx, suite.Client, suite.RefreshTable); err != nil {
		_ = environment.Close(ctx)
		return nil, fmt.Errorf("create E2E refresh table: %w", err)
	}
	environment.PostsTable = "social-media-local-posts"
	environment.AdsTable = "social-media-local-ads"
	if err := createPostsTable(ctx, suite.Client, environment.PostsTable); err != nil {
		_ = environment.Close(ctx)
		return nil, fmt.Errorf("create E2E posts table: %w", err)
	}
	if err := createAdsTable(ctx, suite.Client, environment.AdsTable); err != nil {
		_ = environment.Close(ctx)
		return nil, fmt.Errorf("create E2E ads table: %w", err)
	}
	for _, table := range []string{suite.UsersTable, suite.UniquesTable, suite.RefreshTable, environment.PostsTable, environment.AdsTable} {
		if err := waitForTable(ctx, suite.Client, table); err != nil {
			_ = environment.Close(ctx)
			return nil, fmt.Errorf("wait for E2E table %q: %w", table, err)
		}
	}
	return environment, nil
}

func (e *Environment) Close(ctx context.Context) error {
	var err error
	if e.Suite != nil {
		err = e.Suite.Close(ctx)
	}
	if e.Network != nil {
		if closeErr := e.Network.Remove(ctx); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

func createPostsTable(ctx context.Context, client *dynamodb.Client, name string) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(name),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("postId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("feedPartition"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("createdAtEpoch"), AttributeType: types.ScalarAttributeTypeN},
		},
		KeySchema:   []types.KeySchemaElement{{AttributeName: aws.String("postId"), KeyType: types.KeyTypeHash}},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName: aws.String("FeedPostsIndex"),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("feedPartition"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("createdAtEpoch"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	return err
}

func createUsersTable(ctx context.Context, client *dynamodb.Client, name string) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(name),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("userId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("email"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("username"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema:   []types.KeySchemaElement{{AttributeName: aws.String("userId"), KeyType: types.KeyTypeHash}},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{IndexName: aws.String("EmailIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("email"), KeyType: types.KeyTypeHash}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
			{IndexName: aws.String("UsernameIndex"), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("username"), KeyType: types.KeyTypeHash}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
		},
	})
	return err
}

func createUniquesTable(ctx context.Context, client *dynamodb.Client, name string) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:            aws.String(name),
		AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("uniqueKey"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("uniqueKey"), KeyType: types.KeyTypeHash}},
		BillingMode:          types.BillingModePayPerRequest,
	})
	return err
}

func createRefreshTable(ctx context.Context, client *dynamodb.Client, name string) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(name),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("tokenHash"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sessionId"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema:   []types.KeySchemaElement{{AttributeName: aws.String("tokenHash"), KeyType: types.KeyTypeHash}},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName:  aws.String("SessionIdIndex"),
			KeySchema:  []types.KeySchemaElement{{AttributeName: aws.String("sessionId"), KeyType: types.KeyTypeHash}},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	return err
}

func createAdsTable(ctx context.Context, client *dynamodb.Client, name string) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(name),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("adId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("status"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("createdAtEpoch"), AttributeType: types.ScalarAttributeTypeN},
		},
		KeySchema:   []types.KeySchemaElement{{AttributeName: aws.String("adId"), KeyType: types.KeyTypeHash}},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName: aws.String("ActiveAdsIndex"),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("status"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("createdAtEpoch"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	return err
}

func waitForTable(ctx context.Context, client *dynamodb.Client, name string) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		output, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(name)})
		if err == nil && output.Table != nil && output.Table.TableStatus == types.TableStatusActive {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("table did not become ACTIVE")
}
