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
	"go.uber.org/mock/gomock"
)

type refreshMethod interface {
	Refresh(context.Context, string) (auth.TokenPair, error)
}

type refreshHandlerService struct {
	*MockServicePort
	*MockrefreshMethod
	mu       sync.Mutex
	result   auth.TokenPair
	err      error
	calls    int
	lastBody string
}

func newRefreshHandlerService(result auth.TokenPair, err error) *refreshHandlerService {
	s := &refreshHandlerService{MockrefreshMethod: NewMockrefreshMethod(gomock.NewController(GinkgoT())), result: result, err: err}
	s.MockrefreshMethod.EXPECT().Refresh(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, token string) (auth.TokenPair, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.calls++
		s.lastBody = token
		return s.result, s.err
	})
	return s
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
		service := newRefreshHandlerService(auth.TokenPair{AccessToken: "access", RefreshToken: "next", ExpiresIn: 900, RefreshExpiresIn: 604800}, nil)
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
			service := newRefreshHandlerService(auth.TokenPair{}, nil)
			response := httptest.NewRecorder()
			auth.Handler{Service: service}.Refresh(response, refreshRequest(body, "application/json"))
			Expect(response.Code).To(Equal(http.StatusBadRequest))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"INVALID_JSON"`))
			Expect(service.calls).To(Equal(0))
		}
	})

	It("distinguishes malformed JSON from valid JSON with invalid fields", func() {
		malformed := httptest.NewRecorder()
		auth.Handler{Service: newRefreshHandlerService(auth.TokenPair{}, nil)}.Refresh(malformed, refreshRequest(`{"refreshToken":`, "application/json"))
		Expect(malformed.Code).To(Equal(http.StatusBadRequest))
		Expect(malformed.Body.String()).To(ContainSubstring(`"code":"INVALID_JSON"`))

		invalidFields := httptest.NewRecorder()
		service := newRefreshHandlerService(auth.TokenPair{}, auth.ErrValidation)
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
			service := newRefreshHandlerService(auth.TokenPair{}, nil)
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
			auth.Handler{Service: newRefreshHandlerService(auth.TokenPair{}, tc.err)}.Refresh(response, r)
			Expect(response.Code).To(Equal(tc.status))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"` + tc.code + `"`))
		}
	})

	It("maps an unknown refresh token from the service to 401 INVALID_REFRESH_TOKEN", func() {
		store := newRefreshStore(auth.RefreshSession{}, auth.ErrInvalidRefreshToken, nil, nil)
		service := &auth.Service{RefreshStore: store, Users: newRefreshUser(user.User{}, nil), Clock: newRefreshClock(time.Unix(100, 0)), Access: newRefreshAccess(), Opaque: newRefreshOpaque()}
		response := httptest.NewRecorder()
		auth.Handler{Service: service}.Refresh(response, refreshRequest(`{"refreshToken":"never-issued"}`, "application/json"))

		Expect(response.Code).To(Equal(http.StatusUnauthorized))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"INVALID_REFRESH_TOKEN"`))
		Expect(response.Body.String()).NotTo(ContainSubstring(`"code":"SERVICE_UNAVAILABLE"`))
		Expect(store.revoked).To(BeTrue())
	})

	It("returns unavailable when refresh is not wired", func() {
		response := httptest.NewRecorder()
		auth.Handler{Service: newUnwiredRefreshService()}.Refresh(response, refreshRequest(`{"refreshToken":"old"}`, "application/json"))
		Expect(response.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"SERVICE_UNAVAILABLE"`))
	})
})

func newUnwiredRefreshService() *refreshHandlerService {
	service := &refreshHandlerService{MockServicePort: NewMockServicePort(gomock.NewController(GinkgoT())), MockrefreshMethod: NewMockrefreshMethod(gomock.NewController(GinkgoT()))}
	service.MockrefreshMethod.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return(auth.TokenPair{}, auth.ErrDependencyUnavailable)
	return service
}

type concurrentRefreshStore struct {
	*MockRefreshStore
	mu      sync.Mutex
	active  bool
	rotates int
	rotated int
	revoked bool
	session auth.RefreshSession
}

func newConcurrentRefreshStore(session auth.RefreshSession) *concurrentRefreshStore {
	s := &concurrentRefreshStore{MockRefreshStore: NewMockRefreshStore(gomock.NewController(GinkgoT())), active: true, session: session}
	s.EXPECT().GetRefreshSession(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string) (auth.RefreshSession, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.active {
			return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
		}
		return s.session, nil
	})
	s.EXPECT().RotateRefreshSession(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string, auth.RefreshSession) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.rotates++
		if !s.active {
			return auth.ErrInvalidRefreshToken
		}
		s.active = false
		s.rotated++
		return nil
	})
	s.EXPECT().RevokeSessionFamilyByHash(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string) error { s.mu.Lock(); defer s.mu.Unlock(); s.revoked = true; return nil })
	return s
}
func newConcurrentAccess() *MockAccessTokenIssuer {
	a := NewMockAccessTokenIssuer(gomock.NewController(GinkgoT()))
	a.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return("access", nil)
	return a
}
func newConcurrentOpaque() *MockOpaqueTokenGenerator {
	o := NewMockOpaqueTokenGenerator(gomock.NewController(GinkgoT()))
	o.EXPECT().Generate().AnyTimes().Return("next", "next-hash", nil)
	return o
}
func newConcurrentClock() *MockClock {
	c := NewMockClock(gomock.NewController(GinkgoT()))
	c.EXPECT().Now().AnyTimes().Return(time.Unix(100, 0))
	return c
}
func newConcurrentUser() *MockUserLookup {
	u := NewMockUserLookup(gomock.NewController(GinkgoT()))
	u.EXPECT().FindByID(gomock.Any(), gomock.Any()).AnyTimes().Return(user.User{Role: user.Role("user"), Status: user.StatusActive}, nil)
	return u
}

var _ = Describe("RefreshService concurrency", func() {
	It("allows one rotation and revokes the replay", func() {
		store := newConcurrentRefreshStore(auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000001", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "active"})
		service := &auth.Service{RefreshStore: store, Users: newConcurrentUser(), Clock: newConcurrentClock(), Access: newConcurrentAccess(), Opaque: newConcurrentOpaque(), AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
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
