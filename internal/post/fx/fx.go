package fx

import (
	"net/http"

	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"

	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	platformobservability "github.com/antoniuk-oleksandr/social-media/internal/platform/observability"
	postfeature "github.com/antoniuk-oleksandr/social-media/internal/post"
	postdb "github.com/antoniuk-oleksandr/social-media/internal/post/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewRepository(client *dynamodb.Client, cfg platformconfig.PostsConfig) postfeature.Repository {
	return postdb.NewRepository(client, cfg.PostsTable)
}
func NewService(repository postfeature.Repository, ids platformid.UUIDv7, clock platformclock.Real) postfeature.Service {
	return postfeature.NewService(repository, ids, clock)
}
func NewHandler(service postfeature.Service, logger logging.Logger) *postfeature.Handler {
	return postfeature.NewHandler(service, logger)
}
func NewRouter(handler *postfeature.Handler, access platformjwt.Access) http.Handler {
	return postfeature.NewRouter(handler, access)
}

func NewObservedRouter(handler *postfeature.Handler, access platformjwt.Access, middleware platformobservability.Middleware) http.Handler {
	return middleware(NewRouter(handler, access))
}
