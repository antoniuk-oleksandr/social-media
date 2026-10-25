package ad

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type recordingLogger struct {
	infoMessage  string
	errorMessage string
	infoFields   []logging.Field
	errorFields  []logging.Field
	errorErr     error
}

func (l *recordingLogger) Info(message string, fields ...logging.Field) error {
	l.infoMessage, l.infoFields = message, fields
	return nil
}

func (l *recordingLogger) Error(message string, fields ...logging.Field) error {
	l.errorMessage, l.errorFields = message, fields
	return l.errorErr
}

func (l *recordingLogger) Sync() error { return nil }

var _ = Describe("advertisement request handling", func() {
	const adminID = "0192c5f4-7b2a-7abc-8def-1234567890ab"
	const adID = "0192c5f4-7b2a-7abd-8def-1234567890ab"
	validBody := `{"advertiser":"Example","content":"hello","targetUrl":"https://example.com","startsAt":"2026-10-18T11:00:00Z","endsAt":"2026-10-18T13:00:00Z","status":"active"}`

	newHandler := func(repository Repository) *Handler {
		return NewHandler(NewService(repository, fakeIDs{value: adID}, fakeClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}))
	}
	request := func(body string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/ads", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		return WithAuthenticatedAdminID(r, adminID)
	}

	It("maps request decoding and validation failures", func() {
		for _, body := range []string{"{", `{"unknown":true}`, `{"advertiser":"Example","content":"hello","targetUrl":"https://example.com","startsAt":"not-time","endsAt":"2026-10-18T13:00:00Z","status":"active"}`} {
			response := httptest.NewRecorder()
			newHandler(&fakeRepository{}).Create(response, request(body))
			if body == "{" || body == `{"unknown":true}` {
				Expect(response.Code).To(Equal(http.StatusBadRequest), body)
				Expect(response.Body.String()).To(ContainSubstring(`"code":"INVALID_JSON"`))
			} else {
				Expect(response.Code).To(Equal(http.StatusUnprocessableEntity), body)
				Expect(response.Body.String()).To(ContainSubstring(`"code":"VALIDATION_FAILED"`))
			}
		}
		response := httptest.NewRecorder()
		newHandler(&fakeRepository{}).Create(response, request(`{"advertiser":"","content":"hello","targetUrl":"https://example.com","startsAt":"2026-10-18T11:00:00Z","endsAt":"2026-10-18T13:00:00Z","status":"active"}`))
		Expect(response.Code).To(Equal(http.StatusUnprocessableEntity))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"VALIDATION_FAILED"`))
	})

	It("maps repository failures and preserves request IDs", func() {
		response := httptest.NewRecorder()
		r := request(validBody)
		r.Header.Set("X-Request-ID", "req-ad-123")
		newHandler(&fakeRepository{err: ErrDependencyUnavailable}).Create(response, r)
		Expect(response.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(response.Body.String()).To(ContainSubstring(`"requestId":"req-ad-123"`))

		response = httptest.NewRecorder()
		newHandler(&fakeRepository{err: errors.New("write failed")}).Create(response, request(validBody))
		Expect(response.Code).To(Equal(http.StatusInternalServerError))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"INTERNAL_ERROR"`))
	})

	It("maps ID generation failures to dependency unavailable", func() {
		handler := NewHandler(NewService(&fakeRepository{}, fakeIDs{err: errors.New("id unavailable")}, fakeClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}))
		response := httptest.NewRecorder()
		handler.Create(response, request(validBody))
		Expect(response.Code).To(Equal(http.StatusServiceUnavailable))
	})

	It("rejects malformed date fields before service creation", func() {
		command, err := ToCreateCommand(adminID, CreateRequest{Advertiser: "Example", Content: "hello", TargetURL: "https://example.com", StartsAt: "bad", EndsAt: "2026-10-18T13:00:00Z", Status: StatusActive})
		Expect(command).To(Equal(CreateCommand{}))
		Expect(err).To(MatchError(ErrValidation))
	})

	It("logs successful requests with request metadata", func() {
		logger := &recordingLogger{}
		handler := NewHandler(NewService(&fakeRepository{}, fakeIDs{value: adID}, fakeClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}), logger)
		response := httptest.NewRecorder()
		r := request(validBody)
		r.Header.Set("X-Request-ID", "req-ad-success")
		handler.Create(response, r)
		Expect(response.Code).To(Equal(http.StatusCreated))
		Expect(logger.infoMessage).To(Equal("ad request completed"))
		Expect(logger.infoFields).To(ContainElement(logging.String("request_id", "req-ad-success")))
		Expect(logger.infoFields).To(ContainElement(logging.String("route", "POST /api/v1/ads")))
		Expect(logger.infoFields).To(ContainElement(logging.String("status", "201")))
	})

	It("logs failed requests and tolerates logger failures", func() {
		logger := &recordingLogger{errorErr: errors.New("logger unavailable")}
		handler := NewHandler(NewService(&fakeRepository{err: errors.New("write failed")}, fakeIDs{value: adID}, fakeClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}), logger)
		response := httptest.NewRecorder()
		handler.Create(response, request(validBody))
		Expect(response.Code).To(Equal(http.StatusInternalServerError))
		Expect(logger.errorMessage).To(Equal("ad request failed"))
		Expect(logger.errorFields).To(ContainElement(logging.String("status", "500")))
		Expect(logger.errorFields).To(ContainElement(logging.Error(errors.New("write failed"))))
	})

	It("rejects a non-JSON content type and still logs the failure", func() {
		logger := &recordingLogger{}
		handler := NewHandler(NewService(&fakeRepository{}, fakeIDs{value: adID}, fakeClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}), logger)
		r := request(validBody)
		r.Header.Set("Content-Type", "text/plain")
		response := httptest.NewRecorder()
		handler.Create(response, r)
		Expect(response.Code).To(Equal(http.StatusBadRequest))
		Expect(logger.infoMessage).To(Equal("ad request completed"))
		Expect(logger.infoFields).To(ContainElement(logging.String("status", "400")))
		Expect(logger.errorMessage).To(BeEmpty())
	})
})
