//go:build integration

package authintegration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	authdomain "github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/tests/fixtures"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("logout persistence", func() {
	It("rejects consumed tokens and tokens in revoked families", func() {
		store := integrationStore()
		now := time.Now().UTC().Truncate(time.Second)

		consumedHash := hashToken("logout-consumed-token")
		_, err := integrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(integrationSuite.RefreshTable),
			Item: map[string]types.AttributeValue{
				"tokenHash": &types.AttributeValueMemberS{Value: consumedHash},
				"sessionId": &types.AttributeValueMemberS{Value: "logout-consumed-family"},
				"status":    &types.AttributeValueMemberS{Value: "consumed"},
				"expiresAt": &types.AttributeValueMemberN{Value: fmt.Sprint(now.Add(time.Hour).Unix())},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		revokedTokenHash := hashToken("logout-revoked-family-token")
		revokedFamily := "logout-revoked-family"
		_, err = integrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(integrationSuite.RefreshTable),
			Item: map[string]types.AttributeValue{
				"tokenHash": &types.AttributeValueMemberS{Value: revokedTokenHash},
				"sessionId": &types.AttributeValueMemberS{Value: revokedFamily},
				"status":    &types.AttributeValueMemberS{Value: "active"},
				"expiresAt": &types.AttributeValueMemberN{Value: fmt.Sprint(now.Add(time.Hour).Unix())},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(integrationSuite.RefreshTable),
			Item: map[string]types.AttributeValue{
				"tokenHash": &types.AttributeValueMemberS{Value: "FAMILY#" + revokedFamily},
				"sessionId": &types.AttributeValueMemberS{Value: revokedFamily},
				"status":    &types.AttributeValueMemberS{Value: "revoked"},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = store.GetRefreshSession(context.Background(), consumedHash)
		Expect(err).To(MatchError(authdomain.ErrInvalidRefreshToken))
		_, err = store.GetRefreshSession(context.Background(), revokedTokenHash)
		Expect(err).To(MatchError(authdomain.ErrInvalidRefreshToken))

		h := authdomain.Handler{Service: &authdomain.Service{LogoutStore: store}}
		Expect(logoutHTTP(h, "logout-consumed-token").Code).To(Equal(http.StatusUnauthorized))
		Expect(logoutHTTP(h, "logout-consumed-token").Body.String()).To(ContainSubstring(`"code":"INVALID_REFRESH_TOKEN"`))
		Expect(logoutHTTP(h, "logout-revoked-family-token").Code).To(Equal(http.StatusUnauthorized))
		Expect(logoutHTTP(h, "logout-revoked-family-token").Body.String()).To(ContainSubstring(`"code":"INVALID_REFRESH_TOKEN"`))
	})

	It("enforces authoritative refresh expiration for persisted sessions", func() {
		store := integrationStore()
		now := time.Now().UTC().Truncate(time.Second)
		cases := []struct {
			name      string
			expiresAt types.AttributeValue
		}{
			{name: "missing", expiresAt: nil},
			{name: "malformed", expiresAt: &types.AttributeValueMemberS{Value: "not-a-number"}},
			{name: "expired", expiresAt: &types.AttributeValueMemberN{Value: fmt.Sprint(now.Add(-time.Second).Unix())}},
			{name: "exactly-current", expiresAt: &types.AttributeValueMemberN{Value: fmt.Sprint(now.Unix())}},
		}

		for _, tc := range cases {
			tokenHash := hashToken("expiration-" + tc.name)
			item := map[string]types.AttributeValue{
				"tokenHash": &types.AttributeValueMemberS{Value: tokenHash},
				"sessionId": &types.AttributeValueMemberS{Value: "family-" + tc.name},
				"userId":    &types.AttributeValueMemberS{Value: "expiration-user"},
				"status":    &types.AttributeValueMemberS{Value: "active"},
			}
			if tc.expiresAt != nil {
				item["expiresAt"] = tc.expiresAt
			}
			_, err := integrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
				TableName: aws.String(integrationSuite.RefreshTable),
				Item:      item,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.GetRefreshSession(context.Background(), tokenHash)
			Expect(err).To(MatchError(authdomain.ErrInvalidRefreshToken), tc.name)
		}

		futureHash := hashToken("expiration-future")
		futureFamily := "family-future"
		_, err := integrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(integrationSuite.RefreshTable),
			Item: map[string]types.AttributeValue{
				"tokenHash": &types.AttributeValueMemberS{Value: futureHash},
				"sessionId": &types.AttributeValueMemberS{Value: futureFamily},
				"userId":    &types.AttributeValueMemberS{Value: "expiration-user"},
				"status":    &types.AttributeValueMemberS{Value: "active"},
				"createdAt": &types.AttributeValueMemberN{Value: fmt.Sprint(now.Unix())},
				"expiresAt": &types.AttributeValueMemberN{Value: fmt.Sprint(now.Add(time.Minute).Unix())},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(integrationSuite.RefreshTable),
			Item: map[string]types.AttributeValue{
				"tokenHash": &types.AttributeValueMemberS{Value: "FAMILY#" + futureFamily},
				"sessionId": &types.AttributeValueMemberS{Value: futureFamily},
				"status":    &types.AttributeValueMemberS{Value: "active"},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		got, err := store.GetRefreshSession(context.Background(), futureHash)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.TokenHash).To(Equal(futureHash))
		Expect(got.ExpiresAt).To(Equal(now.Add(time.Minute)))
	})

	It("returns 204 once, then 401 for the reused token, and revokes its entire family", func() {
		store := integrationStore()
		createdAt := time.Now().UTC().Truncate(time.Second)
		familyID := "logout-family"
		for _, rawToken := range []string{"logout-token-a", "logout-token-b", "logout-token-c"} {
			registration := fixtures.Registration("logout-"+rawToken, "logout_"+rawToken, rawToken+"@example.com")
			session := authdomain.RefreshSession{
				TokenHash: hashToken(rawToken),
				SessionID: familyID,
				UserID:    registration.User.ID,
				CreatedAt: createdAt,
				ExpiresAt: createdAt.Add(7 * 24 * time.Hour),
			}
			Expect(store.CreateLoginSession(context.Background(), session)).To(Succeed())
		}
		active, err := store.GetRefreshSession(context.Background(), hashToken("logout-token-b"))
		Expect(err).NotTo(HaveOccurred())
		Expect(active.Status).To(Equal("active"))

		h := authdomain.Handler{Service: &authdomain.Service{LogoutStore: store}}
		first := logoutHTTP(h, "logout-token-b")
		Expect(first.Code).To(Equal(http.StatusNoContent))
		Expect(first.Body.String()).To(BeEmpty())
		second := logoutHTTP(h, "logout-token-b")
		Expect(second.Code).To(Equal(http.StatusUnauthorized))
		Expect(second.Body.String()).To(ContainSubstring(`"code":"INVALID_REFRESH_TOKEN"`))

		assertStringFields(integrationSuite.RefreshTable, "tokenHash", "FAMILY#"+familyID, map[string]string{"sessionId": familyID, "status": "revoked"})
		_, err = store.GetRefreshSession(context.Background(), hashToken("logout-token-a"))
		Expect(err).To(MatchError(authdomain.ErrInvalidRefreshToken))
		_, err = store.GetRefreshSession(context.Background(), hashToken("logout-token-b"))
		Expect(err).To(MatchError(authdomain.ErrInvalidRefreshToken))
		_, err = store.GetRefreshSession(context.Background(), hashToken("logout-token-c"))
		Expect(err).To(MatchError(authdomain.ErrInvalidRefreshToken))
	})

	It("allows exactly one concurrent logout, rejects the other, and persists the revoked family marker", func() {
		store := integrationStore()
		createdAt := time.Now().UTC().Truncate(time.Second)
		familyID := "logout-concurrent-family"
		rawToken := "logout-concurrent-token"
		registration := fixtures.Registration("logout-concurrent", "logout_concurrent", "logout-concurrent@example.com")
		session := authdomain.RefreshSession{
			TokenHash: hashToken(rawToken),
			SessionID: familyID,
			UserID:    registration.User.ID,
			CreatedAt: createdAt,
			ExpiresAt: createdAt.Add(7 * 24 * time.Hour),
		}
		Expect(store.CreateLoginSession(context.Background(), session)).To(Succeed())

		h := authdomain.Handler{Service: &authdomain.Service{LogoutStore: store}}
		start := make(chan struct{})
		responses := make(chan *httptest.ResponseRecorder, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				responses <- logoutHTTP(h, rawToken)
			}()
		}
		close(start)
		wg.Wait()
		close(responses)

		var success, invalid int
		for response := range responses {
			switch response.Code {
			case http.StatusNoContent:
				success++
				Expect(response.Body.String()).To(BeEmpty())
			case http.StatusUnauthorized:
				invalid++
				Expect(response.Body.String()).To(ContainSubstring(`"code":"INVALID_REFRESH_TOKEN"`))
			default:
				Fail(fmt.Sprintf("unexpected concurrent logout status: %d", response.Code))
			}
		}
		Expect(success).To(Equal(1))
		Expect(invalid).To(Equal(1))

		assertStringFields(integrationSuite.RefreshTable, "tokenHash", "FAMILY#"+familyID, map[string]string{"sessionId": familyID, "status": "revoked"})
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", session.TokenHash, map[string]string{"sessionId": familyID, "status": "active"})
		_, err := store.GetRefreshSession(context.Background(), session.TokenHash)
		Expect(err).To(MatchError(authdomain.ErrInvalidRefreshToken))
	})

	It("rejects an unknown refresh token without changing persisted sessions", func() {
		store := integrationStore()
		registration := fixtures.Registration("logout-unknown", "logout_unknown", "logout-unknown@example.com")
		session := authdomain.RefreshSession{
			TokenHash: hashToken("known-logout-token"),
			SessionID: "known-logout-family",
			UserID:    registration.User.ID,
			CreatedAt: time.Now().UTC().Truncate(time.Second),
			ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour),
		}
		Expect(store.CreateLoginSession(context.Background(), session)).To(Succeed())

		svc := &authdomain.Service{LogoutStore: store}
		Expect(svc.Logout(context.Background(), "missing-logout-token")).To(MatchError(authdomain.ErrInvalidRefreshToken))
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", session.TokenHash, map[string]string{"status": "active"})
	})
})

func hashToken(token string) string {
	return authdomain.HashRefreshToken(token)
}

func logoutHTTP(handler authdomain.Handler, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{"refreshToken":"`+token+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.Logout(response, request)
	return response
}
