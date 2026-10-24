package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"strings"
	"time"
)

type Service struct {
	Store                 RegistrationStore
	LoginStore            LoginStore
	Hasher                PasswordHasher
	Verifier              PasswordVerifier
	IDs                   IDGenerator
	Clock                 Clock
	Access                AccessTokenIssuer
	Opaque                OpaqueTokenGenerator
	LogoutStore           LogoutStore
	RefreshStore          RefreshStore
	Users                 UserLookup
	AccessTTL, RefreshTTL time.Duration
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return TokenPair{}, fmt.Errorf("%w: refresh token is required", ErrValidation)
	}
	if s.RefreshStore == nil || s.Users == nil || s.Clock == nil || s.Access == nil || s.Opaque == nil {
		return TokenPair{}, ErrDependencyUnavailable
	}
	hash := HashRefreshToken(refreshToken)
	session, err := s.RefreshStore.GetRefreshSession(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			return TokenPair{}, s.revokeRefreshFamilyAfterInvalid(ctx, hash, err)
		}
		return TokenPair{}, err
	}
	now := s.Clock.Now()
	u, err := s.Users.FindByID(ctx, session.UserID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return TokenPair{}, ErrInvalidRefreshToken
		}
		return TokenPair{}, err
	}
	if u.Status != user.StatusActive {
		return TokenPair{}, ErrInvalidRefreshToken
	}
	access, err := s.Access.Issue(ctx, u, now)
	if err != nil {
		return TokenPair{}, err
	}
	plain, newHash, err := s.Opaque.Generate()
	if err != nil {
		return TokenPair{}, err
	}
	err = s.RefreshStore.RotateRefreshSession(ctx, hash, RefreshSession{TokenHash: newHash, SessionID: session.SessionID, UserID: session.UserID, CreatedAt: now, ExpiresAt: now.Add(s.RefreshTTL)})
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			return TokenPair{}, s.revokeRefreshFamilyAfterInvalid(ctx, hash, err)
		}
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: plain, ExpiresIn: int64(s.AccessTTL / time.Second), RefreshExpiresIn: int64(s.RefreshTTL / time.Second)}, nil
}

func (s *Service) revokeRefreshFamilyAfterInvalid(ctx context.Context, hash string, original error) error {
	if revokeErr := s.RefreshStore.RevokeSessionFamilyByHash(ctx, hash); revokeErr != nil && !errors.Is(revokeErr, ErrInvalidRefreshToken) {
		return fmt.Errorf("%w: revoke refresh session family: %v", ErrDependencyUnavailable, revokeErr)
	}
	return original
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return fmt.Errorf("%w: refresh token is required", ErrValidation)
	}
	if s.LogoutStore == nil {
		return ErrDependencyUnavailable
	}
	return s.LogoutStore.RevokeSessionFamily(ctx, HashRefreshToken(refreshToken))
}

func (s *Service) Register(ctx context.Context, raw RegisterCommand) (RegisterResult, error) {
	c, f := Validate(raw)
	if len(f) > 0 {
		return RegisterResult{}, fmt.Errorf("%w: %v", ErrValidation, f)
	}
	now := s.Clock.Now()
	uid, e := s.IDs.New()
	if e != nil {
		return RegisterResult{}, e
	}
	sid, e := s.IDs.New()
	if e != nil {
		return RegisterResult{}, e
	}
	ph, e := s.Hasher.Hash(c.Password)
	if e != nil {
		return RegisterResult{}, e
	}
	u := user.User{ID: uid, Username: c.Username, Email: c.Email, PasswordHash: ph, DisplayName: c.DisplayName, Role: user.RoleUser, Status: user.StatusActive, CreatedAt: now}
	access, e := s.Access.Issue(ctx, u, now)
	if e != nil {
		return RegisterResult{}, e
	}
	plain, hash, e := s.Opaque.Generate()
	if e != nil {
		return RegisterResult{}, e
	}
	r := Registration{User: u, Refresh: RefreshSession{TokenHash: hash, SessionID: sid, UserID: uid, CreatedAt: now, ExpiresAt: now.Add(s.RefreshTTL)}}
	if e = s.Store.Register(ctx, r); e != nil {
		return RegisterResult{}, e
	}
	return RegisterResult{User: u, Tokens: TokenPair{AccessToken: access, RefreshToken: plain, ExpiresIn: int64(s.AccessTTL / time.Second), RefreshExpiresIn: int64(s.RefreshTTL / time.Second)}}, nil
}
func HashRefreshToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (s *Service) Login(ctx context.Context, raw LoginCommand) (RegisterResult, error) {
	login := strings.ToLower(strings.TrimSpace(raw.Login))
	if !validLoginIdentifier(login) || len([]byte(raw.Password)) < 12 || len([]byte(raw.Password)) > 72 {
		return RegisterResult{}, fmt.Errorf("%w: login is required and password must be 12-72 bytes", ErrValidation)
	}
	store := s.LoginStore
	if store == nil {
		var ok bool
		store, ok = s.Store.(LoginStore)
		if !ok {
			return RegisterResult{}, ErrDependencyUnavailable
		}
	}
	if s.Verifier == nil {
		return RegisterResult{}, ErrDependencyUnavailable
	}
	u, err := store.FindByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, user.ErrNotFound) {
			return RegisterResult{}, ErrInvalidCredentials
		}
		return RegisterResult{}, err
	}
	if u.Status != user.StatusActive || s.Verifier.Compare(u.PasswordHash, raw.Password) != nil {
		return RegisterResult{}, ErrInvalidCredentials
	}
	now := s.Clock.Now()
	sid, err := s.IDs.New()
	if err != nil {
		return RegisterResult{}, err
	}
	access, err := s.Access.Issue(ctx, u, now)
	if err != nil {
		return RegisterResult{}, err
	}
	plain, hash, err := s.Opaque.Generate()
	if err != nil {
		return RegisterResult{}, err
	}
	refresh := RefreshSession{TokenHash: hash, SessionID: sid, UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(s.RefreshTTL)}
	if err := store.CreateLoginSession(ctx, refresh); err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{User: u, Tokens: TokenPair{AccessToken: access, RefreshToken: plain, ExpiresIn: int64(s.AccessTTL / time.Second), RefreshExpiresIn: int64(s.RefreshTTL / time.Second)}}, nil
}
