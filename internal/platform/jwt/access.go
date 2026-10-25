package jwt

import (
	"context"
	"fmt"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

type SigningKeyProvider interface {
	Key(context.Context, string) ([]byte, error)
}
type StaticKey struct {
	KID    string
	Secret []byte
}

func (s StaticKey) Key(_ context.Context, kid string) ([]byte, error) {
	if kid != s.KID {
		return nil, fmt.Errorf("unknown signing key")
	}
	return s.Secret, nil
}

type Access struct {
	Keys                  SigningKeyProvider
	KID, Issuer, Audience string
	TTL                   time.Duration
	Now                   func() time.Time
	ID                    func() string
}

func (a Access) Issue(ctx context.Context, u user.User, now time.Time) (string, error) {
	if a.TTL == 0 {
		a.TTL = 15 * time.Minute
	}
	if a.Now == nil {
		a.Now = time.Now
	}
	if a.ID == nil {
		a.ID = func() string {
			id, err := (platformid.UUIDv7{}).New()
			if err != nil {
				return ""
			}
			return id
		}
	}
	jti := a.ID()
	if !platformid.IsCanonicalUUIDv7(jti) {
		return "", fmt.Errorf("invalid JWT ID")
	}
	if !platformid.IsCanonicalUUIDv7(u.ID) {
		return "", fmt.Errorf("%w: invalid user ID", auth.ErrValidation)
	}
	key, e := a.Keys.Key(ctx, a.KID)
	if e != nil {
		return "", e
	}
	claims := jwt.MapClaims{"iss": a.Issuer, "sub": u.ID, "aud": a.Audience, "role": string(u.Role), "iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(a.TTL).Unix(), "jti": jti}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t.Header["kid"] = a.KID
	t.Header["typ"] = "JWT"
	return t.SignedString(key)
}
