package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

type loggerState struct {
	message string
	fields  []logging.Field
}

func newLogger(ctrl *gomock.Controller) (*MockLogger, *loggerState) {
	state := &loggerState{}
	logger := NewMockLogger(ctrl)
	logger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(message string, fields ...logging.Field) error {
		state.message, state.fields = message, fields
		return nil
	})
	logger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(message string, fields ...logging.Field) error {
		state.message, state.fields = message, fields
		return nil
	})
	logger.EXPECT().Sync().AnyTimes().Return(nil)
	return logger, state
}

func (l *loggerState) field(key string) (any, bool) {
	for _, field := range l.fields {
		if field.Key == key {
			return field.Value, true
		}
	}
	return nil, false
}
func handler() *auth.Handler {
	ctrl := gomock.NewController(GinkgoT())
	store := NewMockRegistrationStore(ctrl)
	store.EXPECT().Register(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	hasher := NewMockPasswordHasher(ctrl)
	hasher.EXPECT().Hash(gomock.Any()).AnyTimes().Return("h", nil)
	ids := NewMockIDGenerator(ctrl)
	ids.EXPECT().New().AnyTimes().Return("0192f8a0-7b00-7000-8000-000000000001", nil)
	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().AnyTimes().Return(time.Unix(1, 0))
	access := NewMockAccessTokenIssuer(ctrl)
	access.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return("a", nil)
	opaque := NewMockOpaqueTokenGenerator(ctrl)
	opaque.EXPECT().Generate().AnyTimes().Return("r", "rh", nil)
	s := &auth.Service{Store: store, Hasher: hasher, IDs: ids, Clock: clock, Access: access, Opaque: opaque, AccessTTL: 900 * time.Second, RefreshTTL: 7 * 24 * time.Hour}
	h := auth.Handler{Service: s}
	return &h
}

var _ = Describe("RegisterHandler", func() {
	It("runs the test", func() {
		r := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"username":"alice","email":"A@EXAMPLE.COM","password":"123456789012","displayName":"Alice"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler().Register(w, r)
		if w.Code != 201 {
			Expect(false).To(BeTrue(), fmt.Sprintf("%d %s", w.Code, w.Body))
		}
		if !strings.Contains(w.Body.String(), `"tokenType":"Bearer"`) {
			Expect(false).To(BeTrue(), fmt.Sprint(w.Body.String()))
		}

	})
})
var _ = Describe("RegisterHandlerRejectsUnknownFields", func() {
	It("runs the test", func() {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"username":"alice","wat":"x"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler().Register(w, r)
		if w.Code != 400 {
			Expect(false).To(BeTrue(), fmt.Sprint(w.Code))
		}

	})
})

func newCountingService(ctrl *gomock.Controller, err error) (*MockServicePort, *int) {
	service := NewMockServicePort(ctrl)
	calls := new(int)
	service.EXPECT().Register(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, auth.RegisterCommand) (auth.RegisterResult, error) {
		*calls++
		return auth.RegisterResult{}, err
	})
	service.EXPECT().Login(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, auth.LoginCommand) (auth.RegisterResult, error) {
		*calls++
		return auth.RegisterResult{}, err
	})
	service.EXPECT().Logout(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string) error { *calls++; return err })
	return service, calls
}

func errorBody(w *httptest.ResponseRecorder) auth.ErrorResponse {
	var got auth.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		Expect(false).To(BeTrue(), fmt.Sprintf("decode error response: %v; body=%s", err, w.Body.String()))
	}
	return got
}

var _ = Describe("RegisterHandlerStrictJSONAndHTTPMappings", func() {
	It("runs the test", func() {
		tests := []struct {
			name        string
			body        string
			contentType string
			status      int
			code        string
		}{
			{name: "duplicate keys", body: `{"username":"alice","username":"bob"}`, status: http.StatusBadRequest, code: "INVALID_JSON"},
			{name: "trailing value", body: `{"username":"alice"} {}`, status: http.StatusBadRequest, code: "INVALID_JSON"},
			{name: "root null", body: `null`, status: http.StatusBadRequest, code: "INVALID_JSON"},
			{name: "malformed", body: `{"username":`, status: http.StatusBadRequest, code: "INVALID_JSON"},
			{name: "unknown field", body: `{"unknown":"value"}`, status: http.StatusBadRequest, code: "INVALID_JSON"},
			{name: "oversized", body: `{"displayName":"` + strings.Repeat("x", 64<<10) + `"}`, status: http.StatusRequestEntityTooLarge, code: "PAYLOAD_TOO_LARGE"},
			{name: "wrong content type", body: `{}`, contentType: "text/plain", status: http.StatusUnsupportedMediaType, code: "INVALID_JSON"},
		}

		for _, tt := range tests {
			{
				service, calls := newCountingService(gomock.NewController(GinkgoT()), nil)
				h := auth.Handler{Service: service}
				r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(tt.body))
				contentType := tt.contentType
				if contentType == "" {
					contentType = "application/json"
				}
				r.Header.Set("Content-Type", contentType)
				r.Header.Set("X-Request-ID", "req-test")
				w := httptest.NewRecorder()
				h.Register(w, r)

				if w.Code != tt.status {
					Expect(false).To(BeTrue(), fmt.Sprintf("status = %d, want %d; body=%s", w.Code, tt.status, w.Body.String()))
				}
				got := errorBody(w)
				if got.Error.Code != tt.code || got.Error.RequestID != "req-test" {
					Expect(false).To(BeTrue(), fmt.Sprintf("error = %+v, want code %q and request ID req-test", got.Error, tt.code))
				}
				if *calls != 0 {
					Expect(false).To(BeTrue(), fmt.Sprintf("service calls = %d, want 0", *calls))
				}
			}
		}

	})
})

var _ = Describe("RegisterHandlerMapsServiceErrors", func() {
	It("runs the test", func() {
		tests := []struct {
			name   string
			err    error
			status int
			code   string
		}{
			{name: "validation", err: auth.ErrValidation, status: http.StatusUnprocessableEntity, code: "VALIDATION_FAILED"},
			{name: "username conflict", err: auth.ErrUsernameTaken, status: http.StatusConflict, code: "USERNAME_TAKEN"},
			{name: "email conflict", err: auth.ErrEmailTaken, status: http.StatusConflict, code: "EMAIL_TAKEN"},
			{name: "dependency unavailable", err: auth.ErrDependencyUnavailable, status: http.StatusServiceUnavailable, code: "SERVICE_UNAVAILABLE"},
			{name: "internal", err: errors.New("dependency failure"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
		}

		for _, tt := range tests {
			{
				service, calls := newCountingService(gomock.NewController(GinkgoT()), tt.err)
				h := auth.Handler{Service: service}
				r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"username":"alice","email":"alice@example.com","password":"123456789012","displayName":"Alice"}`))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-Request-ID", "req-error")
				w := httptest.NewRecorder()
				h.Register(w, r)

				if w.Code != tt.status {
					Expect(false).To(BeTrue(), fmt.Sprintf("status = %d, want %d; body=%s", w.Code, tt.status, w.Body.String()))
				}
				got := errorBody(w)
				if got.Error.Code != tt.code || got.Error.RequestID != "req-error" {
					Expect(false).To(BeTrue(), fmt.Sprintf("error = %+v, want code %q and request ID req-error", got.Error, tt.code))
				}
				if *calls != 1 {
					Expect(false).To(BeTrue(), fmt.Sprintf("service calls = %d, want 1", *calls))
				}
			}
		}

	})
})

var _ = Describe("RegisterHandlerLogsServiceFailures", func() {
	It("logs the request ID and mapped dependency failure", func() {
		logger, loggerState := newLogger(gomock.NewController(GinkgoT()))
		service, _ := newCountingService(gomock.NewController(GinkgoT()), auth.ErrDependencyUnavailable)
		h := auth.Handler{Service: service, Logger: logger}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"username":"alice","email":"alice@example.com","password":"123456789012","displayName":"Alice"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-ID", "req-dependency")
		w := httptest.NewRecorder()

		h.Register(w, r)

		Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(loggerState.message).To(Equal("registration request failed"))
		requestID, found := loggerState.field("request_id")
		Expect(found).To(BeTrue())
		Expect(requestID).To(Equal("req-dependency"))
		code, found := loggerState.field("error_code")
		Expect(found).To(BeTrue())
		Expect(code).To(Equal("SERVICE_UNAVAILABLE"))
	})
})

var _ = Describe("LoginHandlerLogging", func() {
	It("records safe structured fields without credentials or secrets", func() {
		logger, loggerState := newLogger(gomock.NewController(GinkgoT()))
		service, _ := newCountingService(gomock.NewController(GinkgoT()), auth.ErrInvalidCredentials)
		h := auth.Handler{Service: service, Logger: logger}
		password := "correct-password"
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"alice@example.com","password":"`+password+`"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-ID", "req-login")
		w := httptest.NewRecorder()

		h.Login(w, r)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(loggerState.message).To(Equal("login request completed"))
		for _, field := range []string{"request_id", "route", "status", "duration", "error_code"} {
			_, found := loggerState.field(field)
			Expect(found).To(BeTrue(), field)
		}
		requestID, _ := loggerState.field("request_id")
		Expect(requestID).To(Equal("req-login"))
		route, _ := loggerState.field("route")
		Expect(route).To(Equal("POST /api/v1/auth/login"))
		status, _ := loggerState.field("status")
		Expect(status).To(Equal("401"))
		code, _ := loggerState.field("error_code")
		Expect(code).To(Equal("INVALID_CREDENTIALS"))
		for _, field := range loggerState.fields {
			Expect(field.Key).NotTo(Or(Equal("login"), Equal("password"), Equal("authorization"), Equal("access_token"), Equal("refresh_token")))
			Expect(field.Value).NotTo(Equal(password))
			Expect(field.Value).NotTo(Equal("alice@example.com"))
		}
	})
})

var _ = Describe("LoginHandlerInternalFailureLogging", func() {
	It("uses Error level with safe dependency context", func() {
		logger, loggerState := newLogger(gomock.NewController(GinkgoT()))
		service, _ := newCountingService(gomock.NewController(GinkgoT()), errors.New("database credentials leaked?"))
		h := auth.Handler{Service: service, Logger: logger}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"alice","password":"correct-password"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-ID", "req-internal")
		w := httptest.NewRecorder()

		h.Login(w, r)

		Expect(w.Code).To(Equal(http.StatusInternalServerError))
		Expect(loggerState.message).To(Equal("login internal failure"))
		errorValue, found := loggerState.field("error")
		Expect(found).To(BeTrue())
		Expect(errorValue).To(Equal("database credentials leaked?"))
		value, found := loggerState.field("password")
		Expect(found).To(BeFalse())
		Expect(value).To(BeNil())
	})
})

var _ = Describe("RegisterHandlerHandlesResponseWriteFailure", func() {
	It("runs the test", func() {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"username":"alice","email":"alice@example.com","password":"123456789012","displayName":"Alice"}`))
		r.Header.Set("Content-Type", "application/json")
		writer := NewMockResponseWriter(gomock.NewController(GinkgoT()))
		writer.EXPECT().Header().Return(make(http.Header)).AnyTimes()
		writer.EXPECT().WriteHeader(gomock.Any()).AnyTimes()
		writer.EXPECT().Write(gomock.Any()).Return(0, errors.New("write failed")).AnyTimes()
		handler().Register(writer, r)

	})
})
