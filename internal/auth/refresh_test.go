package auth_test

import (
	"context"
	"errors"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type refreshStoreFake struct {
	session   auth.RefreshSession
	getErr    error
	rotated   bool
	revoked   bool
	rotateErr error
	revokeErr error
}

func (s *refreshStoreFake) GetRefreshSession(context.Context, string) (auth.RefreshSession, error) {
	if s.getErr != nil {
		return auth.RefreshSession{}, s.getErr
	}
	if s.session.Status != "active" {
		return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
	}
	return s.session, nil
}
func (s *refreshStoreFake) RotateRefreshSession(context.Context, string, auth.RefreshSession) error {
	if s.rotateErr != nil {
		return s.rotateErr
	}
	if s.rotated {
		return auth.ErrInvalidRefreshToken
	}
	s.rotated = true
	return nil
}
func (s *refreshStoreFake) RevokeSessionFamilyByHash(context.Context, string) error {
	s.revoked = true
	return s.revokeErr
}

type refreshClock struct{ now time.Time }

func (c refreshClock) Now() time.Time { return c.now }

type refreshAccess struct{ issuedUser user.User }

func (a *refreshAccess) Issue(_ context.Context, u user.User, _ time.Time) (string, error) {
	a.issuedUser = u
	return "access", nil
}

type refreshUserLookup struct {
	user user.User
	err  error
}

func (l refreshUserLookup) FindByID(context.Context, string) (user.User, error) {
	if l.err != nil {
		return user.User{}, l.err
	}
	if l.user.ID == "" {
		l.user.ID = "0192f8a0-7b00-7000-8000-000000000001"
	}
	if l.user.Role == "" {
		l.user.Role = user.RoleUser
	}
	if l.user.Status == "" {
		l.user.Status = user.StatusActive
	}
	return l.user, nil
}

type refreshOpaque struct{ n int }

func (o *refreshOpaque) Generate() (string, string, error) {
	o.n++
	return "new-refresh", "new-hash", nil
}

var _ = Describe("RefreshService", func() {
	It("rotates an active refresh session once", func() {
		store := &refreshStoreFake{session: auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000001", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "active", ExpiresAt: time.Now().Add(time.Hour)}}
		access := &refreshAccess{}
		svc := &auth.Service{RefreshStore: store, Users: refreshUserLookup{}, Clock: refreshClock{now: time.Unix(100, 0)}, Access: access, Opaque: &refreshOpaque{}, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
		got, err := svc.Refresh(context.Background(), "old-refresh")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.AccessToken).To(Equal("access"))
		Expect(got.RefreshToken).To(Equal("new-refresh"))
		Expect(store.rotated).To(BeTrue())
	})

	It("revokes the family when a consumed token is replayed", func() {
		store := &refreshStoreFake{session: auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000001", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "consumed"}}
		svc := &auth.Service{RefreshStore: store, Users: refreshUserLookup{}, Clock: refreshClock{now: time.Unix(100, 0)}, Access: &refreshAccess{}, Opaque: &refreshOpaque{}}
		_, err := svc.Refresh(context.Background(), "old-refresh")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(store.revoked).To(BeTrue())
	})

	It("preserves invalid-token mapping for an unknown refresh token", func() {
		store := &refreshStoreFake{getErr: auth.ErrInvalidRefreshToken}
		svc := &auth.Service{RefreshStore: store, Users: refreshUserLookup{}, Clock: refreshClock{now: time.Unix(100, 0)}, Access: &refreshAccess{}, Opaque: &refreshOpaque{}}

		_, err := svc.Refresh(context.Background(), "never-issued")

		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeFalse())
		Expect(store.revoked).To(BeTrue())
	})

	It("preserves an admin role and rejects an inactive user", func() {
		adminAccess := &refreshAccess{}
		adminStore := &refreshStoreFake{session: auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000002", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "active"}}
		admin := &auth.Service{RefreshStore: adminStore, Users: refreshUserLookup{user: user.User{ID: "0192f8a0-7b00-7000-8000-000000000001", Role: user.Role("admin"), Status: user.StatusActive}}, Clock: refreshClock{now: time.Unix(100, 0)}, Access: adminAccess, Opaque: &refreshOpaque{}}
		_, err := admin.Refresh(context.Background(), "admin-refresh")
		Expect(err).NotTo(HaveOccurred())
		Expect(adminAccess.issuedUser.Role).To(Equal(user.Role("admin")))

		inactiveStore := &refreshStoreFake{session: auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000003", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "active"}}
		inactive := &auth.Service{RefreshStore: inactiveStore, Users: refreshUserLookup{user: user.User{ID: "0192f8a0-7b00-7000-8000-000000000002", Status: user.Status("inactive")}}, Clock: refreshClock{now: time.Unix(100, 0)}, Access: &refreshAccess{}, Opaque: &refreshOpaque{}}
		_, err = inactive.Refresh(context.Background(), "inactive-refresh")
		Expect(err).To(MatchError(auth.ErrInvalidRefreshToken))
		Expect(inactiveStore.rotated).To(BeFalse())
	})

	It("returns dependency failure when replay-family revocation fails", func() {
		store := &refreshStoreFake{session: auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000001", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "consumed"}, revokeErr: errors.New("revoke failed")}
		svc := &auth.Service{RefreshStore: store, Users: refreshUserLookup{}, Clock: refreshClock{now: time.Unix(100, 0)}, Access: &refreshAccess{}, Opaque: &refreshOpaque{}}
		_, err := svc.Refresh(context.Background(), "replayed")
		Expect(err).To(MatchError(ContainSubstring("revoke refresh session family")))
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeTrue())
	})

	It("returns dependency failure when rotation conflicts and revocation fails", func() {
		store := &refreshStoreFake{session: auth.RefreshSession{UserID: "0192f8a0-7b00-7000-8000-000000000001", SessionID: "0192f8a0-7b00-7000-8000-000000000010", Status: "active"}, rotateErr: auth.ErrInvalidRefreshToken, revokeErr: errors.New("revoke failed")}
		svc := &auth.Service{RefreshStore: store, Users: refreshUserLookup{}, Clock: refreshClock{now: time.Unix(100, 0)}, Access: &refreshAccess{}, Opaque: &refreshOpaque{}}
		_, err := svc.Refresh(context.Background(), "conflict")
		Expect(err).To(MatchError(ContainSubstring("revoke refresh session family")))
		Expect(errors.Is(err, auth.ErrDependencyUnavailable)).To(BeTrue())
	})
})
