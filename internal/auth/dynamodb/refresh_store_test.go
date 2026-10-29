package dynamodb

import (
	"context"
	"strconv"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

func networkError() *MockError {
	err := NewMockError(gomock.NewController(GinkgoT()))
	err.EXPECT().Error().AnyTimes().Return("network")
	err.EXPECT().Timeout().AnyTimes().Return(true)
	err.EXPECT().Temporary().AnyTimes().Return(true)
	return err
}

func canceled(codes ...string) error {
	reasons := make([]types.CancellationReason, len(codes))
	for i, code := range codes {
		reasons[i] = types.CancellationReason{Code: aws.String(code)}
	}
	return &types.TransactionCanceledException{CancellationReasons: reasons}
}

var _ = Describe("RotateRefreshSession", func() {
	It("builds an atomic consume-and-create transaction", func() {
		now := time.Unix(1_700_000_000, 0).UTC()
		api := newRegistrationAPI(nil, nil, nil, nil)
		store := Store{Client: api, RefreshTable: "refresh", Clock: clock.Fixed{T: now}}
		next := auth.RefreshSession{TokenHash: "next-hash", SessionID: "0192f8a0-7b00-7000-8000-000000000003", UserID: "0192f8a0-7b00-7000-8000-000000000001", CreatedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour)}

		Expect(store.RotateRefreshSession(context.Background(), "old-hash", next)).To(Succeed())
		Expect(api.transactionIn.TransactItems).To(HaveLen(3))
		familyCheck := api.transactionIn.TransactItems[0].ConditionCheck
		Expect(familyCheck).NotTo(BeNil())
		Expect(familyCheck.Key["tokenHash"]).To(Equal(&types.AttributeValueMemberS{Value: "FAMILY#0192f8a0-7b00-7000-8000-000000000003"}))
		Expect(aws.ToString(familyCheck.ConditionExpression)).To(Equal("attribute_not_exists(tokenHash)"))
		update := api.transactionIn.TransactItems[1].Update
		Expect(update).NotTo(BeNil())
		Expect(aws.ToString(update.ConditionExpression)).To(Equal("#status = :active AND expiresAt > :now"))
		put := api.transactionIn.TransactItems[2].Put
		Expect(put).NotTo(BeNil())
		Expect(put.Item["tokenHash"]).To(Equal(&types.AttributeValueMemberS{Value: next.TokenHash}))
		Expect(put.Item["status"]).To(Equal(&types.AttributeValueMemberS{Value: "active"}))
		Expect(put.Item["expiresAt"]).To(Equal(&types.AttributeValueMemberN{Value: strconv.FormatInt(next.ExpiresAt.Unix(), 10)}))
	})

	It("maps conditional and dependency errors", func() {
		next := auth.RefreshSession{TokenHash: "next-hash", SessionID: "0192f8a0-7b00-7000-8000-000000000003", UserID: "0192f8a0-7b00-7000-8000-000000000001", CreatedAt: time.Unix(1, 0), ExpiresAt: time.Unix(2, 0)}
		for _, tc := range []struct {
			name string
			err  error
			want error
		}{
			{name: "conditional", err: canceled("ConditionalCheckFailed"), want: auth.ErrInvalidRefreshToken},
			{name: "dependency", err: networkError(), want: auth.ErrDependencyUnavailable},
		} {
			By(tc.name)
			store := Store{Client: newRegistrationAPI(tc.err, nil, nil, nil), RefreshTable: "refresh"}
			Expect(store.RotateRefreshSession(context.Background(), "old-hash", next)).To(MatchError(tc.want))
		}
	})
})

var _ = Describe("RevokeSessionFamilyByHash", func() {
	It("builds a family revocation marker from the token session", func() {
		now := time.Unix(1_700_000_000, 0).UTC()
		item := map[string]types.AttributeValue{
			"tokenHash": &types.AttributeValueMemberS{Value: "old-hash"},
			"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"},
			"userId":    &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"},
			"status":    &types.AttributeValueMemberS{Value: "consumed"},
		}
		api := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{"old-hash": item})
		store := Store{Client: api, RefreshTable: "refresh", Clock: clock.Fixed{T: now}}

		Expect(store.RevokeSessionFamilyByHash(context.Background(), "old-hash")).To(Succeed())
		put := api.transactionIn.TransactItems[0].Put
		Expect(put.Item["tokenHash"]).To(Equal(&types.AttributeValueMemberS{Value: "FAMILY#0192f8a0-7b00-7000-8000-000000000010"}))
		Expect(put.Item["status"]).To(Equal(&types.AttributeValueMemberS{Value: "revoked"}))
	})

	It("maps lookup and write dependency errors and ignores a conditional race", func() {
		now := time.Unix(1_700_000_000, 0).UTC()
		item := map[string]types.AttributeValue{
			"tokenHash": &types.AttributeValueMemberS{Value: "old-hash"},
			"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"},
			"userId":    &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"},
			"status":    &types.AttributeValueMemberS{Value: "consumed"},
		}
		for _, tc := range []struct {
			name   string
			getErr error
			write  error
			want   error
		}{
			{name: "lookup dependency", getErr: networkError(), want: auth.ErrDependencyUnavailable},
			{name: "write dependency", write: networkError(), want: auth.ErrDependencyUnavailable},
			{name: "conditional write", write: canceled("ConditionalCheckFailed")},
		} {
			By(tc.name)
			api := newRegistrationAPI(tc.write, tc.getErr, nil, map[string]map[string]types.AttributeValue{"old-hash": item})
			store := Store{Client: api, RefreshTable: "refresh", Clock: clock.Fixed{T: now}}
			if tc.want == nil {
				Expect(store.RevokeSessionFamilyByHash(context.Background(), "old-hash")).To(Succeed())
			} else {
				Expect(store.RevokeSessionFamilyByHash(context.Background(), "old-hash")).To(MatchError(tc.want))
			}
		}
	})

	It("rejects malformed persisted user and session IDs before the family-marker transaction", func() {
		for _, tc := range []struct {
			name      string
			sessionID string
			userID    string
		}{
			{name: "malformed session ID", sessionID: "not-a-uuid", userID: "0192f8a0-7b00-7000-8000-000000000001"},
			{name: "malformed user ID", sessionID: "0192f8a0-7b00-7000-8000-000000000010", userID: "not-a-uuid"},
			{name: "non-v7 session ID", sessionID: "0192f8a0-7b00-6000-8000-000000000010", userID: "0192f8a0-7b00-7000-8000-000000000001"},
		} {
			By(tc.name)
			api := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{"old-hash": {
				"tokenHash": &types.AttributeValueMemberS{Value: "old-hash"},
				"sessionId": &types.AttributeValueMemberS{Value: tc.sessionID},
				"userId":    &types.AttributeValueMemberS{Value: tc.userID},
			}})
			store := Store{Client: api, RefreshTable: "refresh", Clock: clock.Fixed{T: time.Unix(1_700_000_000, 0).UTC()}}

			Expect(store.RevokeSessionFamilyByHash(context.Background(), "old-hash")).To(MatchError(auth.ErrInvalidRefreshToken))
			Expect(api.transactionN).To(Equal(0))
			Expect(api.transactionIn).To(BeNil())
		}
	})
})

var _ = Describe("RotateRefreshSessionRejectsInvalidPersistenceIDs", func() {
	It("rejects invalid IDs before issuing a transaction", func() {
		for _, next := range []auth.RefreshSession{
			{SessionID: "invalid", UserID: "0192f8a0-7b00-7000-8000-000000000001"},
			{SessionID: "0192f8a0-7b00-7000-8000-000000000002", UserID: "invalid"},
		} {
			api := newRegistrationAPI(nil, nil, nil, nil)
			err := (Store{Client: api}).RotateRefreshSession(context.Background(), "old", next)
			Expect(err).To(MatchError(ContainSubstring("invalid refresh session ID")))
			Expect(api.transactionIn).To(BeNil())
		}
	})
})
