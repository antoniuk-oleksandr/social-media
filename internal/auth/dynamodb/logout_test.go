package dynamodb

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type logoutAPIStub struct {
	item      map[string]types.AttributeValue
	items     []map[string]types.AttributeValue
	getErr    error
	transErr  error
	gets      []*dynamodb.GetItemInput
	transacts []*dynamodb.TransactWriteItemsInput
}

type fixedStoreClock struct{ at time.Time }

func (c fixedStoreClock) Now() time.Time { return c.at }

func (s *logoutAPIStub) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	s.gets = append(s.gets, in)
	if s.getErr != nil {
		return nil, s.getErr
	}
	if len(s.items) > 0 {
		item := s.items[0]
		s.items = s.items[1:]
		return &dynamodb.GetItemOutput{Item: item}, nil
	}
	return &dynamodb.GetItemOutput{Item: s.item}, nil
}

func (s *logoutAPIStub) TransactWriteItems(_ context.Context, in *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	s.transacts = append(s.transacts, in)
	return &dynamodb.TransactWriteItemsOutput{}, s.transErr
}

var _ = Describe("RevokeSessionFamily", func() {
	It("revokes every token in the session 0192f8a0-7b00-7000-8000-000000000012 and rejects reuse", func() {
		now := time.Now().UTC().Truncate(time.Second)
		token := map[string]types.AttributeValue{
			"tokenHash": &types.AttributeValueMemberS{Value: "hash-1"},
			"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"},
			"userId":    &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"},
			"status":    &types.AttributeValueMemberS{Value: "active"},
			"createdAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(-time.Minute).Unix(), 10)},
			"expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(time.Minute).Unix(), 10)},
		}
		marker := map[string]types.AttributeValue{"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"}, "status": &types.AttributeValueMemberS{Value: "active"}}
		revokedMarker := map[string]types.AttributeValue{"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000010"}, "status": &types.AttributeValueMemberS{Value: "revoked"}}
		stub := &logoutAPIStub{items: []map[string]types.AttributeValue{token, marker, token, revokedMarker}}
		store := Store{Client: stub, RefreshTable: "refresh", Clock: fixedStoreClock{at: now}}
		Expect(store.RevokeSessionFamily(context.Background(), "hash-1")).To(Succeed())
		Expect(stub.transacts).To(HaveLen(1))
		Expect(stub.transacts[0].TransactItems).To(HaveLen(2))
		familyMarker := stub.transacts[0].TransactItems[1].Put.Item
		Expect(familyMarker["tokenHash"]).To(Equal(&types.AttributeValueMemberS{Value: "FAMILY#0192f8a0-7b00-7000-8000-000000000010"}))
		Expect(familyMarker["status"]).To(Equal(&types.AttributeValueMemberS{Value: "revoked"}))
		Expect(*stub.transacts[0].TransactItems[1].Put.ConditionExpression).To(Equal("attribute_not_exists(tokenHash) OR #status = :active"))
		Expect(stub.transacts[0].TransactItems[1].Put.ExpressionAttributeNames).To(Equal(map[string]string{"#status": "status"}))
		Expect(stub.transacts[0].TransactItems[1].Put.ExpressionAttributeValues[":active"]).To(Equal(&types.AttributeValueMemberS{Value: "active"}))
		Expect(store.RevokeSessionFamily(context.Background(), "hash-1")).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(stub.transacts).To(HaveLen(1))
	})

	It("uses the exact refresh key and a strongly consistent read", func() {
		now := time.Now().UTC().Truncate(time.Second)
		stub := &logoutAPIStub{items: []map[string]types.AttributeValue{
			{"tokenHash": &types.AttributeValueMemberS{Value: "hash-key"}, "sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000011"}, "userId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(-time.Minute).Unix(), 10)}, "expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(time.Minute).Unix(), 10)}},
			{"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000011"}, "status": &types.AttributeValueMemberS{Value: "active"}},
		}}
		Expect((Store{Client: stub, RefreshTable: "refresh-table", Clock: fixedStoreClock{at: now}}).RevokeSessionFamily(context.Background(), "hash-key")).To(Succeed())
		Expect(stub.gets).To(HaveLen(2))
		Expect(*stub.gets[0].TableName).To(Equal("refresh-table"))
		Expect(stub.gets[0].Key["tokenHash"]).To(Equal(&types.AttributeValueMemberS{Value: "hash-key"}))
		Expect(*stub.gets[0].ConsistentRead).To(BeTrue())
	})

	It("translates read and transaction failures without attempting partial writes", func() {
		readFailure := &logoutAPIStub{getErr: context.DeadlineExceeded}
		err := (Store{Client: readFailure, RefreshTable: "refresh"}).RevokeSessionFamily(context.Background(), "hash")
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeTrue())
		Expect(readFailure.transacts).To(BeEmpty())

		now := time.Now().UTC().Truncate(time.Second)
		writeFailure := &logoutAPIStub{items: []map[string]types.AttributeValue{
			{"tokenHash": &types.AttributeValueMemberS{Value: "hash"}, "sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}, "userId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(-time.Minute).Unix(), 10)}, "expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(time.Minute).Unix(), 10)}},
			{"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}, "status": &types.AttributeValueMemberS{Value: "active"}},
		}, transErr: context.DeadlineExceeded}
		err = (Store{Client: writeFailure, RefreshTable: "refresh", Clock: fixedStoreClock{at: now}}).RevokeSessionFamily(context.Background(), "hash")
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeTrue())
		Expect(writeFailure.transacts).To(HaveLen(1))
	})

	It("maps a 0192f8a0-7b00-7000-8000-000000000012-marker condition conflict to an invalid refresh token", func() {
		now := time.Now().UTC().Truncate(time.Second)
		stub := &logoutAPIStub{
			items: []map[string]types.AttributeValue{
				{"tokenHash": &types.AttributeValueMemberS{Value: "hash"}, "sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}, "userId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(-time.Minute).Unix(), 10)}, "expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(time.Minute).Unix(), 10)}},
				{"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}, "status": &types.AttributeValueMemberS{Value: "active"}},
			},
			transErr: &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{
				{Code: aws.String("None")},
				{Code: aws.String("ConditionalCheckFailed")},
			}},
		}

		err := (Store{Client: stub, RefreshTable: "refresh", Clock: fixedStoreClock{at: now}}).RevokeSessionFamily(context.Background(), "hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(stub.transacts).To(HaveLen(1))
	})

	It("rejects a malformed session without writing a 0192f8a0-7b00-7000-8000-000000000012 marker", func() {
		stub := &logoutAPIStub{item: map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: "hash"}}}
		err := (Store{Client: stub, RefreshTable: "refresh"}).RevokeSessionFamily(context.Background(), "hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(stub.transacts).To(BeEmpty())
	})

	It("rejects a malformed 0192f8a0-7b00-7000-8000-000000000012 marker without writing a revocation", func() {
		now := time.Now().UTC().Truncate(time.Second)
		token := map[string]types.AttributeValue{
			"tokenHash": &types.AttributeValueMemberS{Value: "hash"},
			"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"},
			"userId":    &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"},
			"status":    &types.AttributeValueMemberS{Value: "active"},
			"createdAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(-time.Minute).Unix(), 10)},
			"expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(time.Minute).Unix(), 10)},
		}
		stub := &logoutAPIStub{items: []map[string]types.AttributeValue{token, {"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}}}}
		err := (Store{Client: stub, RefreshTable: "refresh", Clock: fixedStoreClock{at: now}}).RevokeSessionFamily(context.Background(), "hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(stub.transacts).To(BeEmpty())
	})
})

var _ = Describe("RevokeSessionFamilyUnknownToken", func() {
	It("rejects an unknown refresh token", func() {
		stub := &logoutAPIStub{}
		err := (Store{Client: stub, RefreshTable: "refresh"}).RevokeSessionFamily(context.Background(), "unknown")
		Expect(err).To(Equal(auth.ErrInvalidRefreshToken))
	})
})

var _ = Describe("GetRefreshSession", func() {
	It("requires an active token and an unrevoked 0192f8a0-7b00-7000-8000-000000000012 marker", func() {
		created := time.Unix(100, 0).UTC()
		now := time.Now().UTC().Truncate(time.Second)
		expires := now.Add(time.Minute)
		stub := &logoutAPIStub{items: []map[string]types.AttributeValue{
			{"tokenHash": &types.AttributeValueMemberS{Value: "hash"}, "sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}, "userId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: "100"}, "expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(expires.Unix(), 10)}},
			{"tokenHash": &types.AttributeValueMemberS{Value: "FAMILY#0192f8a0-7b00-7000-8000-000000000012"}, "sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}, "status": &types.AttributeValueMemberS{Value: "active"}},
		}}
		got, err := (Store{Client: stub, RefreshTable: "refresh", Clock: fixedStoreClock{at: now}}).GetRefreshSession(context.Background(), "hash")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.SessionID).To(Equal("0192f8a0-7b00-7000-8000-000000000012"))
		Expect(platformid.IsCanonicalUUIDv7(got.UserID)).To(BeTrue())
		Expect(got.CreatedAt).To(Equal(created))
		Expect(stub.gets).To(HaveLen(2))
		for _, in := range stub.gets {
			Expect(*in.ConsistentRead).To(BeTrue())
		}
		Expect(stub.gets[0].Key["tokenHash"]).To(Equal(&types.AttributeValueMemberS{Value: "hash"}))
		Expect(stub.gets[1].Key["tokenHash"]).To(Equal(&types.AttributeValueMemberS{Value: "FAMILY#0192f8a0-7b00-7000-8000-000000000012"}))
	})

	It("rejects missing, malformed, expired, and exactly-now expiration timestamps", func() {
		now := time.Unix(1_000, 0).UTC()
		for _, expiresAt := range []types.AttributeValue{
			nil,
			&types.AttributeValueMemberS{Value: "not-a-number"},
			&types.AttributeValueMemberN{Value: "999"},
			&types.AttributeValueMemberN{Value: "1000"},
		} {
			item := map[string]types.AttributeValue{
				"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"},
				"status":    &types.AttributeValueMemberS{Value: "active"},
			}
			if expiresAt != nil {
				item["expiresAt"] = expiresAt
			}
			stub := &logoutAPIStub{item: item}
			store := Store{Client: stub, RefreshTable: "refresh", Clock: fixedStoreClock{at: now}}
			_, err := store.GetRefreshSession(context.Background(), "hash")
			Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
			Expect(stub.gets).To(HaveLen(1))
		}
	})

	It("rejects malformed, revoked, and dependency-failed reads", func() {
		malformed := &logoutAPIStub{item: map[string]types.AttributeValue{}}
		_, err := (Store{Client: malformed, RefreshTable: "refresh"}).GetRefreshSession(context.Background(), "hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		revoked := &logoutAPIStub{item: map[string]types.AttributeValue{
			"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"},
			"status":    &types.AttributeValueMemberS{Value: "revoked"},
		}}
		_, err = (Store{Client: revoked, RefreshTable: "refresh"}).GetRefreshSession(context.Background(), "hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))

		validToken := map[string]types.AttributeValue{
			"tokenHash": &types.AttributeValueMemberS{Value: "hash"},
			"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"},
			"userId":    &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"},
			"status":    &types.AttributeValueMemberS{Value: "active"},
			"createdAt": &types.AttributeValueMemberN{Value: "100"},
			"expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
		}
		for _, malformedMarker := range []map[string]types.AttributeValue{
			{"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}},
			{"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}, "status": &types.AttributeValueMemberS{Value: "unknown"}},
			{"status": &types.AttributeValueMemberS{Value: "active"}},
		} {
			malformedMarkerStore := &logoutAPIStub{items: []map[string]types.AttributeValue{validToken, malformedMarker}}
			_, err = (Store{Client: malformedMarkerStore, RefreshTable: "refresh"}).GetRefreshSession(context.Background(), "hash")
			Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		}

		stub := &logoutAPIStub{getErr: context.DeadlineExceeded}
		_, err = (Store{Client: stub, RefreshTable: "refresh"}).GetRefreshSession(context.Background(), "hash")
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeTrue())
	})

	It("rejects consumed tokens and tokens from revoked families", func() {
		consumed := &logoutAPIStub{item: map[string]types.AttributeValue{
			"sessionId": &types.AttributeValueMemberS{Value: "consumed-0192f8a0-7b00-7000-8000-000000000012"},
			"status":    &types.AttributeValueMemberS{Value: "consumed"},
			"expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
		}}
		_, err := (Store{Client: consumed, RefreshTable: "refresh"}).GetRefreshSession(context.Background(), "consumed-hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(consumed.gets).To(HaveLen(1))

		revokedFamily := &logoutAPIStub{items: []map[string]types.AttributeValue{
			{
				"tokenHash": &types.AttributeValueMemberS{Value: "revoked-family-hash"},
				"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"},
				"userId":    &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"},
				"status":    &types.AttributeValueMemberS{Value: "active"},
				"createdAt": &types.AttributeValueMemberN{Value: "100"},
				"expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
			},
			{"sessionId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000012"}, "status": &types.AttributeValueMemberS{Value: "revoked"}},
		}}
		_, err = (Store{Client: revokedFamily, RefreshTable: "refresh"}).GetRefreshSession(context.Background(), "revoked-family-hash")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(revokedFamily.gets).To(HaveLen(2))
	})
})
