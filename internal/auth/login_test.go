package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type loginStore struct {
	user       user.User
	err        error
	session    auth.RefreshSession
	sessionErr error
	findCalls  int
}

func (s *loginStore) FindByLogin(context.Context, string) (user.User, error) {
	s.findCalls++
	return s.user, s.err
}
func (s *loginStore) CreateLoginSession(_ context.Context, session auth.RefreshSession) error {
	s.session = session
	return s.sessionErr
}

type loginVerifier struct{ err error }

func (v loginVerifier) Compare(string, string) error { return v.err }

type loginIDs struct{}

func (loginIDs) New() (string, error) { return "session-id", nil }

type loginClock struct{}

func (loginClock) Now() time.Time { return time.Unix(100, 0).UTC() }

type loginAccess struct{}

func (loginAccess) Issue(context.Context, user.User, time.Time) (string, error) {
	return "access.jwt", nil
}

type loginOpaque struct{}

func (loginOpaque) Generate() (string, string, error) { return "opaque-refresh", "hashed-refresh", nil }

type failingID struct{}

func (failingID) New() (string, error) { return "", errors.New("id failure") }

type loginService struct {
	result auth.RegisterResult
	err    error
}

func (s loginService) Register(context.Context, auth.RegisterCommand) (auth.RegisterResult, error) {
	return auth.RegisterResult{}, nil
}
func (s loginService) Login(context.Context, auth.LoginCommand) (auth.RegisterResult, error) {
	return s.result, s.err
}
func (s loginService) Logout(context.Context, string) error { return s.err }

func newLoginService(store *loginStore, verifierErr error) *auth.Service {
	return &auth.Service{LoginStore: store, Verifier: loginVerifier{err: verifierErr}, IDs: loginIDs{}, Clock: loginClock{}, Access: loginAccess{}, Opaque: loginOpaque{}, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
}

var _ = Describe("LoginService", func() {
	It("accepts username and email and creates a session", func() {
		for _, login := range []string{"alice", "ALICE@EXAMPLE.COM"} {
			store := &loginStore{user: user.User{ID: "user-id", Username: "alice", Email: "alice@example.com", PasswordHash: "hash", DisplayName: "Alice", Role: user.RoleUser, Status: user.StatusActive}}
			got, err := newLoginService(store, nil).Login(context.Background(), auth.LoginCommand{Login: login, Password: "correct-password"})
			Expect(err).NotTo(HaveOccurred())
			Expect(got.User.Email).To(Equal("alice@example.com"))
			Expect(got.Tokens.ExpiresIn).To(Equal(int64(900)))
			Expect(got.Tokens.RefreshExpiresIn).To(Equal(int64(604800)))
			Expect(store.session.UserID).To(Equal("user-id"))
			Expect(store.session.TokenHash).To(Equal("hashed-refresh"))
		}
	})

	It("returns generic invalid credentials for missing, wrong, and blocked users", func() {
		cases := []struct {
			name        string
			store       *loginStore
			verifierErr error
		}{
			{"missing user", &loginStore{err: user.ErrNotFound}, nil},
			{"wrong password", &loginStore{user: user.User{Status: user.StatusActive}}, errors.New("mismatch")},
			{"blocked user", &loginStore{user: user.User{Status: "blocked"}}, nil},
		}
		for _, tc := range cases {
			By(tc.name)
			_, err := newLoginService(tc.store, tc.verifierErr).Login(context.Background(), auth.LoginCommand{Login: "alice", Password: "correct-password"})
			Expect(err).To(MatchError(auth.ErrInvalidCredentials))
		}
	})

	It("rejects malformed username and email identifiers before repository lookup", func() {
		for _, login := range []string{"ab", "alice!", "alice@", "alice@@example.com", "Alice Example <alice@example.com>"} {
			By(login)
			store := &loginStore{}
			_, err := newLoginService(store, nil).Login(context.Background(), auth.LoginCommand{Login: login, Password: "correct-password"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrValidation)).To(BeTrue())
			Expect(store.findCalls).To(Equal(0))
		}
	})
})

var _ = Describe("LoginHandler", func() {
	It("returns a user-safe response", func() {
		result := auth.RegisterResult{User: user.User{ID: "user-id", Username: "alice", Email: "secret@example.com", DisplayName: "Alice", Role: user.RoleUser}, Tokens: auth.TokenPair{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 900, RefreshExpiresIn: 604800}}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"alice@example.com","password":"correct-password"}`))
		r.Header.Set("Content-Type", "application/json")
		(auth.Handler{Service: loginService{result: result}}).Login(w, r)
		Expect(w.Code).To(Equal(http.StatusOK))
		var response map[string]any
		Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
		userValue, ok := response["user"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(userValue).NotTo(HaveKey("email"))
		Expect(response["accessToken"]).To(Equal("access"))
		Expect(response["refreshToken"]).To(Equal("refresh"))
	})

	It("returns generic unauthorized errors for invalid credentials", func() {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"unknown","password":"correct-password"}`))
		r.Header.Set("Content-Type", "application/json")
		(auth.Handler{Service: loginService{err: auth.ErrInvalidCredentials}}).Login(w, r)
		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(w.Body.String()).To(ContainSubstring(`"message":"Invalid credentials"`))
	})

	It("rejects malformed and invalid requests before calling the service", func() {
		cases := []struct {
			name, body, contentType string
			status                  int
		}{
			{name: "malformed JSON", body: `{"login":`, contentType: "application/json", status: http.StatusBadRequest},
			{name: "unknown field", body: `{"login":"alice","password":"correct-password","extra":true}`, contentType: "application/json", status: http.StatusBadRequest},
			{name: "wrong content type", body: `{}`, contentType: "text/plain", status: http.StatusUnsupportedMediaType},
		}
		for _, tc := range cases {
			By(tc.name)
			service := &loginService{}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			(auth.Handler{Service: service}).Login(w, r)
			Expect(w.Code).To(Equal(tc.status))
			Expect(service.result.Tokens.AccessToken).To(BeEmpty())
		}
	})

	It("returns 422 for missing or invalid login and password fields", func() {
		for _, body := range []string{`{}`, `{"login":"","password":"correct-password"}`, `{"login":"alice","password":""}`} {
			service := newLoginService(&loginStore{}, nil)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			(auth.Handler{Service: service}).Login(w, r)
			Expect(w.Code).To(Equal(http.StatusUnprocessableEntity))
		}
	})

	It("translates infrastructure failures", func() {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"alice","password":"correct-password"}`))
		r.Header.Set("Content-Type", "application/json")
		(auth.Handler{Service: loginService{err: auth.ErrDependencyUnavailable}}).Login(w, r)
		Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(w.Body.String()).To(ContainSubstring(`"code":"SERVICE_UNAVAILABLE"`))
	})

	It("does not enumerate accounts", func() {
		bodies := make([]string, 2)
		for i, login := range []string{"unknown", "alice"} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"`+login+`","password":"correct-password"}`))
			r.Header.Set("Content-Type", "application/json")
			(auth.Handler{Service: loginService{err: auth.ErrInvalidCredentials}}).Login(w, r)
			Expect(w.Code).To(Equal(http.StatusUnauthorized))
			bodies[i] = w.Body.String()
		}
		Expect(bodies[0]).To(Equal(bodies[1]))
	})
})
