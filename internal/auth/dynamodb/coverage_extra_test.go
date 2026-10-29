package dynamodb

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func apiError() *smithy.GenericAPIError {
	return &smithy.GenericAPIError{Code: "Unavailable", Message: "unavailable", Fault: smithy.FaultUnknown}
}

var _ = Describe("DynamoDB error and mapping branches", func() {
	It("maps user lookup by email, username, id, and malformed records", func() {
		now := time.Unix(100, 0).UTC()
		item := map[string]types.AttributeValue{
			"userId": &types.AttributeValueMemberS{Value: registration().User.ID}, "username": &types.AttributeValueMemberS{Value: "alice"},
			"email": &types.AttributeValueMemberS{Value: "alice@example.com"}, "passwordHash": &types.AttributeValueMemberS{Value: "hash"},
			"displayName": &types.AttributeValueMemberS{Value: "Alice"}, "role": &types.AttributeValueMemberS{Value: "user"}, "status": &types.AttributeValueMemberS{Value: "active"},
			"createdAt": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		}
		api := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{registration().User.ID: item})
		api.queryOut = &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item}}
		s := Store{Client: api, UsersTable: "users", RefreshTable: "refresh"}
		Expect(s.FindByLogin(context.Background(), "alice@example.com")).To(Equal(registration().User))
		Expect(s.FindByLogin(context.Background(), "alice")).To(Equal(registration().User))
		Expect(s.FindByID(context.Background(), registration().User.ID)).To(Equal(registration().User))
		Expect(platformid.IsCanonicalUUIDv7(registration().User.ID)).To(BeTrue())
		api.queryOut = &dynamodb.QueryOutput{}
		_, err := s.FindByLogin(context.Background(), "nobody@example.com")
		Expect(err).To(MatchError(user.ErrNotFound))
		api.queryOut = &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{{"createdAt": &types.AttributeValueMemberS{Value: "bad"}}}}
		_, err = s.FindByLogin(context.Background(), "bad@example.com")
		Expect(err).To(HaveOccurred())
		api.items[registration().User.ID]["createdAt"] = &types.AttributeValueMemberS{Value: "bad"}
		_, err = s.FindByID(context.Background(), registration().User.ID)
		Expect(err).To(HaveOccurred())
	})

	It("classifies unsupported, provider, and missing lookup errors", func() {
		api := newRegistrationAPI(nil, errors.New("get failed"), nil, nil)
		s := Store{Client: api, UsersTable: "users", UniquesTable: "uniques", RefreshTable: "refresh"}
		_, err := s.FindByLogin(context.Background(), "a@example.com")
		Expect(err).To(HaveOccurred())
		_, err = s.FindByID(context.Background(), "id")
		Expect(err).To(HaveOccurred())
		_, err = s.GetRefreshSession(context.Background(), "hash")
		Expect(err).To(HaveOccurred())
		api.getErr = apiError()
		Expect(isDependencyUnavailable(apiError())).To(BeTrue())
		Expect(isDependencyUnavailable(context.DeadlineExceeded)).To(BeTrue())
		Expect(isDependencyUnavailable(net.UnknownNetworkError("down"))).To(BeTrue())
		Expect(isDependencyUnavailable(errors.New("ordinary"))).To(BeFalse())
		Expect(wrapDependencyError("op", apiError())).To(MatchError(ContainSubstring("dependency unavailable")))
	})

	It("covers registration conflicts and transaction classification", func() {
		failed := "ConditionalCheckFailed"
		api := newRegistrationAPI(&types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String(failed)}, {Code: aws.String("None")}}}, nil, nil, nil)
		Expect((Store{Client: api}).Register(context.Background(), registration())).To(MatchError(auth.ErrEmailTaken))
		api.transactionErr = &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String("None")}, {Code: aws.String(failed)}}}
		Expect((Store{Client: api}).Register(context.Background(), registration())).To(MatchError(auth.ErrUsernameTaken))
		api.transactionErr = errors.New("transaction failed")
		Expect((Store{Client: api}).Register(context.Background(), registration())).To(HaveOccurred())
		Expect(isTransactionCanceled(&types.TransactionCanceledException{})).To(BeTrue())
		Expect(isTransactionCanceled(apiError())).To(BeFalse())
	})

	It("checks reservation conflicts and creates sessions", func() {
		api := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{"EMAIL#alice@example.com": {"x": &types.AttributeValueMemberS{Value: "x"}}, "USERNAME#alice": {"x": &types.AttributeValueMemberS{Value: "x"}}})
		s := Store{Client: api, UniquesTable: "uniques"}
		conflict, err := s.lookupReservationConflict(context.Background(), "alice@example.com", "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflict).To(MatchError(auth.ErrUsernameTaken))
		api.items = map[string]map[string]types.AttributeValue{"EMAIL#alice@example.com": {"x": &types.AttributeValueMemberS{Value: "x"}}}
		conflict, err = s.lookupReservationConflict(context.Background(), "alice@example.com", "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflict).To(MatchError(auth.ErrEmailTaken))
		api.items = map[string]map[string]types.AttributeValue{}
		conflict, err = s.lookupReservationConflict(context.Background(), "alice@example.com", "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflict).To(BeNil())
		r := registration().Refresh
		Expect(s.CreateLoginSession(context.Background(), r)).To(Succeed())
		Expect(api.transactionIn.TransactItems).To(HaveLen(2))
		api.transactionErr = errors.New("write")
		Expect(s.CreateLoginSession(context.Background(), r)).To(HaveOccurred())
	})

	It("rejects malformed refresh sessions and supports active family markers", func() {
		now := time.Unix(100, 0).UTC()
		base := map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: "hash"}, "sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"}, "userId": &types.AttributeValueMemberS{Value: registration().User.ID}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: "100"}, "expiresAt": &types.AttributeValueMemberN{Value: "200"}}
		api := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{"hash": base, "FAMILY#0192f8a0-7b00-7000-8000-000000000010": {"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"}, "status": &types.AttributeValueMemberS{Value: "active"}}})
		s := Store{Client: api, RefreshTable: "refresh", Clock: clock.Fixed{T: now}}
		got, err := s.GetRefreshSession(context.Background(), "hash")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.SessionID).To(Equal("0192f8a0-7b00-7000-8000-000000000010"))
		Expect(got.UserID).To(Equal(registration().User.ID))
		Expect(platformid.IsCanonicalUUIDv7(got.UserID)).To(BeTrue())
		for _, mutate := range []func(){func() { base["status"] = &types.AttributeValueMemberS{Value: "consumed"} }, func() {
			base["status"] = &types.AttributeValueMemberS{Value: "active"}
			base["expiresAt"] = &types.AttributeValueMemberN{Value: "bad"}
		}, func() { base["expiresAt"] = &types.AttributeValueMemberN{Value: "50"} }} {
			mutate()
			_, err = s.GetRefreshSession(context.Background(), "hash")
			Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
			base["status"] = &types.AttributeValueMemberS{Value: "active"}
			base["expiresAt"] = &types.AttributeValueMemberN{Value: "200"}
		}
		api.items["FAMILY#0192f8a0-7b00-7000-8000-000000000010"] = map[string]types.AttributeValue{"sessionId": &types.AttributeValueMemberS{Value: "other"}, "status": &types.AttributeValueMemberS{Value: "active"}}
		_, err = s.GetRefreshSession(context.Background(), "hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
	})

	It("revokes active families and maps conditional failures", func() {
		api := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{
			"hash": {"tokenHash": &types.AttributeValueMemberS{Value: "hash"}, "sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"}, "userId": &types.AttributeValueMemberS{Value: registration().User.ID}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: "100"}, "expiresAt": &types.AttributeValueMemberN{Value: "200"}},
		})
		s := Store{Client: api, RefreshTable: "refresh", Clock: clock.Fixed{T: time.Unix(100, 0).UTC()}}
		Expect(s.RevokeSessionFamily(context.Background(), "hash")).To(Succeed())
		Expect(api.transactionIn.TransactItems).To(HaveLen(2))
		api.transactionErr = &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String("ConditionalCheckFailed")}}}
		Expect(s.RevokeSessionFamily(context.Background(), "hash")).To(MatchError(auth.ErrInvalidRefreshToken))
		api.items["hash"]["status"] = &types.AttributeValueMemberS{Value: "consumed"}
		api.transactionErr = nil
		Expect(s.RevokeSessionFamily(context.Background(), "hash")).To(MatchError(auth.ErrInvalidRefreshToken))
	})

	It("handles ambiguous registration cancellation and family marker states", func() {
		api := newRegistrationAPI(&types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String("None")}, {Code: aws.String("None")}}}, nil, nil, map[string]map[string]types.AttributeValue{})
		Expect((Store{Client: api}).Register(context.Background(), registration())).To(MatchError("registration transaction conflict is ambiguous"))
		base := map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: "hash"}, "sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"}, "userId": &types.AttributeValueMemberS{Value: registration().User.ID}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: "100"}, "expiresAt": &types.AttributeValueMemberN{Value: "200"}}
		api.transactionErr = nil
		api.items = map[string]map[string]types.AttributeValue{"hash": base, "FAMILY#0192f8a0-7b00-7000-8000-000000000010": {"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"}, "status": &types.AttributeValueMemberS{Value: "revoked"}}}
		s := Store{Client: api, Clock: clock.Fixed{T: time.Unix(100, 0).UTC()}}
		_, err := s.GetRefreshSession(context.Background(), "hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		api.items["FAMILY#0192f8a0-7b00-7000-8000-000000000010"]["status"] = &types.AttributeValueMemberS{Value: "unknown"}
		_, err = s.GetRefreshSession(context.Background(), "hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
	})
})
