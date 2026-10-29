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
	"go.uber.org/mock/gomock"
)

func failureService(t gomock.TestReporter, kind string, storeErr error) auth.Service {
	ctrl := gomock.NewController(t)
	store := NewMockRegistrationStore(ctrl)
	store.EXPECT().Register(gomock.Any(), gomock.Any()).AnyTimes().Return(storeErr)
	hasher := NewMockPasswordHasher(ctrl)
	ids := NewMockIDGenerator(ctrl)
	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().AnyTimes().Return(time.Unix(100, 0))
	access := NewMockAccessTokenIssuer(ctrl)
	opaque := NewMockOpaqueTokenGenerator(ctrl)
	s := auth.Service{Store: store, Hasher: hasher, IDs: ids, Clock: clock, Access: access, Opaque: opaque, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
	switch kind {
	case "hash":
		hasher.EXPECT().Hash(gomock.Any()).AnyTimes().Return("", errors.New("hash failure"))
	default:
		hasher.EXPECT().Hash(gomock.Any()).AnyTimes().Return("hash", nil)
	}
	if kind == "id" {
		ids.EXPECT().New().AnyTimes().Return("", errors.New("id failure"))
	} else {
		ids.EXPECT().New().AnyTimes().Return("0192f8a0-7b00-7000-8000-000000000001", nil)
	}
	if kind == "access" {
		access.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return("", errors.New("token failure"))
	} else {
		access.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return("access", nil)
	}
	if kind == "opaque" {
		opaque.EXPECT().Generate().AnyTimes().Return("", "", errors.New("random failure"))
	} else {
		opaque.EXPECT().Generate().AnyTimes().Return("refresh", "hash", nil)
	}
	return s
}

func handlerServiceWithRegistrationError(t gomock.TestReporter, err error) *auth.Service {
	returnService := failureService(t, "", err)
	return &returnService
}

var _ = Describe("RegisterServiceDependencyFailures", func() {
	It("runs the test", func() {
		cases := []struct {
			name, kind string
			err        error
		}{{"hash", "hash", nil}, {"id", "id", nil}, {"access", "access", nil}, {"opaque", "opaque", nil}, {"store", "", errors.New("store failure")}}
		for _, tc := range cases {
			{
				service := failureService(GinkgoT(), tc.kind, tc.err)
				if _, err := service.Register(context.Background(), validRegistration()); err == nil {
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
				h := auth.Handler{Service: handlerServiceWithRegistrationError(GinkgoT(), tc.err)}
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
		service, calls := newCountingService(gomock.NewController(GinkgoT()), nil)
		h := auth.Handler{Service: service}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"login":"alice","password":"`+strings.Repeat("x", 64<<10)+`"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.Login(w, r)

		Expect(w.Code).To(Equal(http.StatusRequestEntityTooLarge))
		Expect(errorBody(w).Error.Code).To(Equal("PAYLOAD_TOO_LARGE"))
		Expect(*calls).To(Equal(0))
	})

	It("rejects empty and unwired refresh requests without touching dependencies", func() {
		service := &auth.Service{}

		_, err := service.Refresh(context.Background(), "  ")
		Expect(err).To(MatchError(ContainSubstring("refresh token is required")))

		_, err = service.Refresh(context.Background(), "token")
		Expect(err).To(MatchError(auth.ErrDependencyUnavailable))
	})

	It("maps a deleted refresh-session user to an invalid token", func() {
		store := newRefreshStore(auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000004", Status: "active"}, nil, nil, nil)
		service := &auth.Service{
			RefreshStore: store,
			Users:        newRefreshUser(user.User{}, user.ErrNotFound),
			Clock:        newRefreshClock(time.Unix(100, 0)),
			Access:       newRefreshAccess(),
			Opaque:       newRefreshOpaque(),
		}

		_, err := service.Refresh(context.Background(), "deleted-user-token")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(store.rotated).To(BeFalse())
	})
})
