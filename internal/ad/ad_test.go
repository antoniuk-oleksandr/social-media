package ad

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeRepository struct {
	value Advertisement
	err   error
}

func (f *fakeRepository) Create(_ context.Context, value Advertisement) error {
	f.value = value
	return f.err
}

type fakeIDs struct {
	value string
	err   error
}

func (f fakeIDs) New() (string, error) { return f.value, f.err }

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeAuth struct {
	id  string
	err error
}

func (f fakeAuth) AuthenticateAdmin(context.Context, string) (string, error) { return f.id, f.err }

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
			_, err := NewService(&fakeRepository{}, fakeIDs{value: id}, fakeClock{now: now}).Create(context.Background(), command)
			Expect(errors.Is(err, ErrValidation)).To(BeTrue(), name)
		}
		value, err := NewService(&fakeRepository{}, fakeIDs{value: id}, fakeClock{now: now}).Create(context.Background(), CreateCommand{AdminID: admin, Advertiser: " Example ", Content: " hello ", TargetURL: "https://example.com", StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: StatusActive})
		Expect(err).NotTo(HaveOccurred())
		Expect(value.Status).To(Equal(StatusActive))
		Expect(value.Content).To(Equal("hello"))
	})
})

var _ = Describe("advertisement router authorization", func() {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	service := NewService(&fakeRepository{}, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, fakeClock{now: now})
	router := NewRouter(NewHandler(service), fakeAuth{id: "0192c5f4-7b2a-7abc-8def-1234567890ab"})
	It("rejects missing and invalid authorization", func() {
		for _, tc := range []struct{ name, authorization, want string }{{"missing", "", "UNAUTHORIZED"}, {"invalid", "Bearer bad", "INVALID_TOKEN"}} {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/ads", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", tc.authorization)
			w := httptest.NewRecorder()
			NewRouter(NewHandler(service), fakeAuth{err: errors.New("invalid access token")}).ServeHTTP(w, r)
			Expect(w.Code).To(Equal(http.StatusUnauthorized), tc.name)
			Expect(w.Body.String()).To(ContainSubstring(tc.want), tc.name)
		}
	})
	It("creates an advertisement for an authorized administrator", func() {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/ads", strings.NewReader(`{"advertiser":"Example","content":"hello","targetUrl":"https://example.com","startsAt":"2026-10-18T11:00:00Z","endsAt":"2026-10-18T13:00:00Z","status":"active"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		Expect(response.Code).To(Equal(http.StatusCreated))
	})
})
