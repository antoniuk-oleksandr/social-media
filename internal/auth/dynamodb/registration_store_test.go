package dynamodb

import (
	"context"
	"errors"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
)

type testNetworkError struct{}

func (testNetworkError) Error() string   { return "network" }
func (testNetworkError) Timeout() bool   { return true }
func (testNetworkError) Temporary() bool { return true }

var _ net.Error = testNetworkError{}

type fakeAPI struct {
	transactionErr error
	transactionIn  *dynamodb.TransactWriteItemsInput
	transactionN   int
	getErr         error
	items          map[string]map[string]types.AttributeValue
	mu             sync.Mutex
}

func (f *fakeAPI) TransactWriteItems(_ context.Context, in *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transactionIn = in
	f.transactionN++
	return &dynamodb.TransactWriteItemsOutput{}, f.transactionErr
}
func (f *fakeAPI) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
}

func registration() auth.Registration {
	return auth.Registration{User: user.User{ID: "new-user", Username: "alice", Email: "alice@example.com", PasswordHash: "hash", Role: user.RoleUser, Status: user.StatusActive}, Refresh: auth.RefreshSession{TokenHash: "refresh-hash", SessionID: "session", UserID: "new-user"}}
}

func canceled(codes ...string) error {
	reasons := make([]types.CancellationReason, len(codes))
	for i, code := range codes {
		reasons[i] = types.CancellationReason{Code: aws.String(code)}
	}
	return &types.TransactionCanceledException{CancellationReasons: reasons}
}

var _ = Describe("RegisterMapsEmailCancellationReason", func() {
	It("runs the test", func() {
		store := Store{Client: &fakeAPI{transactionErr: canceled("ConditionalCheckFailed", "None")}, UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); !errors.Is(err, auth.ErrEmailTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterMapsUsernameCancellationReason", func() {
	It("runs the test", func() {
		store := Store{Client: &fakeAPI{transactionErr: canceled("None", "ConditionalCheckFailed")}, UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); !errors.Is(err, auth.ErrUsernameTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterTreatsBothCancellationReasonsAsAmbiguous", func() {
	It("runs the test", func() {
		store := Store{Client: &fakeAPI{transactionErr: canceled("ConditionalCheckFailed", "ConditionalCheckFailed")}, UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
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
				store := Store{Client: &fakeAPI{transactionErr: canceled()}, UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
				store.Client.(*fakeAPI).items = map[string]map[string]types.AttributeValue{tc.key: {"uniqueKey": &types.AttributeValueMemberS{Value: tc.key}}}
				if err := store.Register(context.Background(), registration()); !errors.Is(err, tc.want) {
					Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
				}
			}
		}

	})
})

var _ = Describe("RegisterFallbackUsesDeterministicUsernamePrecedenceWhenBothExist", func() {
	It("runs the test", func() {
		store := Store{Client: &fakeAPI{transactionErr: canceled(), items: map[string]map[string]types.AttributeValue{
			"EMAIL#alice@example.com": {"uniqueKey": &types.AttributeValueMemberS{Value: "email"}},
			"USERNAME#alice":          {"uniqueKey": &types.AttributeValueMemberS{Value: "username"}},
		}}, UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); !errors.Is(err, auth.ErrUsernameTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterFallbackTreatsNeitherReservationAsAmbiguous", func() {
	It("runs the test", func() {
		store := Store{Client: &fakeAPI{transactionErr: canceled(), items: map[string]map[string]types.AttributeValue{}}, UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); err == nil || errors.Is(err, auth.ErrEmailTaken) || errors.Is(err, auth.ErrUsernameTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterConcurrentDuplicateUsesCancellationReason", func() {
	It("runs the test", func() {
		api := &fakeAPI{transactionErr: canceled("None", "ConditionalCheckFailed")}
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
		api := &fakeAPI{
			transactionErr: &smithy.GenericAPIError{Code: "TransactionCanceledException", Message: "Transaction cancelled"},
			items: map[string]map[string]types.AttributeValue{
				"USERNAME#alice": {"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#alice"}},
			},
		}
		store := Store{Client: api, UniquesTable: "uniques", UsersTable: "users", RefreshTable: "refresh"}
		if err := store.Register(context.Background(), registration()); !errors.Is(err, auth.ErrUsernameTaken) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", err))
		}

	})
})

var _ = Describe("RegisterSuccess", func() {
	It("runs the test", func() {
		store := Store{Client: &fakeAPI{}, UniquesTable: "u", UsersTable: "users", RefreshTable: "r"}
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
		r.User.CreatedAt = created
		r.Refresh.CreatedAt = created
		r.Refresh.ExpiresAt = expires
		api := &fakeAPI{}
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
		store := Store{Client: &fakeAPI{transactionErr: tx}, UniquesTable: "u", UsersTable: "users", RefreshTable: "r"}
		if err := store.Register(context.Background(), registration()); err == nil || !errors.Is(err, tx) {
			Expect(false).To(BeTrue(), fmt.Sprintf("%v", err))
		}
		lookup := errors.New("lookup")
		store.Client = &fakeAPI{transactionErr: canceled(), getErr: lookup}
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
		if !isDependencyUnavailable(testNetworkError{}) {
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

type secondLookupError struct {
	fakeAPI
	calls int
}

func (f *secondLookupError) GetItem(ctx context.Context, in *dynamodb.GetItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.calls++
	if f.calls == 2 {
		return nil, errors.New("second lookup")
	}
	return f.fakeAPI.GetItem(ctx, in, opts...)
}

var _ = Describe("LookupReservationConflictPropagatesSecondLookupError", func() {
	It("runs the test", func() {
		errClient := &secondLookupError{fakeAPI: fakeAPI{items: map[string]map[string]types.AttributeValue{}}}
		store := Store{Client: errClient, UniquesTable: "uniques"}
		if _, err := store.lookupReservationConflict(context.Background(), "email", "username"); err == nil || !strings.Contains(err.Error(), "second lookup") {
			Expect(false).To(BeTrue(), fmt.Sprintf("lookup error = %v", err))
		}
	})
})

var _ = Describe("FindByID", func() {
	It("maps a stored user and uses a strongly consistent lookup", func() {
		created := time.Unix(100, 0).UTC()
		api := &fakeAPI{items: map[string]map[string]types.AttributeValue{
			"user-1": {
				"userId": &types.AttributeValueMemberS{Value: "user-1"}, "username": &types.AttributeValueMemberS{Value: "alice"},
				"email": &types.AttributeValueMemberS{Value: "alice@example.com"}, "passwordHash": &types.AttributeValueMemberS{Value: "hash"},
				"displayName": &types.AttributeValueMemberS{Value: "Alice"}, "role": &types.AttributeValueMemberS{Value: "user"},
				"status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: created.Format(time.RFC3339)},
			},
		}}
		got, err := (Store{Client: api, UsersTable: "users"}).FindByID(context.Background(), "user-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.ID).To(Equal("user-1"))
		Expect(got.CreatedAt).To(Equal(created))
	})

	It("maps missing, dependency, and malformed records", func() {
		missing := &fakeAPI{items: map[string]map[string]types.AttributeValue{}}
		_, err := (Store{Client: missing}).FindByID(context.Background(), "missing")
		Expect(err).To(MatchError(user.ErrNotFound))

		dependency := &fakeAPI{getErr: testNetworkError{}}
		_, err = (Store{Client: dependency}).FindByID(context.Background(), "user")
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeTrue())

		malformed := &fakeAPI{items: map[string]map[string]types.AttributeValue{
			"user": {"createdAt": &types.AttributeValueMemberS{Value: "not-a-time"}},
		}}
		_, err = (Store{Client: malformed}).FindByID(context.Background(), "user")
		Expect(err).To(MatchError(ContainSubstring("decode user")))
	})
})
