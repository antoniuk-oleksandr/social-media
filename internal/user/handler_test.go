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
	"go.uber.org/mock/gomock"
)

func newHandlerService(value User, err error) (*MockService, *int) {
	service := NewMockService(gomock.NewController(GinkgoT()))
	calls := new(int)
	service.EXPECT().GetByID(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string) (User, error) { *calls++; return value, err })
	service.EXPECT().UpdateProfile(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, UpdateProfileCommand) (User, error) { *calls++; return value, err })
	return service, calls
}

func newHandlerAuth(id string, err error) *MockTokenAuthenticator {
	authenticator := NewMockTokenAuthenticator(gomock.NewController(GinkgoT()))
	authenticator.EXPECT().Authenticate(gomock.Any(), gomock.Any()).AnyTimes().Return(id, err)
	return authenticator
}

func newHandlerLogger() (*MockLogger, *int, *int) {
	logger := NewMockLogger(gomock.NewController(GinkgoT()))
	infoCalls, errorCalls := new(int), new(int)
	logger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(string, ...logging.Field) error { *infoCalls++; return nil })
	logger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(string, ...logging.Field) error { *errorCalls++; return nil })
	logger.EXPECT().Sync().AnyTimes().Return(nil)
	return logger, infoCalls, errorCalls
}

func profileID() string {
	return "0192f8a0-7b00-7000-8000-000000000001"
}

var _ = Describe("User service and HTTP handler", func() {
	It("accepts only canonical UUIDv7 IDs before querying the repository", func() {
		repository, calls := newHandlerService(User{}, nil)
		service := NewService(repository)

		for _, invalid := range []string{"", "0", "0192f8a0-7b00-6000-8000-000000000001"} {
			_, err := service.GetByID(context.Background(), invalid)
			Expect(err).To(MatchError(ErrInvalidID))
		}
		for _, valid := range []string{profileID(), "0192f8a0-7b00-7000-8000-000000000002", "0192f8a0-7b00-7000-8000-000000000003"} {
			_, err := service.GetByID(context.Background(), valid)
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(*calls).To(Equal(3))
	})

	It("returns the exact public profile and excludes private fields", func() {
		id := profileID()
		created := time.Date(2026, time.October, 24, 12, 34, 56, 123000000, time.FixedZone("test", 3600))
		service, _ := newHandlerService(User{
			ID: id, Username: "alice", Email: "alice@example.com", PasswordHash: "hash",
			DisplayName: "Alice", Role: RoleUser, Status: StatusActive, CreatedAt: created,
		}, nil)
		handler := NewHandler(service)
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
		service, calls := newHandlerService(User{}, ErrNotFound)
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
		Expect(*calls).To(Equal(1))
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
			service, _ := newHandlerService(User{}, test.err)
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
		service, _ := newHandlerService(User{}, ErrDependencyUnavailable)
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
			service, calls := newHandlerService(User{}, nil)
			handler := NewHandler(service)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+pathID, nil)
			request = withURLParam(request, "userId", pathID)
			response := httptest.NewRecorder()

			handler.GetByID(response, request)

			Expect(response.Code).To(Equal(http.StatusUnprocessableEntity))
			Expect(*calls).To(BeZero())
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
		logger, infoCalls, errorCalls := newHandlerLogger()
		id := profileID()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
		request = withURLParam(request, "userId", id)
		response := httptest.NewRecorder()
		service, _ := newHandlerService(User{ID: id, CreatedAt: time.Now()}, nil)
		NewHandler(service, logger).GetByID(response, request)
		Expect(*infoCalls).To(Equal(1))
		Expect(*errorCalls).To(BeZero())

		failedLogger, failedInfoCalls, failedErrorCalls := newHandlerLogger()
		failedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
		failedRequest = withURLParam(failedRequest, "userId", id)
		failedResponse := httptest.NewRecorder()
		failedService, _ := newHandlerService(User{}, ErrDependencyUnavailable)
		NewHandler(failedService, failedLogger).GetByID(failedResponse, failedRequest)
		Expect(*failedErrorCalls).To(Equal(1))
		Expect(*failedInfoCalls).To(BeZero())
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
		service, _ := newHandlerService(User{ID: id, CreatedAt: time.Now()}, nil)
		handler := NewHandler(service)
		request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil)
		response := httptest.NewRecorder()

		NewRouter(handler).ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Body.String()).To(ContainSubstring(`"id":"` + id + `"`))
		Expect(strings.TrimSpace(response.Body.String())).To(ContainSubstring(`"createdAt"`))
	})

	Describe("PATCH /api/v1/users/me", func() {
		It("updates the authenticated user's profile and returns the public profile", func() {
			id := profileID()
			service, _ := newHandlerService(User{ID: id, Username: "alice_smith", DisplayName: "Alice Smith", Role: RoleUser, CreatedAt: time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC)}, nil)
			request := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"displayName":"Alice Smith","username":"Alice_Smith"}`))
			request.Header.Set("Content-Type", "application/json")
			request = WithAuthenticatedUserID(request, id)
			response := httptest.NewRecorder()

			NewHandler(service).UpdateMe(response, request)

			Expect(response.Code).To(Equal(http.StatusOK))
			Expect(response.Body.String()).To(MatchJSON(`{"id":"0192f8a0-7b00-7000-8000-000000000001","username":"alice_smith","displayName":"Alice Smith","role":"user","createdAt":"2026-10-24T12:00:00Z"}`))
		})

		It("rejects a direct unauthenticated update", func() {
			service, calls := newHandlerService(User{}, nil)
			request := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"displayName":"Alice"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			NewHandler(service).UpdateMe(response, request)

			Expect(response.Code).To(Equal(http.StatusUnauthorized))
			Expect(response.Body.String()).To(MatchJSON(`{"error":{"code":"UNAUTHORIZED","message":"Authentication required","requestId":"req-local"}}`))
			Expect(*calls).To(BeZero())
		})

		DescribeTable("rejects invalid request bodies", func(contentType, body string, status int, code string) {
			service, calls := newHandlerService(User{}, nil)
			request := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(body))
			request.Header.Set("Content-Type", contentType)
			request = WithAuthenticatedUserID(request, profileID())
			response := httptest.NewRecorder()

			NewHandler(service).UpdateMe(response, request)

			Expect(response.Code).To(Equal(status))
			var bodyResponse errorResponse
			Expect(json.Unmarshal(response.Body.Bytes(), &bodyResponse)).To(Succeed())
			Expect(bodyResponse.Error.Code).To(Equal(code))
			Expect(*calls).To(BeZero())
		},
			Entry("wrong content type", "text/plain", `{}`, http.StatusBadRequest, "INVALID_JSON"),
			Entry("malformed JSON", "application/json", `{"displayName":`, http.StatusBadRequest, "INVALID_JSON"),
			Entry("unknown field", "application/json", `{"nickname":"Alice"}`, http.StatusBadRequest, "INVALID_JSON"),
			Entry("duplicate field", "application/json", `{"displayName":"Alice","displayName":"Bob"}`, http.StatusBadRequest, "INVALID_JSON"),
			Entry("trailing value", "application/json", `{"displayName":"Alice"} {}`, http.StatusBadRequest, "INVALID_JSON"),
			Entry("oversized body", "application/json", `{"displayName":"`+strings.Repeat("x", 64<<10)+`"}`, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE"),
		)

		It("translates validation, conflict, not-found, dependency, and internal errors", func() {
			for _, test := range []struct {
				err    error
				status int
				code   string
			}{
				{ErrValidation, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
				{ErrUsernameTaken, http.StatusConflict, "USERNAME_TAKEN"},
				{ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
				{ErrDependencyUnavailable, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
				{errors.New("unexpected"), http.StatusInternalServerError, "INTERNAL_ERROR"},
			} {
				request := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"displayName":"Alice"}`))
				request.Header.Set("Content-Type", "application/json")
				request = WithAuthenticatedUserID(request, profileID())
				response := httptest.NewRecorder()
				service, _ := newHandlerService(User{}, test.err)
				NewHandler(service).UpdateMe(response, request)
				Expect(response.Code).To(Equal(test.status))
				var bodyResponse errorResponse
				Expect(json.Unmarshal(response.Body.Bytes(), &bodyResponse)).To(Succeed())
				Expect(bodyResponse.Error.Code).To(Equal(test.code))
			}
		})

		It("enforces authentication in the router and passes the verified subject", func() {
			service, _ := newHandlerService(User{ID: profileID(), Username: "alice", DisplayName: "Alice", Role: RoleUser, CreatedAt: time.Now()}, nil)
			router := NewRouter(NewHandler(service), newHandlerAuth(profileID(), nil))
			unauthenticated := httptest.NewRecorder()
			router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"displayName":"Alice"}`)))
			Expect(unauthenticated.Code).To(Equal(http.StatusUnauthorized))

			invalid := httptest.NewRecorder()
			badAuth := NewRouter(NewHandler(service), newHandlerAuth("", errors.New("bad token")))
			request := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"displayName":"Alice"}`))
			request.Header.Set("Authorization", "Bearer bad")
			badAuth.ServeHTTP(invalid, request)
			Expect(invalid.Code).To(Equal(http.StatusUnauthorized))
			Expect(invalid.Body.String()).To(MatchJSON(`{"error":{"code":"INVALID_TOKEN","message":"Invalid access token","requestId":"req-local"}}`))

			valid := httptest.NewRecorder()
			request = httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"displayName":"Alice"}`))
			request.Header.Set("Authorization", "Bearer good")
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(valid, request)
			Expect(valid.Code).To(Equal(http.StatusOK))
		})

		It("logs completed and failed profile updates", func() {
			id := profileID()
			successLogger, successInfoCalls, successErrorCalls := newHandlerLogger()
			successRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"displayName":"Alice"}`))
			successRequest.Header.Set("Content-Type", "application/json")
			successRequest.Header.Set("X-Request-ID", "update-success")
			successRequest = WithAuthenticatedUserID(successRequest, id)
			successResponse := httptest.NewRecorder()
			successService, _ := newHandlerService(User{ID: id, DisplayName: "Alice"}, nil)
			NewHandler(successService, successLogger).UpdateMe(successResponse, successRequest)
			Expect(successResponse.Code).To(Equal(http.StatusOK))
			Expect(*successInfoCalls).To(Equal(1))
			Expect(*successErrorCalls).To(BeZero())

			failedLogger, failedInfoCalls, failedErrorCalls := newHandlerLogger()
			failedRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"displayName":"Alice"}`))
			failedRequest.Header.Set("Content-Type", "application/json")
			failedRequest = WithAuthenticatedUserID(failedRequest, id)
			failedResponse := httptest.NewRecorder()
			failedService, _ := newHandlerService(User{}, ErrValidation)
			NewHandler(failedService, failedLogger).UpdateMe(failedResponse, failedRequest)
			Expect(failedResponse.Code).To(Equal(http.StatusUnprocessableEntity))
			Expect(*failedErrorCalls).To(Equal(1))
			Expect(*failedInfoCalls).To(BeZero())
		})
	})
})

func withURLParam(request *http.Request, key, value string) *http.Request {
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add(key, value)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, ctx))
}
