package fx

import (
	"net/http"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	authdb "github.com/antoniuk-oleksandr/social-media/internal/auth/dynamodb"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	jwtkey "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/password"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/token"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewRegistrationStore(client *dynamodb.Client) auth.RegistrationStore {
	return authdb.Store{
		Client:       client,
		UsersTable:   platformfx.Value("USERS_TABLE", "social-media-local-users"),
		UniquesTable: platformfx.Value("UNIQUES_TABLE", "social-media-local-uniques"),
		RefreshTable: platformfx.Value("REFRESH_TABLE", "social-media-local-refresh-sessions"),
	}
}

func NewService(
	store auth.RegistrationStore,
	hasher password.Bcrypt,
	ids id.UUIDv7,
	clk clock.Real,
	access jwtkey.Access,
	opaque token.Opaque,
) *auth.Service {
	return &auth.Service{
		Store:      store,
		Hasher:     hasher,
		IDs:        ids,
		Clock:      clk,
		Access:     access,
		Opaque:     opaque,
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 7 * 24 * time.Hour,
	}
}

func NewServicePort(
	store auth.RegistrationStore,
	hasher password.Bcrypt,
	ids id.UUIDv7,
	clk clock.Real,
	access jwtkey.Access,
	opaque token.Opaque,
) auth.ServicePort {
	return NewService(store, hasher, ids, clk, access, opaque)
}

func NewHandler(service auth.ServicePort) auth.Handler {
	return auth.Handler{Service: service}
}

func NewRouter(handler auth.Handler) http.Handler {
	return auth.Router(handler)
}
