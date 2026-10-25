package dynamodb

import (
	"context"
	"errors"
	"testing"

	"github.com/antoniuk-oleksandr/social-media/internal/post"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakePutAPI struct {
	input *dynamodb.PutItemInput
	err   error
}

func (f *fakePutAPI) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.input = input
	return &dynamodb.PutItemOutput{}, f.err
}

func TestRepositoryCreateMapsPostAndUsesConditionalPut(t *testing.T) {
	api := &fakePutAPI{}
	repository := &Repository{Client: api, TableName: "posts-test"}
	value := post.Post{ID: "0192c5f4-7b2a-7abd-8def-1234567890ab", AuthorID: "0192c5f4-7b2a-7abc-8def-1234567890ab", Content: "hello", CreatedAtEpoch: 1792326900123}
	if err := repository.Create(context.Background(), value); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if api.input == nil || *api.input.TableName != "posts-test" || *api.input.ConditionExpression != "attribute_not_exists(postId)" {
		t.Fatalf("PutItem input = %+v", api.input)
	}
	assertStringAttribute(t, api.input.Item, "postId", value.ID)
	assertStringAttribute(t, api.input.Item, "entityType", "post")
	assertStringAttribute(t, api.input.Item, "authorId", value.AuthorID)
	assertStringAttribute(t, api.input.Item, "content", value.Content)
	assertStringAttribute(t, api.input.Item, "status", "active")
	assertStringAttribute(t, api.input.Item, "feedPartition", "GLOBAL")
	if got := api.input.Item["createdAtEpoch"].(*types.AttributeValueMemberN).Value; got != "1792326900123" {
		t.Fatalf("createdAtEpoch = %s", got)
	}
}

func TestRepositoryCreateTranslatesInfrastructureFailure(t *testing.T) {
	api := &fakePutAPI{err: errors.New("DynamoDB unavailable")}
	repository := &Repository{Client: api, TableName: "posts-test"}
	err := repository.Create(context.Background(), post.Post{ID: "0192c5f4-7b2a-7abd-8def-1234567890ab"})
	if !errors.Is(err, post.ErrDependencyUnavailable) {
		t.Fatalf("error = %v, want dependency error", err)
	}
}

func TestNewRepositoryUsesConfiguredTable(t *testing.T) {
	repository, ok := NewRepository(nil, "configured-posts").(*Repository)
	if !ok {
		t.Fatalf("repository type = %T", repository)
	}
	if repository.TableName != "configured-posts" {
		t.Fatalf("repository = %+v", repository)
	}
}

func assertStringAttribute(t *testing.T, item map[string]types.AttributeValue, key, want string) {
	t.Helper()
	value, ok := item[key].(*types.AttributeValueMemberS)
	if !ok || value.Value != want {
		t.Fatalf("%s = %#v, want %q", key, item[key], want)
	}
}
