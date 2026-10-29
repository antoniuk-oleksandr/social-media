package fx

import (
	adfeature "github.com/antoniuk-oleksandr/social-media/internal/ad"
	addb "github.com/antoniuk-oleksandr/social-media/internal/ad/dynamodb"
	feedfeature "github.com/antoniuk-oleksandr/social-media/internal/feed"
	feeddb "github.com/antoniuk-oleksandr/social-media/internal/feed/dynamodb"
	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	platformobservability "github.com/antoniuk-oleksandr/social-media/internal/platform/observability"
	userdb "github.com/antoniuk-oleksandr/social-media/internal/user/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"net/http"
)

func NewPosts(client *dynamodb.Client, cfg platformconfig.PostsConfig) feedfeature.PostSource {
	return feeddb.NewPostRepository(client, cfg.PostsTable)
}
func NewAds(client *dynamodb.Client, cfg platformconfig.AdsConfig) feedfeature.AdSource {
	return feeddb.NewAdRepository(addb.NewConfiguredRepository(client, cfg))
}
func NewAuthors(client *dynamodb.Client, cfg platformconfig.UsersConfig) feedfeature.AuthorSource {
	return userdb.NewConfiguredRepository(client, cfg.UsersTable, cfg.UniquesTable)
}
func NewService(posts feedfeature.PostSource, ads feedfeature.AdSource, authors feedfeature.AuthorSource, clock platformclock.Real, cfg platformconfig.AdsConfig) feedfeature.Service {
	return feedfeature.NewService(posts, ads, clock, cfg.CursorKey, cfg.CursorKeyID, authors)
}
func NewHandler(s feedfeature.Service, logger logging.Logger) *feedfeature.Handler {
	return feedfeature.NewHandler(s, logger)
}
func NewRouter(h *feedfeature.Handler, a platformjwt.Access) http.Handler {
	return feedfeature.NewRouter(h, a)
}

func NewObservedRouter(h *feedfeature.Handler, a platformjwt.Access, middleware platformobservability.Middleware) http.Handler {
	return middleware(NewRouter(h, a))
}

var _ adfeature.Repository
