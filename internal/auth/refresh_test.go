package auth_test

import (
	"context"
	"errors"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

type refreshStoreFixture struct {
	*MockRefreshStore
	session   auth.RefreshSession
	getErr    error
	rotated   bool
	revoked   bool
	rotateErr error
	revokeErr error
}

func newRefreshStore(session auth.RefreshSession, getErr, rotateErr, revokeErr error) *refreshStoreFixture {
	s := &refreshStoreFixture{MockRefreshStore: NewMockRefreshStore(gomock.NewController(GinkgoT())), session: session, getErr: getErr, rotateErr: rotateErr, revokeErr: revokeErr}
	s.EXPECT().GetRefreshSession(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string) (auth.RefreshSession, error) {
		if s.getErr != nil {
			return auth.RefreshSession{}, s.getErr
		}
		if s.session.Status != "active" {
			return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
		}
		return s.session, nil
	})
	s.EXPECT().RotateRefreshSession(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string, auth.RefreshSession) error {
		if s.rotateErr != nil {
			return s.rotateErr
		}
		if s.rotated {
			return auth.ErrInvalidRefreshToken
		}
		s.rotated = true
		return nil
	})
	s.EXPECT().RevokeSessionFamilyByHash(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string) error { s.revoked = true; return s.revokeErr })
	return s
}

func newRefreshUser(value user.User, err error) *MockUserLookup {
	lookup := NewMockUserLookup(gomock.NewController(GinkgoT()))
	lookup.EXPECT().FindByID(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, string) (user.User, error) {
		if err != nil {
			return user.User{}, err
		}
		if value.ID == "" {
			value.ID = "0192f8a0-7b00-7000-8000-000000000001"
		}
		if value.Role == "" {
			value.Role = user.RoleUser
		}
		if value.Status == "" {
			value.Status = user.StatusActive
		}
		return value, nil
	})
	return lookup
}

type refreshAccessFixture struct {
	*MockAccessTokenIssuer
	issuedUser user.User
}

func newRefreshClock(now time.Time) *MockClock {
	c := NewMockClock(gomock.NewController(GinkgoT()))
	c.EXPECT().Now().AnyTimes().Return(now)
	return c
}
func newRefreshAccess() *refreshAccessFixture {
	a := &refreshAccessFixture{MockAccessTokenIssuer: NewMockAccessTokenIssuer(gomock.NewController(GinkgoT()))}
	a.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, u user.User, _ time.Time) (string, error) {
		a.issuedUser = u
		return "access", nil
	})
	return a
}
func newRefreshOpaque() *MockOpaqueTokenGenerator {
	o := NewMockOpaqueTokenGenerator(gomock.NewController(GinkgoT()))
	o.EXPECT().Generate().AnyTimes().Return("new-refresh", "new-hash", nil)
	return o
}

func newRefreshDeps() (*MockClock, *refreshAccessFixture, *MockOpaqueTokenGenerator) {
	clock := NewMockClock(gomock.NewController(GinkgoT()))
	clock.EXPECT().Now().AnyTimes().Return(time.Unix(100, 0))
	access := &refreshAccessFixture{MockAccessTokenIssuer: NewMockAccessTokenIssuer(gomock.NewController(GinkgoT()))}
	access.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, value user.User, _ time.Time) (string, error) {
		access.issuedUser = value
		return "access", nil
	})
	opaque := NewMockOpaqueTokenGenerator(gomock.NewController(GinkgoT()))
	opaque.EXPECT().Generate().AnyTimes().Return("new-refresh", "new-hash", nil)
	return clock, access, opaque
}

var _ = Describe("RefreshService", func() {
	It("rotates an active refresh session once", func() {
		store := newRefreshStore(auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000001", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "active", ExpiresAt: time.Now().Add(time.Hour)}, nil, nil, nil)
		access := newRefreshAccess()
		svc := &auth.Service{RefreshStore: store, Users: newRefreshUser(user.User{}, nil), Clock: newRefreshClock(time.Unix(100, 0)), Access: access, Opaque: newRefreshOpaque(), AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
		got, err := svc.Refresh(context.Background(), "old-refresh")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.AccessToken).To(Equal("access"))
		Expect(got.RefreshToken).To(Equal("new-refresh"))
		Expect(store.rotated).To(BeTrue())
	})

	It("revokes the family when a consumed token is replayed", func() {
		store := newRefreshStore(auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000001", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "consumed"}, nil, nil, nil)
		svc := &auth.Service{RefreshStore: store, Users: newRefreshUser(user.User{}, nil), Clock: newRefreshClock(time.Unix(100, 0)), Access: newRefreshAccess(), Opaque: newRefreshOpaque()}
		_, err := svc.Refresh(context.Background(), "old-refresh")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(store.revoked).To(BeTrue())
	})

	It("preserves invalid-token mapping for an unknown refresh token", func() {
		store := newRefreshStore(auth.RefreshSession{}, auth.ErrInvalidRefreshToken, nil, nil)
		svc := &auth.Service{RefreshStore: store, Users: newRefreshUser(user.User{}, nil), Clock: newRefreshClock(time.Unix(100, 0)), Access: newRefreshAccess(), Opaque: newRefreshOpaque()}

		_, err := svc.Refresh(context.Background(), "never-issued")

		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeFalse())
		Expect(store.revoked).To(BeTrue())
	})

	It("preserves an admin role and rejects an inactive user", func() {
		adminAccess := newRefreshAccess()
		adminStore := newRefreshStore(auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000002", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "active"}, nil, nil, nil)
		admin := &auth.Service{RefreshStore: adminStore, Users: newRefreshUser(user.User{ID: "0192f8a0-7b00-7000-8000-000000000001", Role: user.Role("admin"), Status: user.StatusActive}, nil), Clock: newRefreshClock(time.Unix(100, 0)), Access: adminAccess, Opaque: newRefreshOpaque()}
		_, err := admin.Refresh(context.Background(), "admin-refresh")
		Expect(err).NotTo(HaveOccurred())
		Expect(adminAccess.issuedUser.Role).To(Equal(user.Role("admin")))

		inactiveStore := newRefreshStore(auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000003", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "active"}, nil, nil, nil)
		inactive := &auth.Service{RefreshStore: inactiveStore, Users: newRefreshUser(user.User{ID: "0192f8a0-7b00-7000-8000-000000000002", Status: user.Status("inactive")}, nil), Clock: newRefreshClock(time.Unix(100, 0)), Access: newRefreshAccess(), Opaque: newRefreshOpaque()}
		_, err = inactive.Refresh(context.Background(), "inactive-refresh")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(inactiveStore.rotated).To(BeFalse())
	})

	It("returns dependency failure when replay-family revocation fails", func() {
		store := newRefreshStore(auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000001", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "consumed"}, nil, nil, errors.New("revoke failed"))
		svc := &auth.Service{RefreshStore: store, Users: newRefreshUser(user.User{}, nil), Clock: newRefreshClock(time.Unix(100, 0)), Access: newRefreshAccess(), Opaque: newRefreshOpaque()}
		_, err := svc.Refresh(context.Background(), "replayed")
		Expect(err).To(MatchError(ContainSubstring("revoke refresh session family")))
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeTrue())
	})

	It("returns dependency failure when rotation conflicts and revocation fails", func() {
		store := newRefreshStore(auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000001", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "active"}, nil, auth.ErrInvalidRefreshToken, errors.New("revoke failed"))
		svc := &auth.Service{RefreshStore: store, Users: newRefreshUser(user.User{}, nil), Clock: newRefreshClock(time.Unix(100, 0)), Access: newRefreshAccess(), Opaque: newRefreshOpaque()}
		_, err := svc.Refresh(context.Background(), "conflict")
		Expect(err).To(MatchError(ContainSubstring("revoke refresh session family")))
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeTrue())
	})
})
