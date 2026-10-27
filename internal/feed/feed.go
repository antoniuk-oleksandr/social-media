package feed

import (
	"context"
	"errors"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/antoniuk-oleksandr/social-media/internal/post"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
)

var (
	ErrInvalidQuery          = errors.New("invalid feed query")
	ErrInvalidCursor         = errors.New("invalid feed cursor")
	ErrCursorExpired         = errors.New("expired feed cursor")
	ErrDependencyUnavailable = errors.New("dependency unavailable")
)

type PostSource interface {
	List(ctx context.Context, snapshotAt time.Time) ([]post.Post, error)
}
type AdSource interface {
	List(ctx context.Context) ([]ad.Advertisement, error)
}
type AuthorSource interface {
	GetByIDs(context.Context, []string) (map[string]user.User, error)
}
type Clock interface{ Now() time.Time }

type Query struct {
	Limit    int
	LimitSet bool
	Cursor   string
	UserID   string
}
type Item struct {
	Type   string
	Post   *post.Post
	Ad     *ad.Advertisement
	Author *user.User
}
type Result struct {
	Items      []Item
	NextCursor string
	HasMore    bool
}

type Service interface {
	Build(context.Context, Query) (Result, error)
}

type service struct {
	posts   PostSource
	ads     AdSource
	clock   Clock
	key     []byte
	keyID   string
	ttl     time.Duration
	authors AuthorSource
}

func NewService(posts PostSource, ads AdSource, clock Clock, key, keyID string, authors ...AuthorSource) Service {
	var source AuthorSource
	if len(authors) > 0 {
		source = authors[0]
	}
	return &service{posts: posts, ads: ads, clock: clock, key: []byte(key), keyID: keyID, ttl: 15 * time.Minute, authors: source}
}
