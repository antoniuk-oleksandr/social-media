package auth_test

import (
	"context"
	"errors"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
)

type failingStore struct{ err error }

func (s failingStore) Register(context.Context, auth.Registration) error { return s.err }

type failingHasher struct{}

func (failingHasher) Hash(string) (string, error) { return "", errors.New("hash failure") }

type failingIDs struct {
	calls     int
	failFirst bool
}

func (f *failingIDs) New() (string, error) {
	f.calls++
	if (f.failFirst && f.calls == 1) || (!f.failFirst && f.calls == 2) {
		return "", errors.New("id failure")
	}
	return "id", nil
}

type failingAccess struct{}

func (failingAccess) Issue(context.Context, user.User, time.Time) (string, error) {
	return "", errors.New("token failure")
}

type failingOpaque struct{}

func (failingOpaque) Generate() (string, string, error) { return "", "", errors.New("random failure") }

func baseService(store auth.RegistrationStore) auth.Service {
	return auth.Service{Store: store, Hasher: hasher{}, IDs: &ids{}, Clock: clk{}, Access: access{}, Opaque: opaque{}, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
}

var _ = Describe("RegisterServiceDependencyFailures", func() {
	It("runs the test", func() {
		cases := []struct {
			name    string
			service auth.Service
		}{
			{"hash", baseService(&store{})},
			{"id", baseService(&store{})},
			{"id-first", baseService(&store{})},
			{"access", baseService(&store{})},
			{"opaque", baseService(&store{})},
			{"store", baseService(failingStore{err: errors.New("store failure")})},
		}
		cases[0].service.Hasher = failingHasher{}
		cases[1].service.IDs = &failingIDs{}
		cases[2].service.IDs = &failingIDs{failFirst: true}
		cases[3].service.Access = failingAccess{}
		cases[4].service.Opaque = failingOpaque{}
		for _, tc := range cases {
			{
				if _, err := tc.service.Register(context.Background(), valid()); err == nil {
					Expect(false).To(BeTrue(), fmt.Sprint("expected failure"))
				}
			}
		}

	})
})

var _ = Describe("RegisterHandlerErrorMappings", func() {
	It("runs the test", func() {
		for _, tc := range []struct {
			name   string
			err    error
			status int
			code   string
		}{
			{"validation", auth.ErrValidation, 422, "VALIDATION_FAILED"},
			{"username", auth.ErrUsernameTaken, 409, "USERNAME_TAKEN"},
			{"email", auth.ErrEmailTaken, 409, "EMAIL_TAKEN"},
			{"internal", errors.New("internal"), 500, "INTERNAL_ERROR"},
		} {
			{
				h := auth.Handler{Service: &auth.Service{Store: failingStore{err: tc.err}, Hasher: hasher{}, IDs: &ids{}, Clock: clk{}, Access: access{}, Opaque: opaque{}}}
				r := httptest.NewRequest("POST", "/", strings.NewReader(`{"username":"alice","email":"a@example.com","password":"123456789012","displayName":"Alice"}`))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				h.Register(w, r)
				if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
					Expect(false).To(BeTrue(), fmt.Sprintf("got %d %s", w.Code, w.Body))
				}
			}
		}

	})
})

var _ = Describe("RegisterHandlerRejectsMalformedAndWrongContentType", func() {
	It("runs the test", func() {
		for _, body := range []string{"{", "[]", `{"username":"alice"} trailing`} {
			r := httptest.NewRequest("POST", "/", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler().Register(w, r)
			if w.Code != 400 {
				Expect(false).To(BeTrue(), fmt.Sprintf("body %q: got %d", body, w.Code))
			}
		}
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		handler().Register(w, r)
		if w.Code != 415 {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %d", w.Code))
		}

	})
})

var _ = Describe("RouterRegistersEndpoint", func() {
	It("runs the test", func() {
		r := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"username":"alice","email":"a@example.com","password":"123456789012","displayName":"Alice"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		auth.Router(*handler()).ServeHTTP(w, r)
		if w.Code != 201 {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %d", w.Code))
		}

	})
})

var _ = Describe("AdditionalServiceAndLoginCoverage", func() {
	It("rejects oversized login payloads before invoking the service", func() {
		service := &countingService{}
		h := auth.Handler{Service: service}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"alice","password":"`+strings.Repeat("x", 64<<10)+`"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.Login(w, r)

		Expect(w.Code).To(Equal(http.StatusRequestEntityTooLarge))
		Expect(errorBody(w).Error.Code).To(Equal("PAYLOAD_TOO_LARGE"))
		Expect(service.calls).To(Equal(0))
	})

	It("rejects empty and unwired refresh requests without touching dependencies", func() {
		service := &auth.Service{}

		_, err := service.Refresh(context.Background(), "  ")
		Expect(err).To(MatchError(ContainSubstring("refresh token is required")))

		_, err = service.Refresh(context.Background(), "token")
		Expect(err).To(MatchError(auth.ErrDependencyUnavailable))
	})

	It("maps a deleted refresh-session user to an invalid token", func() {
		store := &refreshStoreFake{session: auth.RefreshSession{UserID: "deleted", Status: "active"}}
		service := &auth.Service{
			RefreshStore: store,
			Users:        refreshUserLookup{err: user.ErrNotFound},
			Clock:        refreshClock{now: time.Unix(100, 0)},
			Access:       &refreshAccess{},
			Opaque:       &refreshOpaque{},
		}

		_, err := service.Refresh(context.Background(), "deleted-user-token")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(store.rotated).To(BeFalse())
	})
})
