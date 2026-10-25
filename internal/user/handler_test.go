package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"github.com/go-chi/chi/v5"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type handlerService struct {
	value User
	err   error
	id    string
	calls int
}

func (s *handlerService) GetByID(_ context.Context, id string) (User, error) {
	s.calls++
	s.id = id
	return s.value, s.err
}

type handlerLogger struct {
	infoCalls  int
	errorCalls int
}

func (l *handlerLogger) Info(_ string, _ ...logging.Field) error {
	l.infoCalls++
	return nil
}

func (l *handlerLogger) Error(_ string, _ ...logging.Field) error {
	l.errorCalls++
	return nil
}

func (l *handlerLogger) Sync() error { return nil }

func profileID() string {
	return "0192f8a0-7b00-7000-8000-000000000001"
}

var _ = Describe("User service and HTTP handler", func() {
	It("accepts only canonical UUIDv7 IDs before querying the repository", func() {
		repository := &handlerService{}
		service := NewService(repository)

		for _, invalid := range []string{"", "0", "0192f8a0-7b00-6000-8000-000000000001"} {
			_, err := service.GetByID(context.Background(), invalid)
			Expect(err).To(MatchError(ErrInvalidID))
		}
		for _, valid := range []string{profileID(), "0192f8a0-7b00-7000-8000-000000000002", "0192f8a0-7b00-7000-8000-000000000003"} {
			_, err := service.GetByID(context.Background(), valid)
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(repository.calls).To(Equal(3))
	})

	It("returns the exact public profile and excludes private fields", func() {
		id := profileID()
		created := time.Date(2026, time.October, 24, 12, 34, 56, 123000000, time.FixedZone("test", 3600))
		handler := NewHandler(&handlerService{value: User{
			ID: id, Username: "alice", Email: "alice@example.com", PasswordHash: "hash",
			DisplayName: "Alice", Role: RoleUser, Status: StatusActive, CreatedAt: created,
		}})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
		request = withURLParam(request, "userId", id)
		response := httptest.NewRecorder()

		handler.GetByID(response, request)

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))
		var body map[string]any
		Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
		Expect(body).To(HaveLen(5))
		Expect(body).To(HaveKeyWithValue("id", id))
		Expect(body).To(HaveKeyWithValue("username", "alice"))
		Expect(body).To(HaveKeyWithValue("displayName", "Alice"))
		Expect(body).To(HaveKeyWithValue("role", "user"))
		Expect(body).To(HaveKeyWithValue("createdAt", "2026-10-24T11:34:56Z"))
		Expect(body).NotTo(HaveKey("email"))
		Expect(body).NotTo(HaveKey("passwordHash"))
		Expect(body).NotTo(HaveKey("status"))
	})

	It("allows requests without Authorization and maps not found to NOT_FOUND", func() {
		id := profileID()
		service := &handlerService{err: ErrNotFound}
		handler := NewHandler(service)
		request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
		request = withURLParam(request, "userId", id)
		response := httptest.NewRecorder()

		handler.GetByID(response, request)

		Expect(response.Code).To(Equal(http.StatusNotFound))
		var body errorResponse
		Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
		Expect(body.Error.Code).To(Equal("NOT_FOUND"))
		Expect(body.Error.Message).To(Equal("User not found"))
		Expect(service.calls).To(Equal(1))
		Expect(request.Header.Get("Authorization")).To(BeEmpty())
	})

	It("maps invalid IDs, dependency errors, and preserves the request ID", func() {
		for _, test := range []struct {
			err    error
			status int
			code   string
		}{
			{err: ErrInvalidID, status: http.StatusUnprocessableEntity, code: "VALIDATION_FAILED"},
			{err: errors.New("dependency failed"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
		} {
			service := &handlerService{err: test.err}
			handler := NewHandler(service)
			id := profileID()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
			request = withURLParam(request, "userId", id)
			request.Header.Set("X-Request-ID", "request-test")
			response := httptest.NewRecorder()

			handler.GetByID(response, request)

			Expect(response.Code).To(Equal(test.status))
			var body errorResponse
			Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
			Expect(body.Error.Code).To(Equal(test.code))
			Expect(body.Error.RequestID).To(Equal("request-test"))
		}
	})

	It("maps unavailable dependencies to SERVICE_UNAVAILABLE", func() {
		id := profileID()
		service := &handlerService{err: ErrDependencyUnavailable}
		handler := NewHandler(service)
		request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
		request = withURLParam(request, "userId", id)
		request.Header.Set("X-Request-ID", "request-unavailable")
		response := httptest.NewRecorder()

		handler.GetByID(response, request)

		Expect(response.Code).To(Equal(http.StatusServiceUnavailable))
		var body errorResponse
		Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
		Expect(body.Error).To(Equal(errorBody{
			Code:      "SERVICE_UNAVAILABLE",
			Message:   "Service temporarily unavailable",
			RequestID: "request-unavailable",
		}))
	})

	DescribeTable("rejects malformed path IDs at the HTTP boundary",
		func(pathID string) {
			service := &handlerService{}
			handler := NewHandler(service)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+pathID, nil)
			request = withURLParam(request, "userId", pathID)
			response := httptest.NewRecorder()

			handler.GetByID(response, request)

			Expect(response.Code).To(Equal(http.StatusUnprocessableEntity))
			Expect(service.calls).To(BeZero())
			var body errorResponse
			Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
			Expect(body.Error.Code).To(Equal("VALIDATION_FAILED"))
		},
		Entry("missing", ""),
		Entry("zero", "0"),
		Entry("negative", "-1"),
		Entry("wrong version", "0192f8a0-7b00-6000-8000-000000000001"),
		Entry("non-canonical", "0192F8A0-7B00-7000-8000-000000000001"),
		Entry("non-uuid", "12x"),
	)

	It("logs successful and failed requests when a logger is provided", func() {
		logger := &handlerLogger{}
		id := profileID()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
		request = withURLParam(request, "userId", id)
		response := httptest.NewRecorder()
		NewHandler(&handlerService{value: User{ID: id, CreatedAt: time.Now()}}, logger).GetByID(response, request)
		Expect(logger.infoCalls).To(Equal(1))
		Expect(logger.errorCalls).To(BeZero())

		failedLogger := &handlerLogger{}
		failedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
		failedRequest = withURLParam(failedRequest, "userId", id)
		failedResponse := httptest.NewRecorder()
		NewHandler(&handlerService{err: ErrDependencyUnavailable}, failedLogger).GetByID(failedResponse, failedRequest)
		Expect(failedLogger.errorCalls).To(Equal(1))
		Expect(failedLogger.infoCalls).To(BeZero())
	})

	It("defaults a direct response writer write to HTTP 200", func() {
		response := httptest.NewRecorder()
		writer := &responseWriter{ResponseWriter: response}

		written, err := writer.Write([]byte("body"))

		Expect(err).NotTo(HaveOccurred())
		Expect(written).To(Equal(4))
		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Body.String()).To(Equal("body"))
	})

	It("serves the documented route without an auth middleware", func() {
		id := profileID()
		handler := NewHandler(&handlerService{value: User{ID: id, CreatedAt: time.Now()}})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
		response := httptest.NewRecorder()

		NewRouter(handler).ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Body.String()).To(ContainSubstring(`"id":"` + id + `"`))
		Expect(strings.TrimSpace(response.Body.String())).To(ContainSubstring(`"createdAt"`))
	})
})

func withURLParam(request *http.Request, key, value string) *http.Request {
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add(key, value)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, ctx))
}
