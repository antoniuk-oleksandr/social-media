package fx

import (
	"net/http"

	adfeature "github.com/antoniuk-oleksandr/social-media/internal/ad"
	addb "github.com/antoniuk-oleksandr/social-media/internal/ad/dynamodb"
	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	platformobservability "github.com/antoniuk-oleksandr/social-media/internal/platform/observability"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewRepository(client *dynamodb.Client, cfg platformconfig.AdsConfig) adfeature.Repository {
	return addb.NewConfiguredRepository(client, cfg)
}
func NewService(repository adfeature.Repository, ids platformid.UUIDv7, clock platformclock.Real) adfeature.Service {
	return adfeature.NewService(repository, ids, clock)
}
func NewHandler(service adfeature.Service, logger logging.Logger) *adfeature.Handler {
	return adfeature.NewHandler(service, logger)
}
func NewRouter(handler *adfeature.Handler, access platformjwt.Access) http.Handler {
	return adfeature.NewRouter(handler, access)
}

func NewObservedRouter(handler *adfeature.Handler, access platformjwt.Access, middleware platformobservability.Middleware) http.Handler {
	return middleware(NewRouter(handler, access))
}
