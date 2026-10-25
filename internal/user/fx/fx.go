package fx

import (
	"net/http"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	userfeature "github.com/antoniuk-oleksandr/social-media/internal/user"
	userdb "github.com/antoniuk-oleksandr/social-media/internal/user/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewRepository(client *dynamodb.Client) userfeature.Repository {
	return userdb.NewConfiguredRepository(client, fx.Value("USERS_TABLE", "social-media-local-users"))
}

func NewService(repository userfeature.Repository) userfeature.Service {
	return userfeature.NewService(repository)
}

func NewHandler(service userfeature.Service, logger logging.Logger) *userfeature.Handler {
	return userfeature.NewHandler(service, logger)
}

func NewRouter(handler *userfeature.Handler) http.Handler {
	return userfeature.NewRouter(handler)
}
