package post

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"go.uber.org/mock/gomock"
)

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
			ctrl := gomock.NewController(t)
			logger := NewMockLogger(ctrl)
			var infoMessage, errorMessage string
			var infoFields, errorFields []logging.Field
			logger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(message string, fields ...logging.Field) error {
				infoMessage, infoFields = message, fields
				return nil
			})
			logger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(message string, fields ...logging.Field) error {
				errorMessage, errorFields = message, fields
				return nil
			})
			logger.EXPECT().Sync().AnyTimes().Return(nil)
			repository, ids, clock := postDependencies(t, Post{}, validPost, nil, tc.repository)
			handler := NewHandler(NewService(repository, ids, clock), logger)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Request-ID", "request-123")
			r = WithAuthenticatedUserID(r, validAuthor)
			w := httptest.NewRecorder()
			handler.Create(w, r)
			fields, message := infoFields, infoMessage
			if !tc.wantInfo {
				fields, message = errorFields, errorMessage
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
	ctrl := gomock.NewController(t)
	logger := NewMockLogger(ctrl)
	var infoMessage string
	var infoFields []logging.Field
	logger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(message string, fields ...logging.Field) error {
		infoMessage, infoFields = message, fields
		return nil
	})
	logger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	logger.EXPECT().Sync().AnyTimes().Return(nil)
	repository, ids, clock := postDependencies(t, Post{}, "", nil, nil)
	auth := postAuth(t, "", errors.New("token rejected"))
	handler := NewHandler(NewService(repository, ids, clock), logger)
	router := NewRouter(handler, auth)
	for _, tc := range []struct {
		name      string
		authorize string
		code      string
	}{
		{name: "missing authorization", code: "UNAUTHORIZED"},
		{name: "invalid authorization", authorize: "Bearer secret-token", code: "INVALID_TOKEN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			infoMessage, infoFields = "", nil
			r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
			r.Header.Set("Authorization", tc.authorize)
			r.Header.Set("X-Request-ID", "request-auth")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", w.Code)
			}
			if infoMessage != "post request completed" || !hasLogField(infoFields, "request_id", "request-auth") || !hasLogField(infoFields, "route", "POST /api/v1/posts") || !hasLogField(infoFields, "status", "401") || !hasLogField(infoFields, "error_code", tc.code) || !hasLogFieldKey(infoFields, "duration") {
				t.Fatalf("log = %q %#v", infoMessage, infoFields)
			}
			for _, field := range infoFields {
				if field.Key == "authorization" || field.Key == "token" || field.Key == "secret" || field.Value == "secret-token" {
					t.Fatalf("authorization value leaked in log field %#v", field)
				}
			}
		})
	}
}

func TestHandlerMapsOversizedPayload(t *testing.T) {
	repository, ids, clock := postDependencies(t, Post{}, "0192c5f4-7b2a-7abd-8def-1234567890ab", nil, nil)
	handler := NewHandler(NewService(repository, ids, clock), nil)
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

func TestHandlerIgnoresLoggerFailures(t *testing.T) {
	repository, ids, clock := postDependencies(t, Post{}, "0192c5f4-7b2a-7abd-8def-1234567890ab", nil, nil)
	ctrl := gomock.NewController(t)
	logger := NewMockLogger(ctrl)
	logger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().Return(errors.New("info failed"))
	logger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes().Return(errors.New("error failed"))
	logger.EXPECT().Sync().AnyTimes().Return(nil)
	handler := NewHandler(NewService(repository, ids, clock), logger)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
	r.Header.Set("Content-Type", "application/json")
	r = WithAuthenticatedUserID(r, "0192c5f4-7b2a-7abc-8def-1234567890ab")
	w := httptest.NewRecorder()
	handler.Create(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d", w.Code)
	}
}
