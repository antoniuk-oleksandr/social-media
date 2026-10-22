package auth

import (
	"context"
	"errors"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"time"
)

var (
	ErrUsernameTaken         = errors.New("username taken")
	ErrEmailTaken            = errors.New("email taken")
	ErrValidation            = errors.New("validation failed")
	ErrDependencyUnavailable = errors.New("dependency unavailable")
)

type RegisterCommand struct{ Username, Email, Password, DisplayName string }
type RefreshSession struct {
	TokenHash, SessionID, UserID string
	CreatedAt, ExpiresAt         time.Time
}
type Registration struct {
	User    user.User
	Refresh RefreshSession
}
type TokenPair struct {
	AccessToken, RefreshToken   string
	ExpiresIn, RefreshExpiresIn int64
}
type RegisterResult struct {
	User   user.User
	Tokens TokenPair
}
type RegistrationStore interface {
	Register(context.Context, Registration) error
}
type PasswordHasher interface{ Hash(string) (string, error) }
type IDGenerator interface{ New() (string, error) }
type Clock interface{ Now() time.Time }
type AccessTokenIssuer interface {
	Issue(context.Context, user.User, time.Time) (string, error)
}
type OpaqueTokenGenerator interface {
	Generate() (string, string, error)
}
