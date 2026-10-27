//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	authdomain "github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/token"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("refresh persistence", func() {
	It("atomically consumes the old token, stores only the hash, and rotates the absolute expiry", func() {
		store := integrationStore()
		now := time.Now().UTC().Truncate(time.Second)
		raw := "integration-refresh-rotation-token"
		session := authdomain.RefreshSession{TokenHash: authdomain.HashRefreshToken(raw), SessionID: "0192f8a0-7b00-7000-8000-000000000010", UserID: testUserID, CreatedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour)}
		Expect(store.CreateLoginSession(context.Background(), session)).To(Succeed())

		svc := refreshIntegrationService(store, now)
		result, err := svc.Refresh(context.Background(), raw)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AccessToken).NotTo(BeEmpty())
		Expect(result.RefreshToken).NotTo(Equal(raw))
		Expect(result.RefreshExpiresIn).To(Equal(int64(604800)))

		old, oldErr := store.GetRefreshSession(context.Background(), session.TokenHash)
		Expect(oldErr).To(MatchError(authdomain.ErrInvalidRefreshToken))
		Expect(old.Status).To(BeEmpty())
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", session.TokenHash, map[string]string{"status": "consumed", "sessionId": session.SessionID})
		newHash := authdomain.HashRefreshToken(result.RefreshToken)
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", newHash, map[string]string{"status": "active", "sessionId": session.SessionID, "userId": session.UserID})
		assertNumericFields(integrationSuite.RefreshTable, "tokenHash", newHash, map[string]string{"expiresAt": fmt.Sprint(now.Add(7 * 24 * time.Hour).Unix())})
		assertMissing(integrationSuite.RefreshTable, "tokenHash", raw)
	})

	It("rejects expired sessions and revokes the family when a consumed token is replayed", func() {
		store := integrationStore()
		now := time.Now().UTC().Truncate(time.Second)
		expiredRaw := "integration-refresh-expired-token"
		expired := authdomain.RefreshSession{TokenHash: authdomain.HashRefreshToken(expiredRaw), SessionID: "0192f8a0-7b00-7000-8000-000000000011", UserID: testUserID, CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Second)}
		Expect(store.CreateLoginSession(context.Background(), expired)).To(Succeed())
		_, err := store.GetRefreshSession(context.Background(), expired.TokenHash)
		Expect(err).To(MatchError(authdomain.ErrInvalidRefreshToken))

		revokedRaw := "integration-refresh-revoked-token"
		revoked := authdomain.RefreshSession{TokenHash: authdomain.HashRefreshToken(revokedRaw), SessionID: "0192f8a0-7b00-7000-8000-000000000012", UserID: testUserID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), Status: "revoked"}
		_, err = integrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{TableName: aws.String(integrationSuite.RefreshTable), Item: map[string]types.AttributeValue{
			"tokenHash": &types.AttributeValueMemberS{Value: revoked.TokenHash},
			"sessionId": &types.AttributeValueMemberS{Value: revoked.SessionID},
			"userId":    &types.AttributeValueMemberS{Value: revoked.UserID},
			"status":    &types.AttributeValueMemberS{Value: revoked.Status},
			"createdAt": &types.AttributeValueMemberN{Value: fmt.Sprint(revoked.CreatedAt.Unix())},
			"expiresAt": &types.AttributeValueMemberN{Value: fmt.Sprint(revoked.ExpiresAt.Unix())},
		}})
		Expect(err).NotTo(HaveOccurred())
		svc := refreshIntegrationService(store, now)
		_, err = svc.Refresh(context.Background(), revokedRaw)
		Expect(errors.Is(err, authdomain.ErrInvalidRefreshToken)).To(BeTrue())

		replayRaw := "integration-refresh-replay-token"
		replay := authdomain.RefreshSession{TokenHash: authdomain.HashRefreshToken(replayRaw), SessionID: "0192f8a0-7b00-7000-8000-000000000013", UserID: testUserID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
		Expect(store.CreateLoginSession(context.Background(), replay)).To(Succeed())
		svc = refreshIntegrationService(store, now)
		_, err = svc.Refresh(context.Background(), replayRaw)
		Expect(err).NotTo(HaveOccurred())
		_, err = svc.Refresh(context.Background(), replayRaw)
		Expect(errors.Is(err, authdomain.ErrInvalidRefreshToken)).To(BeTrue())
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", "FAMILY#"+replay.SessionID, map[string]string{"sessionId": replay.SessionID, "status": "revoked"})
	})

	It("permits exactly one concurrent rotation and persists replay-family revocation", func() {
		store := integrationStore()
		now := time.Now().UTC().Truncate(time.Second)
		raw := "integration-refresh-concurrent-token"
		session := authdomain.RefreshSession{TokenHash: authdomain.HashRefreshToken(raw), SessionID: "0192f8a0-7b00-7000-8000-000000000014", UserID: testUserID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
		Expect(store.CreateLoginSession(context.Background(), session)).To(Succeed())
		svc := refreshIntegrationService(store, now)
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, refreshErr := svc.Refresh(context.Background(), raw)
				results <- refreshErr
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		var success, invalid int
		for refreshErr := range results {
			switch {
			case refreshErr == nil:
				success++
			case errors.Is(refreshErr, authdomain.ErrInvalidRefreshToken):
				invalid++
			default:
				Fail(fmt.Sprintf("unexpected concurrent refresh error: %v", refreshErr))
			}
		}
		Expect(success).To(Equal(1))
		Expect(invalid).To(Equal(1))
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", "FAMILY#"+session.SessionID, map[string]string{"sessionId": session.SessionID, "status": "revoked"})
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", session.TokenHash, map[string]string{"status": "consumed"})
	})

	It("rejects rotation when logout wins after the refresh lookup", func() {
		store := integrationStore()
		now := time.Now().UTC().Truncate(time.Second)
		raw := "integration-refresh-after-logout-token"
		session := authdomain.RefreshSession{
			TokenHash: authdomain.HashRefreshToken(raw),
			SessionID: "0192f8a0-7b00-7000-8000-000000000015",
			UserID:    testUserID,
			CreatedAt: now,
			ExpiresAt: now.Add(time.Hour),
		}
		Expect(store.CreateLoginSession(context.Background(), session)).To(Succeed())

		// Model the race window: refresh has already validated the token, then
		// logout wins before the rotation transaction is committed.
		_, err := store.GetRefreshSession(context.Background(), session.TokenHash)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.RevokeSessionFamily(context.Background(), session.TokenHash)).To(Succeed())

		err = store.RotateRefreshSession(context.Background(), session.TokenHash, authdomain.RefreshSession{
			TokenHash: authdomain.HashRefreshToken("integration-refresh-after-logout-next"),
			SessionID: session.SessionID,
			UserID:    session.UserID,
			CreatedAt: now,
			ExpiresAt: now.Add(time.Hour),
		})
		Expect(err).To(MatchError(authdomain.ErrInvalidRefreshToken))
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", "FAMILY#"+session.SessionID, map[string]string{"status": "revoked"})
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", session.TokenHash, map[string]string{"status": "active"})
		assertMissing(integrationSuite.RefreshTable, "tokenHash", authdomain.HashRefreshToken("integration-refresh-after-logout-next"))
	})
})

func refreshIntegrationService(store authdomain.RefreshStore, now time.Time) *authdomain.Service {
	return &authdomain.Service{
		RefreshStore: store,
		Users:        refreshIntegrationUserLookup{},
		Clock:        fixedClock{at: now},
		Access:       platformjwt.Access{Keys: platformjwt.StaticKey{KID: "refresh-test-key", Secret: []byte("refresh-test-secret")}, KID: "refresh-test-key", Issuer: "social-media", Audience: "social-media-api", TTL: 15 * time.Minute, ID: func() string { return "0192f8a0-7b00-7000-8000-000000000003" }},
		Opaque:       token.Opaque{},
		AccessTTL:    15 * time.Minute,
		RefreshTTL:   7 * 24 * time.Hour,
	}
}

type refreshIntegrationUserLookup struct{}

func (refreshIntegrationUserLookup) FindByID(_ context.Context, id string) (user.User, error) {
	return user.User{ID: id, Role: user.RoleUser, Status: user.StatusActive}, nil
}

const testUserID = "0192f8a0-7b00-7000-8000-000000000001"
