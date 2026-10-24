package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type refreshHandlerService struct {
	mu       sync.Mutex
	result   auth.TokenPair
	err      error
	calls    int
	lastBody string
}

func (s *refreshHandlerService) Register(context.Context, auth.RegisterCommand) (auth.RegisterResult, error) {
	return auth.RegisterResult{}, nil
}
func (s *refreshHandlerService) Login(context.Context, auth.LoginCommand) (auth.RegisterResult, error) {
	return auth.RegisterResult{}, nil
}
func (s *refreshHandlerService) Logout(context.Context, string) error { return nil }
func (s *refreshHandlerService) Refresh(_ context.Context, token string) (auth.TokenPair, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.lastBody = token
	return s.result, s.err
}

func refreshRequest(body, contentType string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	return r
}

var _ = Describe("RefreshHandler", func() {
	It("returns the rotated token pair", func() {
		service := &refreshHandlerService{result: auth.TokenPair{AccessToken: "access", RefreshToken: "next", ExpiresIn: 900, RefreshExpiresIn: 604800}}
		response := httptest.NewRecorder()
		auth.Handler{Service: service}.Refresh(response, refreshRequest(`{"refreshToken":"old"}`, "application/json"))

		Expect(response.Code).To(Equal(http.StatusOK))
		var body auth.RefreshResponse
		Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
		Expect(body).To(Equal(auth.RefreshResponse{AccessToken: "access", RefreshToken: "next", TokenType: "Bearer", ExpiresIn: 900, RefreshExpiresIn: 604800}))
		Expect(service.calls).To(Equal(1))
		Expect(service.lastBody).To(Equal("old"))
	})

	It("maps malformed JSON to 400 before calling the service", func() {
		cases := []string{
			`{"refreshToken":`,
			`{"refreshToken":"old","unexpected":true}`,
			`{"refreshToken":"old","refreshToken":"other"}`,
			`{"refreshToken":"old"} null`,
		}
		for _, body := range cases {
			service := &refreshHandlerService{}
			response := httptest.NewRecorder()
			auth.Handler{Service: service}.Refresh(response, refreshRequest(body, "application/json"))
			Expect(response.Code).To(Equal(http.StatusBadRequest))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"INVALID_JSON"`))
			Expect(service.calls).To(Equal(0))
		}
	})

	It("distinguishes malformed JSON from valid JSON with invalid fields", func() {
		malformed := httptest.NewRecorder()
		auth.Handler{Service: &refreshHandlerService{}}.Refresh(malformed, refreshRequest(`{"refreshToken":`, "application/json"))
		Expect(malformed.Code).To(Equal(http.StatusBadRequest))
		Expect(malformed.Body.String()).To(ContainSubstring(`"code":"INVALID_JSON"`))

		invalidFields := httptest.NewRecorder()
		service := &refreshHandlerService{err: auth.ErrValidation}
		auth.Handler{Service: service}.Refresh(invalidFields, refreshRequest(`{"refreshToken":""}`, "application/json"))
		Expect(invalidFields.Code).To(Equal(http.StatusBadRequest))
		Expect(invalidFields.Body.String()).To(ContainSubstring(`"code":"INVALID_REQUEST"`))
		Expect(service.calls).To(Equal(1))
	})

	It("maps content type and size violations distinctly", func() {
		cases := []struct {
			body, contentType string
			status            int
			code              string
		}{
			{`{"refreshToken":"old"}`, "", http.StatusUnsupportedMediaType, "INVALID_JSON"},
			{`{"refreshToken":"old"}`, "application/json; charset=utf-8", http.StatusUnsupportedMediaType, "INVALID_JSON"},
			{`{"refreshToken":"` + strings.Repeat("x", 128<<10) + `"}`, "application/json", http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE"},
		}
		for _, tc := range cases {
			service := &refreshHandlerService{}
			response := httptest.NewRecorder()
			auth.Handler{Service: service}.Refresh(response, refreshRequest(tc.body, tc.contentType))
			Expect(response.Code).To(Equal(tc.status))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"` + tc.code + `"`))
			Expect(service.calls).To(Equal(0))
		}
	})

	It("maps service errors to their documented status and code", func() {
		cases := []struct {
			err    error
			status int
			code   string
		}{
			{auth.ErrInvalidRefreshToken, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN"},
			{auth.ErrDependencyUnavailable, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
			{errors.New("backend failure"), http.StatusInternalServerError, "INTERNAL_ERROR"},
		}
		for _, tc := range cases {
			response := httptest.NewRecorder()
			r := refreshRequest(`{"refreshToken":"old"}`, "application/json")
			r.Header.Set("X-Request-ID", "refresh-test")
			auth.Handler{Service: &refreshHandlerService{err: tc.err}}.Refresh(response, r)
			Expect(response.Code).To(Equal(tc.status))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"` + tc.code + `"`))
		}
	})

	It("maps an unknown refresh token from the service to 401 INVALID_REFRESH_TOKEN", func() {
		store := &refreshStoreFake{getErr: auth.ErrInvalidRefreshToken}
		service := &auth.Service{RefreshStore: store, Users: refreshUserLookup{}, Clock: refreshClock{now: time.Unix(100, 0)}, Access: &refreshAccess{}, Opaque: &refreshOpaque{}}
		response := httptest.NewRecorder()
		auth.Handler{Service: service}.Refresh(response, refreshRequest(`{"refreshToken":"never-issued"}`, "application/json"))

		Expect(response.Code).To(Equal(http.StatusUnauthorized))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"INVALID_REFRESH_TOKEN"`))
		Expect(response.Body.String()).NotTo(ContainSubstring(`"code":"SERVICE_UNAVAILABLE"`))
		Expect(store.revoked).To(BeTrue())
	})

	It("returns unavailable when refresh is not wired", func() {
		response := httptest.NewRecorder()
		auth.Handler{Service: &logoutOnlyRefreshService{}}.Refresh(response, refreshRequest(`{"refreshToken":"old"}`, "application/json"))
		Expect(response.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"SERVICE_UNAVAILABLE"`))
	})
})

type logoutOnlyRefreshService struct{}

func (*logoutOnlyRefreshService) Register(context.Context, auth.RegisterCommand) (auth.RegisterResult, error) {
	return auth.RegisterResult{}, nil
}
func (*logoutOnlyRefreshService) Login(context.Context, auth.LoginCommand) (auth.RegisterResult, error) {
	return auth.RegisterResult{}, nil
}
func (*logoutOnlyRefreshService) Logout(context.Context, string) error { return nil }

type concurrentRefreshStore struct {
	mu      sync.Mutex
	active  bool
	rotates int
	rotated int
	revoked bool
	session auth.RefreshSession
}

func (s *concurrentRefreshStore) GetRefreshSession(context.Context, string) (auth.RefreshSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
	}
	return s.session, nil
}
func (s *concurrentRefreshStore) RotateRefreshSession(context.Context, string, auth.RefreshSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rotates++
	if !s.active {
		return auth.ErrInvalidRefreshToken
	}
	s.active = false
	s.rotated++
	return nil
}
func (s *concurrentRefreshStore) RevokeSessionFamilyByHash(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked = true
	return nil
}

type concurrentAccessIssuer struct{}

func (concurrentAccessIssuer) Issue(context.Context, user.User, time.Time) (string, error) {
	return "access", nil
}

type concurrentOpaqueGenerator struct{}

func (concurrentOpaqueGenerator) Generate() (string, string, error) { return "next", "next-hash", nil }

type concurrentClock struct{}

func (concurrentClock) Now() time.Time { return time.Unix(100, 0) }

type concurrentUserLookup struct{}

func (concurrentUserLookup) FindByID(context.Context, string) (user.User, error) {
	return user.User{Role: user.Role("user"), Status: user.StatusActive}, nil
}

var _ = Describe("RefreshService concurrency", func() {
	It("allows one rotation and revokes the replay", func() {
		store := &concurrentRefreshStore{active: true, session: auth.RefreshSession{UserID: "user", SessionID: "family", Status: "active"}}
		service := &auth.Service{RefreshStore: store, Users: concurrentUserLookup{}, Clock: concurrentClock{}, Access: concurrentAccessIssuer{}, Opaque: concurrentOpaqueGenerator{}, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
		start := make(chan struct{})
		results := make(chan error, 2)
		for range 2 {
			go func() { <-start; _, err := service.Refresh(context.Background(), "old"); results <- err }()
		}
		close(start)
		var success, invalid int
		for range 2 {
			switch err := <-results; {
			case err == nil:
				success++
			case errors.Is(err, auth.ErrInvalidRefreshToken):
				invalid++
			default:
				Fail("unexpected concurrent refresh error: " + err.Error())
			}
		}
		Expect(success).To(Equal(1))
		Expect(invalid).To(Equal(1))
		store.mu.Lock()
		defer store.mu.Unlock()
		Expect(store.rotated).To(Equal(1))
		Expect(store.revoked).To(BeTrue())
	})
})
