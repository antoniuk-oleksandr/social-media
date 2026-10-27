package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/post"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type queryClient struct {
	pages  []*dynamodb.QueryOutput
	errors []error
	calls  int
}

func (c *queryClient) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	index := c.calls
	c.calls++
	if index < len(c.errors) && c.errors[index] != nil {
		return nil, c.errors[index]
	}
	return c.pages[index], nil
}

func validPostItem(id string, created time.Time) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"postId":         &types.AttributeValueMemberS{Value: id},
		"authorId":       &types.AttributeValueMemberS{Value: "author"},
		"content":        &types.AttributeValueMemberS{Value: "content"},
		"status":         &types.AttributeValueMemberS{Value: "active"},
		"createdAt":      &types.AttributeValueMemberS{Value: created.Format(time.RFC3339Nano)},
		"createdAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(created.UnixMilli())},
	}
}

var _ = Describe("feed post repository", func() {
	It("queries multiple pages, filters by snapshot, and sorts results", func() {
		old := time.Date(2026, 10, 26, 11, 0, 0, 0, time.UTC)
		boundary := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
		client := &queryClient{pages: []*dynamodb.QueryOutput{
			{Items: []map[string]types.AttributeValue{
				validPostItem("post-old", old),
				validPostItem("post-later", boundary.Add(time.Minute)),
				{"postId": &types.AttributeValueMemberS{Value: "inactive"}, "authorId": &types.AttributeValueMemberS{Value: "author"}, "content": &types.AttributeValueMemberS{Value: "content"}, "status": &types.AttributeValueMemberS{Value: "deleted"}, "createdAt": &types.AttributeValueMemberS{Value: old.Format(time.RFC3339Nano)}, "createdAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(old.UnixMilli())}},
			}, LastEvaluatedKey: map[string]types.AttributeValue{"postId": &types.AttributeValueMemberS{Value: "continue"}}},
			{Items: []map[string]types.AttributeValue{
				validPostItem("post-boundary", boundary),
				validPostItem("post-tie-b", boundary),
				validPostItem("post-tie-a", boundary),
			}},
		}}

		repository := &PostRepository{Client: client, TableName: "posts"}
		items, err := repository.List(context.Background(), boundary)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(4))
		Expect(items[0].ID).To(Equal("post-tie-b"))
		Expect(items[1].ID).To(Equal("post-tie-a"))
		Expect(items[2].ID).To(Equal("post-boundary"))
		Expect(items[3].ID).To(Equal("post-old"))
		Expect(client.calls).To(Equal(2))
	})

	It("returns query and mapping errors", func() {
		queryErr := errors.New("query failed")
		client := &queryClient{errors: []error{queryErr}}
		_, err := (&PostRepository{Client: client, TableName: "posts"}).List(context.Background(), time.Now())
		Expect(err).To(MatchError(queryErr))

		bad := validPostItem("bad", time.Now())
		bad["createdAtEpoch"] = &types.AttributeValueMemberN{Value: "not-a-number"}
		client = &queryClient{pages: []*dynamodb.QueryOutput{{Items: []map[string]types.AttributeValue{bad}}}}
		_, err = (&PostRepository{Client: client, TableName: "posts"}).List(context.Background(), time.Now())
		Expect(err).To(MatchError("strconv.ParseInt: parsing \"not-a-number\": invalid syntax"))
	})

	It("maps valid items and rejects malformed items", func() {
		created := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
		mapped, err := mapPost(validPostItem("post", created))
		Expect(err).NotTo(HaveOccurred())
		Expect(mapped).To(Equal(post.Post{ID: "post", AuthorID: "author", Content: "content", CreatedAt: created, CreatedAtEpoch: created.UnixMilli()}))

		cases := []map[string]types.AttributeValue{
			{},
			{"postId": &types.AttributeValueMemberN{Value: "1"}},
			{"postId": &types.AttributeValueMemberS{Value: "id"}, "authorId": &types.AttributeValueMemberN{Value: "1"}},
			{"postId": &types.AttributeValueMemberS{Value: "id"}, "authorId": &types.AttributeValueMemberS{Value: "author"}, "content": &types.AttributeValueMemberN{Value: "1"}},
			{"postId": &types.AttributeValueMemberS{Value: "id"}, "authorId": &types.AttributeValueMemberS{Value: "author"}, "content": &types.AttributeValueMemberS{Value: "content"}, "status": &types.AttributeValueMemberS{Value: "deleted"}},
			{"postId": &types.AttributeValueMemberS{Value: "id"}, "authorId": &types.AttributeValueMemberS{Value: "author"}, "content": &types.AttributeValueMemberS{Value: "content"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: "1"}},
			{"postId": &types.AttributeValueMemberS{Value: "id"}, "authorId": &types.AttributeValueMemberS{Value: "author"}, "content": &types.AttributeValueMemberS{Value: "content"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "invalid"}, "createdAtEpoch": &types.AttributeValueMemberN{Value: "1"}},
		}
		for _, item := range cases {
			_, err = mapPost(item)
			Expect(err).To(HaveOccurred())
		}
	})

	It("constructs a post repository", func() {
		Expect(NewPostRepository(nil, "posts")).NotTo(BeNil())
	})
})
