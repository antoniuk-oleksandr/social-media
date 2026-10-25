//go:build integration

package userintegration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	userfeature "github.com/antoniuk-oleksandr/social-media/internal/user"
	userdb "github.com/antoniuk-oleksandr/social-media/internal/user/dynamodb"
	"github.com/antoniuk-oleksandr/social-media/tests/integration/support"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var userIntegrationSuite *support.DynamoDBSuite

func TestUserIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "User integration suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var err error
	userIntegrationSuite, err = support.StartDynamoDB(ctx, "tests-users-by-id")
	Expect(err).NotTo(HaveOccurred(), "DynamoDB integration suite must start")
})

var _ = AfterSuite(func() {
	if userIntegrationSuite != nil {
		Expect(userIntegrationSuite.Close(context.Background())).To(Succeed())
	}
})

var _ = Describe("GET user by UUIDv7 persistence", func() {
	It("reads a String-keyed user and returns only its public profile", func() {
		id := "0192f8a0-7b00-7000-8000-000000000001"
		_, err := userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(userIntegrationSuite.UsersTable),
			Item: map[string]types.AttributeValue{
				"userId": &types.AttributeValueMemberS{Value: id}, "username": &types.AttributeValueMemberS{Value: "integration_user"},
				"email": &types.AttributeValueMemberS{Value: "private@example.com"}, "passwordHash": &types.AttributeValueMemberS{Value: "private-hash"},
				"displayName": &types.AttributeValueMemberS{Value: "Integration User"}, "role": &types.AttributeValueMemberS{Value: "user"},
				"status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		repository := userdb.NewConfiguredRepository(userIntegrationSuite.Client, userIntegrationSuite.UsersTable)
		handler := userfeature.NewHandler(userfeature.NewService(repository))
		request := withUserRouteParam(httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil), id)
		response := httptest.NewRecorder()
		handler.GetByID(response, request)
		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Body.String()).To(MatchJSON(`{"id":"0192f8a0-7b00-7000-8000-000000000001","username":"integration_user","displayName":"Integration User","role":"user","createdAt":"2026-10-24T12:00:00Z"}`))
	})

	It("returns NOT_FOUND for an absent valid UUIDv7", func() {
		id := "0192f8a0-7b00-7000-8000-000000000002"
		repository := userdb.NewConfiguredRepository(userIntegrationSuite.Client, userIntegrationSuite.UsersTable)
		handler := userfeature.NewHandler(userfeature.NewService(repository))
		request := withUserRouteParam(httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil), id)
		response := httptest.NewRecorder()
		handler.GetByID(response, request)
		Expect(response.Code).To(Equal(http.StatusNotFound))
	})

	DescribeTable("rejects non-canonical user IDs", func(id string) {
		repository := userdb.NewConfiguredRepository(userIntegrationSuite.Client, userIntegrationSuite.UsersTable)
		handler := userfeature.NewHandler(userfeature.NewService(repository))
		request := withUserRouteParam(httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil), id)
		response := httptest.NewRecorder()
		handler.GetByID(response, request)
		Expect(response.Code).To(Equal(http.StatusUnprocessableEntity))
	},
		Entry("numeric", "123"),
		Entry("wrong UUID version", "0192f8a0-7b00-6000-8000-000000000001"),
		Entry("uppercase", "0192F8A0-7B00-7000-8000-000000000001"),
	)
})

func withUserRouteParam(request *http.Request, id string) *http.Request {
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("userId", id)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, ctx))
}
