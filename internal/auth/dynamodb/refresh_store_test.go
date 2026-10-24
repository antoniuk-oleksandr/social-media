package dynamodb

import (
	"context"
	"strconv"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RotateRefreshSession", func() {
	It("builds an atomic consume-and-create transaction", func() {
		now := time.Unix(1_700_000_000, 0).UTC()
		api := &fakeAPI{}
		store := Store{Client: api, RefreshTable: "refresh", Clock: fixedStoreClock{at: now}}
		next := auth.RefreshSession{TokenHash: "next-hash", SessionID: "family", UserID: "user", CreatedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour)}

		Expect(store.RotateRefreshSession(context.Background(), "old-hash", next)).To(Succeed())
		Expect(api.transactionIn.TransactItems).To(HaveLen(3))
		familyCheck := api.transactionIn.TransactItems[0].ConditionCheck
		Expect(familyCheck).NotTo(BeNil())
		Expect(familyCheck.Key["tokenHash"]).To(Equal(&types.AttributeValueMemberS{Value: "FAMILY#family"}))
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
		next := auth.RefreshSession{TokenHash: "next-hash", SessionID: "family", UserID: "user", CreatedAt: time.Unix(1, 0), ExpiresAt: time.Unix(2, 0)}
		for _, tc := range []struct {
			name string
			err  error
			want error
		}{
			{name: "conditional", err: canceled("ConditionalCheckFailed"), want: auth.ErrInvalidRefreshToken},
			{name: "dependency", err: testNetworkError{}, want: auth.ErrDependencyUnavailable},
		} {
			By(tc.name)
			store := Store{Client: &fakeAPI{transactionErr: tc.err}, RefreshTable: "refresh"}
			Expect(store.RotateRefreshSession(context.Background(), "old-hash", next)).To(MatchError(tc.want))
		}
	})
})

var _ = Describe("RevokeSessionFamilyByHash", func() {
	It("builds a family revocation marker from the token session", func() {
		now := time.Unix(1_700_000_000, 0).UTC()
		item := map[string]types.AttributeValue{
			"tokenHash": &types.AttributeValueMemberS{Value: "old-hash"},
			"sessionId": &types.AttributeValueMemberS{Value: "family"},
			"userId":    &types.AttributeValueMemberS{Value: "user"},
			"status":    &types.AttributeValueMemberS{Value: "consumed"},
		}
		api := &fakeAPI{items: map[string]map[string]types.AttributeValue{"old-hash": item}}
		store := Store{Client: api, RefreshTable: "refresh", Clock: fixedStoreClock{at: now}}

		Expect(store.RevokeSessionFamilyByHash(context.Background(), "old-hash")).To(Succeed())
		put := api.transactionIn.TransactItems[0].Put
		Expect(put.Item["tokenHash"]).To(Equal(&types.AttributeValueMemberS{Value: "FAMILY#family"}))
		Expect(put.Item["status"]).To(Equal(&types.AttributeValueMemberS{Value: "revoked"}))
	})

	It("maps lookup and write dependency errors and ignores a conditional race", func() {
		now := time.Unix(1_700_000_000, 0).UTC()
		item := map[string]types.AttributeValue{
			"tokenHash": &types.AttributeValueMemberS{Value: "old-hash"},
			"sessionId": &types.AttributeValueMemberS{Value: "family"},
			"userId":    &types.AttributeValueMemberS{Value: "user"},
			"status":    &types.AttributeValueMemberS{Value: "consumed"},
		}
		for _, tc := range []struct {
			name   string
			getErr error
			write  error
			want   error
		}{
			{name: "lookup dependency", getErr: testNetworkError{}, want: auth.ErrDependencyUnavailable},
			{name: "write dependency", write: testNetworkError{}, want: auth.ErrDependencyUnavailable},
			{name: "conditional write", write: canceled("ConditionalCheckFailed")},
		} {
			By(tc.name)
			api := &fakeAPI{items: map[string]map[string]types.AttributeValue{"old-hash": item}, getErr: tc.getErr, transactionErr: tc.write}
			store := Store{Client: api, RefreshTable: "refresh", Clock: fixedStoreClock{at: now}}
			if tc.want == nil {
				Expect(store.RevokeSessionFamilyByHash(context.Background(), "old-hash")).To(Succeed())
			} else {
				Expect(store.RevokeSessionFamilyByHash(context.Background(), "old-hash")).To(MatchError(tc.want))
			}
		}
	})
})
