//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	postfeature "github.com/antoniuk-oleksandr/social-media/internal/post"
	postdb "github.com/antoniuk-oleksandr/social-media/internal/post/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var postsTable string

var _ = BeforeEach(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	postsTable = "tests-posts-" + time.Now().UTC().Format("20060102-150405.000000000")
	_, err := userIntegrationSuite.Client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(postsTable),
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
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("POST post persistence", func() {
	It("stores the complete post record in an isolated DynamoDB table", func() {
		id := "0192f8a0-7b00-7000-8000-000000000701"
		authorID := "0192f8a0-7b00-7000-8000-000000000702"
		createdAtEpoch := int64(1792326900123)
		value := postfeature.Post{ID: id, AuthorID: authorID, Content: "Hello 🌍", CreatedAtEpoch: createdAtEpoch}
		repository := &postdb.Repository{Client: userIntegrationSuite.Client, TableName: postsTable}
		Expect(repository.Create(context.Background(), value)).To(Succeed())

		stored, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{
			TableName: aws.String(postsTable),
			Key:       map[string]types.AttributeValue{"postId": &types.AttributeValueMemberS{Value: id}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Item["postId"]).To(Equal(&types.AttributeValueMemberS{Value: id}))
		Expect(stored.Item["authorId"]).To(Equal(&types.AttributeValueMemberS{Value: authorID}))
		Expect(stored.Item["content"]).To(Equal(&types.AttributeValueMemberS{Value: "Hello 🌍"}))
		Expect(stored.Item["entityType"]).To(Equal(&types.AttributeValueMemberS{Value: "post"}))
		Expect(stored.Item["status"]).To(Equal(&types.AttributeValueMemberS{Value: "active"}))
		Expect(stored.Item["feedPartition"]).To(Equal(&types.AttributeValueMemberS{Value: "GLOBAL"}))
		Expect(stored.Item["createdAtEpoch"]).To(Equal(&types.AttributeValueMemberN{Value: "1792326900123"}))
	})
})

var _ = Describe("GET post by UUIDv7 persistence", func() {
	It("round trips a created post with millisecond timestamp precision", func() {
		id := "0192f8a0-7b00-7000-8000-000000000721"
		authorID := "0192f8a0-7b00-7000-8000-000000000722"
		createdAt := time.Date(2026, 10, 24, 12, 0, 0, 123000000, time.UTC)
		repository := &postdb.Repository{Client: userIntegrationSuite.Client, TableName: postsTable}
		service := postfeature.NewService(repository, newIntegrationID(id), fixedIntegrationClock(createdAt))
		created, err := service.Create(context.Background(), postfeature.CreateCommand{AuthorID: authorID, Content: "Created with milliseconds"})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.CreatedAt).To(Equal(createdAt))
		Expect(created.CreatedAtEpoch).To(Equal(createdAt.UnixMilli()))

		stored, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(postsTable), Key: map[string]types.AttributeValue{"postId": &types.AttributeValueMemberS{Value: id}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Item["createdAt"]).To(Equal(&types.AttributeValueMemberS{Value: "2026-10-24T12:00:00.123Z"}))
		Expect(stored.Item["createdAtEpoch"]).To(Equal(&types.AttributeValueMemberN{Value: "1792843200123"}))

		read, err := service.GetByID(context.Background(), id)
		Expect(err).NotTo(HaveOccurred())
		Expect(read.CreatedAt).To(Equal(createdAt))
		Expect(read.CreatedAtEpoch).To(Equal(createdAt.UnixMilli()))

		handler := postfeature.NewHandler(service, nil)
		request := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+id, nil)
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("postId", id)
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
		response := httptest.NewRecorder()
		handler.GetByID(response, request)
		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Body.String()).To(MatchJSON(`{"id":"0192f8a0-7b00-7000-8000-000000000721","authorId":"0192f8a0-7b00-7000-8000-000000000722","content":"Created with milliseconds","createdAt":"2026-10-24T12:00:00.123Z"}`))
	})

	It("returns the active post DTO without authentication", func() {
		id := "0192f8a0-7b00-7000-8000-000000000711"
		authorID := "0192f8a0-7b00-7000-8000-000000000712"
		_, err := userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{TableName: aws.String(postsTable), Item: map[string]types.AttributeValue{
			"postId": &types.AttributeValueMemberS{Value: id}, "entityType": &types.AttributeValueMemberS{Value: "post"}, "authorId": &types.AttributeValueMemberS{Value: authorID},
			"content": &types.AttributeValueMemberS{Value: "Integration GET"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"},
			"createdAtEpoch": &types.AttributeValueMemberN{Value: "1792843200000"}, "feedPartition": &types.AttributeValueMemberS{Value: "GLOBAL"},
		}})
		Expect(err).NotTo(HaveOccurred())
		repository := &postdb.Repository{Client: userIntegrationSuite.Client, TableName: postsTable}
		handler := postfeature.NewHandler(postfeature.NewService(repository, nil, nil), nil)
		request := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+id, nil)
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("postId", id)
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
		response := httptest.NewRecorder()
		handler.GetByID(response, request)
		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Body.String()).To(MatchJSON(`{"id":"0192f8a0-7b00-7000-8000-000000000711","authorId":"0192f8a0-7b00-7000-8000-000000000712","content":"Integration GET","createdAt":"2026-10-24T12:00:00Z"}`))
	})

	It("translates missing and inactive posts to NOT_FOUND", func() {
		missingID := "0192f8a0-7b00-7000-8000-000000000713"
		inactiveID := "0192f8a0-7b00-7000-8000-000000000714"
		_, err := userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{TableName: aws.String(postsTable), Item: map[string]types.AttributeValue{
			"postId": &types.AttributeValueMemberS{Value: inactiveID}, "entityType": &types.AttributeValueMemberS{Value: "post"}, "authorId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000715"},
			"content": &types.AttributeValueMemberS{Value: "deleted"}, "status": &types.AttributeValueMemberS{Value: "deleted"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"},
		}})
		Expect(err).NotTo(HaveOccurred())
		repository := &postdb.Repository{Client: userIntegrationSuite.Client, TableName: postsTable}
		handler := postfeature.NewHandler(postfeature.NewService(repository, nil, nil), nil)
		for _, id := range []string{missingID, inactiveID} {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+id, nil)
			routeContext := chi.NewRouteContext()
			routeContext.URLParams.Add("postId", id)
			request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
			response := httptest.NewRecorder()
			handler.GetByID(response, request)
			Expect(response.Code).To(Equal(http.StatusNotFound))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"NOT_FOUND"`))
		}
	})
})

var _ = Describe("DELETE post persistence", func() {
	const ownerID = "0192f8a0-7b00-7000-8000-000000000731"
	const otherID = "0192f8a0-7b00-7000-8000-000000000732"

	putActivePost := func(id string) {
		_, err := userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(postsTable),
			Item: map[string]types.AttributeValue{
				"postId": &types.AttributeValueMemberS{Value: id}, "entityType": &types.AttributeValueMemberS{Value: "post"}, "authorId": &types.AttributeValueMemberS{Value: ownerID},
				"content": &types.AttributeValueMemberS{Value: "delete me"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"},
				"createdAtEpoch": &types.AttributeValueMemberN{Value: "1792843200000"}, "feedPartition": &types.AttributeValueMemberS{Value: "GLOBAL"},
			},
		})
		Expect(err).NotTo(HaveOccurred())
	}

	It("deletes the owner post and makes a repeated delete not found", func() {
		id := "0192f8a0-7b00-7000-8000-000000000733"
		putActivePost(id)
		repository := &postdb.Repository{Client: userIntegrationSuite.Client, TableName: postsTable}
		service := postfeature.NewService(repository, nil, nil)
		Expect(service.Delete(context.Background(), id, ownerID)).To(Succeed())
		Expect(service.Delete(context.Background(), id, ownerID)).To(MatchError(postfeature.ErrNotFound))
		stored, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(postsTable), Key: map[string]types.AttributeValue{"postId": &types.AttributeValueMemberS{Value: id}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Item).To(BeEmpty())
	})

	It("rejects a non-owner without deleting the existing post", func() {
		id := "0192f8a0-7b00-7000-8000-000000000734"
		putActivePost(id)
		repository := &postdb.Repository{Client: userIntegrationSuite.Client, TableName: postsTable}
		service := postfeature.NewService(repository, nil, nil)
		Expect(service.Delete(context.Background(), id, otherID)).To(MatchError(postfeature.ErrForbidden))
		stored, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(postsTable), Key: map[string]types.AttributeValue{"postId": &types.AttributeValueMemberS{Value: id}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Item["status"]).To(Equal(&types.AttributeValueMemberS{Value: "active"}))
	})

	It("maps a missing post to not found", func() {
		repository := &postdb.Repository{Client: userIntegrationSuite.Client, TableName: postsTable}
		service := postfeature.NewService(repository, nil, nil)
		Expect(service.Delete(context.Background(), "0192f8a0-7b00-7000-8000-000000000735", ownerID)).To(MatchError(postfeature.ErrNotFound))
	})
})
