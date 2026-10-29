package dynamodb

import (
	"context"
	"errors"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

type loginStoreAPIFixture struct {
	*MockAPI
	*MockqueryAPI
	queryOut *dynamodb.QueryOutput
	queryErr error
	transErr error
	queryIn  *dynamodb.QueryInput
	transIn  *dynamodb.TransactWriteItemsInput
}

func newLoginStoreAPI(queryOut *dynamodb.QueryOutput, queryErr, transErr error) *loginStoreAPIFixture {
	f := &loginStoreAPIFixture{MockAPI: NewMockAPI(gomock.NewController(GinkgoT())), MockqueryAPI: NewMockqueryAPI(gomock.NewController(GinkgoT())), queryOut: queryOut, queryErr: queryErr, transErr: transErr}
	f.MockAPI.EXPECT().GetItem(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(&dynamodb.GetItemOutput{}, nil)
	f.MockqueryAPI.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
		f.queryIn = in
		return f.queryOut, f.queryErr
	})
	f.MockAPI.EXPECT().TransactWriteItems(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, in *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
		f.transIn = in
		return &dynamodb.TransactWriteItemsOutput{}, f.transErr
	})
	return f
}

func newLoginStoreWithoutQuery() *MockAPI { return NewMockAPI(gomock.NewController(GinkgoT())) }

var _ = Describe("LoginStoreFindByLogin", func() {
	It("maps users and chooses the username or email index", func() {
		created := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
		for _, tc := range []struct {
			login, index, field string
		}{
			{login: "alice", index: "UsernameIndex", field: "username"},
			{login: "alice@example.com", index: "EmailIndex", field: "email"},
		} {
			api := newLoginStoreAPI(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{{
				"userId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000001"}, "username": &types.AttributeValueMemberS{Value: "alice"}, "email": &types.AttributeValueMemberS{Value: "alice@example.com"}, "passwordHash": &types.AttributeValueMemberS{Value: "hash"}, "displayName": &types.AttributeValueMemberS{Value: "Alice"}, "role": &types.AttributeValueMemberS{Value: "user"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: created.Format(time.RFC3339)},
			}}}, nil, nil)
			store := Store{Client: api, UsersTable: "users"}
			got, err := store.FindByLogin(context.Background(), tc.login)
			Expect(err).NotTo(HaveOccurred())
			Expect(got.ID).To(Equal("0192f8a0-7b00-7000-8000-000000000001"))
			Expect(platformid.IsCanonicalUUIDv7(got.ID)).To(BeTrue())
			Expect(got.CreatedAt).To(Equal(created))
			Expect(aws.ToString(api.queryIn.IndexName)).To(Equal(tc.index))
			Expect(api.queryIn.ExpressionAttributeNames["#login"]).To(Equal(tc.field))
			Expect(aws.ToInt32(api.queryIn.Limit)).To(Equal(int32(1)))
			Expect(api.queryIn.ExclusiveStartKey).To(BeEmpty())
		}
	})

	It("uses the first page deterministically when DynamoDB reports more results", func() {
		api := newLoginStoreAPI(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{{
				"userId":    &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000002"},
				"createdAt": &types.AttributeValueMemberS{Value: time.Unix(100, 0).UTC().Format(time.RFC3339)},
			}}, LastEvaluatedKey: map[string]types.AttributeValue{"email": &types.AttributeValueMemberS{Value: "next"}},
		}, nil, nil)
		got, err := (Store{Client: api}).FindByLogin(context.Background(), "a@example.com")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.ID).To(Equal("0192f8a0-7b00-7000-8000-000000000002"))
		Expect(aws.ToInt32(api.queryIn.Limit)).To(Equal(int32(1)))
	})
})

var _ = Describe("LoginStoreFindByLoginFailures", func() {
	It("maps missing, malformed, and query failures", func() {
		missing := newLoginStoreAPI(&dynamodb.QueryOutput{}, nil, nil)
		_, err := (Store{Client: missing}).FindByLogin(context.Background(), "alice")
		Expect(err).To(MatchError(user.ErrNotFound))

		api := newLoginStoreAPI(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{{"createdAt": &types.AttributeValueMemberS{Value: "bad"}}}}, nil, nil)
		_, err = (Store{Client: api}).FindByLogin(context.Background(), "alice")
		Expect(err).To(HaveOccurred())

		api.queryErr = errors.New("query failed")
		_, err = (Store{Client: api}).FindByLogin(context.Background(), "alice")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("LoginStoreFindByLoginDependencies", func() {
	It("rejects a client without query support", func() {
		_, err := (Store{Client: newLoginStoreWithoutQuery()}).FindByLogin(context.Background(), "alice")
		Expect(err).To(MatchError(auth.ErrDependencyUnavailable))
	})
})

var _ = Describe("LoginStoreAttributeMapping", func() {
	It("maps absent optional attributes to empty values", func() {
		api := newLoginStoreAPI(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{{
			"createdAt": &types.AttributeValueMemberS{Value: time.Unix(100, 0).UTC().Format(time.RFC3339)},
		}}}, nil, nil)
		got, err := (Store{Client: api}).FindByLogin(context.Background(), "alice")
		Expect(err).To(MatchError(auth.ErrDependencyUnavailable))
		Expect(got.ID).To(BeEmpty())
	})
})

var _ = Describe("LoginStoreCreateLoginSession", func() {
	It("writes expiry and translates failures", func() {
		created := time.Unix(100, 0).UTC()
		expires := created.Add(7 * 24 * time.Hour)
		api := newLoginStoreAPI(nil, nil, nil)
		store := Store{Client: api, RefreshTable: "refresh"}
		Expect(store.CreateLoginSession(context.Background(), auth.RefreshSession{TokenHash: "hash", SessionID: "0192f8a0-7b00-7000-8000-000000000003", UserID: "0192f8a0-7b00-7000-8000-000000000001", CreatedAt: created, ExpiresAt: expires})).To(Succeed())
		Expect(platformid.IsCanonicalUUIDv7("0192f8a0-7b00-7000-8000-000000000001")).To(BeTrue())
		Expect(api.transIn).NotTo(BeNil())
		Expect(api.transIn.TransactItems).To(HaveLen(2))
		ttl, ok := api.transIn.TransactItems[1].Put.Item["ttl"].(*types.AttributeValueMemberN)
		Expect(ok).To(BeTrue())
		Expect(ttl.Value).To(Equal("604900"))
		api.transErr = errors.New("write failed")
		Expect(store.CreateLoginSession(context.Background(), auth.RefreshSession{})).To(HaveOccurred())
	})
})

var _ = Describe("CreateLoginSessionRejectsInvalidPersistenceIDs", func() {
	It("rejects invalid IDs before issuing a transaction", func() {
		for _, session := range []auth.RefreshSession{
			{SessionID: "invalid", UserID: "0192f8a0-7b00-7000-8000-000000000001"},
			{SessionID: "0192f8a0-7b00-7000-8000-000000000002", UserID: "invalid"},
		} {
			api := newLoginStoreAPI(nil, nil, nil)
			err := (Store{Client: api}).CreateLoginSession(context.Background(), session)
			Expect(errors.Is(err, auth.ErrValidation)).To(BeTrue())
			Expect(api.transIn).To(BeNil())
		}
	})
})
