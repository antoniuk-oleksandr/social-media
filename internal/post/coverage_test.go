package post

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
)

type captureLogger struct {
	infoMessage  string
	infoFields   []logging.Field
	errorMessage string
	errorFields  []logging.Field
}

func (l *captureLogger) Info(message string, fields ...logging.Field) error {
	l.infoMessage, l.infoFields = message, fields
	return nil
}
func (l *captureLogger) Error(message string, fields ...logging.Field) error {
	l.errorMessage, l.errorFields = message, fields
	return nil
}
func (l *captureLogger) Sync() error { return nil }

func TestHandlerLogsCompletedAndFailedRequests(t *testing.T) {
	validAuthor := "0192c5f4-7b2a-7abc-8def-1234567890ab"
	validPost := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	cases := []struct {
		name       string
		repository error
		wantInfo   bool
		wantStatus string
	}{
		{name: "completed", wantInfo: true, wantStatus: "201"},
		{name: "dependency failure", repository: ErrDependencyUnavailable, wantStatus: "503"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := &captureLogger{}
			handler := NewHandler(NewService(&fakeRepository{err: tc.repository}, fakeIDs{value: validPost}, fakeClock{value: time.Now()}), logger)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Request-ID", "request-123")
			r = WithAuthenticatedUserID(r, validAuthor)
			w := httptest.NewRecorder()
			handler.Create(w, r)
			fields, message := logger.infoFields, logger.infoMessage
			if !tc.wantInfo {
				fields, message = logger.errorFields, logger.errorMessage
			}
			if tc.wantInfo && message != "post request completed" {
				t.Fatalf("info message = %q", message)
			}
			if !tc.wantInfo && message != "post request failed" {
				t.Fatalf("error message = %q", message)
			}
			if !hasLogField(fields, "status", tc.wantStatus) || !hasLogField(fields, "request_id", "request-123") || !hasLogField(fields, "user_id", validAuthor) {
				t.Fatalf("log fields = %#v", fields)
			}
		})
	}
}

func TestRouterLogsAuthenticationFailuresWithoutAuthorizationValue(t *testing.T) {
	logger := &captureLogger{}
	handler := NewHandler(NewService(&fakeRepository{}, fakeIDs{}, fakeClock{}), logger)
	router := NewRouter(handler, fakeAuthenticator{err: errors.New("token rejected")})
	for _, tc := range []struct {
		name      string
		authorize string
		code      string
	}{
		{name: "missing authorization", code: "UNAUTHORIZED"},
		{name: "invalid authorization", authorize: "Bearer secret-token", code: "INVALID_TOKEN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger.infoMessage, logger.infoFields = "", nil
			r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
			r.Header.Set("Authorization", tc.authorize)
			r.Header.Set("X-Request-ID", "request-auth")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", w.Code)
			}
			if logger.infoMessage != "post request completed" || !hasLogField(logger.infoFields, "request_id", "request-auth") || !hasLogField(logger.infoFields, "route", "POST /api/v1/posts") || !hasLogField(logger.infoFields, "status", "401") || !hasLogField(logger.infoFields, "error_code", tc.code) || !hasLogFieldKey(logger.infoFields, "duration") {
				t.Fatalf("log = %q %#v", logger.infoMessage, logger.infoFields)
			}
			for _, field := range logger.infoFields {
				if field.Key == "authorization" || field.Key == "token" || field.Key == "secret" || field.Value == "secret-token" {
					t.Fatalf("authorization value leaked in log field %#v", field)
				}
			}
		})
	}
}

func TestHandlerMapsOversizedPayload(t *testing.T) {
	handler := NewHandler(NewService(&fakeRepository{}, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, fakeClock{value: time.Now()}), nil)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"`+strings.Repeat("x", 1<<20)+`"}`))
	r.Header.Set("Content-Type", "application/json")
	r = WithAuthenticatedUserID(r, "0192c5f4-7b2a-7abc-8def-1234567890ab")
	w := httptest.NewRecorder()
	handler.Create(w, r)
	if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), `"code":"PAYLOAD_TOO_LARGE"`) {
		t.Fatalf("status/body = %d/%s", w.Code, w.Body.String())
	}
}

func TestPostRequestIDAndResponseWriterDefaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	r.Header.Set("X-Request-ID", "request-456")
	if got := requestID(r); got != "request-456" {
		t.Fatalf("requestID() = %q", got)
	}
	w := &responseWriter{ResponseWriter: httptest.NewRecorder()}
	if _, err := w.Write([]byte("ok")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if w.status != http.StatusOK {
		t.Fatalf("implicit status = %d", w.status)
	}
}

func hasLogField(fields []logging.Field, key, want string) bool {
	for _, field := range fields {
		if field.Key == key && field.Value == want {
			return true
		}
	}
	return false
}

func hasLogFieldKey(fields []logging.Field, key string) bool {
	for _, field := range fields {
		if field.Key == key {
			return true
		}
	}
	return false
}

type failingLogger struct{}

func (failingLogger) Info(string, ...logging.Field) error  { return errors.New("info failed") }
func (failingLogger) Error(string, ...logging.Field) error { return errors.New("error failed") }
func (failingLogger) Sync() error                          { return nil }

func TestHandlerIgnoresLoggerFailures(t *testing.T) {
	handler := NewHandler(NewService(&fakeRepository{}, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, fakeClock{value: time.Now()}), failingLogger{})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
	r.Header.Set("Content-Type", "application/json")
	r = WithAuthenticatedUserID(r, "0192c5f4-7b2a-7abc-8def-1234567890ab")
	w := httptest.NewRecorder()
	handler.Create(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d", w.Code)
	}
}
