package dynamodb

import (
	"context"
	"errors"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"go.uber.org/mock/gomock"
)

type registrationAPIFixture struct {
	*MockAPI
	*MockqueryAPI
	transactionErr error
	transactionIn  *dynamodb.TransactWriteItemsInput
	transactionN   int
	getErr         error
	items          map[string]map[string]types.AttributeValue
	queryErr       error
	queryOut       *dynamodb.QueryOutput
	mu             sync.Mutex
	getCalls       int
	secondGetErr   error
}

func newRegistrationAPI(transactionErr, getErr, queryErr error, items map[string]map[string]types.AttributeValue) *registrationAPIFixture {
	f := &registrationAPIFixture{MockAPI: NewMockAPI(gomock.NewController(GinkgoT())), MockqueryAPI: NewMockqueryAPI(gomock.NewController(GinkgoT())), transactionErr: transactionErr, getErr: getErr, queryErr: queryErr, items: items}
	f.MockAPI.EXPECT().TransactWriteItems(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, in *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.transactionIn = in
		f.transactionN++
		return &dynamodb.TransactWriteItemsOutput{}, f.transactionErr
	})
	f.MockAPI.EXPECT().GetItem(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.getCalls++
		if f.secondGetErr != nil && f.getCalls == 2 {
			return nil, f.secondGetErr
		}
		if f.getErr != nil {
			return nil, f.getErr
		}
		keyName := "uniqueKey"
		if _, ok := in.Key["tokenHash"]; ok {
			keyName = "tokenHash"
		} else if _, ok := in.Key["userId"]; ok {
			keyName = "userId"
		}
		key := in.Key[keyName].(*types.AttributeValueMemberS).Value
		return &dynamodb.GetItemOutput{Item: f.items[key]}, nil
	})
	f.MockqueryAPI.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
		if f.queryErr != nil {
			return nil, f.queryErr
		}
		if f.queryOut != nil {
			return f.queryOut, nil
		}
		return &dynamodb.QueryOutput{}, nil
	})
	return f
}

func registration() auth.Registration {
	id := "0192f8a0-7b00-7000-8000-000000000001"
	now := time.Unix(100, 0).UTC()
	return auth.Registration{User: user.User{ID: id, Username: "alice", Email: "alice@example.com", PasswordHash: "hash", DisplayName: "Alice", Role: user.RoleUser, Status: user.StatusActive, CreatedAt: now}, Refresh: auth.RefreshSession{TokenHash: "refresh-hash", SessionID: "0192f8a0-7b00-7000-8000-000000000003", UserID: id, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}}
}

var _ = Describe("RegisterMapsEmailCancellationReason", func() {
	It("runs the test", func() {
		store := Store{Client: newRegistrationAPI(canceled("ConditionalCheckFailed", "None"), nil, nil, nil), UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); !errors.Is(err, auth.ErrEmailTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterMapsUsernameCancellationReason", func() {
	It("runs the test", func() {
		store := Store{Client: newRegistrationAPI(canceled("None", "ConditionalCheckFailed"), nil, nil, nil), UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); !errors.Is(err, auth.ErrUsernameTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterTreatsBothCancellationReasonsAsAmbiguous", func() {
	It("runs the test", func() {
		store := Store{Client: newRegistrationAPI(canceled("ConditionalCheckFailed", "ConditionalCheckFailed"), nil, nil, nil), UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); err == nil || errors.Is(err, auth.ErrEmailTaken) || errors.Is(err, auth.ErrUsernameTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterFallbackIdentifiesSingleReservation", func() {
	It("runs the test", func() {
		for _, tc := range []struct {
			name, key string
			want      error
		}{
			{"email", "EMAIL#alice@example.com", auth.ErrEmailTaken},
			{"username", "USERNAME#alice", auth.ErrUsernameTaken},
		} {
			{
				store := Store{Client: newRegistrationAPI(canceled(), nil, nil, nil), UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
				store.Client.(*registrationAPIFixture).items = map[string]map[string]types.AttributeValue{tc.key: {"uniqueKey": &types.AttributeValueMemberS{Value: tc.key}}}
				if err := store.Register(context.Background(), registration()); !errors.Is(err, tc.want) {
					Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
				}
			}
		}

	})
})

var _ = Describe("RegisterFallbackUsesDeterministicUsernamePrecedenceWhenBothExist", func() {
	It("runs the test", func() {
		store := Store{Client: newRegistrationAPI(canceled(), nil, nil, map[string]map[string]types.AttributeValue{
			"EMAIL#alice@example.com": {"uniqueKey": &types.AttributeValueMemberS{Value: "email"}},
			"USERNAME#alice":          {"uniqueKey": &types.AttributeValueMemberS{Value: "username"}},
		}), UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); !errors.Is(err, auth.ErrUsernameTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterFallbackTreatsNeitherReservationAsAmbiguous", func() {
	It("runs the test", func() {
		store := Store{Client: newRegistrationAPI(canceled(), nil, nil, map[string]map[string]types.AttributeValue{}), UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); err == nil || errors.Is(err, auth.ErrEmailTaken) || errors.Is(err, auth.ErrUsernameTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterConcurrentDuplicateUsesCancellationReason", func() {
	It("runs the test", func() {
		api := newRegistrationAPI(canceled("None", "ConditionalCheckFailed"), nil, nil, nil)
		store := Store{Client: api, UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for range 2 {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- store.Register(context.Background(), registration()) }()
		}
		wg.Wait()
		close(errs)
		var successes, usernames int
		for err := range errs {
			if err == nil {
				successes++
			}
			if errors.Is(err, auth.ErrUsernameTaken) {
				usernames++
			}
		}
		if successes != 0 || usernames != 2 {
			Expect(false).To(BeTrue(), fmt.Sprintf("successes=%d usernames=%d", successes, usernames))
		}

	})
})

var _ = Describe("RegisterMapsGenericTransactionCanceledErrorUsingReservations", func() {
	It("runs the test", func() {
		api := newRegistrationAPI(&smithy.GenericAPIError{Code: "TransactionCanceledException", Message: "Transaction cancelled"}, nil, nil, map[string]map[string]types.AttributeValue{
			"USERNAME#alice": {"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#alice"}},
		})
		store := Store{Client: api, UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); !errors.Is(err, auth.ErrUsernameTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterSuccess", func() {
	It("runs the test", func() {
		store := Store{Client: newRegistrationAPI(nil, nil, nil, nil), UniquesTable: "u", UsersTable: "users", RefreshTable: "r"}
		if err := store.Register(context.Background(), registration()); err != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(err))
		}

	})
})

var _ = Describe("RegisterBuildsCompleteTransactionPayload", func() {
	It("runs the test", func() {
		created := time.Unix(1_700_000_000, 0).UTC()
		expires := created.Add(7 * 24 * time.Hour)
		r := registration()
		Expect(platformid.IsCanonicalUUIDv7(r.User.ID)).To(BeTrue())
		Expect(platformid.IsCanonicalUUIDv7(r.Refresh.UserID)).To(BeTrue())
		r.Refresh.CreatedAt = created
		r.Refresh.ExpiresAt = expires
		api := newRegistrationAPI(nil, nil, nil, nil)
		store := Store{Client: api, UniquesTable: "uniques-table", UsersTable: "users-table", RefreshTable: "refresh-table"}

		if err := store.Register(context.Background(), r); err != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(err))
		}
		api.mu.Lock()
		in := api.transactionIn
		calls := api.transactionN
		api.mu.Unlock()
		if calls != 1 || in == nil {
			Expect(false).To(BeTrue(), fmt.Sprintf("transaction calls=%d input=%v, want one captured input", calls, in))
		}
		if len(in.TransactItems) != 4 {
			Expect(false).To(BeTrue(), fmt.Sprintf("transaction item count=%d, want 4", len(in.TransactItems)))
		}

		assertString := func(index int, table, key, want string) {
			put := in.TransactItems[index].Put
			if put == nil || aws.ToString(put.TableName) != table {
				Expect(false).To(BeTrue(), fmt.Sprintf("item %d table=%v, want %q", index, put, table))
			}
			got, ok := put.Item[key].(*types.AttributeValueMemberS)
			if !ok || got.Value != want {
				Expect(false).To(BeTrue(), fmt.Sprintf("item %d %s=%v, want %q", index, key, put.Item[key], want))
			}
		}

		assertString(0, "uniques-table", "uniqueKey", "EMAIL#alice@example.com")
		assertString(1, "uniques-table", "uniqueKey", "USERNAME#alice")
		userPut := in.TransactItems[2].Put
		if userPut == nil || aws.ToString(userPut.TableName) != "users-table" {
			Expect(false).To(BeTrue(), fmt.Sprintf("user item table=%v, want users-table", userPut))
		}
		for key, want := range map[string]string{
			"userId": r.User.ID, "username": r.User.Username, "email": r.User.Email,
			"passwordHash": r.User.PasswordHash, "displayName": r.User.DisplayName,
			"role": string(r.User.Role), "status": string(r.User.Status),
			"createdAt": r.User.CreatedAt.UTC().Format(time.RFC3339),
		} {
			got, ok := userPut.Item[key].(*types.AttributeValueMemberS)
			if !ok || got.Value != want {
				Expect(false).To(BeTrue(), fmt.Sprintf("user item %s=%v, want %q", key, userPut.Item[key], want))
			}
		}
		if aws.ToString(userPut.ConditionExpression) != "attribute_not_exists(userId)" {
			Expect(false).To(BeTrue(), fmt.Sprintf("user condition=%q", aws.ToString(userPut.ConditionExpression)))
		}

		refreshPut := in.TransactItems[3].Put
		Expect(platformid.IsCanonicalUUIDv7(refreshPut.Item["userId"].(*types.AttributeValueMemberS).Value)).To(BeTrue())
		if refreshPut == nil || aws.ToString(refreshPut.TableName) != "refresh-table" {
			Expect(false).To(BeTrue(), fmt.Sprintf("refresh item table=%v, want refresh-table", refreshPut))
		}
		for key, want := range map[string]string{
			"tokenHash": r.Refresh.TokenHash, "sessionId": r.Refresh.SessionID, "userId": r.Refresh.UserID,
			"status": "active", "createdAt": strconv.FormatInt(created.Unix(), 10),
			"expiresAt": strconv.FormatInt(expires.Unix(), 10), "ttl": strconv.FormatInt(expires.Unix(), 10), "version": "1",
		} {
			got, ok := refreshPut.Item[key].(*types.AttributeValueMemberS)
			if ok {
				if got.Value != want {
					Expect(false).To(BeTrue(), fmt.Sprintf("refresh item %s=%q, want %q", key, got.Value, want))
				}
				continue
			}
			number, numberOK := refreshPut.Item[key].(*types.AttributeValueMemberN)
			if !numberOK || number.Value != want {
				Expect(false).To(BeTrue(), fmt.Sprintf("refresh item %s=%v, want %q", key, refreshPut.Item[key], want))
			}
		}
		if aws.ToString(refreshPut.ConditionExpression) != "attribute_not_exists(tokenHash)" {
			Expect(false).To(BeTrue(), fmt.Sprintf("refresh condition=%q", aws.ToString(refreshPut.ConditionExpression)))
		}

	})
})

var _ = Describe("RegisterWrapsTransactionAndLookupErrors", func() {
	It("runs the test", func() {
		tx := errors.New("transaction")
		store := Store{Client: newRegistrationAPI(tx, nil, nil, nil), UniquesTable: "u", UsersTable: "users", RefreshTable: "r"}
		if err := store.Register(context.Background(), registration()); err == nil || !errors.Is(err, tx) {
			Expect(false).To(BeTrue(), fmt.Sprintf("%v", err))
		}
		lookup := errors.New("lookup")
		store.Client = newRegistrationAPI(canceled(), lookup, nil, nil)
		if err := store.Register(context.Background(), registration()); err == nil || !errors.Is(err, lookup) {
			Expect(false).To(BeTrue(), fmt.Sprintf("%v", err))
		}

	})
})
var _ = Describe("CancellationHelpers", func() {
	It("runs the test", func() {
		if _, ok := cancellationConflict(errors.New("not canceled")); ok {
			Expect(false).To(BeTrue(), fmt.Sprint("matched ordinary error"))
		}
		if conditionalFailure(types.CancellationReason{}) {
			Expect(false).To(BeTrue(), fmt.Sprint("nil code matched"))
		}
		if isTransactionCanceled(errors.New("ordinary")) {
			Expect(false).To(BeTrue(), fmt.Sprint("ordinary error matched"))
		}
		if !isTransactionCanceled(&smithy.GenericAPIError{Code: "TransactionCanceledException"}) {
			Expect(false).To(BeTrue(), fmt.Sprint("transaction API error was not recognized"))
		}

	})
})

var _ = Describe("DependencyErrorClassification", func() {
	It("runs the test", func() {
		if !isDependencyUnavailable(context.DeadlineExceeded) {
			Expect(false).To(BeTrue(), fmt.Sprint("deadline should be unavailable"))
		}
		apiErr := &smithy.GenericAPIError{Code: "ProvisionedThroughputExceededException", Message: "throttled"}
		if !isDependencyUnavailable(apiErr) {
			Expect(false).To(BeTrue(), fmt.Sprint("API error should be unavailable"))
		}
		if !isDependencyUnavailable(networkError()) {
			Expect(false).To(BeTrue(), fmt.Sprint("network error should be unavailable"))
		}
		ordinary := errors.New("ordinary")
		if got := wrapDependencyError("lookup", ordinary); !errors.Is(got, ordinary) {
			Expect(false).To(BeTrue(), fmt.Sprintf("wrapped error = %v", got))
		}
		if got := wrapDependencyError("lookup", context.DeadlineExceeded); !errors.Is(got, auth.ErrDependencyUnavailable) {
			Expect(false).To(BeTrue(), fmt.Sprintf("dependency error = %v", got))
		}

	})
})

var _ = Describe("LookupReservationConflictPropagatesSecondLookupError", func() {
	It("runs the test", func() {
		errClient := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{})
		errClient.secondGetErr = errors.New("second lookup")
		store := Store{Client: errClient, UniquesTable: "uniques"}
		if _, err := store.lookupReservationConflict(context.Background(), "email", "username"); err == nil || !strings.Contains(err.Error(), "second lookup") {
			Expect(false).To(BeTrue(), fmt.Sprintf("lookup error = %v", err))
		}
	})
})

var _ = Describe("FindByID", func() {
	It("maps a stored user and uses a strongly consistent lookup", func() {
		created := time.Unix(100, 0).UTC()
		api := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{
			"0192f8a0-7b00-7000-8000-000000000002": {
				"userId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000002"}, "username": &types.AttributeValueMemberS{Value: "alice"},
				"email": &types.AttributeValueMemberS{Value: "alice@example.com"}, "passwordHash": &types.AttributeValueMemberS{Value: "hash"},
				"displayName": &types.AttributeValueMemberS{Value: "Alice"}, "role": &types.AttributeValueMemberS{Value: "user"},
				"status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: created.Format(time.RFC3339)},
			},
		})
		got, err := (Store{Client: api, UsersTable: "users"}).FindByID(context.Background(), "0192f8a0-7b00-7000-8000-000000000002")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.ID).To(Equal("0192f8a0-7b00-7000-8000-000000000002"))
		Expect(got.CreatedAt).To(Equal(created))
	})

	It("maps missing, dependency, and malformed records", func() {
		missing := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{})
		_, err := (Store{Client: missing}).FindByID(context.Background(), "0192f8a0-7b00-7000-8000-000000000099")
		Expect(err).To(MatchError(user.ErrNotFound))

		dependency := newRegistrationAPI(nil, networkError(), nil, nil)
		_, err = (Store{Client: dependency}).FindByID(context.Background(), "0192f8a0-7b00-7000-8000-000000000003")
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeTrue())

		malformed := newRegistrationAPI(nil, nil, nil, map[string]map[string]types.AttributeValue{
			"0192f8a0-7b00-7000-8000-000000000003": {"createdAt": &types.AttributeValueMemberS{Value: "not-a-time"}},
		})
		_, err = (Store{Client: malformed}).FindByID(context.Background(), "0192f8a0-7b00-7000-8000-000000000003")
		Expect(err).To(MatchError(user.ErrDependencyUnavailable))
	})
})

var _ = Describe("RegisterRejectsInvalidPersistenceIDs", func() {
	It("rejects invalid user or session IDs before starting a transaction", func() {
		for _, mutate := range []func(*auth.Registration){
			func(r *auth.Registration) { r.User.ID = "numeric-user" },
			func(r *auth.Registration) { r.Refresh.SessionID = "numeric-session" },
			func(r *auth.Registration) { r.Refresh.UserID = "numeric-user" },
		} {
			api := newRegistrationAPI(nil, nil, nil, nil)
			r := registration()
			mutate(&r)
			err := (Store{Client: api}).Register(context.Background(), r)
			Expect(errors.Is(err, auth.ErrValidation)).To(BeTrue())
			Expect(api.transactionN).To(Equal(0))
		}
	})
})
