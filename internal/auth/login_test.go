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
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

type loginStoreFixture struct {
	*MockLoginStore
	session    auth.RefreshSession
	findCalls  int
	sessionErr error
}

func newLoginStore(userValue user.User, findErr error) *loginStoreFixture {
	return newLoginStoreWithSessionErr(userValue, findErr, nil)
}

func newLoginStoreWithSessionErr(userValue user.User, findErr, sessionErr error) *loginStoreFixture {
	store := &loginStoreFixture{MockLoginStore: NewMockLoginStore(gomock.NewController(GinkgoT()))}
	store.EXPECT().FindByLogin(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string) (user.User, error) { store.findCalls++; return userValue, findErr })
	store.EXPECT().CreateLoginSession(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, session auth.RefreshSession) error { store.session = session; return sessionErr })
	return store
}

func newLoginService(store *loginStoreFixture, verifierErr error) *auth.Service {
	verifier := NewMockPasswordVerifier(gomock.NewController(GinkgoT()))
	verifier.EXPECT().Compare(gomock.Any(), gomock.Any()).AnyTimes().Return(verifierErr)
	ids := NewMockIDGenerator(gomock.NewController(GinkgoT()))
	ids.EXPECT().New().AnyTimes().Return("0192f8a0-7b00-7000-8000-000000000002", nil)
	clock := NewMockClock(gomock.NewController(GinkgoT()))
	clock.EXPECT().Now().AnyTimes().Return(time.Unix(100, 0).UTC())
	access := NewMockAccessTokenIssuer(gomock.NewController(GinkgoT()))
	access.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return("access.jwt", nil)
	opaque := NewMockOpaqueTokenGenerator(gomock.NewController(GinkgoT()))
	opaque.EXPECT().Generate().AnyTimes().Return("opaque-refresh", "hashed-refresh", nil)
	return &auth.Service{LoginStore: store, Verifier: verifier, IDs: ids, Clock: clock, Access: access, Opaque: opaque, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
}

var _ = Describe("LoginService", func() {
	It("accepts username and email and creates a session", func() {
		for _, login := range []string{"alice", "ALICE@EXAMPLE.COM"} {
			store := newLoginStore(user.User{ID: "0192f8a0-7b00-7000-8000-000000000001", Username: "alice", Email: "alice@example.com", PasswordHash: "hash", DisplayName: "Alice", Role: user.RoleUser, Status: user.StatusActive}, nil)
			got, err := newLoginService(store, nil).Login(context.Background(), auth.LoginCommand{Login: login, Password: "correct-password"})
			Expect(err).NotTo(HaveOccurred())
			Expect(got.User.Email).To(Equal("alice@example.com"))
			Expect(got.Tokens.ExpiresIn).To(Equal(int64(900)))
			Expect(got.Tokens.RefreshExpiresIn).To(Equal(int64(604800)))
			Expect(platformid.IsCanonicalUUIDv7(store.session.SessionID)).To(BeTrue())
			Expect(store.session.UserID).To(Equal("0192f8a0-7b00-7000-8000-000000000001"))
			Expect(store.session.TokenHash).To(Equal("hashed-refresh"))
		}
	})

	It("returns generic invalid credentials for missing, wrong, and blocked users", func() {
		cases := []struct {
			name        string
			store       *loginStoreFixture
			verifierErr error
		}{
			{"missing user", newLoginStore(user.User{}, user.ErrNotFound), nil},
			{"wrong password", newLoginStore(user.User{Status: user.StatusActive}, nil), errors.New("mismatch")},
			{"blocked user", newLoginStore(user.User{Status: "blocked"}, nil), nil},
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
			store := newLoginStore(user.User{}, nil)
			_, err := newLoginService(store, nil).Login(context.Background(), auth.LoginCommand{Login: login, Password: "correct-password"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrValidation)).To(BeTrue())
			Expect(store.findCalls).To(Equal(0))
		}
	})
})

var _ = Describe("LoginHandler", func() {
	It("returns a user-safe response", func() {
		result := auth.RegisterResult{User: user.User{ID: "0192f8a0-7b00-7000-8000-000000000001", Username: "alice", Email: "secret@example.com", DisplayName: "Alice", Role: user.RoleUser}, Tokens: auth.TokenPair{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 900, RefreshExpiresIn: 604800}}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"alice@example.com","password":"correct-password"}`))
		r.Header.Set("Content-Type", "application/json")
		service := NewMockServicePort(gomock.NewController(GinkgoT()))
		service.EXPECT().Login(gomock.Any(), gomock.Any()).Return(result, nil)
		(auth.Handler{Service: service}).Login(w, r)
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
		service := NewMockServicePort(gomock.NewController(GinkgoT()))
		service.EXPECT().Login(gomock.Any(), gomock.Any()).Return(auth.RegisterResult{}, auth.ErrInvalidCredentials)
		(auth.Handler{Service: service}).Login(w, r)
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
			service := NewMockServicePort(gomock.NewController(GinkgoT()))
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			(auth.Handler{Service: service}).Login(w, r)
			Expect(w.Code).To(Equal(tc.status))
		}
	})

	It("returns 422 for missing or invalid login and password fields", func() {
		for _, body := range []string{`{}`, `{"login":"","password":"correct-password"}`, `{"login":"alice","password":""}`} {
			service := newLoginService(newLoginStore(user.User{}, nil), nil)
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
		service := NewMockServicePort(gomock.NewController(GinkgoT()))
		service.EXPECT().Login(gomock.Any(), gomock.Any()).Return(auth.RegisterResult{}, auth.ErrDependencyUnavailable)
		(auth.Handler{Service: service}).Login(w, r)
		Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(w.Body.String()).To(ContainSubstring(`"code":"SERVICE_UNAVAILABLE"`))
	})

	It("does not enumerate accounts", func() {
		bodies := make([]string, 2)
		for i, login := range []string{"unknown", "alice"} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"`+login+`","password":"correct-password"}`))
			r.Header.Set("Content-Type", "application/json")
			service := NewMockServicePort(gomock.NewController(GinkgoT()))
			service.EXPECT().Login(gomock.Any(), gomock.Any()).Return(auth.RegisterResult{}, auth.ErrInvalidCredentials)
			(auth.Handler{Service: service}).Login(w, r)
			Expect(w.Code).To(Equal(http.StatusUnauthorized))
			bodies[i] = w.Body.String()
		}
		Expect(bodies[0]).To(Equal(bodies[1]))
	})
})
