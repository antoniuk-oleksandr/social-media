//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	userfeature "github.com/antoniuk-oleksandr/social-media/internal/user"
	userdb "github.com/antoniuk-oleksandr/social-media/internal/user/dynamodb"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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
		repository := userdb.NewConfiguredRepository(userIntegrationSuite.Client, userIntegrationSuite.UsersTable, userIntegrationSuite.UniquesTable)
		handler := userfeature.NewHandler(userfeature.NewService(repository))
		request := withUserRouteParam(httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil), id)
		response := httptest.NewRecorder()
		handler.GetByID(response, request)
		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Body.String()).To(MatchJSON(`{"id":"0192f8a0-7b00-7000-8000-000000000001","username":"integration_user","displayName":"Integration User","role":"user","createdAt":"2026-10-24T12:00:00Z"}`))
	})

	It("returns NOT_FOUND for an absent valid UUIDv7", func() {
		id := "0192f8a0-7b00-7000-8000-000000000002"
		repository := userdb.NewConfiguredRepository(userIntegrationSuite.Client, userIntegrationSuite.UsersTable, userIntegrationSuite.UniquesTable)
		handler := userfeature.NewHandler(userfeature.NewService(repository))
		request := withUserRouteParam(httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil), id)
		response := httptest.NewRecorder()
		handler.GetByID(response, request)
		Expect(response.Code).To(Equal(http.StatusNotFound))
	})

	DescribeTable("rejects non-canonical user IDs", func(id string) {
		repository := userdb.NewConfiguredRepository(userIntegrationSuite.Client, userIntegrationSuite.UsersTable, userIntegrationSuite.UniquesTable)
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

var _ = Describe("PATCH user profile persistence", func() {
	It("updates display name and reserves a normalized username atomically", func() {
		id := "0192f8a0-7b00-7000-8000-000000000101"
		oldUsername := "integration_old"
		newUsername := "integration_new"
		displayName := "Updated Integration User"
		originalUpdatedAt := "2026-10-24T11:00:00Z"
		fixedUpdatedAt := "2026-10-25T12:34:56Z"
		_, err := userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(userIntegrationSuite.UsersTable),
			Item: map[string]types.AttributeValue{
				"userId": &types.AttributeValueMemberS{Value: id}, "username": &types.AttributeValueMemberS{Value: oldUsername},
				"email": &types.AttributeValueMemberS{Value: "patch-101@example.com"}, "passwordHash": &types.AttributeValueMemberS{Value: "hash"},
				"displayName": &types.AttributeValueMemberS{Value: "Original Name"}, "role": &types.AttributeValueMemberS{Value: "user"},
				"status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"}, "updatedAt": &types.AttributeValueMemberS{Value: originalUpdatedAt},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(userIntegrationSuite.UniquesTable),
			Item:      map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#" + oldUsername}, "userId": &types.AttributeValueMemberS{Value: id}},
		})
		Expect(err).NotTo(HaveOccurred())

		repository := userdb.NewConfiguredRepositoryWithClock(userIntegrationSuite.Client, userIntegrationSuite.UsersTable, userIntegrationSuite.UniquesTable, platformclock.Fixed{T: time.Date(2026, 10, 25, 12, 34, 56, 0, time.UTC)})
		updated, err := repository.UpdateProfile(context.Background(), userfeature.UpdateProfileCommand{UserID: id, Username: &newUsername, DisplayName: &displayName})
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Username).To(Equal(newUsername))
		Expect(updated.DisplayName).To(Equal(displayName))
		stored, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(userIntegrationSuite.UsersTable), Key: map[string]types.AttributeValue{"userId": &types.AttributeValueMemberS{Value: id}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Item["updatedAt"]).To(Equal(&types.AttributeValueMemberS{Value: fixedUpdatedAt}))
		Expect(stored.Item["updatedAt"]).NotTo(Equal(&types.AttributeValueMemberS{Value: originalUpdatedAt}))

		old, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(userIntegrationSuite.UniquesTable), Key: map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#" + oldUsername}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(old.Item).To(BeEmpty())
		reserved, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(userIntegrationSuite.UniquesTable), Key: map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#" + newUsername}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(reserved.Item["userId"]).To(Equal(&types.AttributeValueMemberS{Value: id}))
	})

	It("rejects a conflicting username without changing the existing user", func() {
		id := "0192f8a0-7b00-7000-8000-000000000102"
		username := "integration_conflict"
		originalUpdatedAt := "2026-10-24T11:00:00Z"
		_, err := userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(userIntegrationSuite.UsersTable),
			Item: map[string]types.AttributeValue{
				"userId": &types.AttributeValueMemberS{Value: id}, "username": &types.AttributeValueMemberS{Value: "integration_original"},
				"email": &types.AttributeValueMemberS{Value: "patch-102@example.com"}, "passwordHash": &types.AttributeValueMemberS{Value: "hash"},
				"displayName": &types.AttributeValueMemberS{Value: "Original"}, "role": &types.AttributeValueMemberS{Value: "user"},
				"status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"}, "updatedAt": &types.AttributeValueMemberS{Value: originalUpdatedAt},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{TableName: aws.String(userIntegrationSuite.UniquesTable), Item: map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#integration_original"}, "userId": &types.AttributeValueMemberS{Value: id}}})
		Expect(err).NotTo(HaveOccurred())
		_, err = userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{TableName: aws.String(userIntegrationSuite.UniquesTable), Item: map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#" + username}, "userId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000199"}}})
		Expect(err).NotTo(HaveOccurred())

		repository := &userdb.Repository{Client: userIntegrationSuite.Client, TableName: userIntegrationSuite.UsersTable, UniqueTableName: userIntegrationSuite.UniquesTable}
		_, err = repository.UpdateProfile(context.Background(), userfeature.UpdateProfileCommand{UserID: id, Username: &username})
		Expect(err).To(MatchError(userfeature.ErrUsernameTaken))
		stored, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(userIntegrationSuite.UsersTable), Key: map[string]types.AttributeValue{"userId": &types.AttributeValueMemberS{Value: id}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Item["username"]).To(Equal(&types.AttributeValueMemberS{Value: "integration_original"}))
		Expect(stored.Item["updatedAt"]).To(Equal(&types.AttributeValueMemberS{Value: originalUpdatedAt}))
	})

	It("allows only one concurrent username update to reserve the same value", func() {
		ids := []string{
			"0192f8a0-7b00-7000-8000-000000000103",
			"0192f8a0-7b00-7000-8000-000000000104",
		}
		target := "integration_concurrent_target"
		for index, id := range ids {
			oldUsername := "integration_concurrent_old_" + string(rune('a'+index))
			_, err := userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
				TableName: aws.String(userIntegrationSuite.UsersTable),
				Item: map[string]types.AttributeValue{
					"userId": &types.AttributeValueMemberS{Value: id}, "username": &types.AttributeValueMemberS{Value: oldUsername},
					"email": &types.AttributeValueMemberS{Value: "patch-concurrent-" + string(rune('a'+index)) + "@example.com"}, "passwordHash": &types.AttributeValueMemberS{Value: "hash"},
					"displayName": &types.AttributeValueMemberS{Value: "Concurrent"}, "role": &types.AttributeValueMemberS{Value: "user"},
					"status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
				TableName: aws.String(userIntegrationSuite.UniquesTable),
				Item:      map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#" + oldUsername}, "userId": &types.AttributeValueMemberS{Value: id}},
			})
			Expect(err).NotTo(HaveOccurred())
		}

		results := make(chan error, len(ids))
		var group sync.WaitGroup
		for _, id := range ids {
			id := id
			group.Add(1)
			go func() {
				defer group.Done()
				username := target
				repository := &userdb.Repository{Client: userIntegrationSuite.Client, TableName: userIntegrationSuite.UsersTable, UniqueTableName: userIntegrationSuite.UniquesTable}
				_, err := repository.UpdateProfile(context.Background(), userfeature.UpdateProfileCommand{UserID: id, Username: &username})
				results <- err
			}()
		}
		group.Wait()
		close(results)

		var successful, conflicts int
		for err := range results {
			switch {
			case err == nil:
				successful++
			case errors.Is(err, userfeature.ErrUsernameTaken):
				conflicts++
			default:
				Fail("unexpected concurrent update error: " + err.Error())
			}
		}
		Expect(successful).To(Equal(1))
		Expect(conflicts).To(Equal(1))

		reserved, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(userIntegrationSuite.UniquesTable), Key: map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#" + target}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(reserved.Item["userId"]).NotTo(BeNil())
	})
})

func withUserRouteParam(request *http.Request, id string) *http.Request {
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("userId", id)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, ctx))
}
