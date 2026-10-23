package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type logoutHandlerService struct {
	err   error
	token string
}

type logoutStore struct {
	mu          sync.Mutex
	hash        string
	calls       int
	familyState string
}

func (s *logoutStore) RevokeSessionFamily(_ context.Context, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hash = hash
	s.calls++
	if s.familyState == "revoked" {
		return auth.ErrInvalidRefreshToken
	}
	s.familyState = "revoked"
	return nil
}

func (s *logoutHandlerService) Register(context.Context, auth.RegisterCommand) (auth.RegisterResult, error) {
	return auth.RegisterResult{}, nil
}
func (s *logoutHandlerService) Login(context.Context, auth.LoginCommand) (auth.RegisterResult, error) {
	return auth.RegisterResult{}, nil
}
func (s *logoutHandlerService) Logout(_ context.Context, token string) error {
	s.token = token
	return s.err
}

type logoutRecordingLogger struct {
	fields []logging.Field
}

func (l *logoutRecordingLogger) Info(_ string, fields ...logging.Field) error {
	l.fields = fields
	return nil
}
func (l *logoutRecordingLogger) Error(_ string, fields ...logging.Field) error {
	l.fields = fields
	return nil
}
func (l *logoutRecordingLogger) Sync() error { return nil }

var _ = Describe("LogoutService", func() {
	It("hashes the refresh token before revoking its session family", func() {
		store := &logoutStore{}
		svc := &auth.Service{LogoutStore: store}
		Expect(svc.Logout(context.Background(), "opaque-refresh-token")).To(Succeed())
		Expect(store.hash).To(Equal(auth.HashRefreshToken("opaque-refresh-token")))
	})

	It("rejects an empty refresh token without touching the repository", func() {
		store := &logoutStore{}
		svc := &auth.Service{LogoutStore: store}
		err := svc.Logout(context.Background(), " ")
		Expect(errors.Is(err, auth.ErrValidation)).To(BeTrue())
		Expect(store.hash).To(BeEmpty())
	})

	It("propagates the invalid-token transition when a refresh token is reused", func() {
		store := &logoutStore{}
		svc := &auth.Service{LogoutStore: store}

		Expect(svc.Logout(context.Background(), "opaque-refresh-token")).To(Succeed())
		Expect(svc.Logout(context.Background(), "opaque-refresh-token")).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(store.calls).To(Equal(2))
		Expect(store.hash).To(Equal(auth.HashRefreshToken("opaque-refresh-token")))
		Expect(store.familyState).To(Equal("revoked"))
	})

	It("allows exactly one concurrent logout and records the revoked family state", func() {
		store := &logoutStore{}
		svc := &auth.Service{LogoutStore: store}
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results <- svc.Logout(context.Background(), "concurrent-refresh-token")
			}()
		}
		close(start)
		wg.Wait()
		close(results)

		var successes, invalid int
		for err := range results {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, auth.ErrInvalidRefreshToken):
				invalid++
			default:
				Fail("unexpected concurrent logout error: " + err.Error())
			}
		}
		Expect(successes).To(Equal(1))
		Expect(invalid).To(Equal(1))
		Expect(store.familyState).To(Equal("revoked"))
		Expect(store.calls).To(Equal(2))
	})
})

var _ = Describe("LogoutHandler", func() {
	It("returns 204 and does not expose the raw token in responses", func() {
		service := &logoutHandlerService{}
		h := auth.Handler{Service: service}
		rawToken := "opaque-refresh-token"
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{"refreshToken":"`+rawToken+`"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.Logout(w, r)
		Expect(w.Code).To(Equal(http.StatusNoContent))
		Expect(w.Body.String()).To(BeEmpty())
		Expect(service.token).To(Equal(rawToken))
		Expect(w.Body.String()).NotTo(ContainSubstring(rawToken))
	})

	It("maps an invalid refresh token to the documented error", func() {
		h := auth.Handler{Service: &logoutHandlerService{err: auth.ErrInvalidRefreshToken}}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{"refreshToken":"token"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.Logout(w, r)
		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(w.Body.String()).To(ContainSubstring(`"code":"INVALID_REFRESH_TOKEN"`))
	})

	It("enforces the strict request contract", func() {
		for _, body := range []string{`{"refreshToken":"token","extra":true}`, `{"refreshToken":"token"} {}`, `{`} {
			service := &logoutHandlerService{}
			h := auth.Handler{Service: service}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.Logout(w, r)
			Expect(w.Code).To(Equal(http.StatusBadRequest))
			Expect(service.token).To(BeEmpty())
		}
	})

	It("rejects a non-JSON content type before invoking the service", func() {
		service := &logoutHandlerService{}
		h := auth.Handler{Service: service}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{"refreshToken":"opaque-refresh-token"}`))
		w := httptest.NewRecorder()
		h.Logout(w, r)
		Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType))
		Expect(service.token).To(BeEmpty())
		Expect(w.Body.String()).NotTo(ContainSubstring("opaque-refresh-token"))
	})

	It("maps validation, dependency, and internal service failures", func() {
		for _, tc := range []struct {
			err    error
			status int
			code   string
		}{
			{auth.ErrValidation, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
			{auth.ErrDependencyUnavailable, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
			{errors.New("internal failure"), http.StatusInternalServerError, "INTERNAL_ERROR"},
		} {
			h := auth.Handler{Service: &logoutHandlerService{err: tc.err}}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{"refreshToken":"token"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Request-ID", "req-logout-error")
			w := httptest.NewRecorder()
			h.Logout(w, r)
			Expect(w.Code).To(Equal(tc.status))
			Expect(w.Body.String()).To(ContainSubstring(`"code":"` + tc.code + `"`))
			Expect(w.Body.String()).To(ContainSubstring(`"requestId":"req-logout-error"`))
		}
	})

	It("rejects duplicate keys, oversized bodies, and content-type variants", func() {
		cases := []struct {
			body, contentType string
			status            int
		}{
			{`{"refreshToken":"a","refreshToken":"b"}`, "application/json", http.StatusBadRequest},
			{`{"refreshToken":"` + strings.Repeat("x", 128<<10) + `"}`, "application/json", http.StatusRequestEntityTooLarge},
			{`{"refreshToken":"token"}`, "application/json; charset=utf-8", http.StatusUnsupportedMediaType},
			{`{"refreshToken":"token"}`, "text/json", http.StatusUnsupportedMediaType},
		}
		for _, tc := range cases {
			service := &logoutHandlerService{}
			h := auth.Handler{Service: service}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			h.Logout(w, r)
			Expect(w.Code).To(Equal(tc.status))
			Expect(service.token).To(BeEmpty())
		}
	})

	It("keeps the raw refresh token out of structured logs", func() {
		rawToken := "opaque-secret-refresh-token"
		logger := &logoutRecordingLogger{}
		h := auth.Handler{Service: &logoutHandlerService{err: errors.New("backend failed")}, Logger: logger}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{"refreshToken":"`+rawToken+`"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-ID", "req-safe-logout")
		w := httptest.NewRecorder()
		h.Logout(w, r)
		Expect(w.Code).To(Equal(http.StatusInternalServerError))
		for _, field := range logger.fields {
			Expect(field.Key).NotTo(Equal("refresh_token"))
			Expect(field.Value).NotTo(Equal(rawToken))
			Expect(field.Value).NotTo(ContainSubstring(rawToken))
		}
	})
})

var _ = Describe("LogoutRouter", func() {
	It("registers the POST logout route", func() {
		router := auth.Router(auth.Handler{Service: &logoutHandlerService{}})
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{"refreshToken":"token"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		Expect(response.Code).To(Equal(http.StatusNoContent))
	})
})
