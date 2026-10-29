package ad

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

type repositoryFixture struct {
	*MockRepository
	value      Advertisement
	err        error
	getID      string
	getCalls   int
	deleteID   string
	listQuery  ListQuery
	listCalls  int
	listResult ListResult
}

func newRepositoryFixture(value Advertisement, err error, listResult ListResult) *repositoryFixture {
	return newRepositoryFixtureWithReporter(GinkgoT(), value, err, listResult)
}

func newRepositoryFixtureWithReporter(reporter gomock.TestReporter, value Advertisement, err error, listResult ListResult) *repositoryFixture {
	fixture := &repositoryFixture{MockRepository: NewMockRepository(gomock.NewController(reporter)), value: value, err: err, listResult: listResult}
	fixture.EXPECT().Create(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, value Advertisement) error { fixture.value = value; return fixture.err })
	fixture.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, value Advertisement) error { fixture.value = value; return fixture.err })
	fixture.EXPECT().GetByID(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, id string) (Advertisement, error) {
		fixture.getID = id
		fixture.getCalls++
		return fixture.value, fixture.err
	})
	fixture.EXPECT().Delete(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, id string) error { fixture.deleteID = id; return fixture.err })
	fixture.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, query ListQuery) (ListResult, error) {
		fixture.listQuery = query
		fixture.listCalls++
		if fixture.listResult.Items != nil || fixture.listResult.NextCursor != "" || fixture.listResult.HasMore {
			return fixture.listResult, fixture.err
		}
		return ListResult{Items: []Advertisement{fixture.value}}, fixture.err
	})
	return fixture
}

type loggerFixture struct {
	*MockLogger
	infoMessage  string
	errorMessage string
	infoFields   []logging.Field
	errorFields  []logging.Field
}

func newAdLogger(errorErr error) *loggerFixture {
	logger := &loggerFixture{MockLogger: NewMockLogger(gomock.NewController(GinkgoT()))}
	logger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(message string, fields ...logging.Field) error {
		logger.infoMessage, logger.infoFields = message, fields
		return nil
	})
	logger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(message string, fields ...logging.Field) error {
		logger.errorMessage, logger.errorFields = message, fields
		return errorErr
	})
	logger.EXPECT().Sync().AnyTimes().Return(nil)
	return logger
}

func newAdIDs(value string, err error) *MockIDGenerator {
	return newAdIDsWithReporter(GinkgoT(), value, err)
}

func newAdIDsWithReporter(reporter gomock.TestReporter, value string, err error) *MockIDGenerator {
	ids := NewMockIDGenerator(gomock.NewController(reporter))
	ids.EXPECT().New().AnyTimes().Return(value, err)
	return ids
}

func newAdClock(now time.Time) *MockClock {
	return newAdClockWithReporter(GinkgoT(), now)
}

func newAdClockWithReporter(reporter gomock.TestReporter, now time.Time) *MockClock {
	clock := NewMockClock(gomock.NewController(reporter))
	clock.EXPECT().Now().AnyTimes().Return(now)
	return clock
}

func newAdminAuth(id string, err error) *MockTokenAuthenticator {
	return newAdminAuthWithReporter(GinkgoT(), id, err)
}

func newAdminAuthWithReporter(reporter gomock.TestReporter, id string, err error) *MockTokenAuthenticator {
	authenticator := NewMockTokenAuthenticator(gomock.NewController(reporter))
	authenticator.EXPECT().AuthenticateAdmin(gomock.Any(), gomock.Any()).AnyTimes().Return(id, err)
	return authenticator
}

var _ = Describe("advertisement lifecycle", func() {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	admin := "0192c5f4-7b2a-7abc-8def-1234567890ab"
	id := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	It("validates lifecycle constraints and creates a normalized advertisement", func() {
		for name, command := range map[string]CreateCommand{
			"active outside window": {AdminID: admin, Advertiser: "Example", Content: "hello", TargetURL: "https://example.com", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), Status: StatusActive},
			"expired before end":    {AdminID: admin, Advertiser: "Example", Content: "hello", TargetURL: "https://example.com", StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(time.Hour), Status: StatusExpired},
			"invalid URL":           {AdminID: admin, Advertiser: "Example", Content: "hello", TargetURL: "http://example.com", StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: StatusPaused},
		} {
			_, err := NewService(newRepositoryFixture(Advertisement{}, nil, ListResult{}), newAdIDs(id, nil), newAdClock(now)).Create(context.Background(), command)
			Expect(errors.Is(err, ErrValidation)).To(BeTrue(), name)
		}
		value, err := NewService(newRepositoryFixture(Advertisement{}, nil, ListResult{}), newAdIDs(id, nil), newAdClock(now)).Create(context.Background(), CreateCommand{AdminID: admin, Advertiser: " Example ", Content: " hello ", TargetURL: "https://example.com", StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: StatusActive})
		Expect(err).NotTo(HaveOccurred())
		Expect(value.Status).To(Equal(StatusActive))
		Expect(value.Content).To(Equal("hello"))
	})
})

var _ = Describe("advertisement router authorization", func() {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	It("rejects missing and invalid authorization", func() {
		service := NewService(newRepositoryFixture(Advertisement{}, nil, ListResult{}), newAdIDs("0192c5f4-7b2a-7abd-8def-1234567890ab", nil), newAdClock(now))
		for _, tc := range []struct{ name, authorization, want string }{{"missing", "", "UNAUTHORIZED"}, {"invalid", "Bearer bad", "INVALID_TOKEN"}} {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/ads", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", tc.authorization)
			w := httptest.NewRecorder()
			NewRouter(NewHandler(service), newAdminAuth("", errors.New("invalid access token"))).ServeHTTP(w, r)
			Expect(w.Code).To(Equal(http.StatusUnauthorized), tc.name)
			Expect(w.Body.String()).To(ContainSubstring(tc.want), tc.name)
		}
	})
	It("creates an advertisement for an authorized administrator", func() {
		service := NewService(newRepositoryFixture(Advertisement{}, nil, ListResult{}), newAdIDs("0192c5f4-7b2a-7abd-8def-1234567890ab", nil), newAdClock(now))
		router := NewRouter(NewHandler(service), newAdminAuth("0192c5f4-7b2a-7abc-8def-1234567890ab", nil))
		request := httptest.NewRequest(http.MethodPost, "/api/v1/ads", strings.NewReader(`{"advertiser":"Example","content":"hello","targetUrl":"https://example.com","startsAt":"2026-10-18T11:00:00Z","endsAt":"2026-10-18T13:00:00Z","status":"active"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		Expect(response.Code).To(Equal(http.StatusCreated))
	})
})
