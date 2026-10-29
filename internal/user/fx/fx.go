package fx

import (
	"net/http"

	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	platformobservability "github.com/antoniuk-oleksandr/social-media/internal/platform/observability"
	userfeature "github.com/antoniuk-oleksandr/social-media/internal/user"
	userdb "github.com/antoniuk-oleksandr/social-media/internal/user/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewRepository(client *dynamodb.Client) userfeature.Repository {
	return userdb.NewConfiguredRepository(client, fx.Value("USERS_TABLE", "social-media-local-users"), fx.Value("UNIQUES_TABLE", "social-media-local-uniques"))
}

func NewConfiguredRepository(client *dynamodb.Client, cfg platformconfig.UsersConfig, clk platformclock.Real) userfeature.Repository {
	return userdb.NewConfiguredRepositoryWithClock(client, cfg.UsersTable, cfg.UniquesTable, clk)
}

func NewService(repository userfeature.Repository) userfeature.Service {
	return userfeature.NewService(repository)
}

func NewHandler(service userfeature.Service, logger logging.Logger) *userfeature.Handler {
	return userfeature.NewHandler(service, logger)
}

func NewRouter(handler *userfeature.Handler, access platformjwt.Access) http.Handler {
	return userfeature.NewRouter(handler, access)
}

func NewObservedRouter(handler *userfeature.Handler, access platformjwt.Access, middleware platformobservability.Middleware) http.Handler {
	return middleware(NewRouter(handler, access))
}
