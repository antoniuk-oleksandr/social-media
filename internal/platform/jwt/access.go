package jwt

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/golang-jwt/jwt/v5"
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

func (a Access) Authenticate(ctx context.Context, token string) (string, error) {
	sub, _, err := a.authenticate(ctx, token)
	return sub, err
}

func (a Access) AuthenticateAdmin(ctx context.Context, token string) (string, error) {
	sub, role, err := a.authenticate(ctx, token)
	if err != nil {
		return "", err
	}
	if role != "admin" {
		return "", fmt.Errorf("forbidden: administrator role required")
	}
	return sub, nil
}

func (a Access) authenticate(ctx context.Context, token string) (string, string, error) {
	parsed, err := jwt.Parse(token, func(value *jwt.Token) (any, error) {
		if value.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("invalid signing method")
		}
		kid, ok := value.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing key id")
		}
		return a.Keys.Key(ctx, kid)
	}, jwt.WithIssuer(a.Issuer), jwt.WithAudience(a.Audience), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || parsed == nil || !parsed.Valid {
		return "", "", fmt.Errorf("invalid access token")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", fmt.Errorf("invalid access claims")
	}
	role, ok := claims["role"].(string)
	if !ok || (role != string(user.RoleUser) && role != "admin") {
		return "", "", fmt.Errorf("invalid access role")
	}
	for _, name := range []string{"iat", "nbf", "exp"} {
		if !numericClaim(claims[name]) {
			return "", "", fmt.Errorf("invalid access %s", name)
		}
	}
	jti, ok := claims["jti"].(string)
	if !ok || !platformid.IsCanonicalUUIDv7(jti) {
		return "", "", fmt.Errorf("invalid access jti")
	}
	sub, ok := claims["sub"].(string)
	if !ok || !platformid.IsCanonicalUUIDv7(sub) {
		return "", "", fmt.Errorf("invalid access subject")
	}
	return sub, role, nil
}

func numericClaim(value any) bool {
	switch value.(type) {
	case float64, float32, int, int64, uint64, json.Number:
		return true
	default:
		return false
	}
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
