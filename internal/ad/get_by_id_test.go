package ad

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GET /api/v1/ads/{adId}", func() {
	const adminID = "0192c5f4-7b2a-7abc-8def-1234567890ab"
	const adID = "0192f8a0-7b00-7000-8000-000000000001"

	advertisement := Advertisement{
		ID: adID, Type: "ad", Advertiser: "Example Company", Content: "Try our product",
		TargetURL: "https://example.com", Status: StatusActive,
		StartsAt:  time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC),
		EndsAt:    time.Date(2026, 10, 18, 13, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 10, 18, 12, 40, 0, 123000000, time.UTC),
	}

	request := func(id string) *http.Request {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/ads/"+id, nil)
		request.Header.Set("Authorization", "Bearer admin-token")
		return request
	}

	It("returns the documented ad response and logs administrator metadata", func() {
		repository := &fakeRepository{value: advertisement}
		logger := &recordingLogger{}
		router := NewRouter(NewHandler(NewService(repository, fakeIDs{}, fakeClock{}), logger), fakeAuth{id: adminID})
		req := request(adID)
		req.Header.Set("X-Request-ID", "req-ad-get")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Body.String()).To(MatchJSON(`{"id":"0192f8a0-7b00-7000-8000-000000000001","type":"ad","advertiser":"Example Company","content":"Try our product","targetUrl":"https://example.com","status":"active","startsAt":"2026-10-18T11:00:00Z","endsAt":"2026-10-18T13:00:00Z","createdAt":"2026-10-18T12:40:00.123Z"}`))
		Expect(repository.getID).To(Equal(adID))
		Expect(logger.infoMessage).To(Equal("ad request completed"))
		Expect(logger.infoFields).To(ContainElement(logging.String("request_id", "req-ad-get")))
		Expect(logger.infoFields).To(ContainElement(logging.String("route", "GET /api/v1/ads/{adId}")))
		Expect(logger.infoFields).To(ContainElement(logging.String("status", "200")))
		Expect(logger.infoFields).To(ContainElement(logging.String("admin_id", adminID)))
		Expect(fieldValue(logger.infoFields, "duration")).NotTo(BeEmpty())
	})

	It("maps not-found and infrastructure errors without leaking details", func() {
		for _, test := range []struct {
			name    string
			err     error
			code    string
			status  int
			message string
		}{
			{name: "not found", err: ErrNotFound, code: "NOT_FOUND", status: http.StatusNotFound, message: "Advertisement not found"},
			{name: "dependency unavailable", err: ErrDependencyUnavailable, code: "SERVICE_UNAVAILABLE", status: http.StatusServiceUnavailable, message: "Service temporarily unavailable"},
			{name: "unexpected failure", err: errors.New("DynamoDB credentials leaked"), code: "INTERNAL_ERROR", status: http.StatusInternalServerError, message: "Internal server error"},
		} {
			repository := &fakeRepository{err: test.err}
			logger := &recordingLogger{}
			router := NewRouter(NewHandler(NewService(repository, fakeIDs{}, fakeClock{}), logger), fakeAuth{id: adminID})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request(adID))

			Expect(response.Code).To(Equal(test.status))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"` + test.code + `"`))
			Expect(response.Body.String()).To(ContainSubstring(test.message))
			Expect(response.Body.String()).NotTo(ContainSubstring("DynamoDB credentials leaked"))
			Expect(logger.errorMessage).To(Equal("ad request failed"))
			Expect(logger.errorFields).To(ContainElement(logging.String("route", "GET /api/v1/ads/{adId}")))
			Expect(logger.errorFields).To(ContainElement(logging.String("status", strconv.Itoa(test.status))))
			Expect(logger.errorFields).To(ContainElement(logging.String("admin_id", adminID)))
			Expect(fieldValue(logger.errorFields, "duration")).NotTo(BeEmpty())
		}
	})

	It("rejects invalid IDs before calling the repository and logs validation", func() {
		for _, id := range []string{
			"123",
			"0192f8a0-7b00-6000-8000-000000000001",
			"0192F8A0-7B00-7000-8000-000000000001",
			"0192f8a0-7b00-7000-c000-000000000001",
		} {
			repository := &fakeRepository{value: advertisement}
			logger := &recordingLogger{}
			router := NewRouter(NewHandler(NewService(repository, fakeIDs{}, fakeClock{}), logger), fakeAuth{id: adminID})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request(id))

			Expect(response.Code).To(Equal(http.StatusUnprocessableEntity), id)
			Expect(response.Body.String()).To(ContainSubstring(`"code":"VALIDATION_FAILED"`), id)
			Expect(repository.getCalls).To(Equal(0), id)
			Expect(logger.errorFields).To(ContainElement(logging.String("status", "422")), id)
			Expect(logger.errorFields).To(ContainElement(logging.String("admin_id", adminID)), id)
		}
	})

	It("enforces admin authentication and safely logs GET 401/403 failures", func() {
		for _, test := range []struct {
			name          string
			authorization string
			authError     error
			status        int
			code          string
		}{
			{name: "missing", status: http.StatusUnauthorized, code: "UNAUTHORIZED"},
			{name: "invalid token", authorization: "Bearer invalid", authError: errors.New("invalid access token"), status: http.StatusUnauthorized, code: "INVALID_TOKEN"},
			{name: "non-admin", authorization: "Bearer user", authError: auth.ErrForbidden, status: http.StatusForbidden, code: "FORBIDDEN"},
		} {
			repository := &fakeRepository{value: advertisement}
			logger := &recordingLogger{}
			router := NewRouter(NewHandler(NewService(repository, fakeIDs{}, fakeClock{}), logger), fakeAuth{err: test.authError})
			req := request(adID)
			req.Header.Set("Authorization", test.authorization)
			req.Header.Set("X-Request-ID", "req-ad-auth")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)

			Expect(response.Code).To(Equal(test.status))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"` + test.code + `"`))
			Expect(repository.getCalls).To(Equal(0))
			Expect(logger.errorMessage).To(Equal("ad request failed"))
			Expect(logger.errorFields).To(ContainElement(logging.String("request_id", "req-ad-auth")))
			Expect(logger.errorFields).To(ContainElement(logging.String("route", "GET /api/v1/ads/{adId}")))
			Expect(logger.errorFields).To(ContainElement(logging.String("status", strconv.Itoa(test.status))))
			Expect(fieldValue(logger.errorFields, "duration")).NotTo(BeEmpty())
			Expect(fieldValue(logger.errorFields, "admin_id")).To(BeNil())
			Expect(fieldsText(logger.errorFields)).NotTo(ContainSubstring("admin-token"))
			Expect(fieldsText(logger.errorFields)).NotTo(ContainSubstring("Authorization"))
		}
	})
})

func fieldValue(fields []logging.Field, key string) any {
	for _, field := range fields {
		if field.Key == key {
			return field.Value
		}
	}
	return nil
}

func fieldsText(fields []logging.Field) string {
	text := ""
	for _, field := range fields {
		text += field.Key + "=" + fmt.Sprint(field.Value) + " "
	}
	return text
}
