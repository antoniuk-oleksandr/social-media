package ad

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PATCH /api/v1/ads/{adId}", func() {
	const adminID = "0192c5f4-7b2a-7abc-8def-1234567890ab"
	const adID = "0192f8a0-7b00-7000-8000-000000000001"
	var now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	advertisement := Advertisement{
		ID: adID, Type: "ad", Advertiser: "Example", Content: "old content", TargetURL: "https://example.com",
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: StatusActive,
		CreatedAt: now, CreatedBy: adminID,
	}

	It("updates content and status through the authenticated admin route", func() {
		repository := newRepositoryFixture(advertisement, nil, ListResult{})
		router := NewRouter(NewHandler(NewService(repository, newAdIDs("", nil), newAdClock(now))), newAdminAuth(adminID, nil))
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/ads/"+adID, strings.NewReader(`{"content":"new content","status":"paused"}`))
		request.Header.Set("Authorization", "Bearer admin-token")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(repository.value.Content).To(Equal("new content"))
		Expect(repository.value.Status).To(Equal(StatusPaused))
	})

	It("returns conflict for an invalid lifecycle transition", func() {
		repository := newRepositoryFixture(advertisement, nil, ListResult{})
		service := NewService(repository, newAdIDs("", nil), newAdClock(now))
		_, err := service.Update(context.Background(), UpdateCommand{AdID: adID, Status: statusPointer(StatusDraft)})
		Expect(err).To(MatchError(ErrInvalidState))
	})

	It("validates changed URLs and schedules", func() {
		repository := newRepositoryFixture(advertisement, nil, ListResult{})
		service := NewService(repository, newAdIDs("", nil), newAdClock(now))
		badURL := "http://example.com"
		_, err := service.Update(context.Background(), UpdateCommand{AdID: adID, TargetURL: &badURL})
		Expect(err).To(MatchError(ErrValidation))

		badEnd := now.Add(-2 * time.Hour)
		_, err = service.Update(context.Background(), UpdateCommand{AdID: adID, EndsAt: &badEnd})
		Expect(err).To(MatchError(ErrValidation))
	})

	It("maps all supported lifecycle transitions", func() {
		for _, transition := range []struct {
			from Status
			to   Status
		}{
			{StatusDraft, StatusDraft}, {StatusDraft, StatusActive},
			{StatusActive, StatusActive}, {StatusActive, StatusPaused}, {StatusActive, StatusExpired},
			{StatusPaused, StatusPaused}, {StatusPaused, StatusActive}, {StatusPaused, StatusExpired},
			{StatusExpired, StatusExpired},
		} {
			value := advertisement
			value.Status = transition.from
			if transition.from == StatusExpired || transition.to == StatusExpired {
				value.EndsAt = now.Add(-30 * time.Minute)
			}
			if transition.to == StatusActive {
				value.StartsAt = now.Add(-time.Hour)
				value.EndsAt = now.Add(time.Hour)
			}
			_, err := NewService(newRepositoryFixture(value, nil, ListResult{}), newAdIDs("", nil), newAdClock(now)).Update(context.Background(), UpdateCommand{AdID: adID, Status: statusPointer(transition.to)})
			Expect(err).NotTo(HaveOccurred(), "%s to %s", transition.from, transition.to)
		}
	})

	It("rejects invalid commands before persistence and propagates repository errors", func() {
		for _, command := range []UpdateCommand{{AdID: "123", Content: stringPointer("new")}, {AdID: adID}} {
			repository := newRepositoryFixture(advertisement, nil, ListResult{})
			_, err := NewService(repository, newAdIDs("", nil), newAdClock(now)).Update(context.Background(), command)
			Expect(err).To(MatchError(func() error {
				if command.AdID == adID {
					return ErrValidation
				}
				return ErrInvalidID
			}()))
			Expect(repository.getCalls).To(Equal(0))
		}
		repository := newRepositoryFixture(advertisement, ErrNotFound, ListResult{})
		_, err := NewService(repository, newAdIDs("", nil), newAdClock(now)).Update(context.Background(), UpdateCommand{AdID: adID, Content: stringPointer("new")})
		Expect(err).To(MatchError(ErrNotFound))
	})

	It("parses partial update timestamps and rejects malformed timestamps", func() {
		starts := "2026-10-18T11:00:00Z"
		ends := "2026-10-18T13:00:00Z"
		command, err := ToUpdateCommand(adID, UpdateRequest{StartsAt: &starts, EndsAt: &ends})
		Expect(err).NotTo(HaveOccurred())
		Expect(command.StartsAt).NotTo(BeNil())
		Expect(command.EndsAt).NotTo(BeNil())
		bad := "not-a-timestamp"
		_, err = ToUpdateCommand(adID, UpdateRequest{EndsAt: &bad})
		Expect(err).To(MatchError(ErrValidation))
	})

	It("maps update errors and malformed requests to documented HTTP responses", func() {
		validRequest := func(body string) *http.Request {
			r := httptest.NewRequest(http.MethodPatch, "/api/v1/ads/"+adID, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer admin-token")
			return r
		}
		for _, test := range []struct {
			err    error
			status int
			code   string
		}{
			{ErrValidation, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
			{ErrInvalidState, http.StatusConflict, "INVALID_STATE"},
			{ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
			{ErrDependencyUnavailable, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
			{errors.New("unexpected failure"), http.StatusInternalServerError, "INTERNAL_ERROR"},
		} {
			repository := newRepositoryFixture(advertisement, test.err, ListResult{})
			router := NewRouter(NewHandler(NewService(repository, newAdIDs("", nil), newAdClock(now))), newAdminAuth(adminID, nil))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, validRequest(`{"content":"new content"}`))
			Expect(response.Code).To(Equal(test.status))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"` + test.code + `"`))
		}
		response := httptest.NewRecorder()
		NewHandler(NewService(newRepositoryFixture(Advertisement{}, nil, ListResult{}), newAdIDs("", nil), newAdClock(now))).Update(response, validRequest("{"))
		Expect(response.Code).To(Equal(http.StatusBadRequest))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"INVALID_JSON"`))

		response = httptest.NewRecorder()
		noContentType := validRequest(`{"content":"new content"}`)
		noContentType.Header.Del("Content-Type")
		NewHandler(NewService(newRepositoryFixture(Advertisement{}, nil, ListResult{}), newAdIDs("", nil), newAdClock(now))).Update(response, noContentType)
		Expect(response.Code).To(Equal(http.StatusBadRequest))

		response = httptest.NewRecorder()
		missingAuth := httptest.NewRequest(http.MethodPatch, "/api/v1/ads/"+adID, strings.NewReader(`{"content":"new content"}`))
		missingAuth.Header.Set("Content-Type", "application/json")
		NewRouter(NewHandler(NewService(newRepositoryFixture(advertisement, nil, ListResult{}), newAdIDs("", nil), newAdClock(now))), newAdminAuth(adminID, nil)).ServeHTTP(response, missingAuth)
		Expect(response.Code).To(Equal(http.StatusUnauthorized))
	})

	It("returns the documented 404 response when persistence reports not found", func() {
		repository := newRepositoryFixture(advertisement, ErrNotFound, ListResult{})
		router := NewRouter(NewHandler(NewService(repository, newAdIDs("", nil), newAdClock(now))), newAdminAuth(adminID, nil))
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/ads/"+adID, strings.NewReader(`{"content":"new content"}`))
		request.Header.Set("Authorization", "Bearer admin-token")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Request-ID", "req-ad-update-not-found")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusNotFound))
		var body errorResponse
		Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
		Expect(body).To(Equal(errorResponse{Error: errorBody{
			Code: "NOT_FOUND", Message: "Advertisement not found", RequestID: "req-ad-update-not-found",
		}}))
	})
})

func statusPointer(value Status) *Status { return &value }

func stringPointer(value string) *string { return &value }
