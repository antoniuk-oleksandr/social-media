package post

import (
	"context"
	"errors"
	"strings"
	"time"

	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
)

var (
	ErrValidation            = errors.New("invalid post content")
	ErrInvalidID             = errors.New("invalid post id")
	ErrNotFound              = errors.New("post not found")
	ErrDependencyUnavailable = errors.New("dependency unavailable")
)

type Post struct {
	ID             string
	AuthorID       string
	Content        string
	CreatedAt      time.Time
	CreatedAtEpoch int64
}

type CreateCommand struct {
	AuthorID string
	Content  string
}

type Repository interface {
	Create(context.Context, Post) error
	GetByID(context.Context, string) (Post, error)
}

type IDGenerator interface {
	New() (string, error)
}

type Clock interface {
	Now() time.Time
}

func NormalizeContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	count := len([]rune(content))
	if count < 1 || count > 500 {
		return "", ErrValidation
	}
	return content, nil
}

func validUUIDv7(value string) bool {
	return platformid.IsCanonicalUUIDv7(value)
}
