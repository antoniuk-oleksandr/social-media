package ad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAdBehaviorSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Advertisement behavior suite")
}

var _ = Describe("POST /api/v1/ads", func() {
	const adminID = "0192c5f4-7b2a-7abc-8def-1234567890ab"
	const adID = "0192c5f4-7b2a-7abd-8def-1234567890ab"
	var now time.Time

	BeforeEach(func() {
		now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	})

	It("rejects every unsupported status and invalid schedule", func() {
		for _, command := range []CreateCommand{
			{AdminID: adminID, Advertiser: "Example", Content: "hello", TargetURL: "https://example.com", StartsAt: now, EndsAt: now.Add(time.Hour), Status: Status("unknown")},
			{AdminID: adminID, Advertiser: "Example", Content: "hello", TargetURL: "https://example.com", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), Status: StatusActive},
			{AdminID: adminID, Advertiser: "Example", Content: "hello", TargetURL: "https://example.com", StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(time.Hour), Status: StatusExpired},
		} {
			_, err := NewService(&fakeRepository{}, fakeIDs{value: adID}, fakeClock{now: now}).Create(context.Background(), command)
			Expect(err).To(MatchError(ErrValidation))
		}
	})

	It("rejects invalid HTTPS URL forms and oversized content", func() {
		for _, url := range []string{
			"http://example.com",
			"https://user:pass@example.com",
			"https://example.com/path#fragment",
			"https://",
			"https://-bad.com",
			"https://bad-.com",
			"https://example..com",
			"https://foo_bar.com",
			"https://" + strings.Repeat("a", 64) + ".com",
			"https://" + strings.Repeat("a.", 126) + "a.com",
		} {
			_, err := NewService(&fakeRepository{}, fakeIDs{value: adID}, fakeClock{now: now}).Create(context.Background(), CreateCommand{
				AdminID: adminID, Advertiser: "Example", Content: "hello", TargetURL: url, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: StatusPaused,
			})
			Expect(err).To(MatchError(ErrValidation))
		}
		for _, url := range []string{"https://example.com", "https://example.com:8443/path", "https://127.0.0.1", "https://[2001:db8::1]/ad"} {
			_, err := NewService(&fakeRepository{}, fakeIDs{value: adID}, fakeClock{now: now}).Create(context.Background(), CreateCommand{
				AdminID: adminID, Advertiser: "Example", Content: "hello", TargetURL: url, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: StatusPaused,
			})
			Expect(err).NotTo(HaveOccurred(), url)
		}
		_, err := NewService(&fakeRepository{}, fakeIDs{value: adID}, fakeClock{now: now}).Create(context.Background(), CreateCommand{
			AdminID: adminID, Advertiser: "Example", Content: strings.Repeat("x", 1001), TargetURL: "https://example.com", StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: StatusPaused,
		})
		Expect(err).To(MatchError(ErrValidation))
	})

	It("returns 403 for a valid non-admin token", func() {
		router := NewRouter(NewHandler(NewService(&fakeRepository{}, fakeIDs{value: adID}, fakeClock{now: now})), fakeAuth{err: auth.ErrForbidden})
		request := httptest.NewRequest(http.MethodPost, "/api/v1/ads", strings.NewReader(`{"advertiser":"Example","content":"hello","targetUrl":"https://example.com","startsAt":"2026-10-18T11:00:00Z","endsAt":"2026-10-18T13:00:00Z","status":"active"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer user-token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		Expect(response.Code).To(Equal(http.StatusForbidden))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"FORBIDDEN"`))
	})

	It("returns 401 when authentication is absent", func() {
		router := NewRouter(NewHandler(NewService(&fakeRepository{}, fakeIDs{value: adID}, fakeClock{now: now})), fakeAuth{id: adminID})
		request := httptest.NewRequest(http.MethodPost, "/api/v1/ads", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		Expect(response.Code).To(Equal(http.StatusUnauthorized))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"UNAUTHORIZED"`))
	})
})
