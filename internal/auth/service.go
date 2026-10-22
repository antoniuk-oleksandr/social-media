package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"time"
)

type Service struct {
	Store                 RegistrationStore
	Hasher                PasswordHasher
	IDs                   IDGenerator
	Clock                 Clock
	Access                AccessTokenIssuer
	Opaque                OpaqueTokenGenerator
	AccessTTL, RefreshTTL time.Duration
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
	return hex.EncodeToString(h[:])
}
