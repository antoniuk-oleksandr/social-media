package dynamodb

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/post"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakePutAPI struct {
	input    *dynamodb.PutItemInput
	getInput *dynamodb.GetItemInput
	get      *dynamodb.GetItemOutput
	err      error
}

func (f *fakePutAPI) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.input = input
	return &dynamodb.PutItemOutput{}, f.err
}

func (f *fakePutAPI) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.getInput = input
	if f.get != nil {
		return f.get, f.err
	}
	return &dynamodb.GetItemOutput{}, f.err
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

func TestRepositoryCreateAndGetPreserveMillisecondTimestamp(t *testing.T) {
	api := &fakePutAPI{}
	repository := &Repository{Client: api, TableName: "posts-test"}
	createdAt := time.Date(2026, 10, 18, 12, 35, 0, 123000000, time.UTC)
	value := post.Post{ID: "0192c5f4-7b2a-7abd-8def-1234567890ab", AuthorID: "0192c5f4-7b2a-7abc-8def-1234567890ab", Content: "hello", CreatedAt: createdAt, CreatedAtEpoch: createdAt.UnixMilli()}
	if err := repository.Create(context.Background(), value); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := api.input.Item["createdAt"].(*types.AttributeValueMemberS).Value; got != "2026-10-18T12:35:00.123Z" {
		t.Fatalf("persisted createdAt = %q", got)
	}
	if got := api.input.Item["createdAtEpoch"].(*types.AttributeValueMemberN).Value; got != "1792326900123" {
		t.Fatalf("persisted createdAtEpoch = %q", got)
	}
	api.get = &dynamodb.GetItemOutput{Item: api.input.Item}
	read, err := repository.GetByID(context.Background(), value.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !read.CreatedAt.Equal(createdAt) || read.CreatedAtEpoch != value.CreatedAtEpoch {
		t.Fatalf("read timestamp = %s/%d, want %s/%d", read.CreatedAt, read.CreatedAtEpoch, createdAt, value.CreatedAtEpoch)
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

func TestRepositoryGetByIDMapsActiveRecord(t *testing.T) {
	id := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	authorID := "0192c5f4-7b2a-7abc-8def-1234567890ab"
	api := &fakePutAPI{get: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
		"postId": &types.AttributeValueMemberS{Value: id}, "entityType": &types.AttributeValueMemberS{Value: "post"},
		"authorId": &types.AttributeValueMemberS{Value: authorID}, "content": &types.AttributeValueMemberS{Value: "Hello"},
		"status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-18T12:35:00Z"},
		"createdAtEpoch": &types.AttributeValueMemberN{Value: "1792326900000"}, "feedPartition": &types.AttributeValueMemberS{Value: "GLOBAL"},
	}}}
	repository := &Repository{Client: api, TableName: "posts-test"}
	value, err := repository.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if api.getInput == nil || *api.getInput.TableName != "posts-test" || api.getInput.ConsistentRead == nil || !*api.getInput.ConsistentRead {
		t.Fatalf("GetItem input = %+v", api.getInput)
	}
	if got := api.getInput.Key["postId"].(*types.AttributeValueMemberS).Value; got != id {
		t.Fatalf("key postId = %q, want %q", got, id)
	}
	if value.ID != id || value.AuthorID != authorID || value.Content != "Hello" || value.CreatedAtEpoch != 1792326900000 {
		t.Fatalf("domain value = %+v", value)
	}
}

func TestRepositoryGetByIDMapsMissingAndInactiveToNotFound(t *testing.T) {
	id := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	for _, item := range []map[string]types.AttributeValue{
		nil,
		{"postId": &types.AttributeValueMemberS{Value: id}, "authorId": &types.AttributeValueMemberS{Value: "0192c5f4-7b2a-7abc-8def-1234567890ab"}, "content": &types.AttributeValueMemberS{Value: "deleted"}, "status": &types.AttributeValueMemberS{Value: "deleted"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-18T12:35:00Z"}},
	} {
		api := &fakePutAPI{get: &dynamodb.GetItemOutput{Item: item}}
		_, err := (&Repository{Client: api, TableName: "posts-test"}).GetByID(context.Background(), id)
		if !errors.Is(err, post.ErrNotFound) {
			t.Errorf("item %#v: error = %v, want ErrNotFound", item, err)
		}
	}
}

func TestRepositoryGetByIDTranslatesInfrastructureAndMalformedRecords(t *testing.T) {
	id := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	api := &fakePutAPI{err: errors.New("DynamoDB unavailable")}
	_, err := (&Repository{Client: api, TableName: "posts-test"}).GetByID(context.Background(), id)
	if !errors.Is(err, post.ErrDependencyUnavailable) {
		t.Fatalf("infrastructure error = %v, want dependency error", err)
	}
	api = &fakePutAPI{get: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
		"postId": &types.AttributeValueMemberS{Value: id}, "authorId": &types.AttributeValueMemberS{Value: "not-a-uuid"},
		"content": &types.AttributeValueMemberS{Value: "bad"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "bad-time"},
	}}}
	_, err = (&Repository{Client: api, TableName: "posts-test"}).GetByID(context.Background(), id)
	if !errors.Is(err, post.ErrDependencyUnavailable) {
		t.Fatalf("malformed record error = %v, want dependency error", err)
	}
}

func TestRepositoryGetByIDRejectsMalformedPersistenceRecords(t *testing.T) {
	id := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	base := map[string]types.AttributeValue{
		"postId":         &types.AttributeValueMemberS{Value: id},
		"entityType":     &types.AttributeValueMemberS{Value: "post"},
		"authorId":       &types.AttributeValueMemberS{Value: "0192c5f4-7b2a-7abc-8def-1234567890ab"},
		"content":        &types.AttributeValueMemberS{Value: "Hello"},
		"status":         &types.AttributeValueMemberS{Value: "active"},
		"createdAt":      &types.AttributeValueMemberS{Value: "2026-10-18T12:35:00Z"},
		"createdAtEpoch": &types.AttributeValueMemberN{Value: "1792326900000"},
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string]types.AttributeValue)
	}{
		{name: "wrong entity type", mutate: func(item map[string]types.AttributeValue) {
			item["entityType"] = &types.AttributeValueMemberS{Value: "comment"}
		}},
		{name: "requested and stored IDs mismatch", mutate: func(item map[string]types.AttributeValue) {
			item["postId"] = &types.AttributeValueMemberS{Value: "0192c5f4-7b2a-7abd-8def-1234567890ac"}
		}},
		{name: "missing CreatedAtEpoch", mutate: func(item map[string]types.AttributeValue) {
			delete(item, "createdAtEpoch")
		}},
		{name: "inconsistent CreatedAtEpoch", mutate: func(item map[string]types.AttributeValue) {
			item["createdAtEpoch"] = &types.AttributeValueMemberN{Value: "1792326900001"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := make(map[string]types.AttributeValue, len(base))
			for key, value := range base {
				item[key] = value
			}
			tc.mutate(item)
			api := &fakePutAPI{get: &dynamodb.GetItemOutput{Item: item}}
			_, err := (&Repository{Client: api, TableName: "posts-test"}).GetByID(context.Background(), id)
			if !errors.Is(err, post.ErrDependencyUnavailable) {
				t.Fatalf("error = %v, want dependency error", err)
			}
		})
	}
}

func TestRepositoryGetByIDRejectsNonCanonicalCreatedAtEpoch(t *testing.T) {
	id := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	base := map[string]types.AttributeValue{
		"postId":        &types.AttributeValueMemberS{Value: id},
		"entityType":    &types.AttributeValueMemberS{Value: "post"},
		"authorId":      &types.AttributeValueMemberS{Value: "0192c5f4-7b2a-7abc-8def-1234567890ab"},
		"content":       &types.AttributeValueMemberS{Value: "Hello"},
		"status":        &types.AttributeValueMemberS{Value: "active"},
		"createdAt":     &types.AttributeValueMemberS{Value: "2026-10-18T12:35:00Z"},
		"feedPartition": &types.AttributeValueMemberS{Value: "GLOBAL"},
	}
	for _, tc := range []struct {
		name   string
		number string
	}{
		{name: "numeric suffix", number: "1792326900000abc"},
		{name: "leading whitespace", number: " 1792326900000"},
		{name: "trailing whitespace", number: "1792326900000 "},
		{name: "embedded whitespace", number: "179232 6900000"},
		{name: "decimal", number: "1792326900000.0"},
		{name: "plus sign", number: "+1792326900000"},
		{name: "minus sign", number: "-1792326900000"},
		{name: "leading zero", number: "01792326900000"},
		{name: "zero", number: "0"},
		{name: "int64 overflow", number: "9223372036854775808"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := make(map[string]types.AttributeValue, len(base)+1)
			for key, value := range base {
				item[key] = value
			}
			item["createdAtEpoch"] = &types.AttributeValueMemberN{Value: tc.number}
			_, err := (&Repository{Client: &fakePutAPI{get: &dynamodb.GetItemOutput{Item: item}}, TableName: "posts-test"}).GetByID(context.Background(), id)
			if err != post.ErrDependencyUnavailable {
				t.Fatalf("error = %v, want exactly ErrDependencyUnavailable", err)
			}
		})
	}
}

func TestToDomainRejectsUntrustedPersistedPostFields(t *testing.T) {
	id := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	base := PostRecord{ID: id, EntityType: "post", AuthorID: "0192c5f4-7b2a-7abc-8def-1234567890ab", Content: "Hello", Status: "active", CreatedAt: "2026-10-18T12:35:00.123Z", CreatedAtEpoch: 1792326900123, FeedPartition: "GLOBAL"}
	for _, tc := range []struct {
		name   string
		mutate func(*PostRecord)
		leak   string
	}{
		{name: "timestamp with offset", mutate: func(record *PostRecord) { record.CreatedAt = "2026-10-18T13:35:00.123+01:00" }, leak: "+01:00"},
		{name: "sub-millisecond timestamp", mutate: func(record *PostRecord) { record.CreatedAt = "2026-10-18T12:35:00.123456Z" }, leak: ".123456Z"},
		{name: "noncanonical timestamp string", mutate: func(record *PostRecord) { record.CreatedAt = "2026-10-18T12:35:00.1230Z" }, leak: ".1230Z"},
		{name: "whitespace-only content", mutate: func(record *PostRecord) { record.Content = " 	\n " }, leak: "storage detail"},
		{name: "content over 500 Unicode code points", mutate: func(record *PostRecord) { record.Content = strings.Repeat("界", 501) }, leak: "界"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := base
			tc.mutate(&record)
			_, err := toDomain(record, id)
			if err == nil {
				t.Fatal("toDomain() error = nil, want malformed record error")
			}
			if strings.Contains(err.Error(), tc.leak) {
				t.Fatalf("toDomain() leaked storage detail %q in %q", tc.leak, err)
			}
		})
	}
}

func TestRepositoryGetByIDTranslatesUntrustedPersistedPostFields(t *testing.T) {
	id := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	base := map[string]types.AttributeValue{
		"postId": &types.AttributeValueMemberS{Value: id}, "entityType": &types.AttributeValueMemberS{Value: "post"},
		"authorId": &types.AttributeValueMemberS{Value: "0192c5f4-7b2a-7abc-8def-1234567890ab"}, "content": &types.AttributeValueMemberS{Value: "Hello"},
		"status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-18T12:35:00.123Z"},
		"createdAtEpoch": &types.AttributeValueMemberN{Value: "1792326900123"}, "feedPartition": &types.AttributeValueMemberS{Value: "GLOBAL"},
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string]types.AttributeValue)
		leak   string
	}{
		{name: "timestamp with offset", mutate: func(item map[string]types.AttributeValue) {
			item["createdAt"] = &types.AttributeValueMemberS{Value: "2026-10-18T13:35:00.123+01:00"}
		}, leak: "+01:00"},
		{name: "sub-millisecond timestamp", mutate: func(item map[string]types.AttributeValue) {
			item["createdAt"] = &types.AttributeValueMemberS{Value: "2026-10-18T12:35:00.123456Z"}
		}, leak: ".123456Z"},
		{name: "noncanonical timestamp string", mutate: func(item map[string]types.AttributeValue) {
			item["createdAt"] = &types.AttributeValueMemberS{Value: "2026-10-18T12:35:00.1230Z"}
		}, leak: ".1230Z"},
		{name: "whitespace-only content", mutate: func(item map[string]types.AttributeValue) {
			item["content"] = &types.AttributeValueMemberS{Value: " 	\n "}
		}, leak: "storage detail"},
		{name: "content over 500 Unicode code points", mutate: func(item map[string]types.AttributeValue) {
			item["content"] = &types.AttributeValueMemberS{Value: strings.Repeat("界", 501)}
		}, leak: "界"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := make(map[string]types.AttributeValue, len(base))
			for key, value := range base {
				item[key] = value
			}
			tc.mutate(item)
			_, err := (&Repository{Client: &fakePutAPI{get: &dynamodb.GetItemOutput{Item: item}}, TableName: "posts-test"}).GetByID(context.Background(), id)
			if err != post.ErrDependencyUnavailable {
				t.Fatalf("error = %v, want exactly ErrDependencyUnavailable", err)
			}
			if strings.Contains(err.Error(), tc.leak) {
				t.Fatalf("repository error leaked storage detail %q in %q", tc.leak, err)
			}
		})
	}
}
